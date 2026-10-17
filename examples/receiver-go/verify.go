package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// tolerance rejects replays of old requests and requests from badly skewed clocks.
const tolerance = 5 * time.Minute

var errInvalid = errors.New("invalid webhook signature")

// verify checks a Hooklane (Standard Webhooks) signature. body must be the
// raw request bytes, before any JSON decoding.
func verify(secret string, h http.Header, body []byte, now time.Time) error {
	id, ts, sigs := h.Get("webhook-id"), h.Get("webhook-timestamp"), h.Get("webhook-signature")
	if id == "" || ts == "" || sigs == "" {
		return errInvalid
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errInvalid
	}
	if age := now.Sub(time.Unix(unix, 0)); age > tolerance || age < -tolerance {
		return errInvalid
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return err
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(id + "." + ts + "."))
	m.Write(body)
	expected := m.Sum(nil)

	// The header may carry several signatures, e.g. during secret rotation.
	for _, candidate := range strings.Fields(sigs) {
		version, encoded, ok := strings.Cut(candidate, ",")
		if !ok || version != "v1" {
			continue
		}
		if got, err := base64.StdEncoding.DecodeString(encoded); err == nil && hmac.Equal(got, expected) {
			return nil
		}
	}
	return errInvalid
}
