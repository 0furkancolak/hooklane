package domain

import (
	"time"

	"github.com/google/uuid"
)

type APIKey struct {
	ID             uuid.UUID
	ProjectID      uuid.UUID
	OrganizationID uuid.UUID
	Name           string
	Prefix         string
	Hash           []byte
	Scopes         []Scope
	LastUsedAt     *time.Time
	RevokedAt      *time.Time
	CreatedAt      time.Time
}

func (k APIKey) IsActive() bool {
	return k.RevokedAt == nil
}
