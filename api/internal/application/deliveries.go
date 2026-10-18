package application

import (
	"context"
	"errors"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type DeliveryService struct {
	projects   ProjectRepository
	events     EventRepository
	deliveries DeliveryRepository
}

func NewDeliveryService(projects ProjectRepository, events EventRepository, deliveries DeliveryRepository) *DeliveryService {
	return &DeliveryService{projects: projects, events: events, deliveries: deliveries}
}

// Get returns one delivery owned by the principal's organization.
func (s *DeliveryService) Get(ctx context.Context, principal Principal, id uuid.UUID) (domain.Delivery, error) {
	delivery, err := s.deliveries.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Delivery{}, ErrNotFound
		}
		return domain.Delivery{}, err
	}
	if delivery.OrganizationID != principal.OrganizationID {
		return domain.Delivery{}, ErrForbidden
	}
	if principal.IsPortal() && !sameCustomer(delivery.EndpointCustomer, principal.Customer) {
		return domain.Delivery{}, ErrNotFound
	}
	return delivery, nil
}

// ListAttempts returns the delivery's attempt history, oldest first.
func (s *DeliveryService) ListAttempts(ctx context.Context, principal Principal, deliveryID uuid.UUID) ([]domain.DeliveryAttempt, error) {
	if _, err := s.Get(ctx, principal, deliveryID); err != nil {
		return nil, err
	}
	return s.deliveries.ListAttempts(ctx, deliveryID)
}

// ListForProject lists the project's deliveries, newest first.
func (s *DeliveryService) ListForProject(ctx context.Context, principal Principal, projectID uuid.UUID, filter DeliveryFilter, page PageRequest) (Page[domain.Delivery], error) {
	if filter.Status != "" && !filter.Status.Valid() {
		return Page[domain.Delivery]{}, ErrInvalidInput
	}
	if _, err := ownedProject(ctx, s.projects, principal, projectID); err != nil {
		return Page[domain.Delivery]{}, err
	}
	if principal.IsPortal() {
		filter.Customer = principal.Customer
	}
	return paginate(page, func(limit, offset int32) ([]domain.Delivery, error) {
		return s.deliveries.ListByProject(ctx, projectID, filter, limit, offset)
	})
}

// Stats summarizes the project's deliveries by status.
func (s *DeliveryService) Stats(ctx context.Context, principal Principal, projectID uuid.UUID) (domain.DeliveryStats, error) {
	if principal.IsPortal() {
		return domain.DeliveryStats{}, ErrForbidden
	}
	if _, err := ownedProject(ctx, s.projects, principal, projectID); err != nil {
		return domain.DeliveryStats{}, err
	}
	return s.deliveries.StatsByProject(ctx, projectID)
}

// ListForEvent returns the event's fan-out deliveries and their replays.
func (s *DeliveryService) ListForEvent(ctx context.Context, principal Principal, eventID uuid.UUID) ([]domain.Delivery, error) {
	if principal.IsPortal() {
		return nil, ErrForbidden
	}
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
	original, err := s.Get(ctx, principal, deliveryID)
	if err != nil {
		return domain.Delivery{}, err
	}
	if !original.Status.IsTerminal() {
		return domain.Delivery{}, ErrReplayNotAllowed
	}
	return s.deliveries.CreateReplay(ctx, original.ID)
}
