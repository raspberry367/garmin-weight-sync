package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/rsb/garmin-weight-sync/internal/domain"
)

// GarminSyncUseCase pushes every not-yet-synced measurement to Garmin Connect.
// It's driven by a cron-style ticker rather than the HTTP intake path, since
// a single Garmin upload can take tens of seconds (SSO login) and shouldn't
// block a measurement POST.
type GarminSyncUseCase struct {
	repo     domain.MeasurementRepository
	syncer   domain.MeasurementSyncer
	notifier domain.Notifier
}

// NewGarminSyncUseCase creates a new GarminSyncUseCase. notifier may be nil.
func NewGarminSyncUseCase(repo domain.MeasurementRepository, syncer domain.MeasurementSyncer, notifier domain.Notifier) *GarminSyncUseCase {
	return &GarminSyncUseCase{repo: repo, syncer: syncer, notifier: notifier}
}

// Execute syncs every unsynced measurement.
//
//   - An auth failure that needs manual intervention (domain.ErrSyncAuthRequired:
//     lockout, MFA, Cloudflare) stops the batch immediately and raises an alert.
//     Retrying wouldn't help and can deepen a lockout — fail fast.
//   - A transient per-measurement failure is logged and the batch continues.
func (u *GarminSyncUseCase) Execute(ctx context.Context) error {
	measurements, err := u.repo.FindUnsynced(ctx)
	if err != nil {
		return err
	}

	var failed int
	for _, m := range measurements {
		if err := u.syncer.Sync(m); err != nil {
			if errors.Is(err, domain.ErrSyncAuthRequired) {
				u.alert(ctx, fmt.Sprintf("⚠️ Garmin sync halted: manual login required.\n%v\n\nRun `make garmin-login` once the account is usable again.", err))
				return err
			}
			log.Printf("garmin sync: failed to sync measurement %s: %v", m.AppleHealthID, err)
			failed++
			continue
		}
		if err := u.repo.MarkSynced(ctx, m); err != nil {
			log.Printf("garmin sync: failed to mark measurement %s synced: %v", m.AppleHealthID, err)
		}
	}

	if failed > 0 {
		u.alert(ctx, fmt.Sprintf("⚠️ Garmin sync: %d of %d measurement(s) failed to upload. See server logs.", failed, len(measurements)))
	}

	return nil
}

// alert sends an operational notification, logging (but not failing) if the
// notifier itself errors — a broken alert channel must not break sync.
func (u *GarminSyncUseCase) alert(ctx context.Context, message string) {
	if u.notifier == nil {
		return
	}
	if err := u.notifier.Notify(ctx, message); err != nil {
		log.Printf("garmin sync: failed to send alert: %v", err)
	}
}
