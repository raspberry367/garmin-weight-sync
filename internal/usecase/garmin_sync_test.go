package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/rsb/garmin-weight-sync/internal/domain"
)

type fakeUnsyncedRepository struct {
	unsynced []*domain.BodyComposition
	synced   []*domain.BodyComposition
}

func (r *fakeUnsyncedRepository) Save(ctx context.Context, measurement *domain.BodyComposition) error {
	return nil
}

func (r *fakeUnsyncedRepository) FindUnsynced(ctx context.Context) ([]*domain.BodyComposition, error) {
	return r.unsynced, nil
}

func (r *fakeUnsyncedRepository) MarkSynced(ctx context.Context, measurement *domain.BodyComposition) error {
	r.synced = append(r.synced, measurement)
	return nil
}

type fakeSyncer struct {
	failFor map[string]error
	synced  []*domain.BodyComposition
}

func (s *fakeSyncer) Sync(m *domain.BodyComposition) error {
	if err, ok := s.failFor[m.AppleHealthID]; ok {
		return err
	}
	s.synced = append(s.synced, m)
	return nil
}

type fakeNotifier struct {
	messages []string
}

func (n *fakeNotifier) Notify(ctx context.Context, message string) error {
	n.messages = append(n.messages, message)
	return nil
}

func TestGarminSyncUseCase_Execute(t *testing.T) {
	t.Run("syncs every unsynced measurement and marks them synced", func(t *testing.T) {
		repo := &fakeUnsyncedRepository{unsynced: []*domain.BodyComposition{
			{AppleHealthID: "day-1", Weight: 80},
			{AppleHealthID: "day-2", Weight: 81},
		}}
		syncer := &fakeSyncer{}
		notifier := &fakeNotifier{}
		uc := NewGarminSyncUseCase(repo, syncer, notifier)

		if err := uc.Execute(context.Background()); err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if len(syncer.synced) != 2 {
			t.Fatalf("expected 2 measurements synced, got %d", len(syncer.synced))
		}
		if len(repo.synced) != 2 {
			t.Fatalf("expected 2 measurements marked synced, got %d", len(repo.synced))
		}
		if len(notifier.messages) != 0 {
			t.Fatalf("expected no alerts on success, got %v", notifier.messages)
		}
	})

	t.Run("a transient sync failure is logged, skipped, and alerted once", func(t *testing.T) {
		repo := &fakeUnsyncedRepository{unsynced: []*domain.BodyComposition{
			{AppleHealthID: "day-1", Weight: 80},
			{AppleHealthID: "day-2", Weight: 81},
		}}
		syncer := &fakeSyncer{failFor: map[string]error{"day-1": errors.New("upload failed")}}
		notifier := &fakeNotifier{}
		uc := NewGarminSyncUseCase(repo, syncer, notifier)

		if err := uc.Execute(context.Background()); err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if len(syncer.synced) != 1 || syncer.synced[0].AppleHealthID != "day-2" {
			t.Fatalf("expected only day-2 to be synced, got %+v", syncer.synced)
		}
		if len(repo.synced) != 1 || repo.synced[0].AppleHealthID != "day-2" {
			t.Fatalf("expected only day-2 to be marked synced, got %+v", repo.synced)
		}
		if len(notifier.messages) != 1 {
			t.Fatalf("expected 1 failure alert, got %d: %v", len(notifier.messages), notifier.messages)
		}
	})

	t.Run("an auth-required failure stops the batch immediately and alerts", func(t *testing.T) {
		repo := &fakeUnsyncedRepository{unsynced: []*domain.BodyComposition{
			{AppleHealthID: "day-1", Weight: 80},
			{AppleHealthID: "day-2", Weight: 81},
		}}
		authErr := fmt.Errorf("%w: account locked", domain.ErrSyncAuthRequired)
		syncer := &fakeSyncer{failFor: map[string]error{"day-1": authErr}}
		notifier := &fakeNotifier{}
		uc := NewGarminSyncUseCase(repo, syncer, notifier)

		err := uc.Execute(context.Background())
		if !errors.Is(err, domain.ErrSyncAuthRequired) {
			t.Fatalf("expected ErrSyncAuthRequired, got %v", err)
		}
		if len(syncer.synced) != 0 {
			t.Fatalf("expected batch to stop before syncing day-2, got %+v", syncer.synced)
		}
		if len(repo.synced) != 0 {
			t.Fatalf("expected nothing marked synced, got %+v", repo.synced)
		}
		if len(notifier.messages) != 1 {
			t.Fatalf("expected 1 auth alert, got %d: %v", len(notifier.messages), notifier.messages)
		}
	})
}
