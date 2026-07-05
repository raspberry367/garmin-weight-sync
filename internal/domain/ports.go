package domain

import "context"

// MeasurementRepository defines the contract for persisting measurements.
type MeasurementRepository interface {
	Save(ctx context.Context, measurement *BodyComposition) error
}

// MeasurementSyncer defines the contract for syncing to Garmin Connect.
type MeasurementSyncer interface {
	Sync(measurement *BodyComposition) error
}
