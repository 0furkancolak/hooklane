package domain

import (
	"time"

	"github.com/google/uuid"
)

type Endpoint struct {
	ID              uuid.UUID
	ProjectID       uuid.UUID
	OrganizationID  uuid.UUID
	URL             string
	SecretEncrypted []byte
	Enabled         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type EndpointUpdate struct {
	URL     *string
	Enabled *bool
}
