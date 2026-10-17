package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
)

func TestIntegrationRetentionDeletesOnlyOldFinishedEvents(t *testing.T) {
	tn := newTenant(t)
	ctx := context.Background()
	tn.addEndpoint(t, true)
	ids := tn.ingest(t, 3) // one delivery per event

	eventOf := func(deliveryID any) string {
		var id string
		if err := tn.pool.QueryRow(ctx, `SELECT event_id::text FROM deliveries WHERE id = $1`, deliveryID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	oldFinished, oldPending, recentFinished := eventOf(ids[0]), eventOf(ids[1]), eventOf(ids[2])
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tn.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE events SET created_at = now() - interval '40 days' WHERE id = ANY($1::uuid[])`, []string{oldFinished, oldPending})
	exec(`UPDATE deliveries SET status = 'succeeded', attempt_count = 1 WHERE id = ANY($1::uuid[])`, []any{ids[0], ids[2]})
	exec(`INSERT INTO delivery_attempts (delivery_id, attempt_number, status_code) VALUES ($1, 1, 200)`, ids[0])

	retention, err := application.NewRetention(tn.store, 30*24*time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retention.PruneOnce(ctx); err != nil {
		t.Fatal(err)
	}

	exists := func(table, id string) bool {
		var ok bool
		if err := tn.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+table+` WHERE id = $1)`, id).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if exists("events", oldFinished) {
		t.Fatal("an old event whose deliveries finished must be deleted")
	}
	if !exists("events", oldPending) {
		t.Fatal("an old event with a pending delivery must be kept")
	}
	if !exists("events", recentFinished) {
		t.Fatal("a recent event must be kept")
	}
	var orphans int
	if err := tn.pool.QueryRow(ctx, `SELECT count(*) FROM delivery_attempts WHERE delivery_id = $1`, ids[0]).Scan(&orphans); err != nil {
		t.Fatal(err)
	}
	if exists("deliveries", ids[0].String()) || orphans != 0 {
		t.Fatal("deliveries and attempts must cascade with the event")
	}
}
