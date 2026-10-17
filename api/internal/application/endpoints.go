package application

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const maxEndpointURLLength = 2048

type EndpointService struct {
	projects          ProjectRepository
	endpoints         EndpointRepository
	secrets           *SecretBox
	allowInsecureHTTP bool
}

// NewEndpointService builds the endpoint service. allowInsecureHTTP permits
// http:// destinations and must only be enabled in development.
func NewEndpointService(
	projects ProjectRepository,
	endpoints EndpointRepository,
	secrets *SecretBox,
	allowInsecureHTTP bool,
) *EndpointService {
	return &EndpointService{
		projects:          projects,
		endpoints:         endpoints,
		secrets:           secrets,
		allowInsecureHTTP: allowInsecureHTTP,
	}
}

type CreateEndpointInput struct {
	URL     string
	Enabled *bool
}

// Create registers an endpoint and returns it with its raw signing secret.
func (s *EndpointService) Create(ctx context.Context, principal Principal, projectID uuid.UUID, in CreateEndpointInput) (domain.Endpoint, string, error) {
	destination, err := s.validateURL(in.URL)
	if err != nil {
		return domain.Endpoint{}, "", err
	}
	project, err := ownedProject(ctx, s.projects, principal, projectID)
	if err != nil {
		return domain.Endpoint{}, "", err
	}
	secret, err := GenerateEndpointSecret()
	if err != nil {
		return domain.Endpoint{}, "", err
	}
	sealed, err := s.secrets.Seal(secret)
	if err != nil {
		return domain.Endpoint{}, "", err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	endpoint, err := s.endpoints.Create(ctx, domain.Endpoint{
		ProjectID:       project.ID,
		OrganizationID:  project.OrganizationID,
		URL:             destination,
		SecretEncrypted: sealed,
		Enabled:         enabled,
	})
	if err != nil {
		return domain.Endpoint{}, "", err
	}
	return endpoint, secret, nil
}

func (s *EndpointService) Get(ctx context.Context, principal Principal, id uuid.UUID) (domain.Endpoint, error) {
	endpoint, err := s.endpoints.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Endpoint{}, ErrNotFound
		}
		return domain.Endpoint{}, err
	}
	if endpoint.OrganizationID != principal.OrganizationID {
		return domain.Endpoint{}, ErrForbidden
	}
	return endpoint, nil
}

func (s *EndpointService) List(ctx context.Context, principal Principal, projectID uuid.UUID, limit, offset int32) ([]domain.Endpoint, error) {
	if _, err := ownedProject(ctx, s.projects, principal, projectID); err != nil {
		return nil, err
	}
	limit, offset = clampPage(limit, offset)
	return s.endpoints.ListByProject(ctx, projectID, limit, offset)
}

func (s *EndpointService) Update(ctx context.Context, principal Principal, id uuid.UUID, update domain.EndpointUpdate) (domain.Endpoint, error) {
	if update.URL == nil && update.Enabled == nil {
		return domain.Endpoint{}, ErrInvalidInput
	}
	if update.URL != nil {
		destination, err := s.validateURL(*update.URL)
		if err != nil {
			return domain.Endpoint{}, err
		}
		update.URL = &destination
	}
	current, err := s.Get(ctx, principal, id)
	if err != nil {
		return domain.Endpoint{}, err
	}
	updated, err := s.endpoints.Update(ctx, id, update)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Endpoint{}, ErrNotFound
		}
		return domain.Endpoint{}, err
	}
	updated.OrganizationID = current.OrganizationID
	return updated, nil
}

// Secret returns the decrypted signing secret so receivers can verify signatures.
func (s *EndpointService) Secret(ctx context.Context, principal Principal, id uuid.UUID) (string, error) {
	endpoint, err := s.Get(ctx, principal, id)
	if err != nil {
		return "", err
	}
	return s.secrets.Open(endpoint.SecretEncrypted)
}

// validateURL performs structural checks only. Destination address filtering
// (SSRF, DNS rebinding, redirects) happens at delivery time in Phase 6.
func (s *EndpointService) validateURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxEndpointURLLength {
		return "", ErrInvalidInput
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", ErrInvalidInput
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if !s.allowInsecureHTTP {
			return "", ErrInvalidInput
		}
	default:
		return "", ErrInvalidInput
	}
	return parsed.String(), nil
}
