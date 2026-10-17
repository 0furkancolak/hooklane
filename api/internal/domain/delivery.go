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
	DeliveryFailed     DeliveryStatus = "failed"
	DeliveryDeadLetter DeliveryStatus = "dead_letter"
)

type Delivery struct {
	ID                 uuid.UUID
	EventID            uuid.UUID
	EndpointID         uuid.UUID
	Status             DeliveryStatus
	AttemptCount       int32
	NextAttemptAt      time.Time
	LockedUntil        *time.Time
	LastResponseStatus *int32
	LastError          *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
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

func (s DeliveryStatus) IsTerminal() bool {
	switch s {
	case DeliverySucceeded, DeliveryDeadLetter:
		return true
	case DeliveryPending, DeliveryProcessing, DeliveryFailed:
		return false
	default:
		panic(fmt.Sprintf("unhandled delivery status: %q", s))
	}
}
