package http

import (
	"context"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/config"
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
}

type Server struct {
	app  *fiber.App
	port string
}

func NewServer(cfg config.Config, svc Services) *Server {
	app := fiber.New(fiber.Config{
		BodyLimit:    maxRequestBodyBytes,
		ErrorHandler: frameworkErrorHandler,
		// Bound slow clients so they cannot hold connections open.
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	})
	app.Use(securityHeaders)
	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	app.Get("/readyz", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ready"})
	})

	projects := projectHandler{projects: svc.Projects}
	endpoints := endpointHandler{endpoints: svc.Endpoints}
	events := eventHandler{events: svc.Events}
	deliveries := deliveryHandler{deliveries: svc.Deliveries}

	v1 := app.Group("/v1", authMiddleware(svc.Auth))
	v1.Post("/projects", projects.create)
	v1.Get("/projects", projects.list)
	v1.Get("/projects/:id", projects.get)
	v1.Post("/projects/:id/api-keys", projects.createKey)
	v1.Get("/projects/:id/api-keys", projects.listKeys)
	v1.Post("/api-keys/:id/revoke", projects.revokeKey)

	v1.Post("/projects/:id/endpoints", endpoints.create)
	v1.Get("/projects/:id/endpoints", endpoints.list)
	v1.Get("/endpoints/:id", endpoints.get)
	v1.Patch("/endpoints/:id", endpoints.update)
	v1.Get("/endpoints/:id/secret", endpoints.secret)

	v1.Post("/projects/:id/events", events.ingest)
	v1.Get("/projects/:id/events", events.list)
	v1.Get("/events/:id", events.get)
	v1.Get("/events/:id/deliveries", deliveries.listForEvent)
	v1.Post("/deliveries/:id/replay", deliveries.replay)

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
