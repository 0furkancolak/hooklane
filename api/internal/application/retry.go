package application

import (
	"errors"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/domain"
)

// RetryPolicy decides what happens after a failed attempt.
//
// After the n-th failed attempt (n >= 1) the nominal delay is
// d = min(MaxDelay, BaseDelay * 2^(n-1)) and the scheduled delay is drawn
// uniformly from [d/2, d] ("equal jitter"). A Retry-After hint from the
// receiver raises the delay but never above MaxDelay. Once MaxAttempts
// attempts have failed, or a failure is permanent, the delivery is
// dead-lettered.
type RetryPolicy struct {
	MaxAttempts int32
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	// Rand returns a value in [0, 1). Nil uses math/rand/v2.
	Rand func() float64
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 10, BaseDelay: 30 * time.Second, MaxDelay: 4 * time.Hour}
}

func (p RetryPolicy) Validate() error {
	if p.MaxAttempts < 1 || p.BaseDelay <= 0 || p.MaxDelay < p.BaseDelay {
		return errors.New("retry policy requires MaxAttempts >= 1 and 0 < BaseDelay <= MaxDelay")
	}
	return nil
}

// NominalDelay is the upper bound of the delay after the given failed attempt.
func (p RetryPolicy) NominalDelay(attempt int32) time.Duration {
	d := p.BaseDelay
	for i := int32(1); i < attempt && d < p.MaxDelay; i++ {
		d *= 2
	}
	return min(d, p.MaxDelay)
}

// Backoff returns the jittered delay after the given failed attempt.
func (p RetryPolicy) Backoff(attempt int32) time.Duration {
	d := p.NominalDelay(attempt)
	r := rand.Float64
	if p.Rand != nil {
		r = p.Rand
	}
	return d/2 + time.Duration(r()*float64(d-d/2))
}

// Next returns the status after a failed attempt and, for retries, the delay.
func (p RetryPolicy) Next(attempt int32, retryable bool, retryAfter time.Duration) (domain.DeliveryStatus, *time.Duration) {
	if !retryable || attempt >= p.MaxAttempts {
		return domain.DeliveryDeadLetter, nil
	}
	delay := min(max(p.Backoff(attempt), retryAfter), p.MaxDelay)
	return domain.DeliveryPending, &delay
}

// Classify reports whether an attempt succeeded and, if not, whether retrying
// can help. Transport failures (DNS, connect, TLS, timeouts) and 408, 425,
// 429 and 5xx responses are retryable. Other responses, including redirects,
// which are not followed, are permanent.
func Classify(result WebhookResult) (succeeded, retryable bool) {
	switch code := result.StatusCode; {
	case code >= 200 && code < 300:
		return true, false
	case code == 0:
		// A blocked destination stays blocked; everything else without a
		// response (DNS, connect, TLS, timeout) may be transient.
		return false, result.Err != nil && !errors.Is(result.Err, ErrDestinationNotAllowed)
	case code == http.StatusRequestTimeout, code == http.StatusTooEarly, code == http.StatusTooManyRequests, code >= 500:
		return false, true
	default:
		return false, false
	}
}
