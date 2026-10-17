package application

import "errors"

var (
	ErrInvalidInput        = errors.New("invalid input")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden")
	ErrNotFound            = errors.New("not found")
	ErrAlreadyBootstrapped = errors.New("already bootstrapped")
	ErrRevoked             = errors.New("api key revoked")
	ErrAlreadyRevoked      = errors.New("api key already revoked")
	ErrPayloadTooLarge     = errors.New("payload too large")
	ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")
)
