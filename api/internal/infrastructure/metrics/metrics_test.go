package metrics

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
)

func scrape(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return rec.Body.String()
}

func TestWorkerAndHTTPMetrics(t *testing.T) {
	reg := NewRegistry()
	w := NewWorker(reg)
	h := NewHTTP(reg)
	w.Claimed(3)
	w.InFlight(2)
	w.InFlight(-1)
	w.AttemptRecorded(domain.DeliverySucceeded, 204, 120*time.Millisecond)
	w.AttemptRecorded(domain.DeliveryPending, 503, time.Second)
	w.AttemptRecorded(domain.DeliveryDeadLetter, 0, time.Millisecond)
	w.LeaseLost()
	h.ObserveRequest("GET", "/v1/events/:id", 200, 5*time.Millisecond)

	out := scrape(t, Handler(reg))
	for _, want := range []string{
		"hooklane_worker_claimed_deliveries_total 3",
		"hooklane_worker_deliveries_in_flight 1",
		`hooklane_delivery_attempts_total{response="2xx",result="succeeded"} 1`,
		`hooklane_delivery_attempts_total{response="5xx",result="retry_scheduled"} 1`,
		`hooklane_delivery_attempts_total{response="transport_error",result="dead_letter"} 1`,
		"hooklane_worker_lease_lost_total 1",
		`hooklane_http_requests_total{method="GET",route="/v1/events/:id",status="200"} 1`,
		"go_goroutines",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestBacklogCollector(t *testing.T) {
	reg := NewRegistry()
	var fail bool
	RegisterBacklog(reg, func(context.Context) (application.QueueBacklog, error) {
		if fail {
			return application.QueueBacklog{}, errors.New("db down")
		}
		return application.QueueBacklog{Due: 7, Scheduled: 2, Held: 4, Processing: 1, OldestDueSeconds: 42.5}, nil
	})
	out := scrape(t, Handler(reg))
	for _, want := range []string{
		"hooklane_queue_due_deliveries 7",
		"hooklane_queue_scheduled_deliveries 2",
		"hooklane_queue_processing_deliveries 1",
		"hooklane_queue_held_deliveries 4",
		"hooklane_queue_oldest_due_seconds 42.5",
		"hooklane_queue_backlog_query_up 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	fail = true
	out = scrape(t, Handler(reg))
	if !strings.Contains(out, "hooklane_queue_backlog_query_up 0") || strings.Contains(out, "hooklane_queue_due_deliveries") {
		t.Fatalf("failed query must report up=0 and no stale gauges:\n%s", out)
	}
}

func TestOpsServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()

	var notReady error
	ops := NewOpsServer(addr, NewRegistry(), func(context.Context) error { return notReady }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ops.Start()
	t.Cleanup(func() { _ = ops.Shutdown(context.Background()) })

	get := func(path string) (int, string) {
		var resp *http.Response
		var err error
		for i := 0; i < 50; i++ {
			if resp, err = http.Get("http://" + addr + path); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	if code, _ := get("/healthz"); code != http.StatusOK {
		t.Fatalf("healthz %d", code)
	}
	if code, body := get("/metrics"); code != http.StatusOK || !strings.Contains(body, "go_goroutines") {
		t.Fatalf("metrics %d", code)
	}
	if code, _ := get("/readyz"); code != http.StatusOK {
		t.Fatalf("ready %d", code)
	}
	notReady = errors.New("database unavailable")
	if code, body := get("/readyz"); code != http.StatusServiceUnavailable || !strings.Contains(body, "database unavailable") {
		t.Fatalf("not ready %d %s", code, body)
	}
}
