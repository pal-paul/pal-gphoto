package googlephotos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/palpal/gphoto/internal/domain"
	"github.com/palpal/gphoto/internal/secure"
	"golang.org/x/oauth2"
)

type oauthMemoryRepository struct {
	state         string
	accountName   string
	expectedEmail string
	account       domain.GoogleAccount
}

func (repository *oauthMemoryRepository) PutOAuthState(_ context.Context, state, accountName, expectedEmail string, _ time.Time) error {
	repository.state, repository.accountName, repository.expectedEmail = state, accountName, expectedEmail
	return nil
}

func (repository *oauthMemoryRepository) ConsumeOAuthState(_ context.Context, state string, _ time.Time) (string, string, error) {
	if state != repository.state {
		return "", "", fmt.Errorf("state mismatch")
	}
	repository.state = ""
	return repository.accountName, repository.expectedEmail, nil
}

func (repository *oauthMemoryRepository) SaveGoogleAccount(_ context.Context, account domain.GoogleAccount) (domain.GoogleAccount, error) {
	repository.account = account
	return account, nil
}

func TestOAuthServiceCompletesAndEncryptsToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			writer.Header().Set("Content-Type", "application/json")
			json.NewEncoder(writer).Encode(map[string]any{"access_token": "access-secret", "refresh_token": "refresh-secret", "token_type": "Bearer", "expires_in": 3600})
		case "/userinfo":
			json.NewEncoder(writer).Encode(map[string]string{"sub": "google-subject", "email": "me@example.com"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	repository := &oauthMemoryRepository{}
	vault, err := secure.NewTokenVault(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := newOAuthService("client", "secret", "http://localhost/callback", repository, vault, oauth2.Endpoint{AuthURL: server.URL + "/auth", TokenURL: server.URL + "/token"}, server.URL+"/userinfo")
	authorizationURL, err := service.BeginForEmail(context.Background(), "Personal", "me@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(authorizationURL, "access_type=offline") || !strings.Contains(authorizationURL, "login_hint=me%40example.com") || repository.state == "" {
		t.Fatalf("Begin() URL = %q, state = %q", authorizationURL, repository.state)
	}
	account, err := service.Complete(context.Background(), repository.state, "authorization-code")
	if err != nil {
		t.Fatal(err)
	}
	if account.GoogleSubject != "google-subject" || account.Email != "me@example.com" || account.Name != "Personal" {
		t.Fatalf("Complete() = %+v", account)
	}
	if strings.Contains(string(account.TokenCiphertext), "access-secret") || strings.Contains(string(account.TokenCiphertext), "refresh-secret") {
		t.Fatal("stored OAuth token is not encrypted")
	}

	mismatchRepository := &oauthMemoryRepository{}
	mismatchService := newOAuthService("client", "secret", "http://localhost/callback", mismatchRepository, vault, oauth2.Endpoint{AuthURL: server.URL + "/auth", TokenURL: server.URL + "/token"}, server.URL+"/userinfo")
	if _, err := mismatchService.BeginForEmail(context.Background(), "Other", "other@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := mismatchService.Complete(context.Background(), mismatchRepository.state, "authorization-code"); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("Complete() mismatch error = %v", err)
	}
	if mismatchRepository.account.ID != "" {
		t.Fatal("mismatched Google account was persisted")
	}
}
