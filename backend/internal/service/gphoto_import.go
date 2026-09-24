package service

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/palpal/gphoto/internal/domain"
)

type ImportResult struct {
	Discovered int
	Imported   int
	Skipped    int
	Failed     int
}

type GoogleMediaRepository interface {
	IsDownloaded(context.Context, string, string) (bool, error)
	RecordDownloaded(context.Context, string, domain.GoogleMediaItem, string, time.Time) error
}

type GoogleMediaDownloader interface {
	Download(context.Context, domain.GoogleMediaItem) (io.ReadCloser, error)
}

type GoogleMediaStorage interface {
	Save(context.Context, string, domain.GoogleMediaItem, io.Reader) (string, error)
}

type GooglePhotosImporter struct {
	repository GoogleMediaRepository
	downloader GoogleMediaDownloader
	storage    GoogleMediaStorage
	now        func() time.Time
}

func NewGooglePhotosImporter(repository GoogleMediaRepository, downloader GoogleMediaDownloader, storage GoogleMediaStorage) *GooglePhotosImporter {
	return &GooglePhotosImporter{repository: repository, downloader: downloader, storage: storage, now: time.Now}
}

func (importer *GooglePhotosImporter) Import(ctx context.Context, account domain.GoogleAccount, items []domain.GoogleMediaItem) (ImportResult, error) {
	result := ImportResult{Discovered: len(items)}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !mediaTypeAllowed(account.MediaType, item.MimeType) {
			result.Skipped++
			continue
		}
		downloaded, err := importer.repository.IsDownloaded(ctx, account.ID, item.ProviderID)
		if err != nil {
			return result, fmt.Errorf("check downloaded media %q: %w", item.ProviderID, err)
		}
		if downloaded {
			result.Skipped++
			continue
		}
		reader, err := importer.downloader.Download(ctx, item)
		if err != nil {
			result.Failed++
			continue
		}
		relativePath, saveErr := importer.storage.Save(ctx, account.ID, item, reader)
		closeErr := reader.Close()
		if saveErr != nil || closeErr != nil {
			result.Failed++
			continue
		}
		if err := importer.repository.RecordDownloaded(ctx, account.ID, item, relativePath, importer.now().UTC()); err != nil {
			return result, fmt.Errorf("record downloaded media %q: %w", item.ProviderID, err)
		}
		result.Imported++
	}
	return result, nil
}

func mediaTypeAllowed(filter domain.MediaType, mimeType string) bool {
	switch filter {
	case domain.MediaTypeImages:
		return strings.HasPrefix(mimeType, "image/")
	case domain.MediaTypeVideos:
		return strings.HasPrefix(mimeType, "video/")
	default:
		return strings.HasPrefix(mimeType, "image/") || strings.HasPrefix(mimeType, "video/")
	}
}
