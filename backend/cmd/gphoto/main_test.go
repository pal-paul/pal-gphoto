package main

import (
	"bytes"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthCommandPrintsConfiguredAccountURL(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_ACCOUNT_EMAIL", "me@example.com")
	t.Setenv("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/google/callback")
	t.Setenv("PAL_GPHOTO_TOKEN_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("PAL_GPHOTO_DATABASE_PATH", filepath.Join(directory, "state.db"))
	t.Setenv("PAL_GPHOTO_MEDIA_DIR", filepath.Join(directory, "media"))

	var output bytes.Buffer
	if err := run([]string{"auth"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "login_hint=me%40example.com") || !strings.Contains(output.String(), "access_type=offline") {
		t.Fatalf("auth output = %q", output.String())
	}
}
