package usecase

import (
	"context"
	"testing"

	"github.com/rsb/garmin-weight-sync/internal/domain"
)

type mockMeasurementRepository struct {
	saved []*domain.BodyComposition
	err   error
}

func (m *mockMeasurementRepository) Save(ctx context.Context, measurement *domain.BodyComposition) error {
	if m.err != nil {
		return m.err
	}
	m.saved = append(m.saved, measurement)
	return nil
}

func TestSyncMeasurementUseCase_Execute(t *testing.T) {
	t.Run("successful save of full body composition", func(t *testing.T) {
		repo := &mockMeasurementRepository{}
		uc := NewSyncMeasurementUseCase(repo)

		m := &domain.BodyComposition{
			BMI:           22.5,
			FatPercentage: 15.0,
			LeanBodyMass:  68.0,
			Weight:        80.0,
			Timestamp:     1625345600000,
			AppleHealthID: "test-uuid-1",
		}

		err := uc.Execute(context.Background(), m)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if len(repo.saved) != 1 {
			t.Fatalf("expected 1 saved measurement, got %d", len(repo.saved))
		}

		if repo.saved[0].AppleHealthID != "test-uuid-1" {
			t.Errorf("expected AppleHealthID to be test-uuid-1, got %s", repo.saved[0].AppleHealthID)
		}
	})

	t.Run("validates partial composition", func(t *testing.T) {
		repo := &mockMeasurementRepository{}
		uc := NewSyncMeasurementUseCase(repo)

		// Missing weight, BMI, fat, lean body mass (invalid partial)
		m := &domain.BodyComposition{
			Timestamp:     1625345600000,
			AppleHealthID: "test-uuid-1",
		}

		err := uc.Execute(context.Background(), m)
		if err == nil {
			t.Fatalf("expected validation error, got nil")
		}
	})
}
