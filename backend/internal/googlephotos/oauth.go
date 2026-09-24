package googlephotos

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/palpal/gphoto/internal/domain"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const pickerScope = "https://www.googleapis.com/auth/photospicker.mediaitems.readonly"

type OAuthRepository interface {
	PutOAuthState(context.Context, string, string, string, time.Time) error
	ConsumeOAuthState(context.Context, string, time.Time) (string, string, error)
	SaveGoogleAccount(context.Context, domain.GoogleAccount) (domain.GoogleAccount, error)
}

type TokenCipher interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}

type OAuthService struct {
	configuration oauth2.Config
	repository    OAuthRepository
	vault         TokenCipher
	userinfoURL   string
	now           func() time.Time
}

func NewOAuthService(clientID, clientSecret, redirectURL string, repository OAuthRepository, vault TokenCipher) *OAuthService {
	return newOAuthService(clientID, clientSecret, redirectURL, repository, vault, google.Endpoint, "https://openidconnect.googleapis.com/v1/userinfo")
}

func newOAuthService(clientID, clientSecret, redirectURL string, repository OAuthRepository, vault TokenCipher, endpoint oauth2.Endpoint, userinfoURL string) *OAuthService {
	return &OAuthService{
		configuration: oauth2.Config{
			ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL, Endpoint: endpoint,
			Scopes: []string{pickerScope, "openid", "email", "profile"},
		},
		repository:  repository,
		vault:       vault,
		userinfoURL: userinfoURL,
		now:         time.Now,
	}
}

func (service *OAuthService) Begin(ctx context.Context, accountName string) (string, error) {
	return service.BeginForEmail(ctx, accountName, "")
}

func (service *OAuthService) BeginForEmail(ctx context.Context, accountName, expectedEmail string) (string, error) {
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("generate OAuth state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	if err := service.repository.PutOAuthState(ctx, state, accountName, expectedEmail, service.now().Add(10*time.Minute)); err != nil {
		return "", fmt.Errorf("store OAuth state: %w", err)
	}
	options := []oauth2.AuthCodeOption{oauth2.AccessTypeOffline, oauth2.ApprovalForce}
	if expectedEmail != "" {
		options = append(options, oauth2.SetAuthURLParam("login_hint", expectedEmail))
	}
	return service.configuration.AuthCodeURL(state, options...), nil
}

func (service *OAuthService) Complete(ctx context.Context, state, code string) (domain.GoogleAccount, error) {
	accountName, expectedEmail, err := service.repository.ConsumeOAuthState(ctx, state, service.now())
	if err != nil {
		return domain.GoogleAccount{}, fmt.Errorf("consume OAuth state: %w", err)
	}
	token, err := service.configuration.Exchange(ctx, code)
	if err != nil {
		return domain.GoogleAccount{}, fmt.Errorf("exchange OAuth code: %w", err)
	}
	identity, err := service.userinfo(ctx, token)
	if err != nil {
		return domain.GoogleAccount{}, err
	}
	if expectedEmail != "" && !strings.EqualFold(identity.Email, expectedEmail) {
		return domain.GoogleAccount{}, fmt.Errorf("authenticated Google account %q does not match configured email %q", identity.Email, expectedEmail)
	}
	encodedToken, err := json.Marshal(token)
	if err != nil {
		return domain.GoogleAccount{}, fmt.Errorf("encode OAuth token: %w", err)
	}
	ciphertext, err := service.vault.Encrypt(encodedToken)
	if err != nil {
		return domain.GoogleAccount{}, fmt.Errorf("encrypt OAuth token: %w", err)
	}
	account := domain.GoogleAccount{
		ID: uuid.NewString(), GoogleSubject: identity.Subject, Email: identity.Email, Name: accountName,
		MediaType: domain.MediaTypeAll, TokenCiphertext: ciphertext, CreatedAt: service.now().UTC(),
	}
	account, err = service.repository.SaveGoogleAccount(ctx, account)
	if err != nil {
		return domain.GoogleAccount{}, fmt.Errorf("save Google account: %w", err)
	}
	return account, nil
}

func (service *OAuthService) HTTPClient(ctx context.Context, ciphertext []byte) (*http.Client, error) {
	plaintext, err := service.vault.Decrypt(ciphertext)
	if err != nil {
		return nil, err
	}
	var token oauth2.Token
	if err := json.Unmarshal(plaintext, &token); err != nil {
		return nil, fmt.Errorf("decode OAuth token: %w", err)
	}
	return service.configuration.Client(ctx, &token), nil
}

func (service *OAuthService) userinfo(ctx context.Context, token *oauth2.Token) (struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
}, error) {
	var identity struct {
		Subject string `json:"sub"`
		Email   string `json:"email"`
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, service.userinfoURL, nil)
	if err != nil {
		return identity, err
	}
	response, err := service.configuration.Client(ctx, token).Do(request)
	if err != nil {
		return identity, fmt.Errorf("get Google identity: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return identity, fmt.Errorf("get Google identity: returned %s", response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(&identity); err != nil {
		return identity, fmt.Errorf("decode Google identity: %w", err)
	}
	if identity.Subject == "" || identity.Email == "" {
		return identity, fmt.Errorf("Google identity is missing subject or email")
	}
	return identity, nil
}
