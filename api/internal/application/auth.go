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
	Scopes         []domain.Scope
}

// Can reports whether the key's scopes permit an action needing required.
func (p Principal) Can(required domain.Scope) bool {
	return domain.Allows(p.Scopes, required)
}

type Authenticator struct {
	keys APIKeyRepository
}

func NewAuthenticator(keys APIKeyRepository) *Authenticator {
	return &Authenticator{keys: keys}
}

func (a *Authenticator) Authenticate(ctx context.Context, rawKey string) (Principal, error) {
	rawKey = strings.TrimSpace(rawKey)
	if !wellFormedAPIKey(rawKey) {
		// Reject garbage before it costs a database lookup.
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
		Scopes:         key.Scopes,
	}, nil
}

func wellFormedAPIKey(raw string) bool {
	if len(raw) != len(apiKeyPrefix)+apiKeySecretHexLength || !strings.HasPrefix(raw, apiKeyPrefix) {
		return false
	}
	for i := len(apiKeyPrefix); i < len(raw); i++ {
		if c := raw[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func OwnsProject(principal Principal, project domain.Project) bool {
	return principal.OrganizationID == project.OrganizationID
}
