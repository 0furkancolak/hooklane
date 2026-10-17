package database

import (
	"context"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database/db"
	"github.com/google/uuid"
)

func (s *Store) CreateOrganization(ctx context.Context, org domain.Organization) (domain.Organization, error) {
	row, err := s.q(ctx).CreateOrganization(ctx, db.CreateOrganizationParams{
		Name: org.Name,
		Slug: org.Slug,
	})
	if err != nil {
		return domain.Organization{}, err
	}
	return domain.Organization{
		ID:        pgToUUID(row.ID),
		Name:      row.Name,
		Slug:      row.Slug,
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

func (s *Store) GetOrganization(ctx context.Context, id uuid.UUID) (domain.Organization, error) {
	row, err := s.q(ctx).GetOrganization(ctx, uuidToPg(id))
	if err != nil {
		return domain.Organization{}, err
	}
	return domain.Organization{
		ID:        pgToUUID(row.ID),
		Name:      row.Name,
		Slug:      row.Slug,
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

func (s *Store) CountOrganizations(ctx context.Context) (int64, error) {
	return s.q(ctx).CountOrganizations(ctx)
}
