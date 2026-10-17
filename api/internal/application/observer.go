package application

import (
	"time"

	"github.com/0furkancolak/hooklane/api/internal/domain"
)

// WorkerObserver receives worker events for metrics. Implementations must be
// safe for concurrent use and must not block.
type WorkerObserver interface {
	// Claimed reports how many deliveries one claim returned.
	Claimed(n int)
	ClaimFailed()
	// InFlight changes the number of deliveries being sent by delta.
	InFlight(delta int)
	// AttemptRecorded reports a stored attempt: the delivery's new status,
	// the HTTP status code (0 for transport failures) and the request time.
	AttemptRecorded(status domain.DeliveryStatus, statusCode int, duration time.Duration)
	LeaseLost()
	RecordFailed()
}

type nopObserver struct{}

func (nopObserver) Claimed(int)                                               {}
func (nopObserver) ClaimFailed()                                              {}
func (nopObserver) InFlight(int)                                              {}
func (nopObserver) AttemptRecorded(domain.DeliveryStatus, int, time.Duration) {}
func (nopObserver) LeaseLost()                                                {}
func (nopObserver) RecordFailed()                                             {}
