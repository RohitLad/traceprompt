package httpapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"gorm.io/gorm"

	"github.com/traceprompt/traceprompt/backend/internal/queue"
)

// Deps are injected by main; tests supply SQLite + memory queue + fixed secret.
type Deps struct {
	DB        *gorm.DB
	JWTSecret string
	Queue     queue.Queue
}

// New builds the Fiber app with production-safe middleware.
// Pass nil Deps to get a dependency-free app (health only, used by smoke tests).
func New(d *Deps) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "traceprompt",
		ServerHeader: "traceprompt",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})

	app.Use(requestid.New())
	app.Use(logger.New(logger.Config{Format: "${time} ${method} ${path} ${status} ${latency}\n"}))
	app.Use(helmet.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "http://localhost:5173",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	app.Get("/api/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "traceprompt"})
	})

	if d == nil || d.DB == nil {
		return app
	}
	q := d.Queue
	if q == nil {
		q = queue.NewMemory() // tests may omit; handlers stay functional
	}
	h := &Handler{db: d.DB, jwtSecret: d.JWTSecret, queue: q}

	// UI API (JWT session auth).
	v1 := app.Group("/api/v1")
	v1.Post("/auth/register", h.Register)
	v1.Post("/auth/login", h.Login)
	v1.Get("/me", h.requireJWT, h.Me)
	v1.Get("/projects", h.requireJWT, h.ListProjects)
	v1.Post("/projects", h.requireJWT, h.CreateProject)
	v1.Get("/projects/:id", h.requireJWT, h.requireProjectMember, h.GetProject)
	v1.Get("/projects/:id/keys", h.requireJWT, h.requireProjectMember, h.ListKeys)
	v1.Post("/projects/:id/keys", h.requireJWT, h.requireProjectMember, h.CreateKey)
	v1.Post("/projects/:id/keys/:keyId/revoke", h.requireJWT, h.requireProjectMember, h.RevokeKey)
	v1.Get("/projects/:id/traces", h.requireJWT, h.requireProjectMember, h.ListTracesUI)
	v1.Get("/projects/:id/traces/:traceId", h.requireJWT, h.requireProjectMember, h.GetTraceUI)
	v1.Get("/projects/:id/prompts", h.requireJWT, h.requireProjectMember, h.ListPrompts)
	v1.Post("/projects/:id/prompts", h.requireJWT, h.requireProjectMember, h.CreatePrompt)
	v1.Get("/projects/:id/prompts/:name", h.requireJWT, h.requireProjectMember, h.GetPrompt)
	v1.Post("/projects/:id/prompts/:name/versions", h.requireJWT, h.requireProjectMember, h.CreatePromptVersion)
	v1.Post("/projects/:id/prompts/:name/labels", h.requireJWT, h.requireProjectMember, h.SetPromptLabels)
	v1.Get("/projects/:id/datasets", h.requireJWT, h.requireProjectMember, h.ListDatasets)
	v1.Post("/projects/:id/datasets", h.requireJWT, h.requireProjectMember, h.CreateDataset)
	v1.Get("/projects/:id/datasets/:did", h.requireJWT, h.requireProjectMember, h.GetDataset)
	v1.Post("/projects/:id/datasets/:did/items", h.requireJWT, h.requireProjectMember, h.CreateDatasetItem)
	v1.Post("/projects/:id/datasets/:did/runs", h.requireJWT, h.requireProjectMember, h.CreateDatasetRun)
	v1.Get("/projects/:id/datasets/:did/runs", h.requireJWT, h.requireProjectMember, h.ListDatasetRuns)
	v1.Get("/projects/:id/runs/:runId", h.requireJWT, h.requireProjectMember, h.GetDatasetRun)
	v1.Post("/projects/:id/runs/:runId/items", h.requireJWT, h.requireProjectMember, h.LinkRunItem)
	v1.Get("/projects/:id/metrics/overview", h.requireJWT, h.requireProjectMember, h.MetricsOverview)
	v1.Post("/projects/:id/playground/run", h.requireJWT, h.requireProjectMember, h.PlaygroundRun)
	v1.Get("/projects/:id/sessions", h.requireJWT, h.requireProjectMember, h.ListSessionsUI)
	v1.Get("/projects/:id/sessions/:sid/traces", h.requireJWT, h.requireProjectMember, h.ListSessionTracesUI)
	v1.Get("/projects/:id/scores", h.requireJWT, h.requireProjectMember, h.ListScoresUI)

	// Public API (BasicAuth pk:sk, Langfuse-compatible).
	pub := app.Group("/api/public", h.requireAPIKey)
	pub.Get("/projects", h.PublicProjects)
	pub.Post("/ingestion", h.Ingestion)
	pub.Post("/otel/v1/traces", h.OtelIngestion)
	pub.Get("/v2/observations", h.ListObservationsV2)
	pub.Post("/scores", h.CreateScore)
	pub.Get("/v3/scores", h.ListScoresV3)
	pub.Get("/prompts/:name", h.GetPublicPrompt)
	pub.Get("/datasets", h.ListPublicDatasets)
	pub.Get("/datasets/:name", h.GetPublicDataset)

	return app
}
