package service

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/palpal/gphoto/internal/domain"
)

type pickerMemoryRepository struct {
	googleMediaMemoryRepository
	account  domain.GoogleAccount
	sessions map[string]domain.PickerSession
}

func (repository *pickerMemoryRepository) GetGoogleAccount(context.Context, string) (domain.GoogleAccount, error) {
	return repository.account, nil
}

func (repository *pickerMemoryRepository) CreatePickerSession(_ context.Context, session domain.PickerSession) error {
	repository.sessions[session.ID] = session
	return nil
}

func (repository *pickerMemoryRepository) ListPickerSessions(context.Context, bool) ([]domain.PickerSession, error) {
	sessions := make([]domain.PickerSession, 0)
	for _, session := range repository.sessions {
		if session.Status == "waiting" || session.Status == "importing" {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

func (repository *pickerMemoryRepository) UpdatePickerSession(_ context.Context, id, status, lastError string, now time.Time) error {
	session := repository.sessions[id]
	session.Status, session.LastError, session.UpdatedAt = status, lastError, now
	repository.sessions[id] = session
	return nil
}

type pickerMemoryProvider struct {
	items []domain.GoogleMediaItem
}

func (*pickerMemoryProvider) CreateSession(context.Context) (domain.ProviderSession, error) {
	return domain.ProviderSession{ID: "session-1", PickerURI: "https://picker"}, nil
}

func (*pickerMemoryProvider) GetSession(context.Context, string) (domain.ProviderSession, error) {
	return domain.ProviderSession{ID: "session-1", MediaItemsSet: true}, nil
}

func (provider *pickerMemoryProvider) ListMedia(context.Context, string) ([]domain.GoogleMediaItem, error) {
	return provider.items, nil
}

func (*pickerMemoryProvider) Download(context.Context, domain.GoogleMediaItem) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("photo bytes")), nil
}

func TestGooglePhotosSyncCompletesPickedSession(t *testing.T) {
	account := domain.GoogleAccount{ID: "account-1", MediaType: domain.MediaTypeAll}
	repository := &pickerMemoryRepository{
		googleMediaMemoryRepository: googleMediaMemoryRepository{downloaded: map[string]string{}},
		account:                     account, sessions: map[string]domain.PickerSession{},
	}
	provider := &pickerMemoryProvider{items: []domain.GoogleMediaItem{{ProviderID: "photo-1", Filename: "photo.jpg", MimeType: "image/jpeg", CreatedAt: time.Now()}}}
	syncer := NewGooglePhotosSync(repository, NewAccountMediaStore(t.TempDir()), func(context.Context, domain.GoogleAccount) (GooglePhotosProvider, error) {
		return provider, nil
	})
	session, err := syncer.StartSession(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncer.ProcessPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := repository.sessions[session.ID].Status; got != "completed" {
		t.Fatalf("session status = %q, want completed", got)
	}
	if _, exists := repository.downloaded[account.ID+":photo-1"]; !exists {
		t.Fatal("picked media was not recorded")
	}
}
