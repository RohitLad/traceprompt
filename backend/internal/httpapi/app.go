package httpapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"gorm.io/gorm"
)

// Deps are injected by main; tests supply SQLite + fixed secret.
type Deps struct {
	DB        *gorm.DB
	JWTSecret string
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

	h := &Handler{db: d.DB, jwtSecret: d.JWTSecret}

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

	// Public API (BasicAuth pk:sk, Langfuse-compatible).
	pub := app.Group("/api/public", h.requireAPIKey)
	pub.Get("/projects", h.PublicProjects)

	return app
}
