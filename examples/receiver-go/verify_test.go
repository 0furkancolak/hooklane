package main

import (
	"net/http"
	"testing"
	"time"
)

// Test vector from the Standard Webhooks specification, which Hooklane's
// signer is also tested against.
func TestVerifyStandardWebhooksVector(t *testing.T) {
	secret := "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	body := []byte(`{"test": 2432232314}`)
	h := http.Header{}
	h.Set("webhook-id", "msg_p5jXN8AQM9LWM0D4loKWxJek")
	h.Set("webhook-timestamp", "1614265330")
	h.Set("webhook-signature", "v1,bogus v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=")
	now := time.Unix(1614265330, 0).Add(time.Minute)

	if err := verify(secret, h, body, now); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := verify(secret, h, []byte(`{"test": 1}`), now); err == nil {
		t.Fatal("tampered body accepted")
	}
	if err := verify(secret, h, body, now.Add(time.Hour)); err == nil {
		t.Fatal("stale timestamp accepted")
	}
}
