package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/palpal/gphoto/internal/domain"
)

type googleMediaMemoryRepository struct {
	downloaded map[string]string
}

func (repository *googleMediaMemoryRepository) IsDownloaded(_ context.Context, accountID, providerID string) (bool, error) {
	_, exists := repository.downloaded[accountID+":"+providerID]
	return exists, nil
}

func (repository *googleMediaMemoryRepository) RecordDownloaded(_ context.Context, accountID string, item domain.GoogleMediaItem, relativePath string, _ time.Time) error {
	repository.downloaded[accountID+":"+item.ProviderID] = relativePath
	return nil
}

type googleMediaMemoryDownloader struct {
	contents map[string]string
	calls    int
}

func (downloader *googleMediaMemoryDownloader) Download(_ context.Context, item domain.GoogleMediaItem) (io.ReadCloser, error) {
	downloader.calls++
	return io.NopCloser(strings.NewReader(downloader.contents[item.ProviderID])), nil
}

func TestGooglePhotosImporterFiltersAndDeduplicatesPerAccount(t *testing.T) {
	repository := &googleMediaMemoryRepository{downloaded: map[string]string{}}
	downloader := &googleMediaMemoryDownloader{contents: map[string]string{"photo-1": "photo bytes", "video-1": "video bytes"}}
	directory := t.TempDir()
	importer := NewGooglePhotosImporter(repository, downloader, NewAccountMediaStore(directory))
	account := domain.GoogleAccount{ID: "account-1", MediaType: domain.MediaTypeImages}
	items := []domain.GoogleMediaItem{
		{ProviderID: "photo-1", Filename: "holiday.jpg", MimeType: "image/jpeg", CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{ProviderID: "video-1", Filename: "holiday.mp4", MimeType: "video/mp4", CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
	}

	result, err := importer.Import(context.Background(), account, items)
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 || result.Skipped != 1 || downloader.calls != 1 {
		t.Fatalf("first Import() = %+v, download calls = %d", result, downloader.calls)
	}
	result, err = importer.Import(context.Background(), account, items)
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 0 || result.Skipped != 2 || downloader.calls != 1 {
		t.Fatalf("second Import() = %+v, download calls = %d", result, downloader.calls)
	}

	path := filepath.Join(directory, "account-1", "2026", "09", "holiday.jpg")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "photo bytes" {
		t.Fatalf("stored content = %q", content)
	}
}

func TestAccountMediaStoreRejectsUnsafeAccountID(t *testing.T) {
	store := NewAccountMediaStore(t.TempDir())
	_, err := store.Save(context.Background(), "../other-account", domain.GoogleMediaItem{ProviderID: "photo", Filename: "photo.jpg"}, strings.NewReader("photo"))
	if err == nil {
		t.Fatal("Save() accepted an unsafe account ID")
	}
}
