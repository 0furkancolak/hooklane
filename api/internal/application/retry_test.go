package application_test

import (
	"errors"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
)

func TestClassify(t *testing.T) {
	transport := errors.New("dial tcp: i/o timeout")
	cases := []struct {
		name                 string
		result               application.WebhookResult
		succeeded, retryable bool
	}{
		{"200", application.WebhookResult{StatusCode: 200}, true, false},
		{"204", application.WebhookResult{StatusCode: 204}, true, false},
		{"transport error", application.WebhookResult{Err: transport}, false, true},
		{"301 redirect", application.WebhookResult{StatusCode: 301}, false, false},
		{"400", application.WebhookResult{StatusCode: 400}, false, false},
		{"401", application.WebhookResult{StatusCode: 401}, false, false},
		{"404", application.WebhookResult{StatusCode: 404}, false, false},
		{"408", application.WebhookResult{StatusCode: 408}, false, true},
		{"410", application.WebhookResult{StatusCode: 410}, false, false},
		{"425", application.WebhookResult{StatusCode: 425}, false, true},
		{"429", application.WebhookResult{StatusCode: 429}, false, true},
		{"500", application.WebhookResult{StatusCode: 500}, false, true},
		{"503", application.WebhookResult{StatusCode: 503}, false, true},
		{"2xx with body read error", application.WebhookResult{StatusCode: 200, Err: transport}, true, false},
	}
	for _, tc := range cases {
		s, r := application.Classify(tc.result)
		if s != tc.succeeded || r != tc.retryable {
			t.Errorf("%s: got (%v,%v) want (%v,%v)", tc.name, s, r, tc.succeeded, tc.retryable)
		}
	}
}

func TestBackoffStaysWithinDocumentedBounds(t *testing.T) {
	src := rand.New(rand.NewPCG(1, 2))
	p := application.RetryPolicy{MaxAttempts: 10, BaseDelay: 30 * time.Second, MaxDelay: 4 * time.Hour, Rand: src.Float64}
	wantNominal := []time.Duration{
		30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute,
		16 * time.Minute, 32 * time.Minute, 64 * time.Minute, 128 * time.Minute, 4 * time.Hour,
	}
	for i, want := range wantNominal {
		attempt := int32(i + 1)
		if got := p.NominalDelay(attempt); got != want {
			t.Fatalf("attempt %d nominal = %v, want %v", attempt, got, want)
		}
		lo, hi := want/2, want
		var seenLow, seenHigh bool
		for range 2000 {
			d := p.Backoff(attempt)
			if d < lo || d > hi {
				t.Fatalf("attempt %d backoff %v outside [%v, %v]", attempt, d, lo, hi)
			}
			seenLow = seenLow || d < lo+(hi-lo)/10
			seenHigh = seenHigh || d > hi-(hi-lo)/10
		}
		if !seenLow || !seenHigh {
			t.Fatalf("attempt %d jitter does not span the range", attempt)
		}
	}
	if got := p.NominalDelay(1000); got != 4*time.Hour {
		t.Fatalf("large attempt numbers must cap without overflow: %v", got)
	}
}

func TestBackoffIsDeterministicForFixedRandom(t *testing.T) {
	p := application.RetryPolicy{MaxAttempts: 5, BaseDelay: time.Minute, MaxDelay: time.Hour, Rand: func() float64 { return 0 }}
	if d := p.Backoff(3); d != 2*time.Minute {
		t.Fatalf("rand=0 gives the lower bound: %v", d)
	}
	p.Rand = func() float64 { return 0.999999999 }
	if d := p.Backoff(3); d < 4*time.Minute-time.Millisecond || d > 4*time.Minute {
		t.Fatalf("rand→1 gives the upper bound: %v", d)
	}
}

func TestNextSchedulesRetriesThenDeadLetters(t *testing.T) {
	p := application.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: 10 * time.Second, Rand: func() float64 { return 0 }}

	status, delay := p.Next(1, true, 0)
	if status != domain.DeliveryPending || delay == nil || *delay != 500*time.Millisecond {
		t.Fatalf("attempt 1: %s %v", status, delay)
	}
	status, delay = p.Next(2, true, 5*time.Second)
	if status != domain.DeliveryPending || *delay != 5*time.Second {
		t.Fatalf("Retry-After above backoff wins: %s %v", status, *delay)
	}
	_, delay = p.Next(2, true, time.Hour)
	if *delay != 10*time.Second {
		t.Fatalf("Retry-After is capped at MaxDelay: %v", *delay)
	}
	if status, delay = p.Next(3, true, 0); status != domain.DeliveryDeadLetter || delay != nil {
		t.Fatalf("exhausted: %s %v", status, delay)
	}
	if status, delay = p.Next(1, false, 0); status != domain.DeliveryDeadLetter || delay != nil {
		t.Fatalf("permanent failure: %s %v", status, delay)
	}
}

func TestRetryPolicyValidate(t *testing.T) {
	if err := application.DefaultRetryPolicy().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []application.RetryPolicy{
		{MaxAttempts: 0, BaseDelay: time.Second, MaxDelay: time.Minute},
		{MaxAttempts: 3, BaseDelay: 0, MaxDelay: time.Minute},
		{MaxAttempts: 3, BaseDelay: time.Minute, MaxDelay: time.Second},
	} {
		if p.Validate() == nil {
			t.Fatalf("%+v should be invalid", p)
		}
	}
}
