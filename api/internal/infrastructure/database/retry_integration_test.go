package database_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/webhook"
	"github.com/google/uuid"
)

func TestIntegrationRetryIsScheduledWithDatabaseClock(t *testing.T) {
	tn := newTenant(t)
	tn.addEndpoint(t, true)
	ids := tn.ingest(t, 1)
	queue := tn.store.Queue()
	ctx := context.Background()

	claims, err := queue.Claim(ctx, 10, time.Minute)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claim: %v %d", err, len(claims))
	}
	owned(t, ids, claims)
	delay := time.Hour
	code := int32(503)
	if err := queue.Complete(ctx, claims[0], application.DeliveryOutcome{
		Status: domain.DeliveryPending, RetryDelay: &delay, Attempt: domain.DeliveryAttempt{StatusCode: &code},
	}); err != nil {
		t.Fatal(err)
	}
	var status string
	var scheduled, leaseCleared bool
	if err := tn.pool.QueryRow(ctx, `
		SELECT status,
		       next_attempt_at BETWEEN now() + interval '59 minutes' AND now() + interval '61 minutes',
		       lease_token IS NULL AND locked_until IS NULL
		FROM deliveries WHERE id = $1`, ids[0]).Scan(&status, &scheduled, &leaseCleared); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || !scheduled || !leaseCleared {
		t.Fatalf("status=%s scheduled=%v leaseCleared=%v", status, scheduled, leaseCleared)
	}
	if again, _ := queue.Claim(ctx, 10, time.Minute); len(again) != 0 {
		t.Fatal("a retry must not be claimable before next_attempt_at")
	}
	if _, err := tn.pool.Exec(ctx, `UPDATE deliveries SET next_attempt_at = now() WHERE id = $1`, ids[0]); err != nil {
		t.Fatal(err)
	}
	again, err := queue.Claim(ctx, 10, time.Minute)
	if err != nil || len(again) != 1 || again[0].Attempt != 2 {
		t.Fatalf("due retry: %v %+v", err, again)
	}
}

// flakyServer fails the first `failures` requests with 503, then returns 200.
func flakyServer(failures int32) (*httptest.Server, *atomic.Int32) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) <= failures {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	return server, &hits
}

