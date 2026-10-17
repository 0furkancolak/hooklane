package database

import (
	"context"
	"errors"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const activeReplayIndex = "deliveries_active_replay_idx"

func (s *Store) GetDelivery(ctx context.Context, id uuid.UUID) (domain.Delivery, error) {
	row, err := s.q(ctx).GetDelivery(ctx, uuidToPg(id))
	if err != nil {
		return domain.Delivery{}, err
	}
	d := mapDelivery(row.ID, row.EventID, row.EndpointID, row.Status, row.AttemptCount, row.NextAttemptAt,
		row.LastResponseStatus, row.LastError, row.ReplayOf, row.CreatedAt, row.UpdatedAt)
	d.OrganizationID = pgToUUID(row.OrganizationID)
	return d, nil
}

func (s *Store) ListDeliveriesByEvent(ctx context.Context, eventID uuid.UUID) ([]domain.Delivery, error) {
	rows, err := s.q(ctx).ListDeliveriesByEvent(ctx, uuidToPg(eventID))
	if err != nil {
		return nil, err
	}
	out := make([]domain.Delivery, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapDelivery(row.ID, row.EventID, row.EndpointID, row.Status, row.AttemptCount, row.NextAttemptAt,
			row.LastResponseStatus, row.LastError, row.ReplayOf, row.CreatedAt, row.UpdatedAt))
	}
	return out, nil
}

func (s *Store) CreateReplayDelivery(ctx context.Context, originalID uuid.UUID) (domain.Delivery, error) {
	row, err := s.q(ctx).CreateReplayDelivery(ctx, uuidToPg(originalID))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == activeReplayIndex {
			return domain.Delivery{}, application.ErrReplayInProgress
		}
		return domain.Delivery{}, err
	}
	return mapDelivery(row.ID, row.EventID, row.EndpointID, row.Status, row.AttemptCount, row.NextAttemptAt,
		row.LastResponseStatus, row.LastError, row.ReplayOf, row.CreatedAt, row.UpdatedAt), nil
}

func mapDelivery(
	id, eventID, endpointID pgtype.UUID,
	status string,
	attempts int32,
	nextAttemptAt pgtype.Timestamptz,
	lastStatus pgtype.Int4,
	lastError pgtype.Text,
	replayOf pgtype.UUID,
	createdAt, updatedAt pgtype.Timestamptz,
) domain.Delivery {
	d := domain.Delivery{
		ID:            pgToUUID(id),
		EventID:       pgToUUID(eventID),
		EndpointID:    pgToUUID(endpointID),
		Status:        domain.DeliveryStatus(status),
		AttemptCount:  attempts,
		NextAttemptAt: nextAttemptAt.Time,
		CreatedAt:     createdAt.Time,
		UpdatedAt:     updatedAt.Time,
	}
	if lastStatus.Valid {
		v := lastStatus.Int32
		d.LastResponseStatus = &v
	}
	if lastError.Valid {
		v := lastError.String
		d.LastError = &v
	}
	if replayOf.Valid {
		v := pgToUUID(replayOf)
		d.ReplayOf = &v
	}
	return d
}
