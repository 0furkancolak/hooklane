package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/domain"
)

const (
	maxStoredErrorLength = 1024
	completeTimeout      = 10 * time.Second
)

type WorkerConfig struct {
	// Concurrency is the maximum number of deliveries in flight.
	Concurrency int
	// PollInterval is the idle wait when no work is due.
	PollInterval time.Duration
	// Lease is how long a claim stays exclusive. It must exceed the sender's
	// request timeout so a live worker never loses a lease mid-request.
	Lease time.Duration
	// ShutdownTimeout bounds how long in-flight deliveries may finish after
	// shutdown starts; afterwards they are cancelled and their leases expire.
	ShutdownTimeout time.Duration
	Retry           RetryPolicy
}

type DeliveryWorker struct {
	queue   DeliveryQueue
	sender  WebhookSender
	secrets *SecretBox
	cfg     WorkerConfig
	log     *slog.Logger
}

func NewDeliveryWorker(queue DeliveryQueue, sender WebhookSender, secrets *SecretBox, cfg WorkerConfig, log *slog.Logger) (*DeliveryWorker, error) {
	if cfg.Concurrency <= 0 || cfg.PollInterval <= 0 || cfg.Lease <= 0 || cfg.ShutdownTimeout <= 0 {
		return nil, errors.New("worker config values must be positive")
	}
	if err := cfg.Retry.Validate(); err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	return &DeliveryWorker{queue: queue, sender: sender, secrets: secrets, cfg: cfg, log: log}, nil
}

// Run claims and delivers work until ctx is cancelled, then drains in-flight
// deliveries for up to ShutdownTimeout.
func (w *DeliveryWorker) Run(ctx context.Context) error {
	slots := make(chan struct{}, w.cfg.Concurrency)
	var inflight sync.WaitGroup

	// Sends outlive ctx so shutdown can let them finish; cancelSends aborts
	// whatever is still running once the drain deadline passes.
	sendCtx, cancelSends := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelSends()

poll:
	for {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			break poll
		}
		free := 1
		for free < w.cfg.Concurrency {
			select {
			case slots <- struct{}{}:
				free++
				continue
			default:
			}
			break
		}

		claims, err := w.queue.Claim(ctx, int32(free), w.cfg.Lease)
		if err != nil {
			release(slots, free)
			if ctx.Err() != nil {
				break poll
			}
			w.log.Error("claim deliveries", "error", err)
			if !sleep(ctx, w.cfg.PollInterval) {
				break poll
			}
			continue
		}
		release(slots, free-len(claims))
		for _, claim := range claims {
			inflight.Add(1)
			go func() {
				defer inflight.Done()
				defer release(slots, 1)
				w.deliver(sendCtx, claim)
			}()
		}
		if len(claims) == 0 && !sleep(ctx, w.cfg.PollInterval) {
			break poll
		}
	}

	done := make(chan struct{})
	go func() {
		inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(w.cfg.ShutdownTimeout):
		w.log.Warn("shutdown timeout reached; cancelling in-flight deliveries")
		cancelSends()
		<-done
		return nil
	}
}

func (w *DeliveryWorker) deliver(ctx context.Context, claim ClaimedDelivery) {
	log := w.log.With("delivery_id", claim.DeliveryID, "endpoint_id", claim.Endpoint.ID, "attempt", claim.Attempt)
	outcome := w.attempt(ctx, claim)

	// Record the result even if sends were cancelled during shutdown.
	completeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), completeTimeout)
	defer cancel()
	err := w.queue.Complete(completeCtx, claim, outcome)
	switch {
	case errors.Is(err, ErrLeaseLost):
		log.Warn("delivery lease lost; result discarded")
	case err != nil:
		log.Error("record delivery attempt", "error", err)
	default:
		attrs := []any{"status", outcome.Status, "status_code", derefInt32(outcome.Attempt.StatusCode), "duration_ms", derefInt64(outcome.Attempt.DurationMS)}
		if outcome.RetryDelay != nil {
			attrs = append(attrs, "retry_in", outcome.RetryDelay.Round(time.Second).String())
		}
		log.Info("delivery attempted", attrs...)
	}
}

func (w *DeliveryWorker) attempt(ctx context.Context, claim ClaimedDelivery) DeliveryOutcome {
	secret, err := w.secrets.Open(claim.Endpoint.SecretEncrypted)
	if err != nil {
		// Retrying cannot fix a secret encrypted under another key.
		return w.failure(claim.Attempt, false, WebhookResult{Err: errors.New("endpoint secret could not be decrypted")})
	}
	body, err := WebhookBody(claim.Event)
	if err != nil {
		return w.failure(claim.Attempt, false, WebhookResult{Err: fmt.Errorf("encode webhook body: %w", err)})
	}
	result := w.sender.Send(ctx, WebhookRequest{
		URL:       claim.Endpoint.URL,
		Secret:    secret,
		MessageID: claim.Event.ID.String(),
		Body:      body,
	})
	succeeded, retryable := Classify(result)
	if !succeeded {
		return w.failure(claim.Attempt, retryable, result)
	}
	// A 2xx is an acknowledgement even if reading the body then failed; the
	// read error is kept for diagnostics.
	return DeliveryOutcome{Status: domain.DeliverySucceeded, Attempt: attemptRecord(result, result.Err)}
}

func (w *DeliveryWorker) failure(attempt int32, retryable bool, result WebhookResult) DeliveryOutcome {
	err := result.Err
	if err == nil {
		err = fmt.Errorf("endpoint responded with status %d", result.StatusCode)
	}
	status, delay := w.cfg.Retry.Next(attempt, retryable, result.RetryAfter)
	return DeliveryOutcome{Status: status, RetryDelay: delay, Attempt: attemptRecord(result, err)}
}

func attemptRecord(result WebhookResult, err error) domain.DeliveryAttempt {
	var attempt domain.DeliveryAttempt
	if result.StatusCode != 0 {
		code := int32(result.StatusCode)
		attempt.StatusCode = &code
	}
	if result.ResponseBody != "" {
		body := result.ResponseBody
		attempt.ResponseBody = &body
	}
	if result.Duration > 0 {
		ms := result.Duration.Milliseconds()
		attempt.DurationMS = &ms
	}
	if err != nil {
		message := err.Error()
		if len(message) > maxStoredErrorLength {
			message = message[:maxStoredErrorLength]
		}
		attempt.Error = &message
	}
	return attempt
}

// WebhookBody is the JSON document POSTed to endpoints. It depends only on the
// event, so every attempt sends identical bytes; the event ID doubles as the
// webhook-id header for receiver-side deduplication.
func WebhookBody(event domain.Event) ([]byte, error) {
	return json.Marshal(struct {
		ID        string          `json:"id"`
		Type      string          `json:"type"`
		Timestamp string          `json:"timestamp"`
		Data      json.RawMessage `json:"data"`
	}{
		ID:        event.ID.String(),
		Type:      event.EventType,
		Timestamp: event.CreatedAt.UTC().Format(time.RFC3339Nano),
		Data:      event.Payload,
	})
}

func release(slots chan struct{}, n int) {
	for range n {
		<-slots
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func derefInt32(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
