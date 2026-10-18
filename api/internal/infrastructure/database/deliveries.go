package database

import (
	"context"
	"errors"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database/db"
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
	d.EndpointCustomer = textFromPg(row.EndpointCustomer)
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

func (s *Store) ListDeliveriesByProject(ctx context.Context, projectID uuid.UUID, filter application.DeliveryFilter, limit, offset int32) ([]domain.Delivery, error) {
	params := db.ListDeliveriesByProjectParams{ProjectID: uuidToPg(projectID), Limit: limit, Offset: offset}
	if filter.Status != "" {
		params.Status = pgtype.Text{String: string(filter.Status), Valid: true}
	}
	if filter.EndpointID != nil {
		params.EndpointID = uuidToPg(*filter.EndpointID)
	}
	params.Customer = textPtr(filter.Customer)
	rows, err := s.q(ctx).ListDeliveriesByProject(ctx, params)
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

func (s *Store) ListDeliveryAttempts(ctx context.Context, deliveryID uuid.UUID) ([]domain.DeliveryAttempt, error) {
	rows, err := s.q(ctx).ListDeliveryAttempts(ctx, uuidToPg(deliveryID))
	if err != nil {
		return nil, err
	}
	out := make([]domain.DeliveryAttempt, 0, len(rows))
	for _, row := range rows {
		attempt := domain.DeliveryAttempt{
			ID:            pgToUUID(row.ID),
			DeliveryID:    pgToUUID(row.DeliveryID),
			AttemptNumber: row.AttemptNumber,
			AttemptedAt:   row.AttemptedAt.Time,
		}
		if row.StatusCode.Valid {
			v := row.StatusCode.Int32
			attempt.StatusCode = &v
		}
		if row.ResponseBody.Valid {
			v := row.ResponseBody.String
			attempt.ResponseBody = &v
		}
		if row.Error.Valid {
			v := row.Error.String
			attempt.Error = &v
		}
		if row.DurationMs.Valid {
			v := row.DurationMs.Int64
			attempt.DurationMS = &v
		}
		out = append(out, attempt)
	}
	return out, nil
}

func (s *Store) DeliveryStatsByProject(ctx context.Context, projectID uuid.UUID) (domain.DeliveryStats, error) {
	row, err := s.q(ctx).DeliveryStatsByProject(ctx, uuidToPg(projectID))
	if err != nil {
		return domain.DeliveryStats{}, err
	}
	return domain.DeliveryStats{
		Pending:     row.Pending,
		Processing:  row.Processing,
		Succeeded:   row.Succeeded,
		DeadLetter:  row.DeadLetter,
		OldestDueAt: pgTimePtr(row.OldestDueAt),
	}, nil
}

// QueueBacklog reports instance-wide queue depth for worker metrics.
func (s *Store) QueueBacklog(ctx context.Context) (application.QueueBacklog, error) {
	row, err := s.q(ctx).QueueBacklog(ctx)
	if err != nil {
		return application.QueueBacklog{}, err
	}
	return application.QueueBacklog{
		Due:              row.Due,
		Scheduled:        row.Scheduled,
		Held:             row.Held,
		Processing:       row.Processing,
		OldestDueSeconds: row.OldestDueSeconds,
	}, nil
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
