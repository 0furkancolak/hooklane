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

type attemptResponse struct {
	ID            uuid.UUID `json:"id"`
	AttemptNumber int32     `json:"attempt_number"`
	StatusCode    *int32    `json:"status_code"`
	ResponseBody  *string   `json:"response_body"`
	Error         *string   `json:"error"`
	DurationMS    *int64    `json:"duration_ms"`
	AttemptedAt   string    `json:"attempted_at"`
}

type deliveryStatsResponse struct {
	Pending     int64   `json:"pending"`
	Processing  int64   `json:"processing"`
	Succeeded   int64   `json:"succeeded"`
	DeadLetter  int64   `json:"dead_letter"`
	OldestDueAt *string `json:"oldest_due_at"`
}

func (h deliveryHandler) get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	delivery, err := h.deliveries.Get(c.Context(), principalFrom(c), id)
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(mapDelivery(delivery))
}

func (h deliveryHandler) listAttempts(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	attempts, err := h.deliveries.ListAttempts(c.Context(), principalFrom(c), id)
	if err != nil {
		return writeError(c, err)
	}
	out := make([]attemptResponse, 0, len(attempts))
	for _, a := range attempts {
		out = append(out, attemptResponse{
			ID:            a.ID,
			AttemptNumber: a.AttemptNumber,
			StatusCode:    a.StatusCode,
			ResponseBody:  a.ResponseBody,
			Error:         a.Error,
			DurationMS:    a.DurationMS,
			AttemptedAt:   formatTime(a.AttemptedAt),
		})
	}
	return c.JSON(fiber.Map{"attempts": out})
}

func (h deliveryHandler) listForProject(c fiber.Ctx) error {
	projectID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	endpointID, err := uuidQuery(c, "endpoint_id")
	if err != nil {
		return writeError(c, err)
	}
	filter := application.DeliveryFilter{Status: domain.DeliveryStatus(c.Query("status")), EndpointID: endpointID}
	page, err := h.deliveries.ListForProject(c.Context(), principalFrom(c), projectID, filter, pageParams(c))
	if err != nil {
		return writeError(c, err)
	}
	return c.JSON(pageBody("deliveries", page, mapDelivery))
}

func (h deliveryHandler) stats(c fiber.Ctx) error {
	projectID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return writeError(c, application.ErrInvalidInput)
	}
	stats, err := h.deliveries.Stats(c.Context(), principalFrom(c), projectID)
	if err != nil {
		return writeError(c, err)
	}
	resp := deliveryStatsResponse{
		Pending:    stats.Pending,
		Processing: stats.Processing,
		Succeeded:  stats.Succeeded,
		DeadLetter: stats.DeadLetter,
	}
	if stats.OldestDueAt != nil {
		at := formatTime(*stats.OldestDueAt)
		resp.OldestDueAt = &at
	}
	return c.JSON(resp)
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
