package http

import (
	"encoding/json"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

const (
	headerIdempotencyKey     = "Idempotency-Key"
	headerIdempotentReplayed = "Idempotent-Replayed"
)

type eventHandler struct {
	events *application.EventService
}

type ingestEventRequest struct {
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	Customer  *string         `json:"customer"`
}

type eventResponse struct {
	ID             uuid.UUID       `json:"id"`
	ProjectID      uuid.UUID       `json:"project_id"`
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey *string         `json:"idempotency_key"`
	Customer       *string         `json:"customer"`
	CreatedAt      string          `json:"created_at"`
}

type ingestEventResponse struct {
	eventResponse
	DeliveryCount int64 `json:"delivery_count"`
}

func (h eventHandler) ingest(c fiber.Ctx) error {
	projectID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	var req ingestEventRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	result, err := h.events.Ingest(c.Context(), principalFrom(c), projectID, application.IngestInput{
		EventType:      req.EventType,
		Payload:        req.Payload,
		Customer:       req.Customer,
		IdempotencyKey: c.Get(headerIdempotencyKey),
	})
	if err != nil {
		return writeError(c, err)
	}
	status := fiber.StatusAccepted
	if result.Replayed {
		status = fiber.StatusOK
		c.Set(headerIdempotentReplayed, "true")
	}
	return c.Status(status).JSON(ingestEventResponse{
		eventResponse: mapEvent(result.Event),
		DeliveryCount: result.DeliveryCount,
	})
}

func (h eventHandler) list(c fiber.Ctx) error {
	projectID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	filter := application.EventFilter{EventType: c.Query("event_type"), Customer: optionalString(c.Query("customer"))}
	page, err := h.events.List(c.Context(), principalFrom(c), projectID, filter, pageParams(c))
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(pageBody("events", page, mapEvent))
}

func (h eventHandler) get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	event, err := h.events.Get(c.Context(), principalFrom(c), id)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(mapEvent(event))
}

func mapEvent(event domain.Event) eventResponse {
	return eventResponse{
		ID:             event.ID,
		ProjectID:      event.ProjectID,
		EventType:      event.EventType,
		Payload:        event.Payload,
		IdempotencyKey: event.IdempotencyKey,
		Customer:       event.Customer,
		CreatedAt:      formatTime(event.CreatedAt),
	}
}
