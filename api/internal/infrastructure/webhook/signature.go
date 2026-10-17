// Package webhook signs and sends outbound webhook requests following the
// Standard Webhooks conventions (https://www.standardwebhooks.com).
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderID        = "webhook-id"
	HeaderTimestamp = "webhook-timestamp"
	HeaderSignature = "webhook-signature"

	secretPrefix     = "whsec_"
	signatureVersion = "v1"

	// DefaultTolerance is how far a receiver accepts timestamps from its clock.
	DefaultTolerance = 5 * time.Minute
)

var (
	ErrInvalidSecret    = errors.New("invalid webhook secret")
	ErrMissingHeaders   = errors.New("missing webhook headers")
	ErrInvalidTimestamp = errors.New("webhook timestamp outside tolerance")
	ErrNoMatch          = errors.New("no matching webhook signature")
)

func decodeSecret(secret string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, secretPrefix))
	if err != nil || len(key) == 0 {
		return nil, ErrInvalidSecret
	}
	return key, nil
}

// Sign returns the webhook-signature header value: "v1,<base64 HMAC-SHA256>"
// over "<id>.<unix timestamp>.<body>".
func Sign(secret, id string, timestamp time.Time, body []byte) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", err
	}
	return signatureVersion + "," + base64.StdEncoding.EncodeToString(mac(key, id, timestamp.Unix(), body)), nil
}

// Verify checks a received webhook the way a compatible receiver would.
// signatureHeader may hold several space-separated signatures.
func Verify(secret, id, timestampHeader, signatureHeader string, body []byte, now time.Time, tolerance time.Duration) error {
	if id == "" || timestampHeader == "" || signatureHeader == "" {
		return ErrMissingHeaders
	}
	key, err := decodeSecret(secret)
	if err != nil {
		return err
	}
	unix, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return ErrInvalidTimestamp
	}
	if d := now.Sub(time.Unix(unix, 0)); d > tolerance || d < -tolerance {
		return ErrInvalidTimestamp
	}
	expected := mac(key, id, unix, body)
	for _, candidate := range strings.Fields(signatureHeader) {
		version, encoded, ok := strings.Cut(candidate, ",")
		if !ok || version != signatureVersion {
			continue
		}
		got, err := base64.StdEncoding.DecodeString(encoded)
		if err == nil && hmac.Equal(got, expected) {
			return nil
		}
	}
	return ErrNoMatch
}

func mac(key []byte, id string, unix int64, body []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(id))
	h.Write([]byte("."))
	h.Write([]byte(strconv.FormatInt(unix, 10)))
	h.Write([]byte("."))
	h.Write(body)
	return h.Sum(nil)
}
