package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
)

type fakeQueue struct {
	mu              sync.Mutex
	pending         []application.ClaimedDelivery
	completed       map[uuid.UUID]application.DeliveryOutcome
	claimsAfterStop atomic.Int32
	stopped         atomic.Bool
	completeErr     error
}

func (q *fakeQueue) Claim(ctx context.Context, batch int32, _ time.Duration) ([]application.ClaimedDelivery, error) {
	if q.stopped.Load() {
		q.claimsAfterStop.Add(1)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	n := min(int(batch), len(q.pending))
	out := q.pending[:n]
	q.pending = q.pending[n:]
	return out, nil
}

func (q *fakeQueue) Complete(_ context.Context, claim application.ClaimedDelivery, outcome application.DeliveryOutcome) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.completeErr != nil {
		return q.completeErr
	}
	if q.completed == nil {
		q.completed = map[uuid.UUID]application.DeliveryOutcome{}
	}
	q.completed[claim.DeliveryID] = outcome
	return nil
}

func (q *fakeQueue) done() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.completed)
}

type fakeSender struct {
	inflight, peak atomic.Int32
	delay          time.Duration
	respond        func(application.WebhookRequest) application.WebhookResult
	mu             sync.Mutex
	requests       []application.WebhookRequest
}

func (s *fakeSender) Send(ctx context.Context, req application.WebhookRequest) application.WebhookResult {
	n := s.inflight.Add(1)
	defer s.inflight.Add(-1)
	for {
		p := s.peak.Load()
		if n <= p || s.peak.CompareAndSwap(p, n) {
			break
		}
	}
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return application.WebhookResult{Err: ctx.Err()}
		}
	}
	if s.respond != nil {
		return s.respond(req)
	}
	return application.WebhookResult{StatusCode: 200, ResponseBody: "ok", Duration: s.delay}
}

func newClaims(t *testing.T, box *application.SecretBox, n int) []application.ClaimedDelivery {
	t.Helper()
	sealed, err := box.Seal("whsec_dGVzdHNlY3JldA==")
	if err != nil {
		t.Fatal(err)
	}
	out := make([]application.ClaimedDelivery, n)
	for i := range out {
		out[i] = application.ClaimedDelivery{
			DeliveryID: uuid.New(),
			LeaseToken: uuid.New(),
			Attempt:    1,
			Event: domain.Event{
				ID: uuid.New(), EventType: "order.created",
				Payload: json.RawMessage(`{"n":1}`), CreatedAt: time.Unix(1_700_000_000, 0),
			},
			Endpoint: domain.Endpoint{ID: uuid.New(), URL: "https://example.com", SecretEncrypted: sealed, Enabled: true},
		}
	}
	return out
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newWorker(t *testing.T, q application.DeliveryQueue, s application.WebhookSender, box *application.SecretBox, cfg application.WorkerConfig) *application.DeliveryWorker {
	t.Helper()
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Millisecond
	}
	if cfg.Lease == 0 {
		cfg.Lease = time.Minute
	}
	if cfg.Retry.MaxAttempts == 0 {
		cfg.Retry = application.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Minute}
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 5 * time.Second
	}
	w, err := application.NewDeliveryWorker(q, s, box, cfg, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestWorkerRespectsConcurrencyLimit(t *testing.T) {
	box, _ := application.NewSecretBox(make([]byte, 32))
	queue := &fakeQueue{pending: newClaims(t, box, 40)}
	sender := &fakeSender{delay: 10 * time.Millisecond}
	w := newWorker(t, queue, sender, box, application.WorkerConfig{Concurrency: 4})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- w.Run(ctx) }()
	waitFor(t, func() bool { return queue.done() == 40 })
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if peak := sender.peak.Load(); peak > 4 || peak < 2 {
		t.Fatalf("peak concurrency = %d, want 2..4", peak)
	}
}

