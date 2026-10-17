package http

import (
	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type projectHandler struct {
	projects *application.ProjectService
}

type createProjectRequest struct {
	Name string `json:"name"`
}

type createAPIKeyRequest struct {
	Name string `json:"name"`
}

type projectResponse struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Name           string    `json:"name"`
	CreatedAt      string    `json:"created_at"`
}

type apiKeyResponse struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	Name      string    `json:"name"`
	Prefix    string    `json:"prefix"`
	Revoked   bool      `json:"revoked"`
	CreatedAt string    `json:"created_at"`
	RawKey    string    `json:"raw_key,omitempty"`
}

func (h projectHandler) create(c fiber.Ctx) error {
	var req createProjectRequest
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	project, err := h.projects.Create(c.Context(), principalFrom(c), req.Name)
	if err != nil {
		return writeError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(mapProject(project))
}

func (h projectHandler) list(c fiber.Ctx) error {
	limit, offset := pageParams(c)
	projects, err := h.projects.List(c.Context(), principalFrom(c), limit, offset)
	if err != nil {
		return writeError(c, err)
	}
	out := make([]projectResponse, 0, len(projects))
	for _, project := range projects {
		out = append(out, mapProject(project))
	}
	return c.JSON(fiber.Map{"projects": out})
}

func (h projectHandler) get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	project, err := h.projects.Get(c.Context(), principalFrom(c), id)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(mapProject(project))
}

func (h projectHandler) createKey(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	var req createAPIKeyRequest
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	key, raw, err := h.projects.CreateAPIKey(c.Context(), principalFrom(c), id, req.Name)
	if err != nil {
		return writeError(c, err)
	}
	resp := mapAPIKey(key)
	resp.RawKey = raw
	return c.Status(fiber.StatusCreated).JSON(resp)
}

func (h projectHandler) listKeys(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	keys, err := h.projects.ListAPIKeys(c.Context(), principalFrom(c), id)
	if err != nil {
		return writeError(c, err)
	}
	out := make([]apiKeyResponse, 0, len(keys))
	for _, key := range keys {
		out = append(out, mapAPIKey(key))
	}
	return c.JSON(fiber.Map{"api_keys": out})
}

func (h projectHandler) revokeKey(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	key, err := h.projects.RevokeAPIKey(c.Context(), principalFrom(c), id)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(mapAPIKey(key))
}

func mapProject(project domain.Project) projectResponse {
	return projectResponse{
		ID:             project.ID,
		OrganizationID: project.OrganizationID,
		Name:           project.Name,
		CreatedAt:      formatTime(project.CreatedAt),
	}
}

func mapAPIKey(key domain.APIKey) apiKeyResponse {
	return apiKeyResponse{
		ID:        key.ID,
		ProjectID: key.ProjectID,
		Name:      key.Name,
		Prefix:    key.Prefix,
		Revoked:   !key.IsActive(),
		CreatedAt: formatTime(key.CreatedAt),
	}
}
