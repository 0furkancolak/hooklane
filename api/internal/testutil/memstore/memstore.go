// Package memstore is an in-memory implementation of the repository ports for
// unit tests. WithinTx snapshots state and restores it when fn fails, so tests
// can assert rollback behaviour without PostgreSQL.
package memstore

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type state struct {
	projects     []domain.Project
	keys         []domain.APIKey
	portalTokens []domain.PortalToken
	endpoints    []domain.Endpoint
	events       []domain.Event
	deliveries   []domain.Delivery
	attempts     []domain.DeliveryAttempt
}

func (s state) clone() state {
	return state{
		projects:     slices.Clone(s.projects),
		keys:         slices.Clone(s.keys),
		portalTokens: slices.Clone(s.portalTokens),
		endpoints:    slices.Clone(s.endpoints),
		events:       slices.Clone(s.events),
		deliveries:   slices.Clone(s.deliveries),
		attempts:     slices.Clone(s.attempts),
	}
}

type Store struct {
	mu    sync.Mutex
	txMu  sync.Mutex
	state state

	accountsOnce sync.Once
	acct         *accountState

	// FailDeliveries makes delivery creation fail after the event was
	// inserted, to exercise transaction rollback.
	FailDeliveries error
}

func New() *Store { return &Store{} }

func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	s.txMu.Lock()
	defer s.txMu.Unlock()
	s.mu.Lock()
	snapshot := s.state.clone()
	s.mu.Unlock()
	if err := fn(ctx); err != nil {
		s.mu.Lock()
		s.state = snapshot
		s.mu.Unlock()
		return err
	}
	return nil
}

// Counts returns the number of stored events and deliveries.
func (s *Store) Counts() (events, deliveries int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.state.events), len(s.state.deliveries)
}

func (s *Store) Projects() Projects         { return Projects{s} }
func (s *Store) Keys() Keys                 { return Keys{s} }
func (s *Store) PortalTokens() PortalTokens { return PortalTokens{s} }
func (s *Store) Endpoints() Endpoints       { return Endpoints{s} }
func (s *Store) Events() Events             { return Events{s} }
func (s *Store) Deliveries() Deliveries     { return Deliveries{s} }

type Projects struct{ s *Store }

func (r Projects) Create(_ context.Context, project domain.Project) (domain.Project, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	project.ID = uuid.New()
	project.CreatedAt = time.Now().UTC()
	r.s.state.projects = append(r.s.state.projects, project)
	return project, nil
}

func (r Projects) Get(_ context.Context, id uuid.UUID) (domain.Project, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, p := range r.s.state.projects {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Project{}, pgx.ErrNoRows
}

func (r Projects) ListByOrganization(_ context.Context, organizationID uuid.UUID, limit, offset int32) ([]domain.Project, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []domain.Project
	for _, p := range r.s.state.projects {
		if p.OrganizationID == organizationID {
			out = append(out, p)
		}
	}
	return page(out, limit, offset), nil
}

// OldestProject returns the first project created, or pgx.ErrNoRows.
func (s *Store) OldestProject(context.Context) (domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.state.projects) == 0 {
		return domain.Project{}, pgx.ErrNoRows
	}
	return s.state.projects[0], nil
}

type PortalTokens struct{ s *Store }

func (r PortalTokens) Create(_ context.Context, token domain.PortalToken) (domain.PortalToken, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	token.ID = uuid.New()
	token.CreatedAt = time.Now().UTC()
	r.s.state.portalTokens = append(r.s.state.portalTokens, token)
	return token, nil
}

func (r PortalTokens) GetByHash(_ context.Context, hash []byte) (domain.PortalToken, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, t := range r.s.state.portalTokens {
		if bytes.Equal(t.Hash, hash) {
			return t, nil
		}
	}
	return domain.PortalToken{}, pgx.ErrNoRows
}

type Keys struct{ s *Store }

