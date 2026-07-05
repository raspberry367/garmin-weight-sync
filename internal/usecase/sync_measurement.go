package usecase

import (
	"context"
	"fmt"
	"log"

	"github.com/rsb/garmin-weight-sync/internal/domain"
)

// SyncMeasurementUseCase handles checking for duplicates and saving/syncing measurement data.
type SyncMeasurementUseCase struct {
	repo domain.MeasurementRepository
}

// NewSyncMeasurementUseCase creates a new SyncMeasurementUseCase.
func NewSyncMeasurementUseCase(repo domain.MeasurementRepository) *SyncMeasurementUseCase {
	return &SyncMeasurementUseCase{repo: repo}
}

// Execute orchestrates the duplicate check, validation, and database save.
func (u *SyncMeasurementUseCase) Execute(ctx context.Context, m *domain.BodyComposition) error {
	// If all fields are present, perform domain validation
	if m.Weight > 0 && m.BMI > 0 && m.LeanBodyMass > 0 {
		if err := m.Validate(); err != nil {
			return fmt.Errorf("domain validation failed: %w", err)
		}
	} else {
		// Partial validation delegated to the domain layer
		if err := m.ValidatePartial(); err != nil {
			return fmt.Errorf("domain validation failed: %w", err)
		}
	}

	// Save to database (merged into the day's row)
	if err := u.repo.Save(ctx, m); err != nil {
		return fmt.Errorf("failed to save measurement: %w", err)
	}

	log.Printf("Successfully saved measurement %s to database", m.AppleHealthID)
	return nil
}
