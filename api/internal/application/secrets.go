package application

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

const endpointSecretPrefix = "whsec_"

// SecretBox encrypts endpoint signing secrets at rest with AES-256-GCM.
// Secrets must stay recoverable because the worker signs with them.
type SecretBox struct {
	aead cipher.AEAD
}

func NewSecretBox(key []byte) (*SecretBox, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secret key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{aead: aead}, nil
}

func (b *SecretBox) Seal(plaintext string) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return b.aead.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

func (b *SecretBox) Open(sealed []byte) (string, error) {
	size := b.aead.NonceSize()
	if len(sealed) < size {
		return "", errors.New("sealed secret too short")
	}
	plaintext, err := b.aead.Open(nil, sealed[:size], sealed[size:], nil)
	if err != nil {
		return "", fmt.Errorf("open secret: %w", err)
	}
	return string(plaintext), nil
}

// GenerateEndpointSecret returns a Standard Webhooks style secret: "whsec_"
// followed by base64 of 24 random bytes.
func GenerateEndpointSecret() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate endpoint secret: %w", err)
	}
	return endpointSecretPrefix + base64.StdEncoding.EncodeToString(raw), nil
}
