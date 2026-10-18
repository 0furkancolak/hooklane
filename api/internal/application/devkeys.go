package application

import (
	"context"
	"errors"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/jackc/pgx/v5"
)

const devKeyName = "dashboard (development)"

// OldestProjectFinder returns the first project ever created, or
// pgx.ErrNoRows when there is none.
type OldestProjectFinder interface {
	OldestProject(ctx context.Context) (domain.Project, error)
}

// DevelopmentKeyService issues API keys without authentication so a local
// dashboard can be used without the admin CLI. It must only be exposed when
// APP_ENV=development.
type DevelopmentKeyService struct {
	bootstrap *BootstrapService
	projects  OldestProjectFinder
	keys      *ProjectService
}

func NewDevelopmentKeyService(bootstrap *BootstrapService, projects OldestProjectFinder, keys *ProjectService) *DevelopmentKeyService {
	return &DevelopmentKeyService{bootstrap: bootstrap, projects: projects, keys: keys}
}

// Issue creates a key for the oldest project. On an empty database it first
// bootstraps a "Development" organization with a "default" project.
func (s *DevelopmentKeyService) Issue(ctx context.Context) (domain.APIKey, string, error) {
	project, err := s.projects.OldestProject(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		result, bootErr := s.bootstrap.Bootstrap(ctx, BootstrapInput{
			OrganizationName: "Development",
			OrganizationSlug: "dev",
			ProjectName:      "default",
			KeyName:          devKeyName,
		})
		if !errors.Is(bootErr, ErrAlreadyBootstrapped) {
			return result.APIKey, result.RawAPIKey, bootErr
		}
		// Another request bootstrapped first; use its project.
		project, err = s.projects.OldestProject(ctx)
	}
	if err != nil {
		return domain.APIKey{}, "", err
	}
	principal := Principal{OrganizationID: project.OrganizationID}
	return s.keys.CreateAPIKey(ctx, principal, project.ID, devKeyName)
}
