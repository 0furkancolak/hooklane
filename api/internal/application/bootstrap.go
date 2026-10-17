package application

import (
	"context"
	"strings"

	"github.com/0furkancolak/hooklane/api/internal/domain"
)

type BootstrapInput struct {
	OrganizationName string
	OrganizationSlug string
	ProjectName      string
	KeyName          string
}

type BootstrapResult struct {
	Organization domain.Organization
	Project      domain.Project
	APIKey       domain.APIKey
	RawAPIKey    string
}

type BootstrapService struct {
	orgs     OrganizationRepository
	projects ProjectRepository
	keys     APIKeyRepository
	tx       Transactor
}

func NewBootstrapService(
	orgs OrganizationRepository,
	projects ProjectRepository,
	keys APIKeyRepository,
	tx Transactor,
) *BootstrapService {
	return &BootstrapService{orgs: orgs, projects: projects, keys: keys, tx: tx}
}

func (s *BootstrapService) Bootstrap(ctx context.Context, in BootstrapInput) (BootstrapResult, error) {
	in.OrganizationName = strings.TrimSpace(in.OrganizationName)
	in.OrganizationSlug = strings.TrimSpace(in.OrganizationSlug)
	in.ProjectName = strings.TrimSpace(in.ProjectName)
	in.KeyName = strings.TrimSpace(in.KeyName)
	if in.KeyName == "" {
		in.KeyName = "bootstrap"
	}
	if in.OrganizationName == "" || in.OrganizationSlug == "" || in.ProjectName == "" {
		return BootstrapResult{}, ErrInvalidInput
	}

	count, err := s.orgs.Count(ctx)
	if err != nil {
		return BootstrapResult{}, err
	}
	if count > 0 {
		return BootstrapResult{}, ErrAlreadyBootstrapped
	}

	raw, prefix, hash, err := GenerateAPIKey()
	if err != nil {
		return BootstrapResult{}, err
	}

	var result BootstrapResult
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		org, err := s.orgs.Create(ctx, domain.Organization{
			Name: in.OrganizationName,
			Slug: in.OrganizationSlug,
		})
		if err != nil {
			return err
		}
		project, err := s.projects.Create(ctx, domain.Project{
			OrganizationID: org.ID,
			Name:           in.ProjectName,
		})
		if err != nil {
			return err
		}
		key, err := s.keys.Create(ctx, domain.APIKey{
			ProjectID:      project.ID,
			OrganizationID: org.ID,
			Name:           in.KeyName,
			Prefix:         prefix,
			Hash:           hash,
		})
		if err != nil {
			return err
		}
		result = BootstrapResult{
			Organization: org,
			Project:      project,
			APIKey:       key,
			RawAPIKey:    raw,
		}
		return nil
	})
	if err != nil {
		return BootstrapResult{}, err
	}
	return result, nil
}
