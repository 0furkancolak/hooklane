package http

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(c fiber.Ctx, err error) error {
	status, code, message := classifyError(err)
	if status >= http.StatusInternalServerError {
		slog.Error("request failed", "error", err)
		message = "internal error"
	}
	return c.Status(status).JSON(errorBody{Error: errorDetail{Code: code, Message: message}})
}

func classifyError(err error) (int, string, string) {
	switch {
	case errors.Is(err, application.ErrInvalidInput):
		return http.StatusBadRequest, "invalid_input", "invalid input"
	case errors.Is(err, application.ErrUnauthorized):
		return http.StatusUnauthorized, "unauthorized", "unauthorized"
	case errors.Is(err, application.ErrRevoked):
		return http.StatusUnauthorized, "revoked", "api key revoked"
	case errors.Is(err, application.ErrForbidden):
		return http.StatusForbidden, "forbidden", "forbidden"
	case errors.Is(err, application.ErrNotFound), errors.Is(err, pgx.ErrNoRows):
		return http.StatusNotFound, "not_found", "not found"
	case errors.Is(err, application.ErrAlreadyRevoked):
		return http.StatusConflict, "already_revoked", "api key already revoked"
	case errors.Is(err, application.ErrIdempotencyConflict):
		return http.StatusConflict, "idempotency_conflict", "idempotency key reused with a different request"
	case errors.Is(err, application.ErrReplayNotAllowed):
		return http.StatusConflict, "delivery_not_finished", "only succeeded or dead-lettered deliveries can be replayed"
	case errors.Is(err, application.ErrReplayInProgress):
		return http.StatusConflict, "replay_in_progress", "an earlier replay of this delivery has not finished"
	case errors.Is(err, application.ErrDestinationNotAllowed):
		return http.StatusBadRequest, "destination_not_allowed", "endpoint URL points to an address that is not allowed"
	case errors.Is(err, application.ErrPayloadTooLarge):
		return http.StatusRequestEntityTooLarge, "payload_too_large", "payload too large"
	default:
		return http.StatusInternalServerError, "internal_error", "internal error"
	}
}

// frameworkErrorHandler renders errors raised by Fiber itself (unknown routes,
// oversized bodies) in the same envelope as application errors.
func frameworkErrorHandler(c fiber.Ctx, err error) error {
	var fe *fiber.Error
	if !errors.As(err, &fe) {
		return writeError(c, err)
	}
	code := "http_error"
	switch fe.Code {
	case http.StatusNotFound:
		code = "not_found"
	case http.StatusMethodNotAllowed:
		code = "method_not_allowed"
	case http.StatusRequestEntityTooLarge:
		code = "payload_too_large"
	}
	return c.Status(fe.Code).JSON(errorBody{Error: errorDetail{Code: code, Message: strings.ToLower(fe.Message)}})
}
