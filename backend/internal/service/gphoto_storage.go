package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/palpal/gphoto/internal/domain"
)

type AccountMediaStore struct {
	directory string
	mutex     sync.Mutex
}

func NewAccountMediaStore(directory string) *AccountMediaStore {
	return &AccountMediaStore{directory: directory}
}

func (store *AccountMediaStore) Save(ctx context.Context, accountID string, item domain.GoogleMediaItem, reader io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	accountDirectory, err := safePathSegment(accountID)
	if err != nil {
		return "", fmt.Errorf("invalid account ID: %w", err)
	}
	filename := path.Base(strings.ReplaceAll(strings.TrimSpace(item.Filename), "\\", "/"))
	if filename == "" || filename == "." || filename == "/" {
		filename = item.ProviderID
	}
	filename, err = safePathSegment(filename)
	if err != nil {
		return "", fmt.Errorf("invalid media filename: %w", err)
	}
	capturedAt := item.CreatedAt
	if capturedAt.IsZero() {
		capturedAt = time.Now()
	}
	relativeDirectory := filepath.Join(accountDirectory, fmt.Sprintf("%04d", capturedAt.Year()), fmt.Sprintf("%02d", capturedAt.Month()))
	directory := filepath.Join(store.directory, relativeDirectory)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return "", fmt.Errorf("create account media directory: %w", err)
	}

	temporary, err := os.CreateTemp(directory, ".download-*")
	if err != nil {
		return "", fmt.Errorf("create temporary media file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := io.Copy(temporary, reader); err != nil {
		temporary.Close()
		return "", fmt.Errorf("write media file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close media file: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o640); err != nil {
		return "", fmt.Errorf("set media permissions: %w", err)
	}

	store.mutex.Lock()
	destination := uniqueDestination(directory, filename)
	err = os.Rename(temporaryPath, destination)
	store.mutex.Unlock()
	if err != nil {
		return "", fmt.Errorf("commit media file: %w", err)
	}
	relativePath, err := filepath.Rel(store.directory, destination)
	if err != nil {
		return "", fmt.Errorf("resolve media path: %w", err)
	}
	return filepath.ToSlash(relativePath), nil
}

func safePathSegment(value string) (string, error) {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\\`) {
		return "", fmt.Errorf("unsafe path segment")
	}
	return value, nil
}

func uniqueDestination(directory, filename string) string {
	extension := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, extension)
	destination := filepath.Join(directory, filename)
	for attempt := 1; ; attempt++ {
		if _, err := os.Stat(destination); os.IsNotExist(err) {
			return destination
		}
		destination = filepath.Join(directory, fmt.Sprintf("%s-%d%s", base, attempt, extension))
	}
}