func (r Keys) Create(_ context.Context, key domain.APIKey) (domain.APIKey, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	key.ID = uuid.New()
	key.CreatedAt = time.Now().UTC()
	if len(key.Scopes) == 0 {
		key.Scopes = []domain.Scope{domain.ScopeAdmin} // the column default
	}
	r.s.state.keys = append(r.s.state.keys, key)
	return key, nil
}

func (r Keys) GetByHash(_ context.Context, hash []byte) (domain.APIKey, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, k := range r.s.state.keys {
		if bytes.Equal(k.Hash, hash) {
			return k, nil
		}
	}
	return domain.APIKey{}, pgx.ErrNoRows
}

func (r Keys) Get(_ context.Context, id uuid.UUID) (domain.APIKey, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, k := range r.s.state.keys {
		if k.ID == id {
			return k, nil
		}
	}
	return domain.APIKey{}, pgx.ErrNoRows
}

func (r Keys) ListByProject(_ context.Context, projectID uuid.UUID) ([]domain.APIKey, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []domain.APIKey
	for _, k := range r.s.state.keys {
		if k.ProjectID == projectID {
			out = append(out, k)
		}
	}
	return out, nil
}

func (r Keys) Revoke(_ context.Context, id uuid.UUID) (domain.APIKey, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for i, k := range r.s.state.keys {
		if k.ID == id && k.RevokedAt == nil {
			now := time.Now().UTC()
			r.s.state.keys[i].RevokedAt = &now
			return r.s.state.keys[i], nil
		}
	}
	return domain.APIKey{}, pgx.ErrNoRows
}

func (r Keys) TouchLastUsed(context.Context, uuid.UUID) error { return nil }

type Endpoints struct{ s *Store }

func (r Endpoints) Create(_ context.Context, endpoint domain.Endpoint) (domain.Endpoint, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	endpoint.ID = uuid.New()
	endpoint.CreatedAt = time.Now().UTC()
	endpoint.UpdatedAt = endpoint.CreatedAt
	r.s.state.endpoints = append(r.s.state.endpoints, endpoint)
	return endpoint, nil
}

func (r Endpoints) Get(_ context.Context, id uuid.UUID) (domain.Endpoint, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, e := range r.s.state.endpoints {
		if e.ID == id {
			return e, nil
		}
	}
	return domain.Endpoint{}, pgx.ErrNoRows
}

func (r Endpoints) ListByProject(_ context.Context, projectID uuid.UUID, filter application.EndpointFilter, limit, offset int32) ([]domain.Endpoint, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []domain.Endpoint
	for _, e := range r.s.state.endpoints {
		if e.ProjectID == projectID && (filter.Enabled == nil || e.Enabled == *filter.Enabled) && (filter.Customer == nil || sameCustomer(e.Customer, filter.Customer)) {
			out = append(out, e)
		}
	}
	return page(out, limit, offset), nil
}

func (r Endpoints) Update(_ context.Context, id uuid.UUID, update domain.EndpointUpdate) (domain.Endpoint, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for i, e := range r.s.state.endpoints {
		if e.ID != id {
			continue
		}
		if update.URL != nil {
			e.URL = *update.URL
		}
		if update.Enabled != nil {
			e.Enabled = *update.Enabled
		}
		e.UpdatedAt = time.Now().UTC()
		r.s.state.endpoints[i] = e
		return e, nil
	}
	return domain.Endpoint{}, pgx.ErrNoRows
}

type Events struct{ s *Store }

