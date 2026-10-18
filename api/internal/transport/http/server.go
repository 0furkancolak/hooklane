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
	// Portal issues customer portal links; nil disables portal routes.
	Portal *application.PortalService
	// Accounts enables sign-up and sign-in; nil disables /v1/auth.
	Accounts *application.AccountService
	// RateCounter is the PostgreSQL counter used when RATE_LIMIT_STORE=postgres.
	RateCounter SharedCounter
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
	fiberConfig := fiber.Config{
		BodyLimit:    maxRequestBodyBytes,
		ErrorHandler: frameworkErrorHandler,
		// Bound slow clients so they cannot hold connections open.
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	if len(cfg.TrustedProxies) > 0 {
		// Believe X-Forwarded-For only from these proxies, so clients cannot
		// pick their own address.
		fiberConfig.TrustProxy = true
		fiberConfig.ProxyHeader = fiber.HeaderXForwardedFor
		fiberConfig.TrustProxyConfig = fiber.TrustProxyConfig{Proxies: cfg.TrustedProxies}
	}
	app := fiber.New(fiberConfig)
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
	portal := portalHandler{portal: svc.Portal, projects: svc.Projects}

	// Authentication is attached to each route rather than the group so the
	// matched route pattern is known even when a request is rejected; unknown
	// paths fall through to a plain 404.
	auth := authMiddleware(svc.Auth)
	read, ingest, admin := requireScope(domain.ScopeRead), requireScope(domain.ScopeIngest), requireScope(domain.ScopeAdmin)
	v1 := app.Group("/v1")
	if limit := cfg.RateLimit; limit.RequestsPerSecond > 0 {
		var limiter allower = newRateLimiter(limit.RequestsPerSecond, limit.Burst, time.Now)
		if limit.Shared && svc.RateCounter != nil {
			limiter = sharedAllower{counter: svc.RateCounter, onError: func(err error) {
				o.logger.Warn("shared rate limit unavailable; allowing request", "error", err)
			}}
		}
		v1.Use(rateLimit(limiter))
	}

	if svc.Accounts != nil {
		accounts := accountHandler{accounts: svc.Accounts}
		// Passwords are guessable, so sign-up and sign-in get a much tighter
		// limit per client address than the rest of the API.
		credentials := rateLimit(newRateLimiter(0.2, 10, time.Now))
		v1.Post("/auth/signup", credentials, accounts.signUp)
		v1.Post("/auth/signin", credentials, accounts.signIn)
		v1.Get("/auth/me", auth, accounts.me)
		v1.Post("/auth/signout", auth, accounts.signOut)
		v1.Post("/auth/organizations", auth, accounts.createOrganization)
		v1.Post("/auth/switch-organization", auth, accounts.switchOrganization)
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

	v1.Post("/projects/:id/portal-links", auth, admin, portal.createLink)

	// Routes for customers holding a portal link. They are limited to one
	// customer's endpoints and deliveries inside one project.
	portalAuth := portalAuthMiddleware(svc.Auth)
	v1.Get("/portal/me", portalAuth, portal.me)
	v1.Get("/portal/endpoints", portalAuth, endpoints.list)
	v1.Post("/portal/endpoints", portalAuth, endpoints.create)
	v1.Get("/portal/endpoints/:id", portalAuth, endpoints.get)
	v1.Patch("/portal/endpoints/:id", portalAuth, endpoints.update)
	v1.Get("/portal/endpoints/:id/secret", portalAuth, endpoints.secret)
	v1.Get("/portal/deliveries", portalAuth, deliveries.listForProject)
	v1.Get("/portal/deliveries/:id/attempts", portalAuth, deliveries.listAttempts)
	v1.Post("/portal/deliveries/:id/replay", portalAuth, deliveries.replay)

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
	// The process logs its own "listening" line; Fiber's banner only adds noise.
	return s.app.Listen(":"+s.port, fiber.ListenConfig{DisableStartupMessage: true})
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.app.ShutdownWithContext(ctx)
}

func (s *Server) App() *fiber.App {
	return s.app
}
