package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/palpal/gphoto/internal/domain"
)

type PickerRepository interface {
	GoogleMediaRepository
	GetGoogleAccount(context.Context, string) (domain.GoogleAccount, error)
	CreatePickerSession(context.Context, domain.PickerSession) error
	ListPickerSessions(context.Context, bool) ([]domain.PickerSession, error)
	UpdatePickerSession(context.Context, string, string, string, time.Time) error
}

type GooglePhotosProvider interface {
	CreateSession(context.Context) (domain.ProviderSession, error)
	GetSession(context.Context, string) (domain.ProviderSession, error)
	ListMedia(context.Context, string) ([]domain.GoogleMediaItem, error)
	Download(context.Context, domain.GoogleMediaItem) (io.ReadCloser, error)
}

type GooglePhotosProviderFactory func(context.Context, domain.GoogleAccount) (GooglePhotosProvider, error)

type GooglePhotosSync struct {
	repository PickerRepository
	storage    GoogleMediaStorage
	provider   GooglePhotosProviderFactory
	now        func() time.Time
}

func NewGooglePhotosSync(repository PickerRepository, storage GoogleMediaStorage, provider GooglePhotosProviderFactory) *GooglePhotosSync {
	return &GooglePhotosSync{repository: repository, storage: storage, provider: provider, now: time.Now}
}

func (syncer *GooglePhotosSync) StartSession(ctx context.Context, accountID string) (domain.PickerSession, error) {
	account, err := syncer.repository.GetGoogleAccount(ctx, accountID)
	if err != nil {
		return domain.PickerSession{}, fmt.Errorf("get Google account: %w", err)
	}
	provider, err := syncer.provider(ctx, account)
	if err != nil {
		return domain.PickerSession{}, err
	}
	created, err := provider.CreateSession(ctx)
	if err != nil {
		return domain.PickerSession{}, fmt.Errorf("create Picker session: %w", err)
	}
	now := syncer.now().UTC()
	session := domain.PickerSession{ID: created.ID, AccountID: account.ID, PickerURI: created.PickerURI, Status: "waiting", CreatedAt: now, UpdatedAt: now}
	if err := syncer.repository.CreatePickerSession(ctx, session); err != nil {
		return domain.PickerSession{}, fmt.Errorf("save Picker session: %w", err)
	}
	return session, nil
}

func (syncer *GooglePhotosSync) ProcessPending(ctx context.Context) error {
	sessions, err := syncer.repository.ListPickerSessions(ctx, true)
	if err != nil {
		return fmt.Errorf("list pending Picker sessions: %w", err)
	}
	for _, session := range sessions {
		if err := syncer.processSession(ctx, session); err != nil {
			slog.Warn("process Picker session failed", "session_id", session.ID, "error", err)
			_ = syncer.repository.UpdatePickerSession(ctx, session.ID, "failed", err.Error(), syncer.now().UTC())
		}
	}
	return nil
}

func (syncer *GooglePhotosSync) processSession(ctx context.Context, session domain.PickerSession) error {
	account, err := syncer.repository.GetGoogleAccount(ctx, session.AccountID)
	if err != nil {
		return fmt.Errorf("get session account: %w", err)
	}
	provider, err := syncer.provider(ctx, account)
	if err != nil {
		return err
	}
	remote, err := provider.GetSession(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("get Picker session: %w", err)
	}
	if !remote.MediaItemsSet {
		return nil
	}
	if err := syncer.repository.UpdatePickerSession(ctx, session.ID, "importing", "", syncer.now().UTC()); err != nil {
		return err
	}
	items, err := provider.ListMedia(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("list picked media: %w", err)
	}
	importer := NewGooglePhotosImporter(syncer.repository, provider, syncer.storage)
	result, err := importer.Import(ctx, account, items)
	if err != nil {
		return err
	}
	if result.Failed > 0 {
		return fmt.Errorf("%d of %d media downloads failed", result.Failed, result.Discovered)
	}
	return syncer.repository.UpdatePickerSession(ctx, session.ID, "completed", "", syncer.now().UTC())
}

func RunGooglePhotosScheduler(ctx context.Context, interval time.Duration, syncer *GooglePhotosSync) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := syncer.ProcessPending(ctx); err != nil {
			slog.Warn("poll Picker sessions failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
