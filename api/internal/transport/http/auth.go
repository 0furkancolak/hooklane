package http

import (
	"strings"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/gofiber/fiber/v3"
)

type principalKeyType struct{}

var principalKey principalKeyType

func authMiddleware(auth *application.Authenticator) fiber.Handler {
	return func(c fiber.Ctx) error {
		scheme, token, found := strings.Cut(strings.TrimSpace(c.Get("Authorization")), " ")
		if !found || !strings.EqualFold(scheme, "Bearer") {
			token = ""
		}
		principal, err := auth.Authenticate(c.Context(), token)
		if err != nil {
			return writeError(c, err)
		}
		c.Locals(principalKey, principal)
		return c.Next()
	}
}

// requireScope rejects keys whose scopes do not permit the action. It must run
// after authMiddleware.
func requireScope(scope domain.Scope) fiber.Handler {
	return func(c fiber.Ctx) error {
		if !principalFrom(c).Can(scope) {
			return writeError(c, application.ErrInsufficientScope)
		}
		return c.Next()
	}
}

func principalFrom(c fiber.Ctx) application.Principal {
	principal, ok := c.Locals(principalKey).(application.Principal)
	if !ok {
		return application.Principal{}
	}
	return principal
}
