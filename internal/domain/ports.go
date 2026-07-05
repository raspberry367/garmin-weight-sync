package domain

import (
	"context"
	"errors"
)

// ErrSyncAuthRequired marks a sync failure that a retry cannot fix — the
// Garmin account needs manual intervention (MFA prompt, lockout, or a
// Cloudflare block). Syncers wrap their transport-specific errors with this
// so the cron use case can stop the batch early and raise an alert instead
// of hammering Garmin (which itself can trigger a lockout).
var ErrSyncAuthRequired = errors.New("garmin sync requires manual login")

// MeasurementRepository defines the contract for persisting measurements.
type MeasurementRepository interface {
	Save(ctx context.Context, measurement *BodyComposition) error

	// FindUnsynced returns every measurement not yet pushed to Garmin Connect.
	FindUnsynced(ctx context.Context) ([]*BodyComposition, error)

	// MarkSynced records that measurement has been pushed to Garmin Connect.
	MarkSynced(ctx context.Context, measurement *BodyComposition) error
}

// MeasurementSyncer defines the contract for syncing to Garmin Connect.
type MeasurementSyncer interface {
	Sync(measurement *BodyComposition) error
}

// Notifier delivers an operational alert (e.g. to Telegram) when unattended
// sync needs human attention. Implementations should no-op silently when not
// configured.
type Notifier interface {
	Notify(ctx context.Context, message string) error
}
