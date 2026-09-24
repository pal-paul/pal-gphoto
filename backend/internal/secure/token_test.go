package secure

import (
	"bytes"
	"testing"
)

func TestTokenVaultRoundTrip(t *testing.T) {
	vault, err := NewTokenVault(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"access_token":"secret"}`)
	ciphertext, err := vault.Encrypt(want)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("secret")) {
		t.Fatal("ciphertext contains plaintext token")
	}
	got, err := vault.Decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Decrypt() = %q, want %q", got, want)
	}
}
