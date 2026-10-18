package database

import (
	"context"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type PortalTokens struct{ Store *Store }

func (s PortalTokens) Create(ctx context.Context, token domain.PortalToken) (domain.PortalToken, error) {
	row, err := s.Store.q(ctx).CreatePortalToken(ctx, db.CreatePortalTokenParams{
		ProjectID: uuidToPg(token.ProjectID),
		Customer:  token.Customer,
		TokenHash: token.Hash,
		ExpiresAt: pgtype.Timestamptz{Time: token.ExpiresAt, Valid: true},
	})
	if err != nil {
		return domain.PortalToken{}, err
	}
	return domain.PortalToken{
		ID:             pgToUUID(row.ID),
		ProjectID:      pgToUUID(row.ProjectID),
		OrganizationID: token.OrganizationID,
		Customer:       row.Customer,
		Hash:           row.TokenHash,
		ExpiresAt:      row.ExpiresAt.Time,
		CreatedAt:      row.CreatedAt.Time,
	}, nil
}

func (s PortalTokens) GetByHash(ctx context.Context, hash []byte) (domain.PortalToken, error) {
	row, err := s.Store.q(ctx).GetPortalTokenByHash(ctx, hash)
	if err != nil {
		return domain.PortalToken{}, err
	}
	return domain.PortalToken{
		ID:             pgToUUID(row.ID),
		ProjectID:      pgToUUID(row.ProjectID),
		OrganizationID: pgToUUID(row.OrganizationID),
		Customer:       row.Customer,
		Hash:           row.TokenHash,
		ExpiresAt:      row.ExpiresAt.Time,
		CreatedAt:      row.CreatedAt.Time,
	}, nil
}
