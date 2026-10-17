package webhook

import (
	"errors"
	"testing"
	"time"
)

// Test vector published in the Standard Webhooks specification repository.
func TestSignMatchesStandardWebhooksVector(t *testing.T) {
	sig, err := Sign("whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", "msg_p5jXN8AQM9LWM0D4loKWxJek",
		time.Unix(1614265330, 0), []byte(`{"test": 2432232314}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="; sig != want {
		t.Fatalf("signature = %s, want %s", sig, want)
	}
}

func TestVerify(t *testing.T) {
	secret := "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	now := time.Unix(1_700_000_000, 0)
	body := []byte(`{"a":1}`)
	sig, err := Sign(secret, "evt_1", now, body)
	if err != nil {
		t.Fatal(err)
	}
	ts := "1700000000"

	if err := Verify(secret, "evt_1", ts, "v1,bogus "+sig, body, now, DefaultTolerance); err != nil {
		t.Fatalf("valid signature among several: %v", err)
	}
	cases := []struct {
		name              string
		id, ts, sig, body string
		now               time.Time
		want              error
	}{
		{"tampered body", "evt_1", ts, sig, `{"a":2}`, now, ErrNoMatch},
		{"other id", "evt_2", ts, sig, string(body), now, ErrNoMatch},
		{"too old", "evt_1", ts, sig, string(body), now.Add(6 * time.Minute), ErrInvalidTimestamp},
		{"too new", "evt_1", ts, sig, string(body), now.Add(-6 * time.Minute), ErrInvalidTimestamp},
		{"bad timestamp", "evt_1", "abc", sig, string(body), now, ErrInvalidTimestamp},
		{"missing sig", "evt_1", ts, "", string(body), now, ErrMissingHeaders},
		{"wrong version", "evt_1", ts, "v2" + sig[2:], string(body), now, ErrNoMatch},
	}
	for _, tc := range cases {
		if err := Verify(secret, tc.id, tc.ts, tc.sig, []byte(tc.body), tc.now, DefaultTolerance); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v want %v", tc.name, err, tc.want)
		}
	}
	if _, err := Sign("whsec_!!!", "x", now, body); !errors.Is(err, ErrInvalidSecret) {
		t.Fatalf("invalid secret: %v", err)
	}
}
