package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/palpal/gphoto/internal/domain"
)

type gphotoHandlerFake struct{}

func (gphotoHandlerFake) Begin(context.Context, string) (string, error) {
	return "https://accounts.google.test/authorize", nil
}
func (gphotoHandlerFake) Complete(context.Context, string, string) (domain.GoogleAccount, error) {
	return domain.GoogleAccount{ID: "account-1"}, nil
}
func (gphotoHandlerFake) StartSession(context.Context, string) (domain.PickerSession, error) {
	return domain.PickerSession{ID: "session-1", PickerURI: "https://photos.google.test/picker"}, nil
}
func (gphotoHandlerFake) ListGoogleAccounts(context.Context) ([]domain.GoogleAccount, error) {
	return []domain.GoogleAccount{}, nil
}
func (gphotoHandlerFake) UpdateGoogleAccountMediaType(context.Context, string, domain.MediaType) error {
	return nil
}
func (gphotoHandlerFake) ListPickerSessions(context.Context, bool) ([]domain.PickerSession, error) {
	return []domain.PickerSession{}, nil
}

func TestGPhotoHandlerStartsAuthorization(t *testing.T) {
	fake := gphotoHandlerFake{}
	handler := NewGPhotoHandler(fake, fake, fake).Router()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/google/auth", strings.NewReader(`{"name":"Personal"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), "authorizationUrl") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestGPhotoHandlerRejectsMissingAccountName(t *testing.T) {
	fake := gphotoHandlerFake{}
	handler := NewGPhotoHandler(fake, fake, fake).Router()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/google/auth", strings.NewReader(`{"name":""}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}
