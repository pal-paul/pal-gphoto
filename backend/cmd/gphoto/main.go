package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/palpal/gphoto/internal/config"
	"github.com/palpal/gphoto/internal/domain"
	"github.com/palpal/gphoto/internal/googlephotos"
	"github.com/palpal/gphoto/internal/httpapi"
	"github.com/palpal/gphoto/internal/secure"
	"github.com/palpal/gphoto/internal/service"
	"github.com/palpal/gphoto/internal/store"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		slog.Error("Google Photos backup stopped", "error", err)
		os.Exit(1)
	}
}

func run(arguments []string, output io.Writer) error {
	configuration, err := config.LoadGPhoto()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configuration.DatabasePath), 0o750); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if err := os.MkdirAll(configuration.MediaDirectory, 0o750); err != nil {
		return fmt.Errorf("create media directory: %w", err)
	}
	repository, err := store.NewSQLite(configuration.DatabasePath)
	if err != nil {
		return err
	}
	defer repository.Close()
	vault, err := secure.NewTokenVault(configuration.TokenEncryptionKey)
	if err != nil {
		return err
	}
	authorization := googlephotos.NewOAuthService(configuration.GoogleClientID, configuration.GoogleClientSecret, configuration.GoogleRedirectURL, repository, vault)
	providerFactory := func(ctx context.Context, account domain.GoogleAccount) (service.GooglePhotosProvider, error) {
		httpClient, err := authorization.HTTPClient(ctx, account.TokenCiphertext)
		if err != nil {
			return nil, fmt.Errorf("create authenticated Google client: %w", err)
		}
		return googlephotos.NewClient(httpClient, ""), nil
	}
	syncer := service.NewGooglePhotosSync(repository, service.NewAccountMediaStore(configuration.MediaDirectory), providerFactory)
	if len(arguments) > 0 {
		switch arguments[0] {
		case "auth":
			return runAuth(context.Background(), output, configuration.GoogleAccountEmail, authorization)
		case "accounts":
			return runAccounts(context.Background(), output, repository)
		case "pick":
			return runPick(context.Background(), output, configuration.GoogleAccountEmail, repository, syncer)
		default:
			return fmt.Errorf("unknown command %q; available commands: auth, accounts, pick", arguments[0])
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go service.RunGooglePhotosScheduler(ctx, configuration.PollInterval, syncer)

	server := &http.Server{
		Addr: configuration.ListenAddress, Handler: httpapi.NewGPhotoHandler(authorization, syncer, repository).Router(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Minute, IdleTimeout: time.Minute,
	}
	serverError := make(chan error, 1)
	go func() {
		slog.Info("Google Photos backup API listening", "address", configuration.ListenAddress)
		serverError <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return server.Shutdown(shutdownCtx)
	case err := <-serverError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func runAuth(ctx context.Context, output io.Writer, email string, authorization *googlephotos.OAuthService) error {
	if email == "" {
		return fmt.Errorf("GOOGLE_ACCOUNT_EMAIL is required for the auth command")
	}
	authorizationURL, err := authorization.BeginForEmail(ctx, email, email)
	if err != nil {
		return err
	}
	fmt.Fprintln(output, "Open this URL in a browser and complete Google authorization:")
	fmt.Fprintln(output, authorizationURL)
	fmt.Fprintln(output, "The running container will save the encrypted refresh token after Google redirects to the callback URL.")
	return nil
}

func runAccounts(ctx context.Context, output io.Writer, repository *store.SQLite) error {
	accounts, err := repository.ListGoogleAccounts(ctx)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(accounts)
}

func runPick(ctx context.Context, output io.Writer, email string, repository *store.SQLite, syncer *service.GooglePhotosSync) error {
	if email == "" {
		return fmt.Errorf("GOOGLE_ACCOUNT_EMAIL is required for the pick command")
	}
	accounts, err := repository.ListGoogleAccounts(ctx)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if !strings.EqualFold(account.Email, email) {
			continue
		}
		session, err := syncer.StartSession(ctx, account.ID)
		if err != nil {
			return err
		}
		fmt.Fprintln(output, "Open this URL and select the photos, videos, or albums to import:")
		fmt.Fprintln(output, session.PickerURI)
		return nil
	}
	return fmt.Errorf("Google account %q is not authorized; run the auth command first", email)
}
