package http

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/rsb/garmin-weight-sync/internal/domain"
	"github.com/rsb/garmin-weight-sync/internal/usecase"
)

// Handler handles HTTP requests for measurement sync.
type Handler struct {
	router  fiber.Router
	useCase *usecase.SyncMeasurementUseCase
}

// NewHandler creates a new HTTP handler instance.
func NewHandler(router fiber.Router, useCase *usecase.SyncMeasurementUseCase) *Handler {
	return &Handler{
		router:  router,
		useCase: useCase,
	}
}

// SetupRoutes registers all HTTP routes.
func (h *Handler) SetupRoutes() {
	h.router.Post("/api/v1/measurements", h.SyncMeasurement)
	h.router.Get("/health", h.HealthCheck)
}

// SyncMeasurement handles POST /api/v1/measurements
func (h *Handler) SyncMeasurement(c fiber.Ctx) error {
	// Read raw body for logging
	body := c.Request().Body()
	log.Printf("Raw request body: %s", string(body))

	// If body is empty, return error
	if len(body) == 0 {
		log.Printf("Warning: Empty request body received")
		return c.Status(fiber.StatusBadRequest).JSON(MeasurementResponse{
			Synced:  false,
			Message: "empty body",
		})
	}

	// Inspect keys to determine payload type (avoid false positives from unmarshalling into unrelated structs)
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		log.Printf("Error parsing JSON keys: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(MeasurementResponse{Synced: false, Message: "invalid JSON"})
	}

	// 1. Full Body Composition
	if keys["apple_health_id"] != nil {
		var comp domain.BodyComposition
		if err := json.Unmarshal(body, &comp); err != nil {
			log.Printf("Error parsing full body composition: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(MeasurementResponse{Synced: false, Message: "invalid body composition JSON"})
		}

		if err := h.useCase.Execute(c.Context(), &comp); err != nil {
			log.Printf("Usecase execution failed for full body composition: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(MeasurementResponse{Synced: false, Message: err.Error()})
		}

		return c.JSON(MeasurementResponse{
			Synced:      true,
			Measurement: comp.AppleHealthID,
			Message:     "measurement saved to database",
		})
	}

	// 2. Single measurement with explicit type
	if _, ok := keys["type"]; ok {
		var singleReq SingleMeasurementRequest
		if err := json.Unmarshal(body, &singleReq); err != nil {
			log.Printf("Error parsing single measurement request: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(MeasurementResponse{Synced: false, Message: "invalid single measurement JSON"})
		}

		log.Printf("Parsed single measurement request:")
		log.Printf("  Type: %s", singleReq.Type)
		log.Printf("  Value: %.6f", singleReq.Value.Float64())
		if singleReq.Timestamp > 0 {
			log.Printf("  Timestamp: %d", singleReq.Timestamp)
		}
		if singleReq.ID != "" {
			log.Printf("  ID: %s", singleReq.ID)
		}

		val := singleReq.Value.Float64()
		comp := &domain.BodyComposition{
			Timestamp:     singleReq.Timestamp,
			AppleHealthID: singleReq.ID,
		}
		if comp.AppleHealthID == "" {
			comp.AppleHealthID = generateDeterministicID(string(singleReq.Type), val)
		}
		if comp.Timestamp <= 0 {
			comp.Timestamp = time.Now().UnixMilli()
		}

		switch singleReq.Type {
		case MeasurementWeight:
			comp.Weight = val
		case MeasurementBMI:
			comp.BMI = val
		case MeasurementFatPercent:
			comp.FatPercentage = val
		}

		if err := h.useCase.Execute(c.Context(), comp); err != nil {
			log.Printf("Usecase execution failed for single request: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(MeasurementResponse{Synced: false, Message: err.Error()})
		}

		return c.JSON(MeasurementResponse{
			Synced:      true,
			Measurement: comp.AppleHealthID,
			Message:     "single measurement saved to database",
		})
	}

	// 3. Raw numeric fields: check for presence of the specific key
	switch {
	case keys["weight"] != nil:
		var weightReq RawWeightRequest
		if err := json.Unmarshal(body, &weightReq); err != nil {
			log.Printf("Error parsing raw weight request: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(MeasurementResponse{Synced: false, Message: "invalid weight JSON"})
		}

		log.Printf("Parsed raw weight request:")
		log.Printf("  Weight: %.6f kg", weightReq.Weight.Float64())

		comp := &domain.BodyComposition{
			Weight:        weightReq.Weight.Float64(),
			Timestamp:     time.Now().UnixMilli(),
			AppleHealthID: generateDeterministicID("weight", weightReq.Weight.Float64()),
		}
		if err := h.useCase.Execute(c.Context(), comp); err != nil {
			log.Printf("Usecase execution failed for raw weight: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(MeasurementResponse{Synced: false, Message: err.Error()})
		}

		return c.JSON(MeasurementResponse{Synced: true, Measurement: "weight", Message: "weight received and saved to database"})

	case keys["bmi"] != nil:
		var bmiReq RawBMIRequest
		if err := json.Unmarshal(body, &bmiReq); err != nil {
			log.Printf("Error parsing raw BMI request: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(MeasurementResponse{Synced: false, Message: "invalid BMI JSON"})
		}

		log.Printf("Parsed raw BMI request:")
		log.Printf("  BMI: %.6f", bmiReq.BMI.Float64())

		comp := &domain.BodyComposition{
			BMI:           bmiReq.BMI.Float64(),
			Timestamp:     time.Now().UnixMilli(),
			AppleHealthID: generateDeterministicID("bmi", bmiReq.BMI.Float64()),
		}
		if err := h.useCase.Execute(c.Context(), comp); err != nil {
			log.Printf("Usecase execution failed for raw BMI: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(MeasurementResponse{Synced: false, Message: err.Error()})
		}

		return c.JSON(MeasurementResponse{Synced: true, Measurement: "bmi", Message: "BMI received and saved to database"})

	case keys["fat"] != nil || keys["fat_percentage"] != nil:
		var fatReq RawFatPercentageRequest
		if err := json.Unmarshal(body, &fatReq); err != nil {
			log.Printf("Error parsing raw fat request: %v", err)
			return c.Status(fiber.StatusBadRequest).JSON(MeasurementResponse{Synced: false, Message: "invalid fat JSON"})
		}

		val := fatReq.FatPercentage
		if val.Float64() == 0 && fatReq.FatPercentageAlt.Float64() != 0 {
			val = fatReq.FatPercentageAlt
		}
		log.Printf("Parsed raw fat percentage request:")
		log.Printf("  Fat Percentage: %.6f%%", val.Float64())

		comp := &domain.BodyComposition{
			FatPercentage: val.Float64(),
			Timestamp:     time.Now().UnixMilli(),
			AppleHealthID: generateDeterministicID("fat", val.Float64()),
		}
		if err := h.useCase.Execute(c.Context(), comp); err != nil {
			log.Printf("Usecase execution failed for raw fat: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(MeasurementResponse{Synced: false, Message: err.Error()})
		}

		return c.JSON(MeasurementResponse{Synced: true, Measurement: "fat_percentage", Message: "fat percentage received and saved to database"})
	}

	// If we got here, none of the parsing worked
	log.Printf("Error: Could not parse request body as any known format")
	return c.Status(fiber.StatusBadRequest).JSON(MeasurementResponse{
		Synced:  false,
		Message: "invalid request format",
	})
}

// HealthCheck handles GET /health
func (h *Handler) HealthCheck(c fiber.Ctx) error {
	return c.JSON(HealthResponse{Status: "ok"})
}

// generateDeterministicID generates an ID based on metric type, current date, and value.
func generateDeterministicID(metricType string, value float64) string {
	dateStr := time.Now().UTC().Format("2006-01-02")
	return fmt.Sprintf("raw-%s-%s-%.6f", metricType, dateStr, value)
}