// InsertIdempotent compares payloads byte-for-byte; PostgreSQL compares jsonb
// semantically, which integration tests cover.
func (r Events) InsertIdempotent(_ context.Context, event domain.Event) (application.IdempotentInsert, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if event.IdempotencyKey != nil {
		for _, existing := range r.s.state.events {
			if existing.ProjectID == event.ProjectID && existing.IdempotencyKey != nil && *existing.IdempotencyKey == *event.IdempotencyKey {
				matches := existing.EventType == event.EventType && bytes.Equal(existing.Payload, event.Payload) && sameCustomer(existing.Customer, event.Customer)
				return application.IdempotentInsert{Event: existing, Created: false, Matches: matches}, nil
			}
		}
	}
	event.ID = uuid.New()
	event.CreatedAt = time.Now().UTC()
	r.s.state.events = append(r.s.state.events, event)
	return application.IdempotentInsert{Event: event, Created: true, Matches: true}, nil
}

func (r Events) Get(_ context.Context, id uuid.UUID) (domain.Event, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, e := range r.s.state.events {
		if e.ID == id {
			return e, nil
		}
	}
	return domain.Event{}, pgx.ErrNoRows
}

func (r Events) ListByProject(_ context.Context, projectID uuid.UUID, filter application.EventFilter, limit, offset int32) ([]domain.Event, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []domain.Event
	for _, e := range r.s.state.events {
		if e.ProjectID == projectID && (filter.EventType == "" || e.EventType == filter.EventType) && (filter.Customer == nil || sameCustomer(e.Customer, filter.Customer)) {
			out = append(out, e)
		}
	}
	return page(out, limit, offset), nil
}

type Deliveries struct{ s *Store }

func (r Deliveries) CreateForEvent(_ context.Context, eventID, projectID uuid.UUID, customer *string) (int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.FailDeliveries != nil {
		return 0, r.s.FailDeliveries
	}
	var n int64
	for _, e := range r.s.state.endpoints {
		if e.ProjectID == projectID && e.Enabled && sameCustomer(e.Customer, customer) {
			r.s.state.deliveries = append(r.s.state.deliveries, domain.Delivery{
				ID:         uuid.New(),
				EventID:    eventID,
				EndpointID: e.ID,
				Status:     domain.DeliveryPending,
				CreatedAt:  time.Now().UTC(),
			})
			n++
		}
	}
	return n, nil
}

func (r Deliveries) CountByEvent(_ context.Context, eventID uuid.UUID) (int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var n int64
	for _, d := range r.s.state.deliveries {
		if d.EventID == eventID && d.ReplayOf == nil {
			n++
		}
	}
	return n, nil
}

func (r Deliveries) Get(_ context.Context, id uuid.UUID) (domain.Delivery, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, d := range r.s.state.deliveries {
		if d.ID == id {
			d.OrganizationID = r.s.orgOfEvent(d.EventID)
			for _, e := range r.s.state.endpoints {
				if e.ID == d.EndpointID {
					d.EndpointCustomer = e.Customer
				}
			}
			return d, nil
		}
	}
	return domain.Delivery{}, pgx.ErrNoRows
}

func (r Deliveries) ListByEvent(_ context.Context, eventID uuid.UUID) ([]domain.Delivery, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	out := []domain.Delivery{}
	for _, d := range r.s.state.deliveries {
		if d.EventID == eventID {
			out = append(out, d)
		}
	}
	return out, nil
}

func (r Deliveries) CreateReplay(_ context.Context, originalID uuid.UUID) (domain.Delivery, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var original *domain.Delivery
	for i, d := range r.s.state.deliveries {
		if d.ID == originalID {
			original = &r.s.state.deliveries[i]
		}
		if d.ReplayOf != nil && *d.ReplayOf == originalID && (d.Status == domain.DeliveryPending || d.Status == domain.DeliveryProcessing) {
			return domain.Delivery{}, application.ErrReplayInProgress
		}
	}
	if original == nil {
		return domain.Delivery{}, pgx.ErrNoRows
	}
	replay := domain.Delivery{
		ID:         uuid.New(),
		EventID:    original.EventID,
		EndpointID: original.EndpointID,
		Status:     domain.DeliveryPending,
		ReplayOf:   &originalID,
		CreatedAt:  time.Now().UTC(),
	}
	r.s.state.deliveries = append(r.s.state.deliveries, replay)
	return replay, nil
}

