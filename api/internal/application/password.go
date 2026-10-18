package application

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters follow the OWASP minimum recommendation
// (19 MiB, 2 iterations, 1 lane).
const (
	argonMemoryKiB = 19 * 1024
	argonTime      = 2
	argonThreads   = 1
	argonKeyLength = 32
	argonSaltLen   = 16

	// hashSlots bounds concurrent hashing: each one holds argonMemoryKiB.
	hashSlots = 4
)

var hashGate = make(chan struct{}, hashSlots)

// HashPassword returns a PHC-formatted argon2id hash with a random salt.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := deriveKey(password, salt, argonMemoryKiB, argonTime, argonThreads, argonKeyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches an encoded hash. A hash it
// cannot parse never matches.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false
	}
	// Parameters come from the database, but never let a bad row exhaust memory.
	if memory == 0 || memory > 256*1024 || iterations == 0 || iterations > 10 || threads == 0 || threads > 8 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := deriveKey(password, salt, memory, iterations, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func deriveKey(password string, salt []byte, memory, iterations uint32, threads uint8, length uint32) []byte {
	hashGate <- struct{}{}
	defer func() { <-hashGate }()
	return argon2.IDKey([]byte(password), salt, iterations, memory, threads, length)
}

// dummyHash lets a login for an unknown email cost the same as a real one.
var dummyHash = func() string {
	h, err := HashPassword("hooklane-timing-equaliser")
	if err != nil {
		panic(err)
	}
	return h
}()
