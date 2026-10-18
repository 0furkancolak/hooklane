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
	// Customer is the integrator's identifier of who the event is for.
	Customer  *string
	CreatedAt time.Time
}
