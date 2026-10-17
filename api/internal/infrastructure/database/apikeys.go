package database

import (
	"context"
	"errors"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) CreateAPIKey(ctx context.Context, key domain.APIKey) (domain.APIKey, error) {
	row, err := s.q(ctx).CreateAPIKey(ctx, db.CreateAPIKeyParams{
		ProjectID: uuidToPg(key.ProjectID),
		Name:      key.Name,
		KeyPrefix: key.Prefix,
		KeyHash:   key.Hash,
	})
	if err != nil {
		return domain.APIKey{}, err
	}
	mapped := mapCreatedAPIKey(row)
	mapped.OrganizationID = key.OrganizationID
	return mapped, nil
}

func (s *Store) GetAPIKey(ctx context.Context, id uuid.UUID) (domain.APIKey, error) {
	row, err := s.q(ctx).GetAPIKey(ctx, uuidToPg(id))
	if err != nil {
		return domain.APIKey{}, err
	}
	return mapAPIKeyRow(row.ID, row.ProjectID, row.OrganizationID, row.Name, row.KeyPrefix, row.KeyHash, row.LastUsedAt, row.RevokedAt, row.CreatedAt), nil
}

func (s *Store) GetByHash(ctx context.Context, hash []byte) (domain.APIKey, error) {
	row, err := s.q(ctx).GetAPIKeyByHash(ctx, hash)
	if err != nil {
		return domain.APIKey{}, err
	}
	return mapAPIKeyRow(row.ID, row.ProjectID, row.OrganizationID, row.Name, row.KeyPrefix, row.KeyHash, row.LastUsedAt, row.RevokedAt, row.CreatedAt), nil
}

func (s *Store) ListByProject(ctx context.Context, projectID uuid.UUID) ([]domain.APIKey, error) {
	rows, err := s.q(ctx).ListAPIKeysByProject(ctx, uuidToPg(projectID))
	if err != nil {
		return nil, err
	}
	keys := make([]domain.APIKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, mapAPIKeyRow(row.ID, row.ProjectID, row.OrganizationID, row.Name, row.KeyPrefix, row.KeyHash, row.LastUsedAt, row.RevokedAt, row.CreatedAt))
	}
	return keys, nil
}

func (s *Store) Revoke(ctx context.Context, id uuid.UUID) (domain.APIKey, error) {
	row, err := s.q(ctx).RevokeAPIKey(ctx, uuidToPg(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.APIKey{}, err
		}
		return domain.APIKey{}, err
	}
	return mapCreatedAPIKey(row), nil
}

func (s *Store) TouchLastUsed(ctx context.Context, id uuid.UUID) error {
	return s.q(ctx).TouchAPIKeyLastUsed(ctx, uuidToPg(id))
}

func mapCreatedAPIKey(row db.ApiKey) domain.APIKey {
	return domain.APIKey{
		ID:         pgToUUID(row.ID),
		ProjectID:  pgToUUID(row.ProjectID),
		Name:       row.Name,
		Prefix:     row.KeyPrefix,
		Hash:       row.KeyHash,
		LastUsedAt: pgTimePtr(row.LastUsedAt),
		RevokedAt:  pgTimePtr(row.RevokedAt),
		CreatedAt:  row.CreatedAt.Time,
	}
}

func mapAPIKeyRow(
	id, projectID, organizationID pgtype.UUID,
	name, prefix string,
	hash []byte,
	lastUsedAt, revokedAt, createdAt pgtype.Timestamptz,
) domain.APIKey {
	return domain.APIKey{
		ID:             pgToUUID(id),
		ProjectID:      pgToUUID(projectID),
		OrganizationID: pgToUUID(organizationID),
		Name:           name,
		Prefix:         prefix,
		Hash:           hash,
		LastUsedAt:     pgTimePtr(lastUsedAt),
		RevokedAt:      pgTimePtr(revokedAt),
		CreatedAt:      createdAt.Time,
	}
}