func (r Deliveries) ListByProject(_ context.Context, projectID uuid.UUID, filter application.DeliveryFilter, limit, offset int32) ([]domain.Delivery, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []domain.Delivery
	for i := len(r.s.state.deliveries) - 1; i >= 0; i-- {
		d := r.s.state.deliveries[i]
		if r.s.projectOfEndpoint(d.EndpointID) != projectID {
			continue
		}
		if (filter.Status == "" || d.Status == filter.Status) && (filter.EndpointID == nil || d.EndpointID == *filter.EndpointID) && (filter.Customer == nil || sameCustomer(r.s.customerOfEndpoint(d.EndpointID), filter.Customer)) {
			out = append(out, d)
		}
	}
	return page(out, limit, offset), nil
}

func (r Deliveries) ListAttempts(_ context.Context, deliveryID uuid.UUID) ([]domain.DeliveryAttempt, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	out := []domain.DeliveryAttempt{}
	for _, a := range r.s.state.attempts {
		if a.DeliveryID == deliveryID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (r Deliveries) StatsByProject(_ context.Context, projectID uuid.UUID) (domain.DeliveryStats, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var stats domain.DeliveryStats
	for _, d := range r.s.state.deliveries {
		if r.s.projectOfEndpoint(d.EndpointID) != projectID {
			continue
		}
		switch d.Status {
		case domain.DeliveryPending:
			stats.Pending++
		case domain.DeliveryProcessing:
			stats.Processing++
		case domain.DeliverySucceeded:
			stats.Succeeded++
		case domain.DeliveryDeadLetter:
			stats.DeadLetter++
		}
	}
	return stats, nil
}

// AddAttempt records an attempt, as the worker would through the queue.
func (s *Store) AddAttempt(attempt domain.DeliveryAttempt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt.ID = uuid.New()
	if attempt.AttemptedAt.IsZero() {
		attempt.AttemptedAt = time.Now().UTC()
	}
	s.state.attempts = append(s.state.attempts, attempt)
}

func (s *Store) customerOfEndpoint(endpointID uuid.UUID) *string {
	for _, e := range s.state.endpoints {
		if e.ID == endpointID {
			return e.Customer
		}
	}
	return nil
}

func (s *Store) projectOfEndpoint(endpointID uuid.UUID) uuid.UUID {
	for _, e := range s.state.endpoints {
		if e.ID == endpointID {
			return e.ProjectID
		}
	}
	return uuid.Nil
}

// SetDeliveryStatus lets tests move a delivery to a terminal state.
func (s *Store) SetDeliveryStatus(id uuid.UUID, status domain.DeliveryStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.deliveries {
		if s.state.deliveries[i].ID == id {
			s.state.deliveries[i].Status = status
		}
	}
}

func (s *Store) orgOfEvent(eventID uuid.UUID) uuid.UUID {
	for _, e := range s.state.events {
		if e.ID != eventID {
			continue
		}
		for _, p := range s.state.projects {
			if p.ID == e.ProjectID {
				return p.OrganizationID
			}
		}
	}
	return uuid.Nil
}

func page[T any](items []T, limit, offset int32) []T {
	if int(offset) >= len(items) {
		return []T{}
	}
	end := min(len(items), int(offset)+int(limit))
	return items[offset:end]
}

var ErrInjected = errors.New("injected failure")

var (
	_ application.ProjectRepository     = Projects{}
	_ application.APIKeyRepository      = Keys{}
	_ application.PortalTokenRepository = PortalTokens{}
	_ application.EndpointRepository    = Endpoints{}
	_ application.EventRepository       = Events{}
	_ application.DeliveryRepository    = Deliveries{}
	_ application.Transactor            = (*Store)(nil)
)

// sameCustomer mirrors "customer IS NOT DISTINCT FROM": both absent or equal.
func sameCustomer(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
