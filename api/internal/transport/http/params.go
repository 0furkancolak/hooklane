package http

import (
	"strconv"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// pageParams reads limit and offset; the application layer clamps them.
func pageParams(c fiber.Ctx) application.PageRequest {
	limit, _ := strconv.ParseInt(c.Query("limit", "50"), 10, 32)
	offset, _ := strconv.ParseInt(c.Query("offset", "0"), 10, 32)
	return application.PageRequest{Limit: int32(limit), Offset: int32(offset)}
}

// pageBody renders a page as {key: [...], limit, offset, next_offset}.
func pageBody[T, R any](key string, page application.Page[T], mapItem func(T) R) fiber.Map {
	items := make([]R, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, mapItem(item))
	}
	return fiber.Map{key: items, "limit": page.Limit, "offset": page.Offset, "next_offset": page.NextOffset}
}

func boolQuery(c fiber.Ctx, name string) (*bool, error) {
	raw := c.Query(name)
	if raw == "" {
		return nil, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, application.ErrInvalidInput
	}
	return &v, nil
}

func uuidQuery(c fiber.Ctx, name string) (*uuid.UUID, error) {
	raw := c.Query(name)
	if raw == "" {
		return nil, nil
	}
	v, err := uuid.Parse(raw)
	if err != nil {
		return nil, application.ErrInvalidInput
	}
	return &v, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// projectParam returns the project a request is about: the route's :id, or for
// portal principals always their own project.
func projectParam(c fiber.Ctx) (uuid.UUID, error) {
	if principal := principalFrom(c); principal.IsPortal() {
		return principal.ProjectID, nil
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, application.ErrInvalidInput
	}
	return id, nil
}

// optionalString returns a pointer to the query value, or nil when absent.
func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
