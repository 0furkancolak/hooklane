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

func TestReplay(t *testing.T) {
	f := newFixture(t, false)
	ctx := context.Background()
	if _, _, err := f.endpoints.Create(ctx, f.principal, f.project.ID, application.CreateEndpointInput{URL: "https://example.com"}); err != nil {
		t.Fatal(err)
	}
	ingested, err := f.events.Ingest(ctx, f.principal, f.project.ID, application.IngestInput{EventType: "a", Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	svc := application.NewDeliveryService(f.store.Events(), f.store.Deliveries())
	list, err := svc.ListForEvent(ctx, f.principal, ingested.Event.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	original := list[0]

	if _, err := svc.Replay(ctx, f.principal, original.ID); !errors.Is(err, application.ErrReplayNotAllowed) {
		t.Fatalf("pending delivery replay: %v", err)
	}
	f.store.SetDeliveryStatus(original.ID, domain.DeliveryDeadLetter)

	intruder := application.Principal{OrganizationID: uuid.New()}
	if _, err := svc.Replay(ctx, intruder, original.ID); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant replay: %v", err)
	}
	if _, err := svc.ListForEvent(ctx, intruder, ingested.Event.ID); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("cross-tenant list: %v", err)
	}
	if _, err := svc.Replay(ctx, f.principal, uuid.New()); !errors.Is(err, application.ErrNotFound) {
		t.Fatalf("missing delivery: %v", err)
	}

	replay, err := svc.Replay(ctx, f.principal, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID == original.ID || replay.ReplayOf == nil || *replay.ReplayOf != original.ID ||
		replay.Status != domain.DeliveryPending || replay.AttemptCount != 0 ||
		replay.EventID != original.EventID || replay.EndpointID != original.EndpointID {
		t.Fatalf("replay = %+v", replay)
	}
	if _, err := svc.Replay(ctx, f.principal, original.ID); !errors.Is(err, application.ErrReplayInProgress) {
		t.Fatalf("second replay while first is pending: %v", err)
	}

	after, err := svc.ListForEvent(ctx, f.principal, ingested.Event.ID)
	if err != nil || len(after) != 2 {
		t.Fatalf("list after replay: %v %d", err, len(after))
	}
	for _, d := range after {
		if d.ID == original.ID && d.Status != domain.DeliveryDeadLetter {
			t.Fatalf("original changed: %+v", d)
		}
	}
	// The idempotent replay of the ingest request still reports the fan-out only.
	f.store.SetDeliveryStatus(replay.ID, domain.DeliverySucceeded)
	if _, err := svc.Replay(ctx, f.principal, original.ID); err != nil {
		t.Fatalf("replay after the previous replay finished: %v", err)
	}
}
