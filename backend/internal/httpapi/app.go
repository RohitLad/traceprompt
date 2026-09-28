package httpapi

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

// New builds the Fiber app with production-safe middleware.
// DB/queue are injected later per-route; health stays dependency-free.
func New() *fiber.App {
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

	api := app.Group("/api/public")
	api.Get("/projects", func(c *fiber.Ctx) error {
		// Stub until Phase 1 auth lands; keeps SDK health-checks green.
		return c.JSON(fiber.Map{"data": []any{}})
	})

	return app
}
