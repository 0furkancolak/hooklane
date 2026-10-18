package database

import (
	"context"
	"errors"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// InsertEvent relies on ON CONFLICT DO NOTHING against the partial unique index
// on (project_id, idempotency_key). Under READ COMMITTED a concurrent insert of
// the same key blocks until the other transaction finishes; the follow-up
// SELECT then sees the committed row, so every caller converges on one event.
func (s *Store) InsertEvent(ctx context.Context, event domain.Event) (application.IdempotentInsert, error) {
	key := pgtype.Text{}
	if event.IdempotencyKey != nil {
		key = pgtype.Text{String: *event.IdempotencyKey, Valid: true}
	}
	row, err := s.q(ctx).InsertEvent(ctx, db.InsertEventParams{
		ProjectID:      uuidToPg(event.ProjectID),
		EventType:      event.EventType,
		Payload:        event.Payload,
		IdempotencyKey: key,
	})
	if err == nil {
		mapped := mapEvent(row)
		mapped.OrganizationID = event.OrganizationID
		return application.IdempotentInsert{Event: mapped, Created: true, Matches: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) || !key.Valid {
		return application.IdempotentInsert{}, err
	}

	existing, err := s.q(ctx).GetEventByIdempotencyKey(ctx, db.GetEventByIdempotencyKeyParams{
		EventType:      event.EventType,
		Payload:        event.Payload,
		ProjectID:      uuidToPg(event.ProjectID),
		IdempotencyKey: key.String,
	})
	if err != nil {
		return application.IdempotentInsert{}, err
	}
	mapped := mapEvent(db.Event{
		ID:             existing.ID,
		ProjectID:      existing.ProjectID,
		EventType:      existing.EventType,
		Payload:        existing.Payload,
		IdempotencyKey: existing.IdempotencyKey,
		CreatedAt:      existing.CreatedAt,
	})
	mapped.OrganizationID = event.OrganizationID
	return application.IdempotentInsert{Event: mapped, Created: false, Matches: existing.Matches}, nil
}

func (s *Store) GetEvent(ctx context.Context, id uuid.UUID) (domain.Event, error) {
	row, err := s.q(ctx).GetEvent(ctx, uuidToPg(id))
	if err != nil {
		return domain.Event{}, err
	}
	mapped := mapEvent(db.Event{
		ID:             row.ID,
		ProjectID:      row.ProjectID,
		EventType:      row.EventType,
		Payload:        row.Payload,
		IdempotencyKey: row.IdempotencyKey,
		CreatedAt:      row.CreatedAt,
	})
	mapped.OrganizationID = pgToUUID(row.OrganizationID)
	return mapped, nil
}

func (s *Store) ListEventsByProject(ctx context.Context, projectID uuid.UUID, filter application.EventFilter, limit, offset int32) ([]domain.Event, error) {
	eventType := pgtype.Text{}
	if filter.EventType != "" {
		eventType = pgtype.Text{String: filter.EventType, Valid: true}
	}
	rows, err := s.q(ctx).ListEventsByProject(ctx, db.ListEventsByProjectParams{
		ProjectID: uuidToPg(projectID),
		EventType: eventType,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	events := make([]domain.Event, 0, len(rows))
	for _, row := range rows {
		events = append(events, mapEvent(row))
	}
	return events, nil
}

func (s *Store) CreateDeliveriesForEvent(ctx context.Context, eventID, projectID uuid.UUID) (int64, error) {
	return s.q(ctx).CreateDeliveriesForEvent(ctx, db.CreateDeliveriesForEventParams{
		EventID:   uuidToPg(eventID),
		ProjectID: uuidToPg(projectID),
	})
}

func (s *Store) CountDeliveriesByEvent(ctx context.Context, eventID uuid.UUID) (int64, error) {
	return s.q(ctx).CountDeliveriesByEvent(ctx, uuidToPg(eventID))
}

// DeleteExpiredEvents implements application.EventPruner.
func (s *Store) DeleteExpiredEvents(ctx context.Context, retention time.Duration, batch int32) (int64, error) {
	return s.q(ctx).DeleteExpiredEvents(ctx, db.DeleteExpiredEventsParams{
		RetentionSeconds: retention.Seconds(),
		BatchSize:        batch,
	})
}

func mapEvent(row db.Event) domain.Event {
	var key *string
	if row.IdempotencyKey.Valid {
		value := row.IdempotencyKey.String
		key = &value
	}
	return domain.Event{
		ID:             pgToUUID(row.ID),
		ProjectID:      pgToUUID(row.ProjectID),
		EventType:      row.EventType,
		Payload:        row.Payload,
		IdempotencyKey: key,
		CreatedAt:      row.CreatedAt.Time,
	}
}
