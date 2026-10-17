package http

import (
	"context"
	"log/slog"
	"regexp"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

const headerRequestID = "X-Request-ID"

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// RequestObserver records per-request metrics.
type RequestObserver interface {
	ObserveRequest(method, route string, status int, duration time.Duration)
}

// ReadyFunc reports whether the API can serve traffic, typically by pinging
// the database.
type ReadyFunc func(ctx context.Context) error

type Option func(*options)

type options struct {
	logger       *slog.Logger
	observer     RequestObserver
	ready        ReadyFunc
	dashboardDir string
}

func WithLogger(logger *slog.Logger) Option { return func(o *options) { o.logger = logger } }

func WithRequestObserver(observer RequestObserver) Option {
	return func(o *options) { o.observer = observer }
}

func WithReadiness(ready ReadyFunc) Option { return func(o *options) { o.ready = ready } }

type loggerKeyType struct{}

var loggerKey loggerKeyType

// requestContext assigns a request ID, exposes a request-scoped logger and
// writes one access log line per request. It logs the route pattern and
// status, never headers, query strings or bodies.
func requestContext(opts options) fiber.Handler {
	return func(c fiber.Ctx) error {
		started := time.Now()
		id := c.Get(headerRequestID)
		if !requestIDPattern.MatchString(id) {
			id = uuid.NewString()
		}
		c.Set(headerRequestID, id)
		log := opts.logger.With("request_id", id)
		c.Locals(loggerKey, log)

		if err := c.Next(); err != nil {
			// Render here so the logged status matches the response.
			if herr := frameworkErrorHandler(c, err); herr != nil {
				return herr
			}
		}

		status := c.Response().StatusCode()
		route := c.Route().Path
		if status == fiber.StatusNotFound && route == "/" {
			route = "unmatched"
		}
		duration := time.Since(started)
		if opts.observer != nil {
			opts.observer.ObserveRequest(c.Method(), route, status, duration)
		}
		attrs := []any{
			"method", c.Method(),
			"route", route,
			"path", c.Path(),
			"status", status,
			"duration_ms", duration.Milliseconds(),
		}
		if principal := principalFrom(c); principal.APIKeyID != uuid.Nil {
			attrs = append(attrs, "api_key_id", principal.APIKeyID, "organization_id", principal.OrganizationID)
		}
		log.Info("request", attrs...)
		return nil
	}
}

func loggerFrom(c fiber.Ctx) *slog.Logger {
	if log, ok := c.Locals(loggerKey).(*slog.Logger); ok {
		return log
	}
	return slog.Default()
}
