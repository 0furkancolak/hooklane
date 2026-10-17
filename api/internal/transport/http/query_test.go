package http

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	stdhttp "net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
)

func TestDeliveryQueryRoutes(t *testing.T) {
	env := newTestEnv(t, config.AppEnvProduction)
	ctx := context.Background()
	endpoint, err := env.store.Endpoints().Create(ctx, domain.Endpoint{ProjectID: env.project.ID, URL: "https://example.com", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	project := "/v1/projects/" + env.project.ID.String()
	for i := 0; i < 3; i++ {
		env.do(t, "POST", project+"/events", `{"event_type":"a","payload":{}}`)
	}

	resp, body := env.do(t, "GET", project+"/deliveries?limit=2", "")
	if resp.StatusCode != stdhttp.StatusOK || len(body["deliveries"].([]any)) != 2 || body["next_offset"] != float64(2) || body["limit"] != float64(2) {
		t.Fatalf("page 1: %d %v", resp.StatusCode, body)
	}
	resp, body = env.do(t, "GET", project+"/deliveries?limit=2&offset=2", "")
	if resp.StatusCode != stdhttp.StatusOK || len(body["deliveries"].([]any)) != 1 || body["next_offset"] != nil {
		t.Fatalf("page 2: %d %v", resp.StatusCode, body)
	}

	id := body["deliveries"].([]any)[0].(map[string]any)["id"].(string)
	env.store.SetDeliveryStatus(uuid.MustParse(id), domain.DeliveryDeadLetter)
	code := int32(500)
	errText := "endpoint responded with status 500"
	env.store.AddAttempt(domain.DeliveryAttempt{DeliveryID: uuid.MustParse(id), AttemptNumber: 1, StatusCode: &code, Error: &errText})

	resp, body = env.do(t, "GET", project+"/deliveries?status=dead_letter&endpoint_id="+endpoint.ID.String(), "")
	if resp.StatusCode != stdhttp.StatusOK || len(body["deliveries"].([]any)) != 1 {
		t.Fatalf("filtered: %d %v", resp.StatusCode, body)
	}
	resp, body = env.do(t, "GET", "/v1/deliveries/"+id, "")
	if resp.StatusCode != stdhttp.StatusOK || body["status"] != "dead_letter" || body["next_attempt_at"] != nil {
		t.Fatalf("get: %d %v", resp.StatusCode, body)
	}
	resp, body = env.do(t, "GET", "/v1/deliveries/"+id+"/attempts", "")
	attempts := body["attempts"].([]any)
	if resp.StatusCode != stdhttp.StatusOK || len(attempts) != 1 || attempts[0].(map[string]any)["status_code"] != float64(500) {
		t.Fatalf("attempts: %d %v", resp.StatusCode, body)
	}
	resp, body = env.do(t, "GET", project+"/delivery-stats", "")
	if resp.StatusCode != stdhttp.StatusOK || body["pending"] != float64(2) || body["dead_letter"] != float64(1) || body["succeeded"] != float64(0) {
		t.Fatalf("stats: %d %v", resp.StatusCode, body)
	}

	for _, bad := range []string{"?status=failed", "?endpoint_id=nope"} {
		if resp, _ := env.do(t, "GET", project+"/deliveries"+bad, ""); resp.StatusCode != stdhttp.StatusBadRequest {
			t.Errorf("%s: %d", bad, resp.StatusCode)
		}
	}
	if resp, _ := env.do(t, "GET", project+"/endpoints?enabled=maybe", ""); resp.StatusCode != stdhttp.StatusBadRequest {
		t.Errorf("enabled=maybe: %d", resp.StatusCode)
	}
	if resp, body := env.do(t, "GET", project+"/endpoints?enabled=true", ""); resp.StatusCode != stdhttp.StatusOK || len(body["endpoints"].([]any)) != 1 {
		t.Errorf("enabled=true: %d %v", resp.StatusCode, body)
	}
	if resp, body := env.do(t, "GET", project+"/events?event_type=a&limit=1", ""); resp.StatusCode != stdhttp.StatusOK || body["next_offset"] != float64(1) {
		t.Errorf("events page: %d %v", resp.StatusCode, body)
	}
}

type observedRequest struct {
	method, route string
	status        int
}

type fakeObserver struct {
	mu   sync.Mutex
	seen []observedRequest
}

func (o *fakeObserver) ObserveRequest(method, route string, status int, _ time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.seen = append(o.seen, observedRequest{method, route, status})
}

func TestRequestIDsAccessLogsAndMetrics(t *testing.T) {
	var logs bytes.Buffer
	observer := &fakeObserver{}
	env := newTestEnv(t, config.AppEnvProduction,
		WithLogger(slog.New(slog.NewJSONHandler(&logs, nil))),
		WithRequestObserver(observer))
	project := "/v1/projects/" + env.project.ID.String()

	resp, _ := env.do(t, "GET", project+"?token=should-not-be-logged", "", "X-Request-ID", "trace-123")
	if resp.Header.Get("X-Request-ID") != "trace-123" {
		t.Fatalf("request id not echoed: %q", resp.Header.Get("X-Request-ID"))
	}
	resp, _ = env.do(t, "GET", project, "", "X-Request-ID", "bad id with spaces")
	if generated := resp.Header.Get("X-Request-ID"); generated == "" || strings.Contains(generated, " ") {
		t.Fatalf("invalid request id must be replaced: %q", generated)
	}
	env.do(t, "GET", "/v1/nope", "")
	env.do(t, "POST", project+"/events", `{"event_type":"a","payload":{"x":"`+strings.Repeat("x", 300<<10)+`"}}`)

	out := logs.String()
	for _, want := range []string{`"request_id":"trace-123"`, `"route":"/v1/projects/:id"`, `"status":200`, `"route":"unmatched"`, `"status":413`, `"api_key_id"`} {
		if !strings.Contains(out, want) {
			t.Errorf("access log missing %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, "should-not-be-logged") || strings.Contains(out, env.key) {
		t.Fatalf("access log leaked query string or key:\n%s", out)
	}

	observer.mu.Lock()
	defer observer.mu.Unlock()
	if len(observer.seen) != 4 || observer.seen[0] != (observedRequest{"GET", "/v1/projects/:id", 200}) || observer.seen[2].route != "unmatched" {
		t.Fatalf("observed = %+v", observer.seen)
	}
}

func TestReadinessReflectsDependencies(t *testing.T) {
	var failing bool
	env := newTestEnv(t, config.AppEnvProduction, WithReadiness(func(context.Context) error {
		if failing {
			return errors.New("db down")
		}
		return nil
	}))
	if resp, body := env.do(t, "GET", "/readyz", ""); resp.StatusCode != stdhttp.StatusOK || body["status"] != "ready" {
		t.Fatalf("ready: %d %v", resp.StatusCode, body)
	}
	failing = true
	if resp, body := env.do(t, "GET", "/readyz", ""); resp.StatusCode != stdhttp.StatusServiceUnavailable || body["status"] != "unavailable" {
		t.Fatalf("not ready: %d %v", resp.StatusCode, body)
	}
	if resp, _ := env.do(t, "GET", "/healthz", ""); resp.StatusCode != stdhttp.StatusOK {
		t.Fatalf("healthz must not depend on the database: %d", resp.StatusCode)
	}
}
