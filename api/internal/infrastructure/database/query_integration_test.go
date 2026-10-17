package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
)

func TestIntegrationDeliveryQueries(t *testing.T) {
	tn := newTenant(t)
	other := newTenant(t)
	ctx := context.Background()
	tn.addEndpoint(t, true)
	b := tn.addEndpoint(t, true)
	other.addEndpoint(t, true)
	other.ingest(t, 2)

	before, err := tn.store.QueueBacklog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := tn.ingest(t, 3) // 3 events x 2 endpoints
	if len(ids) != 6 {
		t.Fatalf("deliveries = %d", len(ids))
	}
	after, err := tn.store.QueueBacklog(ctx)
	if err != nil || after.Due-before.Due != 6 || after.OldestDueSeconds < 0 {
		t.Fatalf("backlog before=%+v after=%+v err=%v", before, after, err)
	}
	disable := false
	if _, err := tn.store.Endpoints().Update(ctx, b.ID, domain.EndpointUpdate{Enabled: &disable}); err != nil {
		t.Fatal(err)
	}
	held, err := tn.store.QueueBacklog(ctx)
	if err != nil || held.Due-before.Due != 3 || held.Held-before.Held != 3 {
		t.Fatalf("disabled endpoint deliveries must count as held: before=%+v held=%+v err=%v", before, held, err)
	}
	enable := true
	if _, err := tn.store.Endpoints().Update(ctx, b.ID, domain.EndpointUpdate{Enabled: &enable}); err != nil {
		t.Fatal(err)
	}

	// Dead-letter one delivery and record its attempt.
	deadID := ids[0]
	if _, err := tn.pool.Exec(ctx, `UPDATE deliveries SET status = 'dead_letter', attempt_count = 1 WHERE id = $1`, deadID); err != nil {
		t.Fatal(err)
	}
	if _, err := tn.pool.Exec(ctx, `INSERT INTO delivery_attempts (delivery_id, attempt_number, status_code, error, duration_ms)
		VALUES ($1, 1, 410, 'endpoint responded with status 410', 12)`, deadID); err != nil {
		t.Fatal(err)
	}

	repo := tn.store.Deliveries()
	all, err := repo.ListByProject(ctx, tn.project.ID, application.DeliveryFilter{}, 100, 0)
	if err != nil || len(all) != 6 {
		t.Fatalf("project list must exclude other tenants: %d %v", len(all), err)
	}
	for i := 1; i < len(all); i++ {
		if all[i].CreatedAt.After(all[i-1].CreatedAt) {
			t.Fatal("deliveries must be newest first")
		}
	}
	page, err := repo.ListByProject(ctx, tn.project.ID, application.DeliveryFilter{}, 4, 4)
	if err != nil || len(page) != 2 || page[0].ID != all[4].ID {
		t.Fatalf("offset page: %d %v", len(page), err)
	}
	dead, err := repo.ListByProject(ctx, tn.project.ID, application.DeliveryFilter{Status: domain.DeliveryDeadLetter}, 100, 0)
	if err != nil || len(dead) != 1 || dead[0].ID != deadID {
		t.Fatalf("status filter: %+v %v", dead, err)
	}
	forB, err := repo.ListByProject(ctx, tn.project.ID, application.DeliveryFilter{EndpointID: &b.ID}, 100, 0)
	if err != nil || len(forB) != 3 {
		t.Fatalf("endpoint filter: %d %v", len(forB), err)
	}

	attempts, err := repo.ListAttempts(ctx, deadID)
	if err != nil || len(attempts) != 1 || *attempts[0].StatusCode != 410 || *attempts[0].DurationMS != 12 || attempts[0].ResponseBody != nil {
		t.Fatalf("attempts: %+v %v", attempts, err)
	}
	stats, err := repo.StatsByProject(ctx, tn.project.ID)
	if err != nil || stats.Pending != 5 || stats.DeadLetter != 1 || stats.Processing != 0 || stats.OldestDueAt == nil || stats.OldestDueAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("stats: %+v %v", stats, err)
	}

	events, err := tn.store.Events().ListByProject(ctx, tn.project.ID, application.EventFilter{EventType: "nope"}, 10, 0)
	if err != nil || len(events) != 0 {
		t.Fatalf("event type filter: %d %v", len(events), err)
	}
	disabled := false
	endpoints, err := tn.store.Endpoints().ListByProject(ctx, tn.project.ID, application.EndpointFilter{Enabled: &disabled}, 10, 0)
	if err != nil || len(endpoints) != 0 {
		t.Fatalf("enabled filter: %d %v", len(endpoints), err)
	}
}
