package database_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/webhook"
	"github.com/google/uuid"
)

// ingest creates one event fanned out to the tenant's enabled endpoints and
// returns the IDs of the created deliveries.
func (tn tenant) ingest(t *testing.T, n int) []uuid.UUID {
	t.Helper()
	svc := tn.eventService()
	for i := 0; i < n; i++ {
		if _, err := svc.Ingest(context.Background(), tn.principal, tn.project.ID, application.IngestInput{
			EventType: "order.created", Payload: json.RawMessage(`{"n":1}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := tn.pool.Query(context.Background(), `
		SELECT d.id FROM deliveries d JOIN events e ON e.id = d.event_id WHERE e.project_id = $1`, tn.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func (tn tenant) deliveryState(t *testing.T, id uuid.UUID) (status string, attempts int32, leaseSet bool) {
	t.Helper()
	if err := tn.pool.QueryRow(context.Background(),
		`SELECT status, attempt_count, lease_token IS NOT NULL FROM deliveries WHERE id = $1`, id,
	).Scan(&status, &attempts, &leaseSet); err != nil {
		t.Fatal(err)
	}
	return status, attempts, leaseSet
}

func owned(t *testing.T, ids []uuid.UUID, claims []application.ClaimedDelivery) {
	t.Helper()
	set := map[uuid.UUID]bool{}
	for _, id := range ids {
		set[id] = true
	}
	for _, c := range claims {
		if !set[c.DeliveryID] {
			t.Fatalf("claimed delivery %s from outside this test; is the test database clean?", c.DeliveryID)
		}
	}
}

func TestIntegrationConcurrentClaimsAreExclusive(t *testing.T) {
	tn := newTenant(t)
	for range 4 {
		tn.addEndpoint(t, true)
	}
	ids := tn.ingest(t, 25) // 100 deliveries
	queue := tn.store.Queue()

	const claimers = 12
	var mu sync.Mutex
	seen := map[uuid.UUID]int{}
	var all []application.ClaimedDelivery
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range claimers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for {
				claims, err := queue.Claim(context.Background(), 7, time.Minute)
				if err != nil {
					t.Error(err)
					return
				}
				if len(claims) == 0 {
					return
				}
				mu.Lock()
				for _, c := range claims {
					seen[c.DeliveryID]++
				}
				all = append(all, claims...)
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	owned(t, ids, all)
	if len(seen) != len(ids) {
		t.Fatalf("claimed %d distinct deliveries, want %d", len(seen), len(ids))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("delivery %s claimed %d times", id, n)
		}
	}
	for _, c := range all {
		if c.Attempt != 1 || c.LeaseToken == uuid.Nil || c.Endpoint.URL == "" || c.Event.EventType != "order.created" {
			t.Fatalf("incomplete claim: %+v", c)
		}
	}
}

func TestIntegrationExpiredLeaseIsReclaimedAndStaleOwnerRejected(t *testing.T) {
	tn := newTenant(t)
	tn.addEndpoint(t, true)
	ids := tn.ingest(t, 1)
	queue := tn.store.Queue()
	ctx := context.Background()

	first, err := queue.Claim(ctx, 10, 300*time.Millisecond)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim: %v %d", err, len(first))
	}
	owned(t, ids, first)
	if again, _ := queue.Claim(ctx, 10, time.Minute); len(again) != 0 {
		t.Fatal("an active lease must not be claimable")
	}

	time.Sleep(500 * time.Millisecond)
	second, err := queue.Claim(ctx, 10, time.Minute)
	if err != nil || len(second) != 1 {
		t.Fatalf("reclaim: %v %d", err, len(second))
	}
	if second[0].DeliveryID != first[0].DeliveryID || second[0].LeaseToken == first[0].LeaseToken || second[0].Attempt != 2 {
		t.Fatalf("reclaim = %+v", second[0])
	}

	code := int32(200)
	outcome := application.DeliveryOutcome{Status: domain.DeliverySucceeded, Attempt: domain.DeliveryAttempt{StatusCode: &code}}
	if err := queue.Complete(ctx, first[0], outcome); !errors.Is(err, application.ErrLeaseLost) {
		t.Fatalf("stale owner complete: %v", err)
	}
	if err := queue.Complete(ctx, second[0], outcome); err != nil {
		t.Fatal(err)
	}
	if err := queue.Complete(ctx, second[0], outcome); !errors.Is(err, application.ErrLeaseLost) {
		t.Fatalf("double complete: %v", err)
	}

	status, attempts, leaseSet := tn.deliveryState(t, ids[0])
	if status != "succeeded" || attempts != 2 || leaseSet {
		t.Fatalf("state = %s/%d/lease=%v", status, attempts, leaseSet)
	}
	var recorded []int32
	rows, err := tn.pool.Query(ctx, `SELECT attempt_number FROM delivery_attempts WHERE delivery_id = $1`, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n int32
		_ = rows.Scan(&n)
		recorded = append(recorded, n)
	}
	rows.Close()
	if len(recorded) != 1 || recorded[0] != 2 {
		t.Fatalf("attempt rows = %v; the stale owner must not record", recorded)
	}
}

func TestIntegrationClaimSkipsDisabledEndpointsAndFutureWork(t *testing.T) {
	tn := newTenant(t)
	disabledLater := tn.addEndpoint(t, true)
	tn.addEndpoint(t, true)
	ids := tn.ingest(t, 1)
	ctx := context.Background()

	if _, err := tn.pool.Exec(ctx, `UPDATE endpoints SET enabled = false WHERE id = $1`, disabledLater.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tn.pool.Exec(ctx, `UPDATE deliveries SET next_attempt_at = now() + interval '1 hour' WHERE endpoint_id <> $1 AND id = ANY($2)`, disabledLater.ID, ids); err != nil {
		t.Fatal(err)
	}
	claims, err := tn.store.Queue().Claim(ctx, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	owned(t, ids, claims)
	if len(claims) != 0 {
		t.Fatalf("claimed %d deliveries; disabled and future work must wait", len(claims))
	}
	if _, err := tn.pool.Exec(ctx, `UPDATE endpoints SET enabled = true WHERE id = $1`, disabledLater.ID); err != nil {
		t.Fatal(err)
	}
	claims, err = tn.store.Queue().Claim(ctx, 10, time.Minute)
	if err != nil || len(claims) != 1 || claims[0].Endpoint.ID != disabledLater.ID {
		t.Fatalf("re-enabled endpoint: %v %+v", err, claims)
	}
}

func TestIntegrationLeaseConsistencyConstraint(t *testing.T) {
	tn := newTenant(t)
	tn.addEndpoint(t, true)
	ids := tn.ingest(t, 1)
	_, err := tn.pool.Exec(context.Background(), `UPDATE deliveries SET status = 'processing' WHERE id = $1`, ids[0])
	if err == nil {
		t.Fatal("processing without a lease must violate deliveries_lease_consistency")
	}
}

type receivedWebhook struct {
	id, timestamp, signature string
	body                     []byte
}

func TestIntegrationWorkerDeliversSignedWebhooks(t *testing.T) {
	tn := newTenant(t)
	box, err := application.NewSecretBox(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	endpoints := application.NewEndpointService(tn.store.Projects(), tn.store.Endpoints(), box, true)
	ctx := context.Background()

	var mu sync.Mutex
	var received []receivedWebhook
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		received = append(received, receivedWebhook{r.Header.Get(webhook.HeaderID), r.Header.Get(webhook.HeaderTimestamp), r.Header.Get(webhook.HeaderSignature), body})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("thanks"))
	}))
	defer okServer.Close()
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("down for maintenance"))
	}))
	defer failServer.Close()

	okEndpoint, okSecret, err := endpoints.Create(ctx, tn.principal, tn.project.ID, application.CreateEndpointInput{URL: okServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	failEndpoint, _, err := endpoints.Create(ctx, tn.principal, tn.project.ID, application.CreateEndpointInput{URL: failServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	ids := tn.ingest(t, 3)

	worker, err := application.NewDeliveryWorker(tn.store.Queue(), webhook.NewSender(webhook.SenderConfig{RequestTimeout: 5 * time.Second}), box,
		application.WorkerConfig{Concurrency: 4, PollInterval: 20 * time.Millisecond, Lease: time.Minute, ShutdownTimeout: 5 * time.Second},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error)
	go func() { done <- worker.Run(runCtx) }()

	deadline := time.Now().Add(10 * time.Second)
	for {
		var open int
		if err := tn.pool.QueryRow(ctx, `SELECT count(*) FROM deliveries WHERE id = ANY($1) AND status IN ('pending','processing')`, ids).Scan(&open); err != nil {
			t.Fatal(err)
		}
		if open == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d deliveries still open", open)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 3 {
		t.Fatalf("receiver got %d webhooks, want 3", len(received))
	}
	for _, r := range received {
		if err := webhook.Verify(okSecret, r.id, r.timestamp, r.signature, r.body, time.Now(), webhook.DefaultTolerance); err != nil {
			t.Fatalf("receiver could not verify signature: %v", err)
		}
		var body struct {
			ID   string          `json:"id"`
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(r.body, &body); err != nil || body.ID != r.id || body.Type != "order.created" || string(body.Data) != `{"n":1}` {
			t.Fatalf("body = %s", r.body)
		}
	}

	type row struct {
		endpoint uuid.UUID
		status   string
		code     *int32
		respBody *string
		errText  *string
		attempts int
	}
	rows, err := tn.pool.Query(ctx, `
		SELECT d.endpoint_id, d.status, a.status_code, a.response_body, a.error,
		       (SELECT count(*) FROM delivery_attempts x WHERE x.delivery_id = d.id)
		FROM deliveries d JOIN delivery_attempts a ON a.delivery_id = d.id
		WHERE d.id = ANY($1)`, ids)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.endpoint, &r.status, &r.code, &r.respBody, &r.errText, &r.attempts); err != nil {
			t.Fatal(err)
		}
		n++
		if r.attempts != 1 {
			t.Fatalf("delivery has %d attempts", r.attempts)
		}
		switch r.endpoint {
		case okEndpoint.ID:
			if r.status != "succeeded" || *r.code != 200 || *r.respBody != "thanks" || r.errText != nil {
				t.Fatalf("ok delivery = %+v", r)
			}
		case failEndpoint.ID:
			if r.status != "failed" || *r.code != 503 || *r.respBody != "down for maintenance" || r.errText == nil {
				t.Fatalf("failed delivery = %+v", r)
			}
		}
	}
	if n != 6 {
		t.Fatalf("attempt rows = %d, want 6", n)
	}
}
