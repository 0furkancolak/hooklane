package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	ID             uuid.UUID
	ProjectID      uuid.UUID
	OrganizationID uuid.UUID
	EventType      string
	Payload        json.RawMessage
	IdempotencyKey *string
	CreatedAt      time.Time
}
