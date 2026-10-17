package http

import (
	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type deliveryHandler struct {
	deliveries *application.DeliveryService
}

type deliveryResponse struct {
	ID                 uuid.UUID  `json:"id"`
	EventID            uuid.UUID  `json:"event_id"`
	EndpointID         uuid.UUID  `json:"endpoint_id"`
	Status             string     `json:"status"`
	AttemptCount       int32      `json:"attempt_count"`
	NextAttemptAt      *string    `json:"next_attempt_at"`
	LastResponseStatus *int32     `json:"last_response_status"`
	LastError          *string    `json:"last_error"`
	ReplayOf           *uuid.UUID `json:"replay_of"`
	CreatedAt          string     `json:"created_at"`
	UpdatedAt          string     `json:"updated_at"`
}

func (h deliveryHandler) listForEvent(c fiber.Ctx) error {
	eventID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	deliveries, err := h.deliveries.ListForEvent(c.Context(), principalFrom(c), eventID)
	if err != nil {
		return writeError(c, err)
	}
	out := make([]deliveryResponse, 0, len(deliveries))
	for _, d := range deliveries {
		out = append(out, mapDelivery(d))
	}
	return c.JSON(fiber.Map{"deliveries": out})
}

func (h deliveryHandler) replay(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	replay, err := h.deliveries.Replay(c.Context(), principalFrom(c), id)
	if err != nil {
		return writeError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(mapDelivery(replay))
}

func mapDelivery(d domain.Delivery) deliveryResponse {
	resp := deliveryResponse{
		ID:                 d.ID,
		EventID:            d.EventID,
		EndpointID:         d.EndpointID,
		Status:             string(d.Status),
		AttemptCount:       d.AttemptCount,
		LastResponseStatus: d.LastResponseStatus,
		LastError:          d.LastError,
		ReplayOf:           d.ReplayOf,
		CreatedAt:          formatTime(d.CreatedAt),
		UpdatedAt:          formatTime(d.UpdatedAt),
	}
	// next_attempt_at only means something while the delivery is queued.
	if d.Status == domain.DeliveryPending {
		next := formatTime(d.NextAttemptAt)
		resp.NextAttemptAt = &next
	}
	return resp
}
