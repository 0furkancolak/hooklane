package database

import (
	"context"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database/db"
	"github.com/google/uuid"
)

func (s *Store) CreateProject(ctx context.Context, project domain.Project) (domain.Project, error) {
	row, err := s.q(ctx).CreateProject(ctx, db.CreateProjectParams{
		OrganizationID: uuidToPg(project.OrganizationID),
		Name:           project.Name,
	})
	if err != nil {
		return domain.Project{}, err
	}
	return mapProject(row), nil
}

func (s *Store) GetProject(ctx context.Context, id uuid.UUID) (domain.Project, error) {
	row, err := s.q(ctx).GetProject(ctx, uuidToPg(id))
	if err != nil {
		return domain.Project{}, err
	}
	return mapProject(row), nil
}

func (s *Store) ListProjectsByOrganization(ctx context.Context, organizationID uuid.UUID, limit, offset int32) ([]domain.Project, error) {
	rows, err := s.q(ctx).ListProjectsByOrganization(ctx, db.ListProjectsByOrganizationParams{
		OrganizationID: uuidToPg(organizationID),
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		return nil, err
	}
	projects := make([]domain.Project, 0, len(rows))
	for _, row := range rows {
		projects = append(projects, mapProject(row))
	}
	return projects, nil
}

func mapProject(row db.Project) domain.Project {
	return domain.Project{
		ID:             pgToUUID(row.ID),
		OrganizationID: pgToUUID(row.OrganizationID),
		Name:           row.Name,
		CreatedAt:      row.CreatedAt.Time,
	}
}
