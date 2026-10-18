package http

import (
	"net"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/gofiber/fiber/v3"
)

// devKeyHandler lets the dashboard's sign-in screen issue an API key without
// one. Its routes are registered only when APP_ENV=development, and only
// requests from this machine are accepted, because anyone who can call them
// gets full access to the oldest project's organization.
type devKeyHandler struct {
	keys *application.DevelopmentKeyService
}

func (h devKeyHandler) issue(c fiber.Ctx) error {
	if ip := net.ParseIP(c.IP()); ip == nil || !ip.IsLoopback() {
		return writeError(c, application.ErrForbidden)
	}
	key, raw, err := h.keys.Issue(c.Context())
	if err != nil {
		return writeError(c, err)
	}
	resp := mapAPIKey(key)
	resp.RawKey = raw
	return c.Status(fiber.StatusCreated).JSON(resp)
}
