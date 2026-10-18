package domain

import (
	"time"

	"github.com/google/uuid"
)

// User is a person who signs in to the dashboard.
type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

// Session is a sign-in. It works inside one organization at a time; only the
// hash of its token is stored.
type Session struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	Hash           []byte
	ExpiresAt      time.Time
	CreatedAt      time.Time
}