func TestWorkerClassifiesOutcomesAndSignsWithEventID(t *testing.T) {
	box, _ := application.NewSecretBox(make([]byte, 32))
	claims := newClaims(t, box, 4)
	results := map[string]application.WebhookResult{
		claims[0].Event.ID.String(): {StatusCode: 204, Duration: 5 * time.Millisecond},
		claims[1].Event.ID.String(): {StatusCode: 500, ResponseBody: "boom"},
		claims[2].Event.ID.String(): {Err: errors.New("dial tcp: connection refused")},
		claims[3].Event.ID.String(): {StatusCode: 302},
	}
	sender := &fakeSender{respond: func(r application.WebhookRequest) application.WebhookResult { return results[r.MessageID] }}
	queue := &fakeQueue{pending: claims}
	w := newWorker(t, queue, sender, box, application.WorkerConfig{Concurrency: 2})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- w.Run(ctx) }()
	waitFor(t, func() bool { return queue.done() == 4 })
	cancel()
	<-done

	ok := queue.completed[claims[0].DeliveryID]
	if ok.Status != domain.DeliverySucceeded || *ok.Attempt.StatusCode != 204 || ok.Attempt.Error != nil {
		t.Fatalf("2xx outcome = %+v", ok)
	}
	for i, want := range []struct {
		err    string
		status domain.DeliveryStatus
	}{
		{"endpoint responded with status 500", domain.DeliveryPending},
		{"dial tcp: connection refused", domain.DeliveryPending},
		{"endpoint responded with status 302", domain.DeliveryDeadLetter},
	} {
		got := queue.completed[claims[i+1].DeliveryID]
		if got.Status != want.status || got.Attempt.Error == nil || *got.Attempt.Error != want.err {
			t.Fatalf("claim %d outcome = %+v", i+1, got)
		}
		if retry := got.RetryDelay != nil; retry != (want.status == domain.DeliveryPending) {
			t.Fatalf("claim %d retry delay = %v", i+1, got.RetryDelay)
		}
		if got.RetryDelay != nil && (*got.RetryDelay < 500*time.Millisecond || *got.RetryDelay > time.Second) {
			t.Fatalf("first retry delay %v outside [0.5s, 1s]", *got.RetryDelay)
		}
	}
	if code := queue.completed[claims[2].DeliveryID].Attempt.StatusCode; code != nil {
		t.Fatal("transport failure must not record a status code")
	}

	for _, req := range sender.requests {
		if req.Secret != "whsec_dGVzdHNlY3JldA==" {
			t.Fatal("sender must receive the decrypted secret")
		}
		var body struct {
			ID   string          `json:"id"`
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(req.Body, &body); err != nil || body.ID != req.MessageID || body.Type != "order.created" || string(body.Data) != `{"n":1}` {
			t.Fatalf("body = %s", req.Body)
		}
	}
}

func TestWorkerUndecryptableSecretFailsWithoutSending(t *testing.T) {
	box, _ := application.NewSecretBox(make([]byte, 32))
	claims := newClaims(t, box, 1)
	claims[0].Endpoint.SecretEncrypted = []byte("garbage-not-a-valid-ciphertext")
	queue := &fakeQueue{pending: claims}
	sender := &fakeSender{}
	w := newWorker(t, queue, sender, box, application.WorkerConfig{Concurrency: 1})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- w.Run(ctx) }()
	waitFor(t, func() bool { return queue.done() == 1 })
	cancel()
	<-done
	if len(sender.requests) != 0 {
		t.Fatal("must not send without a valid secret")
	}
	if got := queue.completed[claims[0].DeliveryID]; got.Status != domain.DeliveryDeadLetter || got.RetryDelay != nil {
		t.Fatalf("outcome = %+v", got)
	}
}

func TestWorkerDrainsInFlightOnShutdown(t *testing.T) {
	box, _ := application.NewSecretBox(make([]byte, 32))
	queue := &fakeQueue{pending: newClaims(t, box, 3)}
	sender := &fakeSender{delay: 200 * time.Millisecond}
	w := newWorker(t, queue, sender, box, application.WorkerConfig{Concurrency: 3})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- w.Run(ctx) }()
	waitFor(t, func() bool { return sender.inflight.Load() == 3 })
	queue.stopped.Store(true)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if queue.done() != 3 {
		t.Fatalf("in-flight deliveries not recorded: %d", queue.done())
	}
	for id, outcome := range queue.completed {
		if outcome.Status != domain.DeliverySucceeded {
			t.Fatalf("%s drained delivery = %+v", id, outcome)
		}
	}
	if n := queue.claimsAfterStop.Load(); n != 0 {
		t.Fatalf("worker claimed %d times after shutdown began", n)
	}
}

