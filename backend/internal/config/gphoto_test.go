package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadGPhotoDefaults(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_ACCOUNT_EMAIL", " me@example.com ")
	t.Setenv("PAL_GPHOTO_TOKEN_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))

	got, err := LoadGPhoto()
	if err != nil {
		t.Fatalf("LoadGPhoto() error = %v", err)
	}
	if got.ListenAddress != ":8080" || got.MediaDirectory != "/data/media" || got.GoogleAccountEmail != "me@example.com" {
		t.Fatalf("LoadGPhoto() defaults = %#v", got)
	}
}

func TestLoadGPhotoRejectsInvalidTokenKey(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("PAL_GPHOTO_TOKEN_KEY", "too-short")

	_, err := LoadGPhoto()
	if err == nil || !strings.Contains(err.Error(), "TOKEN_KEY") {
		t.Fatalf("LoadGPhoto() error = %v, want token key error", err)
	}
}
