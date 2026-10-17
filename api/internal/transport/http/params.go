package http

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
)

func pageParams(c fiber.Ctx) (int32, int32) {
	limit, _ := strconv.ParseInt(c.Query("limit", "50"), 10, 32)
	offset, _ := strconv.ParseInt(c.Query("offset", "0"), 10, 32)
	return int32(limit), int32(offset)
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
