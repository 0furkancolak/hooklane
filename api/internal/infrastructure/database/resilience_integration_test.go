package database_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/webhook"
)

func newTestWorker(t *testing.T, tn tenant, box *application.SecretBox, timeout time.Duration, lease time.Duration, shutdown time.Duration) *application.DeliveryWorker {
	t.Helper()
	worker, err := application.NewDeliveryWorker(tn.store.Queue(),
		webhook.NewSender(webhook.SenderConfig{RequestTimeout: timeout, Destinations: application.DestinationPolicy{AllowPrivateNetworks: true}}), box,
		application.WorkerConfig{
			Concurrency: 4, PollInterval: 10 * time.Millisecond, Lease: lease, ShutdownTimeout: shutdown,
			Retry: application.RetryPolicy{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond},
		},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

// Three workers share one queue; every delivery must be sent exactly once
// when nothing fails.
func TestIntegrationConcurrentWorkersDeliverEachEventOnce(t *testing.T) {
	tn := newTenant(t)
	box, err := application.NewSecretBox(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	hits := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.Header.Get(webhook.HeaderID)]++
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	tn.endpointFor(t, box, server.URL+"/hook")
	ids := tn.ingest(t, 60)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		worker := newTestWorker(t, tn, box, 2*time.Second, time.Minute, 5*time.Second)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := worker.Run(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	waitUntil(t, 20*time.Second, func() bool {
		for _, id := range ids {
			if tn.statusOf(t, id) != "succeeded" {
				return false
			}
		}
		return true
	})
	cancel()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(hits) != len(ids) {
		t.Fatalf("receiver saw %d distinct events, want %d", len(hits), len(ids))
	}
	for id, n := range hits {
		if n != 1 {
			t.Fatalf("event %s delivered %d times", id, n)
		}
	}
}

// A worker killed after claiming leaves the delivery in processing. Nobody
// may take it over while the lease is valid; once it expires, a surviving
// worker delivers it.
func TestIntegrationAbandonedDeliveryIsRecoveredAfterLeaseExpiry(t *testing.T) {
	tn := newTenant(t)
	box, err := application.NewSecretBox(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	server, hits := flakyServer(0)
	defer server.Close()
	tn.endpointFor(t, box, server.URL+"/hook")
	ids := tn.ingest(t, 1)

	// The crashed worker: claims with a short lease and never completes.
	claims, err := tn.store.Queue().Claim(context.Background(), 1, 500*time.Millisecond)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claim: %v %d", err, len(claims))
	}

	survivor := newTestWorker(t, tn, box, 2*time.Second, time.Minute, 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- survivor.Run(ctx) }()

	time.Sleep(200 * time.Millisecond)
	if status := tn.statusOf(t, ids[0]); status != "processing" || hits.Load() != 0 {
		t.Fatalf("an active lease must not be taken over; status=%s hits=%d", status, hits.Load())
	}
	waitUntil(t, 10*time.Second, func() bool { return tn.statusOf(t, ids[0]) == "succeeded" })
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var attempts int
	if err := tn.pool.QueryRow(context.Background(), `SELECT attempt_count FROM deliveries WHERE id = $1`, ids[0]).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || hits.Load() != 1 {
		t.Fatalf("attempt_count=%d hits=%d, want 2 claims and 1 request", attempts, hits.Load())
	}
}

func waitUntil(t *testing.T, limit time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached in time")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
