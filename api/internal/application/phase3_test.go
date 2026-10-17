package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/application/memstore"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
)

type fixture struct {
	store     *memstore.Store
	endpoints *application.EndpointService
	events    *application.EventService
	principal application.Principal
	project   domain.Project
}

func newFixture(t *testing.T, allowHTTP bool) fixture {
	t.Helper()
	store := memstore.New()
	box, err := application.NewSecretBox(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	orgID := uuid.New()
	project, err := store.Projects().Create(context.Background(), domain.Project{OrganizationID: orgID, Name: "p"})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{
		store:     store,
		endpoints: application.NewEndpointService(store.Projects(), store.Endpoints(), box, allowHTTP),
		events:    application.NewEventService(store.Projects(), store.Events(), store.Deliveries(), store),
		principal: application.Principal{OrganizationID: orgID, ProjectID: project.ID},
		project:   project,
	}
}

func boolPtr(v bool) *bool { return &v }

func TestSecretBoxRoundTripAndTamper(t *testing.T) {
	box, err := application.NewSecretBox(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	secret, err := application.GenerateEndpointSecret()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("secret prefix: %q", secret[:6])
	}
	sealed, err := box.Seal(secret)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), secret) {
		t.Fatal("sealed value contains plaintext")
	}
	opened, err := box.Open(sealed)
	if err != nil || opened != secret {
		t.Fatalf("round trip failed: %v", err)
	}
	sealed[len(sealed)-1] ^= 0xff
	if _, err := box.Open(sealed); err == nil {
		t.Fatal("expected tampered ciphertext to fail")
	}
	if _, err := application.NewSecretBox(make([]byte, 16)); err == nil {
		t.Fatal("expected short key to be rejected")
	}
}

func TestEndpointURLValidation(t *testing.T) {
	prod := newFixture(t, false)
	dev := newFixture(t, true)
	cases := []struct {
		url    string
		prodOK bool
		devOK  bool
	}{
		{"https://example.com/hooks", true, true},
		{"http://localhost:9000/hook", false, true},
		{"ftp://example.com", false, false},
		{"https://user:pass@example.com", false, false},
		{"https:///nohost", false, false},
		{"https://example.com/#frag", false, false},
		{"", false, false},
		{"https://example.com/" + strings.Repeat("a", 2100), false, false},
	}
	for _, tc := range cases {
		for _, env := range []struct {
			f  fixture
			ok bool
		}{{prod, tc.prodOK}, {dev, tc.devOK}} {
			_, _, err := env.f.endpoints.Create(context.Background(), env.f.principal, env.f.project.ID, application.CreateEndpointInput{URL: tc.url})
			if env.ok && err != nil {
				t.Errorf("%q: unexpected error %v", tc.url, err)
			}
			if !env.ok && !errors.Is(err, application.ErrInvalidInput) {
				t.Errorf("%q: expected invalid input, got %v", tc.url, err)
			}
		}
	}
}

func TestEndpointLifecycleAndTenantIsolation(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	endpoint, secret, err := f.endpoints.Create(ctx, f.principal, f.project.ID, application.CreateEndpointInput{URL: "https://example.com/a"})
	if err != nil {
		t.Fatal(err)
	}
	if !endpoint.Enabled {
		t.Fatal("endpoints are enabled by default")
	}
	if strings.Contains(string(endpoint.SecretEncrypted), secret) {
		t.Fatal("secret stored in plaintext")
	}
	got, err := f.endpoints.Secret(ctx, f.principal, endpoint.ID)
	if err != nil || got != secret {
		t.Fatalf("secret retrieval: %v", err)
	}

	updated, err := f.endpoints.Update(ctx, f.principal, endpoint.ID, domain.EndpointUpdate{Enabled: boolPtr(false)})
	if err != nil || updated.Enabled {
		t.Fatalf("disable failed: %v", err)
	}
	if _, err := f.endpoints.Update(ctx, f.principal, endpoint.ID, domain.EndpointUpdate{}); !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("empty update: %v", err)
	}

	intruder := application.Principal{OrganizationID: uuid.New()}
	if _, err := f.endpoints.Get(ctx, intruder, endpoint.ID); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant get: %v", err)
	}
	if _, err := f.endpoints.Secret(ctx, intruder, endpoint.ID); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant secret: %v", err)
	}
	if _, err := f.endpoints.Update(ctx, intruder, endpoint.ID, domain.EndpointUpdate{Enabled: boolPtr(true)}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant update: %v", err)
	}
	if _, _, err := f.endpoints.Create(ctx, intruder, f.project.ID, application.CreateEndpointInput{URL: "https://evil.example"}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant create: %v", err)
	}
	if _, err := f.endpoints.List(ctx, intruder, f.project.ID, 10, 0); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant list: %v", err)
	}
}

