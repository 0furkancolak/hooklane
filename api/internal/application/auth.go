package application

import (
	"context"
	"errors"
	"strings"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Principal struct {
	APIKeyID       uuid.UUID
	ProjectID      uuid.UUID
	OrganizationID uuid.UUID
}

type Authenticator struct {
	keys APIKeyRepository
}

func NewAuthenticator(keys APIKeyRepository) *Authenticator {
	return &Authenticator{keys: keys}
}

func (a *Authenticator) Authenticate(ctx context.Context, rawKey string) (Principal, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return Principal{}, ErrUnauthorized
	}

	key, err := a.keys.GetByHash(ctx, HashAPIKey(rawKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, ErrUnauthorized
		}
		return Principal{}, err
	}
	if !key.IsActive() {
		return Principal{}, ErrRevoked
	}

	_ = a.keys.TouchLastUsed(ctx, key.ID)

	return Principal{
		APIKeyID:       key.ID,
		ProjectID:      key.ProjectID,
		OrganizationID: key.OrganizationID,
	}, nil
}

func OwnsProject(principal Principal, project domain.Project) bool {
	return principal.OrganizationID == project.OrganizationID
}
