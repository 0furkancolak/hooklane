package application

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

const (
	apiKeyPrefix          = "hlk_"
	apiKeySecretHexLength = 64
)

func GenerateAPIKey() (raw string, prefix string, hash []byte, err error) {
	secret := make([]byte, apiKeySecretHexLength/2)
	if _, err := rand.Read(secret); err != nil {
		return "", "", nil, fmt.Errorf("generate api key: %w", err)
	}
	raw = apiKeyPrefix + hex.EncodeToString(secret)
	prefix = raw[:8]
	hash = HashAPIKey(raw)
	return raw, prefix, hash, nil
}

func HashAPIKey(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func EqualHash(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