func TestIngestFansOutToEnabledEndpointsOnly(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	for _, enabled := range []bool{true, true, false} {
		if _, _, err := f.endpoints.Create(ctx, f.principal, f.project.ID, application.CreateEndpointInput{URL: "https://example.com", Enabled: boolPtr(enabled)}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := f.events.Ingest(ctx, f.principal, f.project.ID, application.IngestInput{
		EventType: "invoice.paid",
		Payload:   json.RawMessage(`{"id":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DeliveryCount != 2 || result.Replayed {
		t.Fatalf("result = %+v", result)
	}
	events, deliveries := f.store.Counts()
	if events != 1 || deliveries != 2 {
		t.Fatalf("counts events=%d deliveries=%d", events, deliveries)
	}
}

func TestIngestIdempotency(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	if _, _, err := f.endpoints.Create(ctx, f.principal, f.project.ID, application.CreateEndpointInput{URL: "https://example.com"}); err != nil {
		t.Fatal(err)
	}
	in := application.IngestInput{EventType: "user.created", Payload: json.RawMessage(`{"id":"u1"}`), IdempotencyKey: "req-1"}
	first, err := f.events.Ingest(ctx, f.principal, f.project.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.events.Ingest(ctx, f.principal, f.project.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || second.Event.ID != first.Event.ID || second.DeliveryCount != 1 {
		t.Fatalf("replay result = %+v", second)
	}

	changed := in
	changed.Payload = json.RawMessage(`{"id":"u2"}`)
	if _, err := f.events.Ingest(ctx, f.principal, f.project.ID, changed); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatalf("changed payload: %v", err)
	}
	changed = in
	changed.EventType = "user.deleted"
	if _, err := f.events.Ingest(ctx, f.principal, f.project.ID, changed); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatalf("changed type: %v", err)
	}

	events, deliveries := f.store.Counts()
	if events != 1 || deliveries != 1 {
		t.Fatalf("counts events=%d deliveries=%d", events, deliveries)
	}
}

func TestIngestRollsBackWhenDeliveriesFail(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	if _, _, err := f.endpoints.Create(ctx, f.principal, f.project.ID, application.CreateEndpointInput{URL: "https://example.com"}); err != nil {
		t.Fatal(err)
	}
	f.store.FailDeliveries = memstore.ErrInjected
	_, err := f.events.Ingest(ctx, f.principal, f.project.ID, application.IngestInput{
		EventType: "a.b", Payload: json.RawMessage(`{}`), IdempotencyKey: "k",
	})
	if !errors.Is(err, memstore.ErrInjected) {
		t.Fatalf("err = %v", err)
	}
	events, deliveries := f.store.Counts()
	if events != 0 || deliveries != 0 {
		t.Fatalf("partial write left events=%d deliveries=%d", events, deliveries)
	}

	f.store.FailDeliveries = nil
	result, err := f.events.Ingest(ctx, f.principal, f.project.ID, application.IngestInput{
		EventType: "a.b", Payload: json.RawMessage(`{}`), IdempotencyKey: "k",
	})
	if err != nil || result.Replayed {
		t.Fatalf("retry after rollback should create the event: %+v %v", result, err)
	}
}

func TestIngestValidation(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	big := `{"x":"` + strings.Repeat("a", application.MaxEventPayloadBytes) + `"}`
	cases := []struct {
		name string
		in   application.IngestInput
		want error
	}{
		{"empty type", application.IngestInput{EventType: "", Payload: json.RawMessage(`{}`)}, application.ErrInvalidInput},
		{"bad type", application.IngestInput{EventType: "has space", Payload: json.RawMessage(`{}`)}, application.ErrInvalidInput},
		{"missing payload", application.IngestInput{EventType: "a"}, application.ErrInvalidInput},
		{"null payload", application.IngestInput{EventType: "a", Payload: json.RawMessage(`null`)}, application.ErrInvalidInput},
		{"array payload", application.IngestInput{EventType: "a", Payload: json.RawMessage(`[1]`)}, application.ErrInvalidInput},
		{"oversized", application.IngestInput{EventType: "a", Payload: json.RawMessage(big)}, application.ErrPayloadTooLarge},
		{"key with space", application.IngestInput{EventType: "a", Payload: json.RawMessage(`{}`), IdempotencyKey: "a b"}, application.ErrInvalidInput},
		{"key too long", application.IngestInput{EventType: "a", Payload: json.RawMessage(`{}`), IdempotencyKey: strings.Repeat("k", 256)}, application.ErrInvalidInput},
	}
	for _, tc := range cases {
		if _, err := f.events.Ingest(ctx, f.principal, f.project.ID, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v want %v", tc.name, err, tc.want)
		}
	}
	intruder := application.Principal{OrganizationID: uuid.New()}
	if _, err := f.events.Ingest(ctx, intruder, f.project.ID, application.IngestInput{EventType: "a", Payload: json.RawMessage(`{}`)}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant ingest: %v", err)
	}
	if events, _ := f.store.Counts(); events != 0 {
		t.Fatalf("invalid requests stored %d events", events)
	}
}

func TestEventReadsAreTenantScoped(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	result, err := f.events.Ingest(ctx, f.principal, f.project.ID, application.IngestInput{EventType: "a", Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.events.Get(ctx, f.principal, result.Event.ID); err != nil {
		t.Fatal(err)
	}
	intruder := application.Principal{OrganizationID: uuid.New()}
	if _, err := f.events.Get(ctx, intruder, result.Event.ID); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant get: %v", err)
	}
	if _, err := f.events.List(ctx, intruder, f.project.ID, 10, 0); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant list: %v", err)
	}
	if _, err := f.events.Get(ctx, f.principal, uuid.New()); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("missing event: %v", err)
	}
}
