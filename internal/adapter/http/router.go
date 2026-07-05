package http

import (
	"github.com/gofiber/fiber/v3"
	"github.com/rsb/garmin-weight-sync/internal/usecase"
)

// SetupRouter configures all routes and middlewares.
func SetupRouter(useCase *usecase.SyncMeasurementUseCase) *fiber.App {
	app := fiber.New()

	// TODO: Add middleware (auth, logging, etc.)

	handler := NewHandler(app, useCase)
	handler.SetupRoutes()

	return app
}
