package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type GPhotoConfig struct {
	ListenAddress      string
	MediaDirectory     string
	DatabasePath       string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	GoogleAccountEmail string
	TokenEncryptionKey []byte
	PollInterval       time.Duration
}

func LoadGPhoto() (GPhotoConfig, error) {
	dataDirectory := envOr("PAL_GPHOTO_DATA_DIR", "/data")
	pollInterval, err := time.ParseDuration(envOr("PAL_GPHOTO_POLL_INTERVAL", "30s"))
	if err != nil || pollInterval <= 0 {
		return GPhotoConfig{}, fmt.Errorf("PAL_GPHOTO_POLL_INTERVAL must be a positive duration")
	}

	key, err := base64.StdEncoding.DecodeString(os.Getenv("PAL_GPHOTO_TOKEN_KEY"))
	if err != nil || len(key) != 32 {
		return GPhotoConfig{}, errors.New("PAL_GPHOTO_TOKEN_KEY must be a base64-encoded 32-byte key")
	}

	configuration := GPhotoConfig{
		ListenAddress:      envOr("PAL_GPHOTO_LISTEN_ADDR", ":8080"),
		MediaDirectory:     envOr("PAL_GPHOTO_MEDIA_DIR", dataDirectory+"/media"),
		DatabasePath:       envOr("PAL_GPHOTO_DATABASE_PATH", dataDirectory+"/pal-gphoto.db"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:  envOr("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/google/callback"),
		GoogleAccountEmail: strings.TrimSpace(os.Getenv("GOOGLE_ACCOUNT_EMAIL")),
		TokenEncryptionKey: key,
		PollInterval:       pollInterval,
	}
	if configuration.GoogleClientID == "" || configuration.GoogleClientSecret == "" {
		return GPhotoConfig{}, errors.New("GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET are required")
	}
	return configuration, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
