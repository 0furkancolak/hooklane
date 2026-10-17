package database_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
)

// An endpoint saved with a hostname (or before destination checks existed)
// is still judged by the address it resolves to when the worker connects.
func TestIntegrationProductionWorkerDeadLettersBlockedDestination(t *testing.T) {
	tn := newTenant(t)
	box, err := application.NewSecretBox(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	server, hits := flakyServer(0)
	defer server.Close()
	tn.endpointFor(t, box, strings.Replace(server.URL, "127.0.0.1", "localhost", 1)+"/hook")
	ids := tn.ingest(t, 1)

	policy := application.RetryPolicy{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}
	runWorkerWith(t, tn, box, policy, application.DestinationPolicy{}, func() bool {
		return tn.statusOf(t, ids[0]) == "dead_letter"
	})

	if hits.Load() != 0 {
		t.Fatalf("blocked destination received %d requests", hits.Load())
	}
	var attempts int
	var message string
	if err := tn.pool.QueryRow(context.Background(),
		`SELECT count(*), coalesce(max(error), '') FROM delivery_attempts WHERE delivery_id = $1`, ids[0]).Scan(&attempts, &message); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || !strings.Contains(message, application.ErrDestinationNotAllowed.Error()) {
		t.Fatalf("attempts=%d error=%q", attempts, message)
	}
}
