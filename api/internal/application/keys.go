package application

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

const apiKeyPrefix = "hlk_"

func GenerateAPIKey() (raw string, prefix string, hash []byte, err error) {
	secret := make([]byte, 32)
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
