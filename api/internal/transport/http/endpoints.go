package http

import (
	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type endpointHandler struct {
	endpoints *application.EndpointService
}

type createEndpointRequest struct {
	URL      string  `json:"url"`
	Enabled  *bool   `json:"enabled"`
	Customer *string `json:"customer"`
}

type updateEndpointRequest struct {
	URL     *string `json:"url"`
	Enabled *bool   `json:"enabled"`
}

type endpointResponse struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	URL       string    `json:"url"`
	Enabled   bool      `json:"enabled"`
	Customer  *string   `json:"customer"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
	Secret    string    `json:"secret,omitempty"`
}

func (h endpointHandler) create(c fiber.Ctx) error {
	projectID, err := projectParam(c)
	if err != nil {
		return writeError(c, err)
	}
	var req createEndpointRequest
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	endpoint, secret, err := h.endpoints.Create(c.Context(), principalFrom(c), projectID, application.CreateEndpointInput{
		URL:      req.URL,
		Enabled:  req.Enabled,
		Customer: req.Customer,
	})
	if err != nil {
		return writeError(c, err)
	}
	resp := mapEndpoint(endpoint)
	resp.Secret = secret
	return c.Status(fiber.StatusCreated).JSON(resp)
}

func (h endpointHandler) list(c fiber.Ctx) error {
	projectID, err := projectParam(c)
	if err != nil {
		return writeError(c, err)
	}
	enabled, err := boolQuery(c, "enabled")
	if err != nil {
		return writeError(c, err)
	}
	page, err := h.endpoints.List(c.Context(), principalFrom(c), projectID, application.EndpointFilter{Enabled: enabled, Customer: optionalString(c.Query("customer"))}, pageParams(c))
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(pageBody("endpoints", page, mapEndpoint))
}

func (h endpointHandler) get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	endpoint, err := h.endpoints.Get(c.Context(), principalFrom(c), id)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(mapEndpoint(endpoint))
}

func (h endpointHandler) update(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	var req updateEndpointRequest
	if err := c.Bind().JSON(&req); err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	endpoint, err := h.endpoints.Update(c.Context(), principalFrom(c), id, domain.EndpointUpdate{
		URL:     req.URL,
		Enabled: req.Enabled,
	})
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(mapEndpoint(endpoint))
}

func (h endpointHandler) secret(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	secret, err := h.endpoints.Secret(c.Context(), principalFrom(c), id)
	if err != nil {
		return writeError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"secret": secret})
}

func mapEndpoint(endpoint domain.Endpoint) endpointResponse {
	return endpointResponse{
		ID:        endpoint.ID,
		ProjectID: endpoint.ProjectID,
		URL:       endpoint.URL,
		Enabled:   endpoint.Enabled,
		Customer:  endpoint.Customer,
		CreatedAt: formatTime(endpoint.CreatedAt),
		UpdatedAt: formatTime(endpoint.UpdatedAt),
	}
}
