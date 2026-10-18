package database

import (
	"context"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type Queue struct{ Store *Store }

func (s *Store) Queue() Queue { return Queue{Store: s} }

func (q Queue) Claim(ctx context.Context, batchSize int32, lease time.Duration) ([]application.ClaimedDelivery, error) {
	rows, err := q.Store.q(ctx).ClaimDeliveries(ctx, db.ClaimDeliveriesParams{
		BatchSize:    batchSize,
		LeaseSeconds: lease.Seconds(),
	})
	if err != nil {
		return nil, err
	}
	claims := make([]application.ClaimedDelivery, 0, len(rows))
	for _, row := range rows {
		claims = append(claims, application.ClaimedDelivery{
			DeliveryID:  pgToUUID(row.ID),
			LeaseToken:  pgToUUID(row.LeaseToken),
			Attempt:     row.AttemptCount,
			LockedUntil: row.LockedUntil.Time,
			Event: domain.Event{
				ID:        pgToUUID(row.EventID),
				ProjectID: pgToUUID(row.ProjectID),
				EventType: row.EventType,
				Payload:   row.Payload,
				CreatedAt: row.EventCreatedAt.Time,
			},
			Endpoint: domain.Endpoint{
				ID:              pgToUUID(row.EndpointID),
				ProjectID:       pgToUUID(row.ProjectID),
				URL:             row.Url,
				SecretEncrypted: row.SecretEncrypted,
				Enabled:         true,
			},
		})
	}
	return claims, nil
}

func (q Queue) Complete(ctx context.Context, claim application.ClaimedDelivery, outcome application.DeliveryOutcome) error {
	return q.Store.WithinTx(ctx, func(ctx context.Context) error {
		attempt := outcome.Attempt
		params := db.CompleteDeliveryParams{
			Status:             string(outcome.Status),
			LastResponseStatus: int4Ptr(attempt.StatusCode),
			LastError:          textPtr(attempt.Error),
			ID:                 uuidToPg(claim.DeliveryID),
			LeaseToken:         uuidToPg(claim.LeaseToken),
		}
		if outcome.RetryDelay != nil {
			params.RetryDelaySeconds = pgtype.Float8{Float64: outcome.RetryDelay.Seconds(), Valid: true}
		}
		updated, err := q.Store.q(ctx).CompleteDelivery(ctx, params)
		if err != nil {
			return err
		}
		if updated == 0 {
			return application.ErrLeaseLost
		}
		return q.Store.q(ctx).InsertDeliveryAttempt(ctx, db.InsertDeliveryAttemptParams{
			DeliveryID:    uuidToPg(claim.DeliveryID),
			AttemptNumber: claim.Attempt,
			StatusCode:    int4Ptr(attempt.StatusCode),
			ResponseBody:  textPtr(attempt.ResponseBody),
			Error:         textPtr(attempt.Error),
			DurationMs:    int8Ptr(attempt.DurationMS),
		})
	})
}

var _ application.DeliveryQueue = Queue{}
