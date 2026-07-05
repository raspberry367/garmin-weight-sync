package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/rsb/garmin-weight-sync/internal/domain"
	"github.com/rsb/garmin-weight-sync/internal/usecase"
)

type mockRepository struct {
	saved []*domain.BodyComposition
}

func (m *mockRepository) Save(ctx context.Context, measurement *domain.BodyComposition) error {
	m.saved = append(m.saved, measurement)
	return nil
}

func (m *mockRepository) FindUnsynced(ctx context.Context) ([]*domain.BodyComposition, error) {
	return nil, nil
}

func (m *mockRepository) MarkSynced(ctx context.Context, measurement *domain.BodyComposition) error {
	return nil
}

func TestSyncMeasurementHandler(t *testing.T) {
	const testAPIKey = "test-api-key"

	repo := &mockRepository{}
	uc := usecase.NewSyncMeasurementUseCase(repo)
	app := SetupRouter(uc, testAPIKey)

	t.Run("POST full body composition", func(t *testing.T) {
		repo.saved = nil
		payload := map[string]interface{}{
			"bmi":             22.5,
			"fat_percentage":  15.0,
			"weight":          80.0,
			"timestamp":       1625345600000,
			"apple_health_id": "test-uuid-full",
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", "/api/v1/measurements", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", testAPIKey)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("failed to run test request: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		if len(repo.saved) != 1 {
			t.Fatalf("expected 1 saved measurement, got %d", len(repo.saved))
		}

		m := repo.saved[0]
		if m.AppleHealthID != "test-uuid-full" {
			t.Errorf("expected UUID test-uuid-full, got %s", m.AppleHealthID)
		}
		if m.Weight != 80.0 {
			t.Errorf("expected weight 80.0, got %f", m.Weight)
		}
	})

	t.Run("POST single weight measurement", func(t *testing.T) {
		repo.saved = nil
		payload := map[string]interface{}{
			"type":      "weight",
			"value":     75.5,
			"timestamp": 1625345600000,
			"id":        "test-uuid-single",
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", "/api/v1/measurements", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", testAPIKey)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("failed to run test request: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		if len(repo.saved) != 1 {
			t.Fatalf("expected 1 saved measurement, got %d", len(repo.saved))
		}

		m := repo.saved[0]
		if m.AppleHealthID != "test-uuid-single" {
			t.Errorf("expected UUID test-uuid-single, got %s", m.AppleHealthID)
		}
		if m.Weight != 75.5 {
			t.Errorf("expected weight 75.5, got %f", m.Weight)
		}
	})

	t.Run("POST raw weight request", func(t *testing.T) {
		repo.saved = nil
		payload := map[string]interface{}{
			"weight": "82.4",
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", "/api/v1/measurements", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", testAPIKey)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("failed to run test request: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		if len(repo.saved) != 1 {
			t.Fatalf("expected 1 saved measurement, got %d", len(repo.saved))
		}

		m := repo.saved[0]
		if m.Weight != 82.4 {
			t.Errorf("expected weight 82.4, got %f", m.Weight)
		}
		if m.AppleHealthID == "" {
			t.Errorf("expected generated AppleHealthID, got empty")
		}
	})
}
