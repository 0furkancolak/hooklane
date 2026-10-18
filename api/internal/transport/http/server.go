package http

import (
	"context"
	"log/slog"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/gofiber/fiber/v3"
)

// maxRequestBodyBytes bounds every request body. Event payloads have a
// tighter limit enforced by the application layer.
const maxRequestBodyBytes = 1 << 20

type Services struct {
	Auth       *application.Authenticator
	Projects   *application.ProjectService
	Endpoints  *application.EndpointService
	Events     *application.EventService
	Deliveries *application.DeliveryService
	// DevKeys is only used when APP_ENV=development.
	DevKeys *application.DevelopmentKeyService
}

type Server struct {
	app  *fiber.App
	port string
}

func NewServer(cfg config.Config, svc Services, opts ...Option) *Server {
	o := options{logger: slog.Default()}
	for _, opt := range opts {
		opt(&o)
	}
	app := fiber.New(fiber.Config{
		BodyLimit:    maxRequestBodyBytes,
		ErrorHandler: frameworkErrorHandler,
		// Bound slow clients so they cannot hold connections open.
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	})
	app.Use(requestContext(o))
	app.Use(securityHeaders)
	// healthz: the process is up. readyz: it can serve traffic (database
	// reachable), so load balancers stop routing to it while it cannot.
	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	app.Get("/readyz", func(c fiber.Ctx) error {
		if o.ready != nil {
			ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
			defer cancel()
			if err := o.ready(ctx); err != nil {
				loggerFrom(c).Warn("readiness check failed", "error", err)
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "unavailable"})
			}
		}
		return c.JSON(fiber.Map{"status": "ready"})
	})

	app.Get("/openapi.yaml", serveOpenAPI)

	projects := projectHandler{projects: svc.Projects}
	endpoints := endpointHandler{endpoints: svc.Endpoints}
	events := eventHandler{events: svc.Events}
	deliveries := deliveryHandler{deliveries: svc.Deliveries}

	// Authentication is attached to each route rather than the group so the
	// matched route pattern is known even when a request is rejected; unknown
	// paths fall through to a plain 404.
	auth := authMiddleware(svc.Auth)
	read, ingest, admin := requireScope(domain.ScopeRead), requireScope(domain.ScopeIngest), requireScope(domain.ScopeAdmin)
	v1 := app.Group("/v1")
	if limit := cfg.RateLimit; limit.RequestsPerSecond > 0 {
		v1.Use(rateLimit(newRateLimiter(limit.RequestsPerSecond, limit.Burst, time.Now)))
	}
	v1.Post("/projects", auth, admin, projects.create)
	v1.Get("/projects", auth, read, projects.list)
	v1.Get("/projects/:id", auth, read, projects.get)
	v1.Post("/projects/:id/api-keys", auth, admin, projects.createKey)
	v1.Get("/projects/:id/api-keys", auth, read, projects.listKeys)
	v1.Post("/api-keys/:id/revoke", auth, admin, projects.revokeKey)

	v1.Post("/projects/:id/endpoints", auth, admin, endpoints.create)
	v1.Get("/projects/:id/endpoints", auth, read, endpoints.list)
	v1.Get("/endpoints/:id", auth, read, endpoints.get)
	v1.Patch("/endpoints/:id", auth, admin, endpoints.update)
	v1.Get("/endpoints/:id/secret", auth, admin, endpoints.secret)

	v1.Post("/projects/:id/events", auth, ingest, events.ingest)
	v1.Get("/projects/:id/events", auth, read, events.list)
	v1.Get("/events/:id", auth, read, events.get)
	v1.Get("/events/:id/deliveries", auth, read, deliveries.listForEvent)
	v1.Get("/projects/:id/deliveries", auth, read, deliveries.listForProject)
	v1.Get("/projects/:id/delivery-stats", auth, read, deliveries.stats)
	v1.Get("/deliveries/:id", auth, read, deliveries.get)
	v1.Get("/deliveries/:id/attempts", auth, read, deliveries.listAttempts)
	v1.Post("/deliveries/:id/replay", auth, admin, deliveries.replay)

	// Development only, and deliberately missing from openapi.yaml.
	if cfg.AppEnv == config.AppEnvDevelopment && svc.DevKeys != nil {
		dev := devKeyHandler{keys: svc.DevKeys}
		v1.Post("/dev/api-keys", dev.issue)

		sink := newDevSink("http://127.0.0.1:" + cfg.Port + "/v1/dev/sink")
		v1.Get("/dev", sink.status)
		v1.Post("/dev/sink/:token", sink.receive)
		v1.Get("/dev/sink/:token", sink.list)
	}

	if o.dashboardDir != "" {
		mountDashboard(app, o.dashboardDir)
	}

	return &Server{app: app, port: cfg.Port}
}

// securityHeaders marks every response as non-sniffable JSON and keeps
// tenant data (keys, secrets, payloads) out of shared and browser caches.
func securityHeaders(c fiber.Ctx) error {
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Cache-Control", "no-store")
	return c.Next()
}

func (s *Server) Listen() error {
	return s.app.Listen(":" + s.port)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.app.ShutdownWithContext(ctx)
}

func (s *Server) App() *fiber.App {
	return s.app
}
