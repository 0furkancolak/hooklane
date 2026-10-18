package application

import "errors"

var (
	ErrInvalidInput        = errors.New("invalid input")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrForbidden           = errors.New("forbidden")
	ErrLimitReached        = errors.New("limit reached")
	ErrInsufficientScope   = errors.New("this API key does not have the scope this action needs")
	ErrNotFound            = errors.New("not found")
	ErrAlreadyBootstrapped = errors.New("already bootstrapped")
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrEmailTaken          = errors.New("an account with this email already exists")
	ErrSignupDisabled      = errors.New("sign-up is disabled")
	ErrRevoked             = errors.New("api key revoked")
	ErrAlreadyRevoked      = errors.New("api key already revoked")
	ErrPayloadTooLarge     = errors.New("payload too large")
	ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")
	ErrReplayNotAllowed    = errors.New("only succeeded or dead-lettered deliveries can be replayed")
	ErrReplayInProgress    = errors.New("an earlier replay of this delivery has not finished")
)
