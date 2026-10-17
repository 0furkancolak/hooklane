package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type DeliveryStatus string

const (
	DeliveryPending    DeliveryStatus = "pending"
	DeliveryProcessing DeliveryStatus = "processing"
	DeliverySucceeded  DeliveryStatus = "succeeded"
	DeliveryDeadLetter DeliveryStatus = "dead_letter"
)

type Delivery struct {
	ID                 uuid.UUID
	EventID            uuid.UUID
	EndpointID         uuid.UUID
	OrganizationID     uuid.UUID
	Status             DeliveryStatus
	AttemptCount       int32
	NextAttemptAt      time.Time
	LockedUntil        *time.Time
	LastResponseStatus *int32
	LastError          *string
	// ReplayOf is the delivery this one replays; nil for fan-out originals.
	ReplayOf  *uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

type DeliveryAttempt struct {
	ID            uuid.UUID
	DeliveryID    uuid.UUID
	AttemptNumber int32
	StatusCode    *int32
	ResponseBody  *string
	Error         *string
	DurationMS    *int64
	AttemptedAt   time.Time
}

// DeliveryStats counts a project's deliveries by status. OldestDueAt is the
// earliest next_attempt_at among pending deliveries that are already due; a
// growing gap to now means workers are not keeping up.
type DeliveryStats struct {
	Pending     int64
	Processing  int64
	Succeeded   int64
	DeadLetter  int64
	OldestDueAt *time.Time
}

func (s DeliveryStatus) Valid() bool {
	switch s {
	case DeliveryPending, DeliveryProcessing, DeliverySucceeded, DeliveryDeadLetter:
		return true
	}
	return false
}

func (s DeliveryStatus) IsTerminal() bool {
	switch s {
	case DeliverySucceeded, DeliveryDeadLetter:
		return true
	case DeliveryPending, DeliveryProcessing:
		return false
	default:
		panic(fmt.Sprintf("unhandled delivery status: %q", s))
	}
}
