package http

import (
	"github.com/0furkancolak/hooklane/api/openapi"
	"github.com/gofiber/fiber/v3"
)

func serveOpenAPI(c fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, "application/yaml")
	return c.Send(openapi.Spec)
}
