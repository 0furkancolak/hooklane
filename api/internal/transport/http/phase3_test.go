package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/testutil/memstore"
	"github.com/google/uuid"
)

type testEnv struct {
	server  *Server
	store   *memstore.Store
	key     string
	project domain.Project
	foreign domain.Project
}

func newTestEnv(t *testing.T, env config.AppEnv, opts ...Option) testEnv {
	t.Helper()
	ctx := context.Background()
	store := memstore.New()
	orgID := uuid.New()
	project, err := store.Projects().Create(ctx, domain.Project{OrganizationID: orgID, Name: "mine"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := store.Projects().Create(ctx, domain.Project{OrganizationID: uuid.New(), Name: "theirs"})
	if err != nil {
		t.Fatal(err)
	}
	raw, prefix, hash, err := application.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Keys().Create(ctx, domain.APIKey{ProjectID: project.ID, OrganizationID: orgID, Name: "k", Prefix: prefix, Hash: hash}); err != nil {
		t.Fatal(err)
	}
	box, err := application.NewSecretBox(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Port: "8080", AppEnv: env}
	server := NewServer(cfg, Services{
		Accounts:   application.NewAccountService(store.Accounts(), store.Sessions(), store.Orgs(), store, true),
		Auth:       application.NewAuthenticator(store.Keys()).WithPortalTokens(store.PortalTokens()).WithSessions(store.Sessions()),
		Portal:     application.NewPortalService(store.Projects(), store.PortalTokens()),
		Projects:   application.NewProjectService(store.Projects(), store.Keys()),
		Endpoints:  application.NewEndpointService(store.Projects(), store.Endpoints(), box, application.EndpointRules{AllowInsecureHTTP: cfg.AllowInsecureHTTP(), Destinations: application.DestinationPolicy{AllowPrivateNetworks: cfg.AllowPrivateNetworks()}}),
		Events:     application.NewEventService(store.Projects(), store.Events(), store.Deliveries(), store),
		Deliveries: application.NewDeliveryService(store.Projects(), store.Events(), store.Deliveries()),
	}, append([]Option{WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))}, opts...)...)
	return testEnv{server: server, store: store, key: raw, project: project, foreign: foreign}
}

func (e testEnv) do(t *testing.T, method, path, body string, headers ...string) (*stdhttp.Response, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+e.key)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := e.server.App().Test(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp, out
}

func TestEndpointRoutes(t *testing.T) {
	env := newTestEnv(t, config.AppEnvProduction)
	base := "/v1/projects/" + env.project.ID.String() + "/endpoints"

	resp, _ := env.do(t, "POST", base, `{"url":"http://localhost:9000"}`)
	if resp.StatusCode != stdhttp.StatusBadRequest {
		t.Fatalf("http url in production: %d", resp.StatusCode)
	}

	resp, body := env.do(t, "POST", base, `{"url":"https://example.com/hook"}`)
	if resp.StatusCode != stdhttp.StatusCreated {
		t.Fatalf("create: %d %v", resp.StatusCode, body)
	}
	secret, _ := body["secret"].(string)
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatal("create response must include the signing secret")
	}
	id := body["id"].(string)

	resp, body = env.do(t, "GET", "/v1/endpoints/"+id, "")
	if resp.StatusCode != stdhttp.StatusOK || body["secret"] != nil {
		t.Fatalf("get must not include secret: %d %v", resp.StatusCode, body)
	}
	resp, body = env.do(t, "GET", "/v1/endpoints/"+id+"/secret", "")
	if resp.StatusCode != stdhttp.StatusOK || body["secret"] != secret || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("secret: %d", resp.StatusCode)
	}
	resp, body = env.do(t, "PATCH", "/v1/endpoints/"+id, `{"enabled":false}`)
	if resp.StatusCode != stdhttp.StatusOK || body["enabled"] != false {
		t.Fatalf("patch: %d %v", resp.StatusCode, body)
	}
	resp, body = env.do(t, "GET", base, "")
	if resp.StatusCode != stdhttp.StatusOK || len(body["endpoints"].([]any)) != 1 {
		t.Fatalf("list: %d %v", resp.StatusCode, body)
	}

	resp, _ = env.do(t, "POST", "/v1/projects/"+env.foreign.ID.String()+"/endpoints", `{"url":"https://example.com"}`)
	if resp.StatusCode != stdhttp.StatusForbidden {
		t.Fatalf("cross-tenant create: %d", resp.StatusCode)
	}
}

