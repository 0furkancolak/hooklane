package http

import (
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestRateLimiterRefillsOverTime(t *testing.T) {
	clock := time.Unix(1_000_000, 0)
	limiter := newRateLimiter(2, 3, func() time.Time { return clock })

	for i := range 3 {
		if ok, _ := limiter.allow("a"); !ok {
			t.Fatalf("request %d within the burst was rejected", i+1)
		}
	}
	ok, wait := limiter.allow("a")
	if ok {
		t.Fatal("request beyond the burst was allowed")
	}
	if wait != 500*time.Millisecond {
		t.Fatalf("wait = %s, want 500ms", wait)
	}
	if ok, _ := limiter.allow("b"); !ok {
		t.Fatal("another client was limited by a's usage")
	}

	clock = clock.Add(time.Second) // refills two tokens
	for i := range 2 {
		if ok, _ := limiter.allow("a"); !ok {
			t.Fatalf("request %d after refill was rejected", i+1)
		}
	}
	if ok, _ := limiter.allow("a"); ok {
		t.Fatal("bucket refilled beyond the rate")
	}
}

func TestRateLimiterDropsIdleBuckets(t *testing.T) {
	clock := time.Unix(1_000_000, 0)
	limiter := newRateLimiter(1, 1, func() time.Time { return clock })
	limiter.allow("a")
	clock = clock.Add(idleBucketTTL + time.Second)
	limiter.allow("b")
	if _, kept := limiter.buckets["a"]; kept {
		t.Fatal("idle bucket was not removed")
	}
}

func TestRateLimitMiddlewareReturns429(t *testing.T) {
	app := fiber.New()
	app.Use(rateLimit(newRateLimiter(1, 2, time.Now)))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	statuses := make([]int, 0, 3)
	var last *stdhttp.Response
	for range 3 {
		resp, err := app.Test(httptest.NewRequest(stdhttp.MethodGet, "/", nil))
		if err != nil {
			t.Fatal(err)
		}
		statuses = append(statuses, resp.StatusCode)
		last = resp
	}
	if statuses[0] != 200 || statuses[1] != 200 || statuses[2] != stdhttp.StatusTooManyRequests {
		t.Fatalf("statuses = %v, want [200 200 429]", statuses)
	}
	if last.Header.Get("Retry-After") == "" {
		t.Fatal("429 has no Retry-After header")
	}
	body, _ := io.ReadAll(last.Body)
	if !strings.Contains(string(body), `"rate_limited"`) {
		t.Fatalf("body = %s", body)
	}
}
