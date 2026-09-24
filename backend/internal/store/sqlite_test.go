package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/palpal/gphoto/internal/domain"
)

func TestOAuthStateIsConsumedOnce(t *testing.T) {
	store := openTestSQLite(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := store.PutOAuthState(ctx, "state", "Personal", "me@example.com", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	name, email, err := store.ConsumeOAuthState(ctx, "state", now)
	if err != nil || name != "Personal" || email != "me@example.com" {
		t.Fatalf("ConsumeOAuthState() = %q, %q, %v", name, email, err)
	}
	if _, _, err := store.ConsumeOAuthState(ctx, "state", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second ConsumeOAuthState() error = %v", err)
	}
}

func TestDownloadedItemIsUniquePerAccount(t *testing.T) {
	store := openTestSQLite(t)
	ctx := context.Background()
	now := time.Now().UTC()
	account := domain.GoogleAccount{ID: "account-1", GoogleSubject: "subject", Email: "me@example.com", Name: "Personal", MediaType: domain.MediaTypeAll, TokenCiphertext: []byte("encrypted"), CreatedAt: now}
	if err := store.UpsertGoogleAccount(ctx, account); err != nil {
		t.Fatal(err)
	}
	item := domain.GoogleMediaItem{ProviderID: "photo-1", Filename: "photo.jpg", MimeType: "image/jpeg", CreatedAt: now}
	if err := store.RecordDownloaded(ctx, account.ID, item, "2026/09/photo.jpg", now); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordDownloaded(ctx, account.ID, item, "duplicate.jpg", now); err != nil {
		t.Fatal(err)
	}
	downloaded, err := store.IsDownloaded(ctx, account.ID, item.ProviderID)
	if err != nil || !downloaded {
		t.Fatalf("IsDownloaded() = %v, %v", downloaded, err)
	}
}

func TestGoogleAccountUpsertKeepsStableID(t *testing.T) {
	store := openTestSQLite(t)
	ctx := context.Background()
	now := time.Now().UTC()
	first, err := store.SaveGoogleAccount(ctx, domain.GoogleAccount{ID: "account-1", GoogleSubject: "subject", Email: "old@example.com", Name: "Old", MediaType: domain.MediaTypeAll, TokenCiphertext: []byte("old"), CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.SaveGoogleAccount(ctx, domain.GoogleAccount{ID: "account-2", GoogleSubject: "subject", Email: "new@example.com", Name: "New", MediaType: domain.MediaTypeImages, TokenCiphertext: []byte("new"), CreatedAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != first.ID || updated.Email != "new@example.com" || updated.MediaType != domain.MediaTypeAll {
		t.Fatalf("SaveGoogleAccount() = %+v", updated)
	}
}

func TestPendingPickerSessions(t *testing.T) {
	store := openTestSQLite(t)
	ctx := context.Background()
	now := time.Now().UTC()
	account := domain.GoogleAccount{ID: "account-1", GoogleSubject: "subject", Email: "me@example.com", Name: "Personal", MediaType: domain.MediaTypeAll, TokenCiphertext: []byte("encrypted"), CreatedAt: now}
	if err := store.UpsertGoogleAccount(ctx, account); err != nil {
		t.Fatal(err)
	}
	session := domain.PickerSession{ID: "session-1", AccountID: account.ID, PickerURI: "https://picker", Status: "waiting", CreatedAt: now, UpdatedAt: now}
	if err := store.CreatePickerSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	pending, err := store.ListPickerSessions(ctx, true)
	if err != nil || len(pending) != 1 {
		t.Fatalf("ListPickerSessions() = %+v, %v", pending, err)
	}
	if err := store.UpdatePickerSession(ctx, session.ID, "completed", "", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	pending, err = store.ListPickerSessions(ctx, true)
	if err != nil || len(pending) != 0 {
		t.Fatalf("completed pending sessions = %+v, %v", pending, err)
	}
}

func openTestSQLite(t *testing.T) *SQLite {
	t.Helper()
	store, err := NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}
