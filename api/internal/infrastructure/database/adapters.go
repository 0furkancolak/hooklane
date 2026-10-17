package database

import (
	"context"
	"github.com/0furkancolak/hooklane/api/internal/application"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
)

type Organizations struct{ Store *Store }

func (s Organizations) Create(ctx context.Context, org domain.Organization) (domain.Organization, error) {
	return s.Store.CreateOrganization(ctx, org)
}

func (s Organizations) Get(ctx context.Context, id uuid.UUID) (domain.Organization, error) {
	return s.Store.GetOrganization(ctx, id)
}

func (s Organizations) Count(ctx context.Context) (int64, error) {
	return s.Store.CountOrganizations(ctx)
}

type Projects struct{ Store *Store }

func (s Projects) Create(ctx context.Context, project domain.Project) (domain.Project, error) {
	return s.Store.CreateProject(ctx, project)
}

func (s Projects) Get(ctx context.Context, id uuid.UUID) (domain.Project, error) {
	return s.Store.GetProject(ctx, id)
}

func (s Projects) ListByOrganization(ctx context.Context, organizationID uuid.UUID, limit, offset int32) ([]domain.Project, error) {
	return s.Store.ListProjectsByOrganization(ctx, organizationID, limit, offset)
}

type APIKeys struct{ Store *Store }

func (s APIKeys) Create(ctx context.Context, key domain.APIKey) (domain.APIKey, error) {
	return s.Store.CreateAPIKey(ctx, key)
}

func (s APIKeys) GetByHash(ctx context.Context, hash []byte) (domain.APIKey, error) {
	return s.Store.GetByHash(ctx, hash)
}

func (s APIKeys) Get(ctx context.Context, id uuid.UUID) (domain.APIKey, error) {
	return s.Store.GetAPIKey(ctx, id)
}

func (s APIKeys) ListByProject(ctx context.Context, projectID uuid.UUID) ([]domain.APIKey, error) {
	return s.Store.ListByProject(ctx, projectID)
}

func (s APIKeys) Revoke(ctx context.Context, id uuid.UUID) (domain.APIKey, error) {
	return s.Store.Revoke(ctx, id)
}

func (s APIKeys) TouchLastUsed(ctx context.Context, id uuid.UUID) error {
	return s.Store.TouchLastUsed(ctx, id)
}

type Endpoints struct{ Store *Store }

func (s Endpoints) Create(ctx context.Context, endpoint domain.Endpoint) (domain.Endpoint, error) {
	return s.Store.CreateEndpoint(ctx, endpoint)
}

func (s Endpoints) Get(ctx context.Context, id uuid.UUID) (domain.Endpoint, error) {
	return s.Store.GetEndpoint(ctx, id)
}

func (s Endpoints) ListByProject(ctx context.Context, projectID uuid.UUID, limit, offset int32) ([]domain.Endpoint, error) {
	return s.Store.ListEndpointsByProject(ctx, projectID, limit, offset)
}

func (s Endpoints) Update(ctx context.Context, id uuid.UUID, update domain.EndpointUpdate) (domain.Endpoint, error) {
	return s.Store.UpdateEndpoint(ctx, id, update)
}

type Events struct{ Store *Store }

func (s Events) InsertIdempotent(ctx context.Context, event domain.Event) (application.IdempotentInsert, error) {
	return s.Store.InsertEvent(ctx, event)
}

func (s Events) Get(ctx context.Context, id uuid.UUID) (domain.Event, error) {
	return s.Store.GetEvent(ctx, id)
}

func (s Events) ListByProject(ctx context.Context, projectID uuid.UUID, limit, offset int32) ([]domain.Event, error) {
	return s.Store.ListEventsByProject(ctx, projectID, limit, offset)
}

type Deliveries struct{ Store *Store }

func (s Deliveries) CreateForEvent(ctx context.Context, eventID, projectID uuid.UUID) (int64, error) {
	return s.Store.CreateDeliveriesForEvent(ctx, eventID, projectID)
}

func (s Deliveries) CountByEvent(ctx context.Context, eventID uuid.UUID) (int64, error) {
	return s.Store.CountDeliveriesByEvent(ctx, eventID)
}

func (s Deliveries) Get(ctx context.Context, id uuid.UUID) (domain.Delivery, error) {
	return s.Store.GetDelivery(ctx, id)
}

func (s Deliveries) ListByEvent(ctx context.Context, eventID uuid.UUID) ([]domain.Delivery, error) {
	return s.Store.ListDeliveriesByEvent(ctx, eventID)
}

func (s Deliveries) CreateReplay(ctx context.Context, originalID uuid.UUID) (domain.Delivery, error) {
	return s.Store.CreateReplayDelivery(ctx, originalID)
}

func (s *Store) Orgs() Organizations    { return Organizations{Store: s} }
func (s *Store) Projects() Projects     { return Projects{Store: s} }
func (s *Store) Keys() APIKeys          { return APIKeys{Store: s} }
func (s *Store) Endpoints() Endpoints   { return Endpoints{Store: s} }
func (s *Store) Events() Events         { return Events{Store: s} }
func (s *Store) Deliveries() Deliveries { return Deliveries{Store: s} }

var (
	_ application.OrganizationRepository = Organizations{}
	_ application.ProjectRepository      = Projects{}
	_ application.APIKeyRepository       = APIKeys{}
	_ application.EndpointRepository     = Endpoints{}
	_ application.EventRepository        = Events{}
	_ application.DeliveryRepository     = Deliveries{}
	_ application.Transactor             = (*Store)(nil)
)
