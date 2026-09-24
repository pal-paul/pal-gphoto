package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

type TokenVault struct {
	aead cipher.AEAD
}

func NewTokenVault(key []byte) (*TokenVault, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create token cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create token AEAD: %w", err)
	}
	return &TokenVault{aead: aead}, nil
}

func (vault *TokenVault) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, vault.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate token nonce: %w", err)
	}
	return vault.aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (vault *TokenVault) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < vault.aead.NonceSize() {
		return nil, fmt.Errorf("token ciphertext is truncated")
	}
	nonce, encrypted := ciphertext[:vault.aead.NonceSize()], ciphertext[vault.aead.NonceSize():]
	plaintext, err := vault.aead.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt token: %w", err)
	}
	return plaintext, nil
}
