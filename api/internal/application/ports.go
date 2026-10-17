package application

import (
	"context"
	"errors"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
)

type OrganizationRepository interface {
	Create(ctx context.Context, org domain.Organization) (domain.Organization, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Organization, error)
	Count(ctx context.Context) (int64, error)
}

type ProjectRepository interface {
	Create(ctx context.Context, project domain.Project) (domain.Project, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Project, error)
	ListByOrganization(ctx context.Context, organizationID uuid.UUID, limit, offset int32) ([]domain.Project, error)
}

type APIKeyRepository interface {
	Create(ctx context.Context, key domain.APIKey) (domain.APIKey, error)
	GetByHash(ctx context.Context, hash []byte) (domain.APIKey, error)
	Get(ctx context.Context, id uuid.UUID) (domain.APIKey, error)
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]domain.APIKey, error)
	Revoke(ctx context.Context, id uuid.UUID) (domain.APIKey, error)
	TouchLastUsed(ctx context.Context, id uuid.UUID) error
}

type EndpointRepository interface {
	Create(ctx context.Context, endpoint domain.Endpoint) (domain.Endpoint, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Endpoint, error)
	ListByProject(ctx context.Context, projectID uuid.UUID, limit, offset int32) ([]domain.Endpoint, error)
	Update(ctx context.Context, id uuid.UUID, update domain.EndpointUpdate) (domain.Endpoint, error)
}

// IdempotentInsert reports the outcome of inserting an event that may carry an
// idempotency key. When Created is false, Event is the previously stored event
// and Matches reports whether its type and payload equal the new request.
type IdempotentInsert struct {
	Event   domain.Event
	Created bool
	Matches bool
}

type EventRepository interface {
	InsertIdempotent(ctx context.Context, event domain.Event) (IdempotentInsert, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Event, error)
	ListByProject(ctx context.Context, projectID uuid.UUID, limit, offset int32) ([]domain.Event, error)
}

type DeliveryRepository interface {
	CreateForEvent(ctx context.Context, eventID, projectID uuid.UUID) (int64, error)
	CountByEvent(ctx context.Context, eventID uuid.UUID) (int64, error)
}

// ErrLeaseLost means the delivery's lease expired and another worker may have
// reclaimed it; the caller's result must be discarded.
var ErrLeaseLost = errors.New("delivery lease lost")

// ClaimedDelivery is a delivery leased to one worker until LockedUntil.
type ClaimedDelivery struct {
	DeliveryID  uuid.UUID
	LeaseToken  uuid.UUID
	Attempt     int32
	LockedUntil time.Time
	Event       domain.Event
	Endpoint    domain.Endpoint
}

// DeliveryOutcome is the result of one attempt. NextAttemptAt is only used
// when Status puts the delivery back in the queue.
type DeliveryOutcome struct {
	Status        domain.DeliveryStatus
	NextAttemptAt *time.Time
	Attempt       domain.DeliveryAttempt
}

type DeliveryQueue interface {
	Claim(ctx context.Context, batchSize int32, lease time.Duration) ([]ClaimedDelivery, error)
	// Complete records the attempt and releases the lease atomically. It
	// returns ErrLeaseLost when the lease token no longer matches.
	Complete(ctx context.Context, claim ClaimedDelivery, outcome DeliveryOutcome) error
}

type Transactor interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type WebhookRequest struct {
	URL       string
	Secret    string
	MessageID string
	Body      []byte
}

// WebhookResult describes one outbound attempt. Err is set for transport
// failures (DNS, connect, timeout); StatusCode is zero in that case.
type WebhookResult struct {
	StatusCode   int
	ResponseBody string
	Duration     time.Duration
	Err          error
}

type WebhookSender interface {
	Send(ctx context.Context, req WebhookRequest) WebhookResult
}
