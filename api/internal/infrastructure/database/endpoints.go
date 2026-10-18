package database

import (
	"context"

	"github.com/0furkancolak/hooklane/api/internal/application"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) CreateEndpoint(ctx context.Context, endpoint domain.Endpoint) (domain.Endpoint, error) {
	row, err := s.q(ctx).CreateEndpoint(ctx, db.CreateEndpointParams{
		ProjectID:       uuidToPg(endpoint.ProjectID),
		Url:             endpoint.URL,
		SecretEncrypted: endpoint.SecretEncrypted,
		Enabled:         endpoint.Enabled,
	})
	if err != nil {
		return domain.Endpoint{}, err
	}
	mapped := mapEndpoint(row)
	mapped.OrganizationID = endpoint.OrganizationID
	return mapped, nil
}

func (s *Store) GetEndpoint(ctx context.Context, id uuid.UUID) (domain.Endpoint, error) {
	row, err := s.q(ctx).GetEndpoint(ctx, uuidToPg(id))
	if err != nil {
		return domain.Endpoint{}, err
	}
	return domain.Endpoint{
		ID:              pgToUUID(row.ID),
		ProjectID:       pgToUUID(row.ProjectID),
		OrganizationID:  pgToUUID(row.OrganizationID),
		URL:             row.Url,
		SecretEncrypted: row.SecretEncrypted,
		Enabled:         row.Enabled,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}, nil
}

func (s *Store) ListEndpointsByProject(ctx context.Context, projectID uuid.UUID, filter application.EndpointFilter, limit, offset int32) ([]domain.Endpoint, error) {
	enabled := pgtype.Bool{}
	if filter.Enabled != nil {
		enabled = pgtype.Bool{Bool: *filter.Enabled, Valid: true}
	}
	rows, err := s.q(ctx).ListEndpointsByProject(ctx, db.ListEndpointsByProjectParams{
		ProjectID: uuidToPg(projectID),
		Enabled:   enabled,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	endpoints := make([]domain.Endpoint, 0, len(rows))
	for _, row := range rows {
		endpoints = append(endpoints, mapEndpoint(row))
	}
	return endpoints, nil
}

func (s *Store) UpdateEndpoint(ctx context.Context, id uuid.UUID, update domain.EndpointUpdate) (domain.Endpoint, error) {
	params := db.UpdateEndpointParams{ID: uuidToPg(id)}
	if update.URL != nil {
		params.Url = pgtype.Text{String: *update.URL, Valid: true}
	}
	if update.Enabled != nil {
		params.Enabled = pgtype.Bool{Bool: *update.Enabled, Valid: true}
	}
	row, err := s.q(ctx).UpdateEndpoint(ctx, params)
	if err != nil {
		return domain.Endpoint{}, err
	}
	return mapEndpoint(row), nil
}

func mapEndpoint(row db.Endpoint) domain.Endpoint {
	return domain.Endpoint{
		ID:              pgToUUID(row.ID),
		ProjectID:       pgToUUID(row.ProjectID),
		URL:             row.Url,
		SecretEncrypted: row.SecretEncrypted,
		Enabled:         row.Enabled,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}