func TestWorkerCancelsSendsAfterShutdownTimeout(t *testing.T) {
	box, _ := application.NewSecretBox(make([]byte, 32))
	queue := &fakeQueue{pending: newClaims(t, box, 2)}
	sender := &fakeSender{delay: time.Hour}
	w := newWorker(t, queue, sender, box, application.WorkerConfig{Concurrency: 2, ShutdownTimeout: 100 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- w.Run(ctx) }()
	waitFor(t, func() bool { return sender.inflight.Load() == 2 })
	started := time.Now()
	cancel()
	<-done
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("shutdown took %v", elapsed)
	}
	// Cancelled sends are still recorded as failed attempts.
	if queue.done() != 2 {
		t.Fatalf("recorded = %d", queue.done())
	}
}

func TestWorkerSurvivesLeaseLoss(t *testing.T) {
	box, _ := application.NewSecretBox(make([]byte, 32))
	queue := &fakeQueue{pending: newClaims(t, box, 2), completeErr: application.ErrLeaseLost}
	sender := &fakeSender{}
	w := newWorker(t, queue, sender, box, application.WorkerConfig{Concurrency: 1})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- w.Run(ctx) }()
	waitFor(t, func() bool { sender.mu.Lock(); defer sender.mu.Unlock(); return len(sender.requests) == 2 })
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestNewDeliveryWorkerRejectsInvalidConfig(t *testing.T) {
	if _, err := application.NewDeliveryWorker(&fakeQueue{}, &fakeSender{}, nil, application.WorkerConfig{}, nil); err == nil {
		t.Fatal("expected config validation error")
	}
}

func TestWorkerDeadLettersWhenAttemptsAreExhausted(t *testing.T) {
	box, _ := application.NewSecretBox(make([]byte, 32))
	claims := newClaims(t, box, 2)
	claims[0].Attempt = 2
	claims[1].Attempt = 3 // MaxAttempts in newWorker's default policy
	sender := &fakeSender{respond: func(application.WebhookRequest) application.WebhookResult {
		return application.WebhookResult{StatusCode: 503, RetryAfter: 10 * time.Minute}
	}}
	queue := &fakeQueue{pending: claims}
	w := newWorker(t, queue, sender, box, application.WorkerConfig{Concurrency: 2})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- w.Run(ctx) }()
	waitFor(t, func() bool { return queue.done() == 2 })
	cancel()
	<-done

	retried := queue.completed[claims[0].DeliveryID]
	if retried.Status != domain.DeliveryPending || retried.RetryDelay == nil || *retried.RetryDelay != time.Minute {
		t.Fatalf("Retry-After must raise the delay up to MaxDelay: %+v", retried)
	}
	if exhausted := queue.completed[claims[1].DeliveryID]; exhausted.Status != domain.DeliveryDeadLetter || exhausted.RetryDelay != nil {
		t.Fatalf("exhausted delivery = %+v", exhausted)
	}
}

func TestWorkerTreats2xxWithBodyReadErrorAsSuccess(t *testing.T) {
	box, _ := application.NewSecretBox(make([]byte, 32))
	claims := newClaims(t, box, 1)
	sender := &fakeSender{respond: func(application.WebhookRequest) application.WebhookResult {
		return application.WebhookResult{StatusCode: 200, Err: errors.New("context deadline exceeded while reading body")}
	}}
	queue := &fakeQueue{pending: claims}
	w := newWorker(t, queue, sender, box, application.WorkerConfig{Concurrency: 1})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- w.Run(ctx) }()
	waitFor(t, func() bool { return queue.done() == 1 })
	cancel()
	<-done
	got := queue.completed[claims[0].DeliveryID]
	if got.Status != domain.DeliverySucceeded || got.Attempt.Error == nil {
		t.Fatalf("outcome = %+v", got)
	}
}