func TestEventRoutes(t *testing.T) {
	env := newTestEnv(t, config.AppEnvProduction)
	ctx := context.Background()
	if _, err := env.store.Endpoints().Create(ctx, domain.Endpoint{ProjectID: env.project.ID, URL: "https://example.com", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	path := "/v1/projects/" + env.project.ID.String() + "/events"
	body := `{"event_type":"order.created","payload":{"id":42}}`

	resp, first := env.do(t, "POST", path, body, "Idempotency-Key", "abc")
	if resp.StatusCode != stdhttp.StatusAccepted || first["delivery_count"] != float64(1) {
		t.Fatalf("ingest: %d %v", resp.StatusCode, first)
	}
	resp, second := env.do(t, "POST", path, body, "Idempotency-Key", "abc")
	if resp.StatusCode != stdhttp.StatusOK || resp.Header.Get("Idempotent-Replayed") != "true" || second["id"] != first["id"] {
		t.Fatalf("replay: %d %v", resp.StatusCode, second)
	}
	resp, conflict := env.do(t, "POST", path, `{"event_type":"order.created","payload":{"id":43}}`, "Idempotency-Key", "abc")
	if resp.StatusCode != stdhttp.StatusConflict || conflict["error"].(map[string]any)["code"] != "idempotency_conflict" {
		t.Fatalf("conflict: %d %v", resp.StatusCode, conflict)
	}

	resp, _ = env.do(t, "POST", path, `{"event_type":"x","payload":`)
	if resp.StatusCode != stdhttp.StatusBadRequest {
		t.Fatalf("malformed json: %d", resp.StatusCode)
	}
	big := `{"event_type":"x","payload":{"d":"` + strings.Repeat("a", application.MaxEventPayloadBytes) + `"}}`
	resp, _ = env.do(t, "POST", path, big)
	if resp.StatusCode != stdhttp.StatusRequestEntityTooLarge {
		t.Fatalf("oversized payload: %d", resp.StatusCode)
	}

	resp, got := env.do(t, "GET", "/v1/events/"+first["id"].(string), "")
	if resp.StatusCode != stdhttp.StatusOK || got["payload"].(map[string]any)["id"] != float64(42) {
		t.Fatalf("get: %d %v", resp.StatusCode, got)
	}
	resp, list := env.do(t, "GET", path, "")
	if resp.StatusCode != stdhttp.StatusOK || len(list["events"].([]any)) != 1 {
		t.Fatalf("list: %d %v", resp.StatusCode, list)
	}

	resp, _ = env.do(t, "POST", "/v1/projects/"+env.foreign.ID.String()+"/events", body)
	if resp.StatusCode != stdhttp.StatusForbidden {
		t.Fatalf("cross-tenant ingest: %d", resp.StatusCode)
	}
	if events, deliveries := env.store.Counts(); events != 1 || deliveries != 1 {
		t.Fatalf("counts events=%d deliveries=%d", events, deliveries)
	}
}

func TestDeliveryRoutes(t *testing.T) {
	env := newTestEnv(t, config.AppEnvProduction)
	ctx := context.Background()
	if _, err := env.store.Endpoints().Create(ctx, domain.Endpoint{ProjectID: env.project.ID, URL: "https://example.com", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_, ingested := env.do(t, "POST", "/v1/projects/"+env.project.ID.String()+"/events", `{"event_type":"a","payload":{}}`)
	eventID := ingested["id"].(string)

	resp, list := env.do(t, "GET", "/v1/events/"+eventID+"/deliveries", "")
	if resp.StatusCode != stdhttp.StatusOK {
		t.Fatalf("list: %d", resp.StatusCode)
	}
	deliveries := list["deliveries"].([]any)
	original := deliveries[0].(map[string]any)
	if len(deliveries) != 1 || original["status"] != "pending" || original["next_attempt_at"] == nil || original["replay_of"] != nil {
		t.Fatalf("deliveries = %v", deliveries)
	}
	originalID := original["id"].(string)

	resp, body := env.do(t, "POST", "/v1/deliveries/"+originalID+"/replay", "")
	if resp.StatusCode != stdhttp.StatusConflict || body["error"].(map[string]any)["code"] != "delivery_not_finished" {
		t.Fatalf("replay of pending: %d %v", resp.StatusCode, body)
	}
	env.store.SetDeliveryStatus(uuid.MustParse(originalID), domain.DeliveryDeadLetter)

	resp, body = env.do(t, "POST", "/v1/deliveries/"+originalID+"/replay", "")
	if resp.StatusCode != stdhttp.StatusCreated || body["replay_of"] != originalID || body["status"] != "pending" {
		t.Fatalf("replay: %d %v", resp.StatusCode, body)
	}
	resp, body = env.do(t, "POST", "/v1/deliveries/"+originalID+"/replay", "")
	if resp.StatusCode != stdhttp.StatusConflict || body["error"].(map[string]any)["code"] != "replay_in_progress" {
		t.Fatalf("duplicate replay: %d %v", resp.StatusCode, body)
	}

	foreignEvent, err := env.store.Events().InsertIdempotent(ctx, domain.Event{ProjectID: env.foreign.ID, EventType: "x", Payload: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	resp, _ = env.do(t, "GET", "/v1/events/"+foreignEvent.Event.ID.String()+"/deliveries", "")
	if resp.StatusCode != stdhttp.StatusForbidden {
		t.Fatalf("cross-tenant list: %d", resp.StatusCode)
	}
}

// newClosedSignupEnv is a server where only the first account can be created.
func newClosedSignupEnv(t *testing.T) testEnv {
	t.Helper()
	env := newTestEnv(t, config.AppEnvDevelopment)
	store := env.store
	server := NewServer(config.Config{Port: "8080", AppEnv: config.AppEnvDevelopment}, Services{
		Accounts: application.NewAccountService(store.Accounts(), store.Sessions(), store.Orgs(), store, false),
		Auth:     application.NewAuthenticator(store.Keys()).WithSessions(store.Sessions()),
		Projects: application.NewProjectService(store.Projects(), store.Keys()),
	}, WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	env.server = server
	return env
}