func runWorker(t *testing.T, tn tenant, box *application.SecretBox, policy application.RetryPolicy, until func() bool) {
	t.Helper()
	worker, err := application.NewDeliveryWorker(tn.store.Queue(), webhook.NewSender(webhook.SenderConfig{RequestTimeout: 2 * time.Second}), box,
		application.WorkerConfig{Concurrency: 2, PollInterval: 10 * time.Millisecond, Lease: time.Minute, ShutdownTimeout: 5 * time.Second, Retry: policy},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- worker.Run(ctx) }()
	deadline := time.Now().Add(15 * time.Second)
	for !until() {
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatal("worker did not reach the expected state in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func (tn tenant) statusOf(t *testing.T, id uuid.UUID) string {
	status, _, _ := tn.deliveryState(t, id)
	return status
}

type attemptRow struct {
	number int32
	code   *int32
	at     time.Time
}

func (tn tenant) attempts(t *testing.T, id uuid.UUID) []attemptRow {
	t.Helper()
	rows, err := tn.pool.Query(context.Background(),
		`SELECT attempt_number, status_code, attempted_at FROM delivery_attempts WHERE delivery_id = $1 ORDER BY attempt_number`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []attemptRow
	for rows.Next() {
		var a attemptRow
		if err := rows.Scan(&a.number, &a.code, &a.at); err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

func (tn tenant) endpointFor(t *testing.T, box *application.SecretBox, url string) {
	t.Helper()
	svc := application.NewEndpointService(tn.store.Projects(), tn.store.Endpoints(), box, true)
	if _, _, err := svc.Create(context.Background(), tn.principal, tn.project.ID, application.CreateEndpointInput{URL: url}); err != nil {
		t.Fatal(err)
	}
}

var fastPolicy = application.RetryPolicy{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: 400 * time.Millisecond}

func TestIntegrationWorkerRetriesUntilSuccess(t *testing.T) {
	tn := newTenant(t)
	box, _ := application.NewSecretBox(make([]byte, 32))
	server, hits := flakyServer(2)
	defer server.Close()
	tn.endpointFor(t, box, server.URL)
	id := tn.ingest(t, 1)[0]

	runWorker(t, tn, box, fastPolicy, func() bool { return tn.statusOf(t, id) == "succeeded" })

	attempts := tn.attempts(t, id)
	if len(attempts) != 3 || hits.Load() != 3 {
		t.Fatalf("attempts=%d hits=%d", len(attempts), hits.Load())
	}
	for i, a := range attempts {
		wantCode := int32(503)
		if i == 2 {
			wantCode = 200
		}
		if a.number != int32(i+1) || *a.code != wantCode {
			t.Fatalf("attempt %d = %+v", i+1, a)
		}
	}
	// Equal jitter: the first retry waits at least BaseDelay/2, the second
	// at least BaseDelay.
	if gap := attempts[1].at.Sub(attempts[0].at); gap < 50*time.Millisecond {
		t.Fatalf("first retry after %v, want >= 50ms", gap)
	}
	if gap := attempts[2].at.Sub(attempts[1].at); gap < 100*time.Millisecond {
		t.Fatalf("second retry after %v, want >= 100ms", gap)
	}
}

func TestIntegrationExhaustedDeliveryIsDeadLetteredAndReplayable(t *testing.T) {
	tn := newTenant(t)
	box, _ := application.NewSecretBox(make([]byte, 32))
	server, hits := flakyServer(3) // every attempt of the original fails
	defer server.Close()
	tn.endpointFor(t, box, server.URL)
	id := tn.ingest(t, 1)[0]
	ctx := context.Background()

	runWorker(t, tn, box, fastPolicy, func() bool { return tn.statusOf(t, id) == "dead_letter" })
	if hits.Load() != 3 {
		t.Fatalf("hits = %d, want MaxAttempts", hits.Load())
	}

	var before string
	snapshot := func() string {
		var s string
		if err := tn.pool.QueryRow(ctx, `
			SELECT d.status || d.attempt_count || coalesce(d.last_response_status::text, '') || d.updated_at::text ||
			       (SELECT string_agg(a.id::text || a.attempt_number || coalesce(a.status_code::text,'') || a.attempted_at::text, ',' ORDER BY a.attempt_number)
			        FROM delivery_attempts a WHERE a.delivery_id = d.id)
			FROM deliveries d WHERE d.id = $1`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before = snapshot()

	svc := application.NewDeliveryService(tn.store.Events(), tn.store.Deliveries())
	var wg sync.WaitGroup
	results := make([]error, 8)
	replays := make([]domain.Delivery, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			replays[i], results[i] = svc.Replay(ctx, tn.principal, id)
		}()
	}
	wg.Wait()
	var replay domain.Delivery
	created := 0
	for i, err := range results {
		switch {
		case err == nil:
			created++
			replay = replays[i]
		case errors.Is(err, application.ErrReplayInProgress):
		default:
			t.Fatalf("replay %d: %v", i, err)
		}
	}
	if created != 1 {
		t.Fatalf("concurrent replays created %d deliveries, want 1", created)
	}
	if replay.ReplayOf == nil || *replay.ReplayOf != id || replay.AttemptCount != 0 {
		t.Fatalf("replay = %+v", replay)
	}

	runWorker(t, tn, box, fastPolicy, func() bool { return tn.statusOf(t, replay.ID) == "succeeded" })

	if after := snapshot(); after != before {
		t.Fatalf("original delivery or its attempts changed:\nbefore %s\nafter  %s", before, after)
	}
	if got := tn.attempts(t, replay.ID); len(got) != 1 || got[0].number != 1 || *got[0].code != 200 {
		t.Fatalf("replay attempts = %+v", got)
	}
	all, err := svc.ListForEvent(ctx, tn.principal, replay.EventID)
	if err != nil || len(all) != 2 {
		t.Fatalf("list: %v %d", err, len(all))
	}
	if n, err := tn.store.Deliveries().CountByEvent(ctx, replay.EventID); err != nil || n != 1 {
		t.Fatalf("fan-out count must exclude replays: %d %v", n, err)
	}
	// Once the replay finished, the original can be replayed again.
	if _, err := svc.Replay(ctx, tn.principal, id); err != nil {
		t.Fatalf("replay after previous replay finished: %v", err)
	}
}

func TestIntegrationPermanentFailureDeadLettersImmediately(t *testing.T) {
	tn := newTenant(t)
	box, _ := application.NewSecretBox(make([]byte, 32))
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusGone)
	}))
	defer server.Close()
	tn.endpointFor(t, box, server.URL)
	id := tn.ingest(t, 1)[0]

	runWorker(t, tn, box, fastPolicy, func() bool { return tn.statusOf(t, id) == "dead_letter" })
	time.Sleep(200 * time.Millisecond)
	if hits.Load() != 1 || len(tn.attempts(t, id)) != 1 {
		t.Fatalf("410 must not be retried: hits=%d", hits.Load())
	}
}
