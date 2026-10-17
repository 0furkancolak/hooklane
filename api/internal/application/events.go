package application

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	MaxEventPayloadBytes    = 256 << 10
	maxIdempotencyKeyLength = 255
)

var eventTypePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type EventService struct {
	projects   ProjectRepository
	events     EventRepository
	deliveries DeliveryRepository
	tx         Transactor
}

func NewEventService(
	projects ProjectRepository,
	events EventRepository,
	deliveries DeliveryRepository,
	tx Transactor,
) *EventService {
	return &EventService{projects: projects, events: events, deliveries: deliveries, tx: tx}
}

type IngestInput struct {
	EventType      string
	Payload        json.RawMessage
	IdempotencyKey string
}

type IngestResult struct {
	Event         domain.Event
	DeliveryCount int64
	// Replayed is true when an earlier request with the same idempotency key
	// already created the event; nothing new was written.
	Replayed bool
}

// Ingest stores the event and one pending delivery per enabled endpoint in a
// single transaction. The deliveries are the outbox the worker consumes (see
// ADR 0001). Repeating a request with the same
// idempotency key returns the original event; reusing the key with a
// different event type or payload fails with ErrIdempotencyConflict.
func (s *EventService) Ingest(ctx context.Context, principal Principal, projectID uuid.UUID, in IngestInput) (IngestResult, error) {
	event, err := validateIngest(in)
	if err != nil {
		return IngestResult{}, err
	}
	project, err := ownedProject(ctx, s.projects, principal, projectID)
	if err != nil {
		return IngestResult{}, err
	}
	event.ProjectID = project.ID
	event.OrganizationID = project.OrganizationID

	var result IngestResult
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		inserted, err := s.events.InsertIdempotent(ctx, event)
		if err != nil {
			return err
		}
		if !inserted.Created {
			if !inserted.Matches {
				return ErrIdempotencyConflict
			}
			count, err := s.deliveries.CountByEvent(ctx, inserted.Event.ID)
			if err != nil {
				return err
			}
			result = IngestResult{Event: inserted.Event, DeliveryCount: count, Replayed: true}
			return nil
		}

		count, err := s.deliveries.CreateForEvent(ctx, inserted.Event.ID, project.ID)
		if err != nil {
			return err
		}
		result = IngestResult{Event: inserted.Event, DeliveryCount: count}
		return nil
	})
	if err != nil {
		return IngestResult{}, err
	}
	return result, nil
}

func (s *EventService) Get(ctx context.Context, principal Principal, id uuid.UUID) (domain.Event, error) {
	event, err := s.events.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Event{}, ErrNotFound
		}
		return domain.Event{}, err
	}
	if event.OrganizationID != principal.OrganizationID {
		return domain.Event{}, ErrForbidden
	}
	return event, nil
}

func (s *EventService) List(ctx context.Context, principal Principal, projectID uuid.UUID, limit, offset int32) ([]domain.Event, error) {
	if _, err := ownedProject(ctx, s.projects, principal, projectID); err != nil {
		return nil, err
	}
	limit, offset = clampPage(limit, offset)
	return s.events.ListByProject(ctx, projectID, limit, offset)
}

func validateIngest(in IngestInput) (domain.Event, error) {
	eventType := strings.TrimSpace(in.EventType)
	if !eventTypePattern.MatchString(eventType) {
		return domain.Event{}, ErrInvalidInput
	}
	if len(in.Payload) > MaxEventPayloadBytes {
		return domain.Event{}, ErrPayloadTooLarge
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(in.Payload, &object); err != nil || object == nil {
		return domain.Event{}, ErrInvalidInput
	}

	event := domain.Event{EventType: eventType, Payload: in.Payload}
	if in.IdempotencyKey != "" {
		key := in.IdempotencyKey
		if len(key) > maxIdempotencyKeyLength || !isPrintableASCII(key) {
			return domain.Event{}, ErrInvalidInput
		}
		event.IdempotencyKey = &key
	}
	return event, nil
}

func isPrintableASCII(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < 0x21 || value[i] > 0x7e {
			return false
		}
	}
	return true
}
