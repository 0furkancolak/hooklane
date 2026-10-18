package database_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
)

func (tn tenant) addCustomerEndpoint(t *testing.T, customer string) domain.Endpoint {
	t.Helper()
	endpoint, err := tn.store.Endpoints().Create(context.Background(), domain.Endpoint{
		ProjectID:       tn.project.ID,
		OrganizationID:  tn.principal.OrganizationID,
		URL:             "https://example.com/" + customer,
		SecretEncrypted: []byte("sealed"),
		Enabled:         true,
		Customer:        &customer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func TestIntegrationCustomerFanOutAndFilters(t *testing.T) {
	tn := newTenant(t)
	ctx := context.Background()
	own := tn.addEndpoint(t, true)
	acme := tn.addCustomerEndpoint(t, "acme")
	tn.addCustomerEndpoint(t, "globex")

	svc := tn.eventService()
	ingest := func(customer *string) application.IngestResult {
		res, err := svc.Ingest(ctx, tn.principal, tn.project.ID, application.IngestInput{
			EventType: "a.b", Payload: json.RawMessage(`{}`), Customer: customer,
		})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	acmeName := "acme"
	if res := ingest(&acmeName); res.DeliveryCount != 1 {
		t.Fatalf("acme event fan-out = %d, want 1", res.DeliveryCount)
	}
	if res := ingest(nil); res.DeliveryCount != 1 {
		t.Fatalf("customer-less event fan-out = %d, want 1", res.DeliveryCount)
	}

	// Filters: deliveries by endpoint customer, endpoints by customer, events by customer.
	deliveries, err := tn.store.Deliveries().ListByProject(ctx, tn.project.ID, application.DeliveryFilter{Customer: &acmeName}, 50, 0)
	if err != nil || len(deliveries) != 1 || deliveries[0].EndpointID != acme.ID {
		t.Fatalf("acme deliveries: %v %v", deliveries, err)
	}
	if d, err := tn.store.Deliveries().Get(ctx, deliveries[0].ID); err != nil || d.EndpointCustomer == nil || *d.EndpointCustomer != "acme" {
		t.Fatalf("delivery endpoint customer: %+v %v", d.EndpointCustomer, err)
	}
	endpoints, err := tn.store.Endpoints().ListByProject(ctx, tn.project.ID, application.EndpointFilter{Customer: &acmeName}, 50, 0)
	if err != nil || len(endpoints) != 1 || endpoints[0].ID != acme.ID || endpoints[0].Customer == nil {
		t.Fatalf("acme endpoints: %v %v", endpoints, err)
	}
	all, _ := tn.store.Endpoints().ListByProject(ctx, tn.project.ID, application.EndpointFilter{}, 50, 0)
	if len(all) != 3 {
		t.Fatalf("unfiltered endpoints = %d, want 3 (own %s)", len(all), own.ID)
	}
	events, err := tn.store.Events().ListByProject(ctx, tn.project.ID, application.EventFilter{Customer: &acmeName}, 50, 0)
	if err != nil || len(events) != 1 || events[0].Customer == nil || *events[0].Customer != "acme" {
		t.Fatalf("acme events: %v %v", events, err)
	}
}

func TestIntegrationIdempotencyIncludesCustomer(t *testing.T) {
	tn := newTenant(t)
	ctx := context.Background()
	svc := tn.eventService()
	acme, globex := "acme", "globex"
	in := func(customer *string) application.IngestInput {
		return application.IngestInput{EventType: "a.b", Payload: json.RawMessage(`{"n":1}`), IdempotencyKey: "k1", Customer: customer}
	}
	if _, err := svc.Ingest(ctx, tn.principal, tn.project.ID, in(&acme)); err != nil {
		t.Fatal(err)
	}
	if res, err := svc.Ingest(ctx, tn.principal, tn.project.ID, in(&acme)); err != nil || !res.Replayed {
		t.Fatalf("same customer must replay: %+v %v", res, err)
	}
	if _, err := svc.Ingest(ctx, tn.principal, tn.project.ID, in(&globex)); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatalf("different customer: %v, want idempotency conflict", err)
	}
}

func TestIntegrationScopesAndPortalTokensRoundTrip(t *testing.T) {
	tn := newTenant(t)
	ctx := context.Background()

	projects := application.NewProjectService(tn.store.Projects(), tn.store.Keys())
	admin := application.Principal{OrganizationID: tn.principal.OrganizationID, Scopes: []domain.Scope{domain.ScopeAdmin}}
	key, raw, err := projects.CreateAPIKey(ctx, admin, tn.project.ID, "sender", []domain.Scope{domain.ScopeIngest})
	if err != nil {
		t.Fatal(err)
	}
	if len(key.Scopes) != 1 || key.Scopes[0] != domain.ScopeIngest {
		t.Fatalf("created scopes = %v", key.Scopes)
	}
	auth := application.NewAuthenticator(tn.store.Keys()).WithPortalTokens(tn.store.PortalTokens())
	principal, err := auth.Authenticate(ctx, raw)
	if err != nil || principal.Can(domain.ScopeRead) || !principal.Can(domain.ScopeIngest) {
		t.Fatalf("authenticated scopes = %v, %v", principal.Scopes, err)
	}
	// The database rejects scopes outside the known set.
	if _, err := tn.pool.Exec(ctx, `UPDATE api_keys SET scopes = ARRAY['root'] WHERE id = $1`, key.ID); err == nil {
		t.Fatal("unknown scope was accepted by the database")
	}

	portal := application.NewPortalService(tn.store.Projects(), tn.store.PortalTokens())
	link, err := portal.CreateLink(ctx, admin, tn.project.ID, "acme", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := auth.AuthenticatePortal(ctx, link.Token)
	if err != nil || p.Customer == nil || *p.Customer != "acme" || p.ProjectID != tn.project.ID || p.OrganizationID != tn.principal.OrganizationID {
		t.Fatalf("portal principal = %+v, %v", p, err)
	}
	var stored int
	if err := tn.pool.QueryRow(ctx, `SELECT count(*) FROM portal_tokens WHERE project_id = $1 AND token_hash <> $2`, tn.project.ID, []byte(link.Token)).Scan(&stored); err != nil || stored != 1 {
		t.Fatalf("token must be stored hashed: %d %v", stored, err)
	}
	if _, _, err := auth.AuthenticatePortal(ctx, "hlp_"+link.Token[4:len(link.Token)-1]+"0"); !errors.Is(err, application.ErrUnauthorized) {
		t.Fatalf("wrong token: %v", err)
	}
}
