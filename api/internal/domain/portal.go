package domain

import (
	"time"

	"github.com/google/uuid"
)

// PortalToken is a link credential that lets one customer manage their own
// endpoints and see their deliveries. Only its hash is stored.
type PortalToken struct {
	ID             uuid.UUID
	ProjectID      uuid.UUID
	OrganizationID uuid.UUID
	Customer       string
	Hash           []byte
	ExpiresAt      time.Time
	CreatedAt      time.Time
}
