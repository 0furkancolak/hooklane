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

type PortalTokenRepository interface {
	Create(ctx context.Context, token domain.PortalToken) (domain.PortalToken, error)
	GetByHash(ctx context.Context, hash []byte) (domain.PortalToken, error)
}

type AccountRepository interface {
	// CreateUser returns ErrEmailTaken when the email is already registered.
	CreateUser(ctx context.Context, email, passwordHash string) (domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	GetUser(ctx context.Context, id uuid.UUID) (domain.User, error)
	CountUsers(ctx context.Context) (int64, error)
	AddMembership(ctx context.Context, userID, organizationID uuid.UUID) error
	ListOrganizations(ctx context.Context, userID uuid.UUID) ([]domain.Organization, error)
	IsMember(ctx context.Context, userID, organizationID uuid.UUID) (bool, error)
}

type SessionRepository interface {
	Create(ctx context.Context, session domain.Session) (domain.Session, error)
	GetByHash(ctx context.Context, hash []byte) (domain.Session, error)
	SetOrganization(ctx context.Context, id, organizationID uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteExpired(ctx context.Context) (int64, error)
}

type EndpointRepository interface {
	Create(ctx context.Context, endpoint domain.Endpoint) (domain.Endpoint, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Endpoint, error)
	ListByProject(ctx context.Context, projectID uuid.UUID, filter EndpointFilter, limit, offset int32) ([]domain.Endpoint, error)
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
	ListByProject(ctx context.Context, projectID uuid.UUID, filter EventFilter, limit, offset int32) ([]domain.Event, error)
}

type DeliveryRepository interface {
	// CreateForEvent queues one delivery per enabled endpoint of the project
	// whose customer equals the event's (both absent counts as equal).
	CreateForEvent(ctx context.Context, eventID, projectID uuid.UUID, customer *string) (int64, error)
	CountByEvent(ctx context.Context, eventID uuid.UUID) (int64, error)
	// Get returns the delivery with its owning organization.
	Get(ctx context.Context, id uuid.UUID) (domain.Delivery, error)
	ListByEvent(ctx context.Context, eventID uuid.UUID) ([]domain.Delivery, error)
	// CreateReplay inserts a pending delivery for the same event and endpoint.
	// It returns ErrReplayInProgress when an unfinished replay already exists.
	CreateReplay(ctx context.Context, originalID uuid.UUID) (domain.Delivery, error)
	ListByProject(ctx context.Context, projectID uuid.UUID, filter DeliveryFilter, limit, offset int32) ([]domain.Delivery, error)
	ListAttempts(ctx context.Context, deliveryID uuid.UUID) ([]domain.DeliveryAttempt, error)
	StatsByProject(ctx context.Context, projectID uuid.UUID) (domain.DeliveryStats, error)
}

// QueueBacklog is the instance-wide queue depth exported as worker metrics.
type QueueBacklog struct {
	Due       int64
	Scheduled int64
	// Held counts pending deliveries of disabled endpoints.
	Held             int64
	Processing       int64
	OldestDueSeconds float64
}

// Zero-valued filter fields match everything.
type EndpointFilter struct {
	Enabled  *bool
	Customer *string
}

type EventFilter struct {
	EventType string
	Customer  *string
}

type DeliveryFilter struct {
	Status     domain.DeliveryStatus
	EndpointID *uuid.UUID
	Customer   *string
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

// DeliveryOutcome is the result of one attempt. RetryDelay is set when Status
// is pending; the store applies it relative to the database clock.
type DeliveryOutcome struct {
	Status     domain.DeliveryStatus
	RetryDelay *time.Duration
	Attempt    domain.DeliveryAttempt
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
	// RetryAfter is the receiver's Retry-After hint, zero when absent.
	RetryAfter time.Duration
	Duration   time.Duration
	Err        error
}

type WebhookSender interface {
	Send(ctx context.Context, req WebhookRequest) WebhookResult
}
