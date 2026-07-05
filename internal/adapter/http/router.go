package http

import (
	"github.com/gofiber/fiber/v3"
	"github.com/rsb/garmin-weight-sync/internal/usecase"
)

// SetupRouter configures all routes and middlewares. apiKey is required on
// every request except /health via the X-API-Key header.
func SetupRouter(useCase *usecase.SyncMeasurementUseCase, apiKey string) *fiber.App {
	app := fiber.New()

	app.Use(newAPIKeyMiddleware(apiKey))

	handler := NewHandler(app, useCase)
	handler.SetupRoutes()

	return app
}

// newAPIKeyMiddleware rejects any request whose X-API-Key header doesn't
// match apiKey, except /health (used for liveness probes with no client
// identity to authenticate).
func newAPIKeyMiddleware(apiKey string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Path() == "/health" {
			return c.Next()
		}
		if c.Get("X-API-Key") != apiKey {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "invalid or missing API key"})
		}
		return c.Next()
	}
}
