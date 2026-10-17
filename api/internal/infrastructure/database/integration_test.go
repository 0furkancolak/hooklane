package database_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration tests run against a migrated database named by
// HOOKLANE_TEST_DATABASE_URL (see `make test-integration`). Each test creates
// its own organization so tests never depend on or delete existing rows.

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("HOOKLANE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("HOOKLANE_TEST_DATABASE_URL not set")
	}
	pool, err := database.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type tenant struct {
	store     *database.Store
	pool      *pgxpool.Pool
	principal application.Principal
	project   domain.Project
}

func newTenant(t *testing.T) tenant {
	t.Helper()
	pool := testPool(t)
	store := database.NewStore(pool)
	ctx := context.Background()
	slug := "it-" + uuid.NewString()
	org, err := store.Orgs().Create(ctx, domain.Organization{Name: "Integration", Slug: slug})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
	})
	project, err := store.Projects().Create(ctx, domain.Project{OrganizationID: org.ID, Name: "p"})
	if err != nil {
		t.Fatal(err)
	}
	return tenant{
		store:     store,
		pool:      pool,
		principal: application.Principal{OrganizationID: org.ID, ProjectID: project.ID},
		project:   project,
	}
}

func (tn tenant) addEndpoint(t *testing.T, enabled bool) domain.Endpoint {
	t.Helper()
	endpoint, err := tn.store.Endpoints().Create(context.Background(), domain.Endpoint{
		ProjectID:       tn.project.ID,
		OrganizationID:  tn.principal.OrganizationID,
		URL:             "https://example.com/hook",
		SecretEncrypted: []byte("sealed"),
		Enabled:         enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func (tn tenant) eventService() *application.EventService {
	return tn.eventServiceWith(tn.store.Deliveries())
}

func (tn tenant) eventServiceWith(deliveries application.DeliveryRepository) *application.EventService {
	return application.NewEventService(tn.store.Projects(), tn.store.Events(), deliveries, tn.store)
}

// counts returns rows owned by the tenant's project.
func (tn tenant) counts(t *testing.T) (events, deliveries int) {
	t.Helper()
	err := tn.pool.QueryRow(context.Background(), `
		SELECT
		  (SELECT count(*) FROM events WHERE project_id = $1),
		  (SELECT count(*) FROM deliveries d JOIN events e ON e.id = d.event_id WHERE e.project_id = $1)
	`, tn.project.ID).Scan(&events, &deliveries)
	if err != nil {
		t.Fatal(err)
	}
	return events, deliveries
}

func TestIntegrationIngestWritesEventAndDeliveriesAtomically(t *testing.T) {
	tn := newTenant(t)
	tn.addEndpoint(t, true)
	tn.addEndpoint(t, true)
	tn.addEndpoint(t, false)

	result, err := tn.eventService().Ingest(context.Background(), tn.principal, tn.project.ID, application.IngestInput{
		EventType: "invoice.paid",
		Payload:   json.RawMessage(`{"amount": 100, "currency": "EUR"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DeliveryCount != 2 {
		t.Fatalf("delivery count = %d", result.DeliveryCount)
	}
	if events, deliveries := tn.counts(t); events != 1 || deliveries != 2 {
		t.Fatalf("events=%d deliveries=%d", events, deliveries)
	}

	var pending int
	if err := tn.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM deliveries WHERE event_id = $1 AND status = $2 AND next_attempt_at <= now() AND lease_token IS NULL`,
		result.Event.ID, string(domain.DeliveryPending)).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 2 {
		t.Fatalf("due pending deliveries = %d, want 2", pending)
	}
}

// failingDeliveries fails after the event row was inserted in the same
// transaction, so a missing rollback would leave an orphaned event.
type failingDeliveries struct{ application.DeliveryRepository }

func (failingDeliveries) CreateForEvent(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, errors.New("injected delivery failure")
}

func TestIntegrationDeliveryFailureRollsBackEvent(t *testing.T) {
	tn := newTenant(t)
	tn.addEndpoint(t, true)

	_, err := tn.eventServiceWith(failingDeliveries{tn.store.Deliveries()}).Ingest(context.Background(), tn.principal, tn.project.ID, application.IngestInput{
		EventType: "a.b", Payload: json.RawMessage(`{}`), IdempotencyKey: "rollback-key",
	})
	if err == nil {
		t.Fatal("expected injected failure")
	}
	if events, deliveries := tn.counts(t); events+deliveries != 0 {
		t.Fatalf("partial write: events=%d deliveries=%d", events, deliveries)
	}

	// The idempotency key must be free again after the rollback.
	result, err := tn.eventService().Ingest(context.Background(), tn.principal, tn.project.ID, application.IngestInput{
		EventType: "a.b", Payload: json.RawMessage(`{}`), IdempotencyKey: "rollback-key",
	})
	if err != nil || result.Replayed {
		t.Fatalf("retry after rollback: %+v %v", result, err)
	}
}

func TestIntegrationIdempotencyComparesJSONSemantically(t *testing.T) {
	tn := newTenant(t)
	tn.addEndpoint(t, true)
	svc := tn.eventService()
	ctx := context.Background()

	first, err := svc.Ingest(ctx, tn.principal, tn.project.ID, application.IngestInput{
		EventType: "user.created", Payload: json.RawMessage(`{"a":1,"b":[1,2]}`), IdempotencyKey: "same",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Same JSON, different key order and whitespace.
	second, err := svc.Ingest(ctx, tn.principal, tn.project.ID, application.IngestInput{
		EventType: "user.created", Payload: json.RawMessage(`{ "b": [1, 2], "a": 1 }`), IdempotencyKey: "same",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || second.Event.ID != first.Event.ID || second.DeliveryCount != 1 {
		t.Fatalf("replay = %+v", second)
	}
	if _, err := svc.Ingest(ctx, tn.principal, tn.project.ID, application.IngestInput{
		EventType: "user.created", Payload: json.RawMessage(`{"a":2,"b":[1,2]}`), IdempotencyKey: "same",
	}); !errors.Is(err, application.ErrIdempotencyConflict) {
		t.Fatalf("different payload: %v", err)
	}
	if events, deliveries := tn.counts(t); events != 1 || deliveries != 1 {
		t.Fatalf("events=%d deliveries=%d", events, deliveries)
	}
}

func TestIntegrationIdempotencyKeysAreScopedPerProject(t *testing.T) {
	a := newTenant(t)
	b := newTenant(t)
	ctx := context.Background()
	in := application.IngestInput{EventType: "x", Payload: json.RawMessage(`{}`), IdempotencyKey: "shared"}
	ra, err := a.eventService().Ingest(ctx, a.principal, a.project.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := b.eventService().Ingest(ctx, b.principal, b.project.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Replayed || ra.Event.ID == rb.Event.ID {
		t.Fatal("idempotency key leaked across projects")
	}
}

func TestIntegrationConcurrentIngestWithSameKeyCreatesOneEvent(t *testing.T) {
	tn := newTenant(t)
	tn.addEndpoint(t, true)
	tn.addEndpoint(t, true)
	svc := tn.eventService()

	const workers = 24
	var wg sync.WaitGroup
	results := make([]application.IngestResult, workers)
	errs := make([]error, workers)
	start := make(chan struct{})
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = svc.Ingest(context.Background(), tn.principal, tn.project.ID, application.IngestInput{
				EventType: "order.created", Payload: json.RawMessage(`{"id":7}`), IdempotencyKey: "race",
			})
		}()
	}
	close(start)
	wg.Wait()

	created := 0
	for i := range workers {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		if results[i].Event.ID != results[0].Event.ID {
			t.Fatal("workers observed different events")
		}
		if results[i].DeliveryCount != 2 {
			t.Fatalf("worker %d delivery count = %d", i, results[i].DeliveryCount)
		}
		if !results[i].Replayed {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created = %d, want exactly 1", created)
	}
	if events, deliveries := tn.counts(t); events != 1 || deliveries != 2 {
		t.Fatalf("events=%d deliveries=%d", events, deliveries)
	}
}

func TestIntegrationConcurrentIngestWithoutKeyCreatesDistinctEvents(t *testing.T) {
	tn := newTenant(t)
	tn.addEndpoint(t, true)
	svc := tn.eventService()

	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Ingest(context.Background(), tn.principal, tn.project.ID, application.IngestInput{
				EventType: "tick", Payload: json.RawMessage(fmt.Sprintf(`{"n":%d}`, i)),
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if events, deliveries := tn.counts(t); events != workers || deliveries != workers {
		t.Fatalf("events=%d deliveries=%d", events, deliveries)
	}
}

func TestIntegrationEndpointOwnershipAndUpdate(t *testing.T) {
	tn := newTenant(t)
	other := newTenant(t)
	box, err := application.NewSecretBox(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	svc := application.NewEndpointService(tn.store.Projects(), tn.store.Endpoints(), box, false)
	ctx := context.Background()

	endpoint, secret, err := svc.Create(ctx, tn.principal, tn.project.ID, application.CreateEndpointInput{URL: "https://example.com/a"})
	if err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err := tn.pool.QueryRow(ctx, "SELECT secret_encrypted FROM endpoints WHERE id = $1", endpoint.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if string(stored) == secret {
		t.Fatal("secret stored in plaintext")
	}
	if got, err := svc.Secret(ctx, tn.principal, endpoint.ID); err != nil || got != secret {
		t.Fatalf("secret round trip: %v", err)
	}
	newURL := "https://example.com/b"
	updated, err := svc.Update(ctx, tn.principal, endpoint.ID, domain.EndpointUpdate{URL: &newURL})
	if err != nil || updated.URL != newURL || !updated.Enabled {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := svc.Get(ctx, other.principal, endpoint.ID); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant get: %v", err)
	}
	if _, err := svc.Update(ctx, other.principal, endpoint.ID, domain.EndpointUpdate{URL: &newURL}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant update: %v", err)
	}
}
