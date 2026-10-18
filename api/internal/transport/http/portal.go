package http

import (
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type portalTokenKeyType struct{}

var portalTokenKey portalTokenKeyType

// portalAuthMiddleware authenticates "Bearer hlp_…" portal tokens. It does not
// accept API keys, and authMiddleware does not accept portal tokens, so the
// two credential types can never reach each other's routes.
func portalAuthMiddleware(auth *application.Authenticator) fiber.Handler {
	return func(c fiber.Ctx) error {
		token := bearerToken(c)
		principal, stored, err := auth.AuthenticatePortal(c.Context(), token)
		if err != nil {
			return writeError(c, err)
		}
		c.Locals(principalKey, principal)
		c.Locals(portalTokenKey, stored)
		return c.Next()
	}
}

type portalHandler struct {
	portal   *application.PortalService
	projects *application.ProjectService
}

type createPortalLinkRequest struct {
	Customer   string `json:"customer"`
	TTLSeconds int64  `json:"ttl_seconds"`
}

type portalLinkResponse struct {
	Token     string `json:"token"`
	Customer  string `json:"customer"`
	ExpiresAt string `json:"expires_at"`
	// Path is the dashboard route that opens the portal with this token.
	Path string `json:"path"`
}

func (h portalHandler) createLink(c fiber.Ctx) error {
	projectID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	var req createPortalLinkRequest
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	link, err := h.portal.CreateLink(c.Context(), principalFrom(c), projectID, req.Customer, time.Duration(req.TTLSeconds)*time.Second)
	if err != nil {
		return writeError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Status(fiber.StatusCreated).JSON(portalLinkResponse{
		Token:     link.Token,
		Customer:  link.Customer,
		ExpiresAt: formatTime(link.ExpiresAt),
		Path:      "/#/portal/" + link.Token,
	})
}

// me describes what the portal token grants, for the portal page header.
func (h portalHandler) me(c fiber.Ctx) error {
	principal := principalFrom(c)
	stored, _ := c.Locals(portalTokenKey).(domain.PortalToken)
	project, err := h.projects.Get(c.Context(), principal, principal.ProjectID)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(fiber.Map{
		"customer":     stored.Customer,
		"project_name": project.Name,
		"expires_at":   formatTime(stored.ExpiresAt),
	})
}
