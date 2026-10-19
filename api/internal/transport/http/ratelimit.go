package http

import (
	"context"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

// idleBucketTTL is how long an untouched client keeps its bucket. A bucket
// idle for this long would have refilled completely anyway, so dropping it
// changes nothing for the client and bounds memory.
const idleBucketTTL = 10 * time.Minute

type bucket struct {
	tokens float64
	last   time.Time
}

// rateLimiter is a token bucket per client address, held in process memory.
// With several API replicas each enforces its own limit.
type rateLimiter struct {
	rate  float64
	burst float64
	now   func() time.Time

	mu        sync.Mutex
	buckets   map[string]*bucket
	lastSweep time.Time
}

func newRateLimiter(ratePerSecond float64, burst int, now func() time.Time) *rateLimiter {
	return &rateLimiter{
		rate:      ratePerSecond,
		burst:     float64(burst),
		now:       now,
		buckets:   map[string]*bucket{},
		lastSweep: now(),
	}
}

// allow takes one token for key. When none is left it returns how long until
// one is.
func (l *rateLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
	return false, wait
}

func (l *rateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < idleBucketTTL {
		return
	}
	l.lastSweep = now
	for key, b := range l.buckets {
		if now.Sub(b.last) >= idleBucketTTL {
			delete(l.buckets, key)
		}
	}
}

// allower decides whether a request from key may proceed and, when it may
// not, how long the caller should wait.
type allower interface {
	Allow(ctx context.Context, key string) (bool, time.Duration)
}

// Allow makes the in-memory limiter an allower.
func (l *rateLimiter) Allow(_ context.Context, key string) (bool, time.Duration) {
	return l.allow(key)
}

// SharedCounter counts requests across API replicas (PostgreSQL).
type SharedCounter interface {
	Allow(ctx context.Context, key string) (bool, time.Duration, error)
}

// sharedAllower lets requests through when the shared counter is unavailable:
// a database hiccup should not turn into an outage.
type sharedAllower struct {
	counter SharedCounter
	onError func(error)
}

func (s sharedAllower) Allow(ctx context.Context, key string) (bool, time.Duration) {
	ok, wait, err := s.counter.Allow(ctx, key)
	if err != nil {
		s.onError(err)
		return true, 0
	}
	return ok, wait
}

// rateLimit rejects requests over the limit with 429 and a Retry-After. It
// keys on the client address: the connection address, or the forwarded one
// when the connection comes from a configured trusted proxy.
func rateLimit(limiter allower) fiber.Handler {
	return func(c fiber.Ctx) error {
		ok, wait := limiter.Allow(c.Context(), strings.Clone(c.IP()))
		if ok {
			return c.Next()
		}
		seconds := int(math.Ceil(wait.Seconds()))
		c.Set("Retry-After", strconv.Itoa(max(seconds, 1)))
		return c.Status(fiber.StatusTooManyRequests).JSON(errorBody{Error: errorDetail{
			Code:    "rate_limited",
			Message: "too many requests",
		}})
	}
}
