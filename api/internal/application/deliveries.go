package application

import (
	"context"
	"errors"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type DeliveryService struct {
	events     EventRepository
	deliveries DeliveryRepository
}

func NewDeliveryService(events EventRepository, deliveries DeliveryRepository) *DeliveryService {
	return &DeliveryService{events: events, deliveries: deliveries}
}

// ListForEvent returns the event's fan-out deliveries and their replays.
func (s *DeliveryService) ListForEvent(ctx context.Context, principal Principal, eventID uuid.UUID) ([]domain.Delivery, error) {
	event, err := s.events.Get(ctx, eventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if event.OrganizationID != principal.OrganizationID {
		return nil, ErrForbidden
	}
	return s.deliveries.ListByEvent(ctx, eventID)
}

// Replay queues a new delivery of the same event to the same endpoint. Only
// finished deliveries (succeeded or dead-lettered) can be replayed; the
// original delivery and its attempt history are left untouched. The new
// delivery gets a fresh retry budget and is held back while its endpoint is
// disabled.
func (s *DeliveryService) Replay(ctx context.Context, principal Principal, deliveryID uuid.UUID) (domain.Delivery, error) {
	original, err := s.deliveries.Get(ctx, deliveryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Delivery{}, ErrNotFound
		}
		return domain.Delivery{}, err
	}
	if original.OrganizationID != principal.OrganizationID {
		return domain.Delivery{}, ErrForbidden
	}
	if !original.Status.IsTerminal() {
		return domain.Delivery{}, ErrReplayNotAllowed
	}
	return s.deliveries.CreateReplay(ctx, original.ID)
}
