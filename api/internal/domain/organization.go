package domain

import (
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID        uuid.UUID
	Name      string
	Slug      string
	CreatedAt time.Time
}
