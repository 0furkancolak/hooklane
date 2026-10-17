package http

import (
	"bytes"
	"context"
	"log/slog"
	stdhttp "net/http"
	"strings"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/0furkancolak/hooklane/api/internal/domain"
)

func TestEndpointDestinationErrors(t *testing.T) {
	env := newTestEnv(t, config.AppEnvProduction)
	base := "/v1/projects/" + env.project.ID.String() + "/endpoints"
	for _, url := range []string{"https://127.0.0.1/hook", "https://169.254.169.254/", "https://localhost:8443/"} {
		resp, body := env.do(t, "POST", base, `{"url":"`+url+`"}`)
		if resp.StatusCode != stdhttp.StatusBadRequest || body["error"].(map[string]any)["code"] != "destination_not_allowed" {
			t.Fatalf("%s: %d %v", url, resp.StatusCode, body)
		}
	}
}

// Every route that takes a resource ID must refuse resources owned by another
// organization, whatever the HTTP method.
func TestCrossTenantAccessIsRefusedOnEveryRoute(t *testing.T) {
	env := newTestEnv(t, config.AppEnvProduction)
	ctx := context.Background()
	endpoint, err := env.store.Endpoints().Create(ctx, domain.Endpoint{ProjectID: env.foreign.ID, URL: "https://example.com", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	event, err := env.store.Events().InsertIdempotent(ctx, domain.Event{ProjectID: env.foreign.ID, EventType: "x", Payload: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.Deliveries().CreateForEvent(ctx, event.Event.ID, env.foreign.ID); err != nil {
		t.Fatal(err)
	}
	deliveries, err := env.store.Deliveries().ListByEvent(ctx, event.Event.ID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("deliveries: %v %v", deliveries, err)
	}
	env.store.SetDeliveryStatus(deliveries[0].ID, domain.DeliveryDeadLetter)
	foreignKey, err := env.store.Keys().Create(ctx, domain.APIKey{ProjectID: env.foreign.ID, OrganizationID: env.foreign.OrganizationID, Name: "theirs", Prefix: "hlk_0000", Hash: make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}

	project := "/v1/projects/" + env.foreign.ID.String()
	routes := []struct{ method, path, body string }{
		{"GET", project, ""},
		{"GET", project + "/api-keys", ""},
		{"POST", project + "/api-keys", `{"name":"x"}`},
		{"GET", project + "/endpoints", ""},
		{"POST", project + "/endpoints", `{"url":"https://example.com"}`},
		{"GET", project + "/events", ""},
		{"POST", project + "/events", `{"event_type":"a","payload":{}}`},
		{"GET", "/v1/endpoints/" + endpoint.ID.String(), ""},
		{"PATCH", "/v1/endpoints/" + endpoint.ID.String(), `{"enabled":false}`},
		{"GET", "/v1/endpoints/" + endpoint.ID.String() + "/secret", ""},
		{"GET", "/v1/events/" + event.Event.ID.String(), ""},
		{"GET", "/v1/events/" + event.Event.ID.String() + "/deliveries", ""},
		{"POST", "/v1/deliveries/" + deliveries[0].ID.String() + "/replay", ""},
		{"POST", "/v1/api-keys/" + foreignKey.ID.String() + "/revoke", ""},
	}
	for _, r := range routes {
		resp, body := env.do(t, r.method, r.path, r.body)
		if resp.StatusCode != stdhttp.StatusForbidden {
			t.Errorf("%s %s: %d %v", r.method, r.path, resp.StatusCode, body)
		}
	}
	if events, deliveries := env.store.Counts(); events != 1 || deliveries != 1 {
		t.Fatalf("cross-tenant writes happened: events=%d deliveries=%d", events, deliveries)
	}
}

func TestRequestsDoNotLogCredentials(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	env := newTestEnv(t, config.AppEnvProduction)
	base := "/v1/projects/" + env.project.ID.String()
	resp, created := env.do(t, "POST", base+"/endpoints", `{"url":"https://example.com/hook"}`)
	if resp.StatusCode != stdhttp.StatusCreated {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	env.do(t, "GET", "/v1/endpoints/"+created["id"].(string)+"/secret", "")
	env.do(t, "POST", base+"/events", `{"event_type":"a","payload":{"card":"4242"}}`)
	env.do(t, "GET", base, "", "Authorization", "Bearer "+env.key+"tampered")

	for _, secret := range []string{env.key, created["secret"].(string), "4242"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("logs contain a credential or payload value:\n%s", logs.String())
		}
	}
}

func TestResponsesCarrySecurityHeaders(t *testing.T) {
	env := newTestEnv(t, config.AppEnvProduction)
	for _, path := range []string{"/healthz", "/v1/projects/" + env.project.ID.String(), "/v1/nope"} {
		resp, _ := env.do(t, "GET", path, "")
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s: headers = %v", path, resp.Header)
		}
	}
}

func TestMalformedKeysAreRejected(t *testing.T) {
	env := newTestEnv(t, config.AppEnvProduction)
	path := "/v1/projects/" + env.project.ID.String()
	for _, key := range []string{
		strings.ToUpper(env.key),
		env.key[:len(env.key)-1],
		env.key + "0",
		"hlk_" + strings.Repeat("z", 64),
		strings.Repeat("a", 4096),
	} {
		resp, body := env.do(t, "GET", path, "", "Authorization", "Bearer "+key)
		if resp.StatusCode != stdhttp.StatusUnauthorized {
			t.Errorf("key len %d: %d %v", len(key), resp.StatusCode, body)
		}
	}
}
