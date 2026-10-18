package application

import (
	"context"
	"errors"
	"strings"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ProjectService struct {
	projects ProjectRepository
	keys     APIKeyRepository
}

func NewProjectService(projects ProjectRepository, keys APIKeyRepository) *ProjectService {
	return &ProjectService{projects: projects, keys: keys}
}

// maxNameLength bounds project and API key names.
const maxNameLength = 200

func (s *ProjectService) Create(ctx context.Context, principal Principal, name string) (domain.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxNameLength {
		return domain.Project{}, ErrInvalidInput
	}
	return s.projects.Create(ctx, domain.Project{
		OrganizationID: principal.OrganizationID,
		Name:           name,
	})
}

func (s *ProjectService) Get(ctx context.Context, principal Principal, id uuid.UUID) (domain.Project, error) {
	return ownedProject(ctx, s.projects, principal, id)
}

func (s *ProjectService) List(ctx context.Context, principal Principal, page PageRequest) (Page[domain.Project], error) {
	return paginate(page, func(limit, offset int32) ([]domain.Project, error) {
		return s.projects.ListByOrganization(ctx, principal.OrganizationID, limit, offset)
	})
}

// ownedProject loads a project and verifies the principal's organization owns it.
func ownedProject(ctx context.Context, projects ProjectRepository, principal Principal, id uuid.UUID) (domain.Project, error) {
	project, err := projects.Get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Project{}, ErrNotFound
		}
		return domain.Project{}, err
	}
	if !OwnsProject(principal, project) || (principal.IsPortal() && principal.ProjectID != project.ID) {
		return domain.Project{}, ErrForbidden
	}
	return project, nil
}

func (s *ProjectService) CreateAPIKey(ctx context.Context, principal Principal, projectID uuid.UUID, name string, scopes []domain.Scope) (domain.APIKey, string, error) {
	name = strings.TrimSpace(name)
	scopes, err := normalizeScopes(scopes)
	if err != nil {
		return domain.APIKey{}, "", err
	}
	if name == "" || len(name) > maxNameLength {
		return domain.APIKey{}, "", ErrInvalidInput
	}
	project, err := s.Get(ctx, principal, projectID)
	if err != nil {
		return domain.APIKey{}, "", err
	}
	raw, prefix, hash, err := GenerateAPIKey()
	if err != nil {
		return domain.APIKey{}, "", err
	}
	key, err := s.keys.Create(ctx, domain.APIKey{
		ProjectID:      project.ID,
		OrganizationID: project.OrganizationID,
		Name:           name,
		Prefix:         prefix,
		Hash:           hash,
		Scopes:         scopes,
	})
	if err != nil {
		return domain.APIKey{}, "", err
	}
	return key, raw, nil
}

func (s *ProjectService) ListAPIKeys(ctx context.Context, principal Principal, projectID uuid.UUID) ([]domain.APIKey, error) {
	if _, err := s.Get(ctx, principal, projectID); err != nil {
		return nil, err
	}
	return s.keys.ListByProject(ctx, projectID)
}

func (s *ProjectService) RevokeAPIKey(ctx context.Context, principal Principal, keyID uuid.UUID) (domain.APIKey, error) {
	key, err := s.keys.Get(ctx, keyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.APIKey{}, ErrNotFound
		}
		return domain.APIKey{}, err
	}
	if key.OrganizationID != principal.OrganizationID {
		return domain.APIKey{}, ErrForbidden
	}
	revoked, err := s.keys.Revoke(ctx, keyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.APIKey{}, ErrAlreadyRevoked
		}
		return domain.APIKey{}, err
	}
	revoked.OrganizationID = key.OrganizationID
	return revoked, nil
}

// normalizeScopes defaults to admin, drops duplicates and rejects unknown scopes.
func normalizeScopes(scopes []domain.Scope) ([]domain.Scope, error) {
	if len(scopes) == 0 {
		return []domain.Scope{domain.ScopeAdmin}, nil
	}
	seen := map[domain.Scope]bool{}
	out := make([]domain.Scope, 0, len(scopes))
	for _, scope := range scopes {
		if !scope.Valid() {
			return nil, ErrInvalidInput
		}
		if !seen[scope] {
			seen[scope] = true
			out = append(out, scope)
		}
	}
	return out, nil
}
