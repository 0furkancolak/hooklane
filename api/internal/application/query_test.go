package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
)

func TestEventListPaginatesWithNextOffset(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		eventType := "order.created"
		if i%2 == 1 {
			eventType = "order.paid"
		}
		if _, err := f.events.Ingest(ctx, f.principal, f.project.ID, application.IngestInput{EventType: eventType, Payload: json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := f.events.List(ctx, f.principal, f.project.ID, application.EventFilter{}, application.PageRequest{Limit: 2})
	if err != nil || len(first.Items) != 2 || first.NextOffset == nil || *first.NextOffset != 2 {
		t.Fatalf("first page: %+v %v", first, err)
	}
	last, err := f.events.List(ctx, f.principal, f.project.ID, application.EventFilter{}, application.PageRequest{Limit: 2, Offset: 4})
	if err != nil || len(last.Items) != 1 || last.NextOffset != nil {
		t.Fatalf("last page: %+v %v", last, err)
	}
	exact, err := f.events.List(ctx, f.principal, f.project.ID, application.EventFilter{}, application.PageRequest{Limit: 5})
	if err != nil || len(exact.Items) != 5 || exact.NextOffset != nil {
		t.Fatalf("a page that ends exactly at the last item has no next page: %+v %v", exact, err)
	}
	clamped, err := f.events.List(ctx, f.principal, f.project.ID, application.EventFilter{}, application.PageRequest{Limit: 10_000, Offset: -3})
	if err != nil || clamped.Limit != 50 || clamped.Offset != 0 {
		t.Fatalf("clamp: %+v %v", clamped, err)
	}
	paid, err := f.events.List(ctx, f.principal, f.project.ID, application.EventFilter{EventType: "order.paid"}, application.PageRequest{})
	if err != nil || len(paid.Items) != 2 {
		t.Fatalf("type filter: %+v %v", paid, err)
	}
	if _, err := f.events.List(ctx, f.principal, f.project.ID, application.EventFilter{EventType: "bad type"}, application.PageRequest{}); !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("invalid type filter: %v", err)
	}
	empty, err := f.events.List(ctx, f.principal, f.project.ID, application.EventFilter{EventType: "nothing"}, application.PageRequest{})
	if err != nil || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatalf("empty page must have a non-nil slice: %+v %v", empty, err)
	}
}

func TestEndpointListFiltersByEnabled(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	a, _, _ := f.endpoints.Create(ctx, f.principal, f.project.ID, application.CreateEndpointInput{URL: "https://a.example.com"})
	if _, _, err := f.endpoints.Create(ctx, f.principal, f.project.ID, application.CreateEndpointInput{URL: "https://b.example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.endpoints.Update(ctx, f.principal, a.ID, domain.EndpointUpdate{Enabled: boolPtr(false)}); err != nil {
		t.Fatal(err)
	}
	disabled, err := f.endpoints.List(ctx, f.principal, f.project.ID, application.EndpointFilter{Enabled: boolPtr(false)}, application.PageRequest{})
	if err != nil || len(disabled.Items) != 1 || disabled.Items[0].ID != a.ID {
		t.Fatalf("disabled: %+v %v", disabled, err)
	}
	all, err := f.endpoints.List(ctx, f.principal, f.project.ID, application.EndpointFilter{}, application.PageRequest{})
	if err != nil || len(all.Items) != 2 {
		t.Fatalf("all: %+v %v", all, err)
	}
}

func TestDeliveryQueries(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	a, _, _ := f.endpoints.Create(ctx, f.principal, f.project.ID, application.CreateEndpointInput{URL: "https://a.example.com"})
	b, _, _ := f.endpoints.Create(ctx, f.principal, f.project.ID, application.CreateEndpointInput{URL: "https://b.example.com"})
	ingested, err := f.events.Ingest(ctx, f.principal, f.project.ID, application.IngestInput{EventType: "a", Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	svc := application.NewDeliveryService(f.store.Projects(), f.store.Events(), f.store.Deliveries())
	deliveries, err := svc.ListForEvent(ctx, f.principal, ingested.Event.ID)
	if err != nil || len(deliveries) != 2 {
		t.Fatalf("deliveries: %v %v", deliveries, err)
	}
	var toA domain.Delivery
	for _, d := range deliveries {
		if d.EndpointID == a.ID {
			toA = d
		}
	}
	f.store.SetDeliveryStatus(toA.ID, domain.DeliveryDeadLetter)
	code := int32(503)
	f.store.AddAttempt(domain.DeliveryAttempt{DeliveryID: toA.ID, AttemptNumber: 1, StatusCode: &code})

	got, err := svc.Get(ctx, f.principal, toA.ID)
	if err != nil || got.Status != domain.DeliveryDeadLetter {
		t.Fatalf("get: %+v %v", got, err)
	}
	attempts, err := svc.ListAttempts(ctx, f.principal, toA.ID)
	if err != nil || len(attempts) != 1 || *attempts[0].StatusCode != 503 {
		t.Fatalf("attempts: %+v %v", attempts, err)
	}
	dead, err := svc.ListForProject(ctx, f.principal, f.project.ID, application.DeliveryFilter{Status: domain.DeliveryDeadLetter}, application.PageRequest{})
	if err != nil || len(dead.Items) != 1 || dead.Items[0].ID != toA.ID {
		t.Fatalf("status filter: %+v %v", dead, err)
	}
	forB, err := svc.ListForProject(ctx, f.principal, f.project.ID, application.DeliveryFilter{EndpointID: &b.ID}, application.PageRequest{})
	if err != nil || len(forB.Items) != 1 || forB.Items[0].EndpointID != b.ID {
		t.Fatalf("endpoint filter: %+v %v", forB, err)
	}
	if _, err := svc.ListForProject(ctx, f.principal, f.project.ID, application.DeliveryFilter{Status: "failed"}, application.PageRequest{}); !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("unknown status: %v", err)
	}
	stats, err := svc.Stats(ctx, f.principal, f.project.ID)
	if err != nil || stats.Pending != 1 || stats.DeadLetter != 1 {
		t.Fatalf("stats: %+v %v", stats, err)
	}

	intruder := application.Principal{OrganizationID: uuid.New(), ProjectID: uuid.New()}
	if _, err := svc.Get(ctx, intruder, toA.ID); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant get: %v", err)
	}
	if _, err := svc.ListAttempts(ctx, intruder, toA.ID); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant attempts: %v", err)
	}
	if _, err := svc.ListForProject(ctx, intruder, f.project.ID, application.DeliveryFilter{}, application.PageRequest{}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant list: %v", err)
	}
	if _, err := svc.Stats(ctx, intruder, f.project.ID); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant stats: %v", err)
	}
	if _, err := svc.Get(ctx, f.principal, uuid.New()); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}
