package database

import (
	"context"
	"errors"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/0furkancolak/hooklane/api/internal/infrastructure/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type Accounts struct{ Store *Store }

func toUser(row db.User) domain.User {
	return domain.User{ID: pgToUUID(row.ID), Email: row.Email, PasswordHash: row.PasswordHash, CreatedAt: row.CreatedAt.Time}
}

func (a Accounts) CreateUser(ctx context.Context, email, passwordHash string) (domain.User, error) {
	row, err := a.Store.q(ctx).CreateUser(ctx, db.CreateUserParams{Email: email, PasswordHash: passwordHash})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.User{}, application.ErrEmailTaken
		}
		return domain.User{}, err
	}
	return toUser(row), nil
}

func (a Accounts) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	row, err := a.Store.q(ctx).GetUserByEmail(ctx, email)
	if err != nil {
		return domain.User{}, err
	}
	return toUser(row), nil
}

func (a Accounts) GetUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	row, err := a.Store.q(ctx).GetUser(ctx, uuidToPg(id))
	if err != nil {
		return domain.User{}, err
	}
	return toUser(row), nil
}

func (a Accounts) CountUsers(ctx context.Context) (int64, error) {
	return a.Store.q(ctx).CountUsers(ctx)
}

func (a Accounts) AddMembership(ctx context.Context, userID, organizationID uuid.UUID) error {
	return a.Store.q(ctx).CreateMembership(ctx, db.CreateMembershipParams{
		UserID: uuidToPg(userID), OrganizationID: uuidToPg(organizationID), Role: "owner",
	})
}

func (a Accounts) ListOrganizations(ctx context.Context, userID uuid.UUID) ([]domain.Organization, error) {
	rows, err := a.Store.q(ctx).ListMemberships(ctx, uuidToPg(userID))
	if err != nil {
		return nil, err
	}
	orgs := make([]domain.Organization, 0, len(rows))
	for _, r := range rows {
		orgs = append(orgs, domain.Organization{ID: pgToUUID(r.ID), Name: r.Name, Slug: r.Slug, CreatedAt: r.CreatedAt.Time})
	}
	return orgs, nil
}

func (a Accounts) IsMember(ctx context.Context, userID, organizationID uuid.UUID) (bool, error) {
	return a.Store.q(ctx).IsMember(ctx, db.IsMemberParams{UserID: uuidToPg(userID), OrganizationID: uuidToPg(organizationID)})
}

type Sessions struct{ Store *Store }

func toSession(row db.Session) domain.Session {
	return domain.Session{
		ID: pgToUUID(row.ID), UserID: pgToUUID(row.UserID), OrganizationID: pgToUUID(row.OrganizationID),
		Hash: row.TokenHash, ExpiresAt: row.ExpiresAt.Time, CreatedAt: row.CreatedAt.Time,
	}
}

func (s Sessions) Create(ctx context.Context, session domain.Session) (domain.Session, error) {
	row, err := s.Store.q(ctx).CreateSession(ctx, db.CreateSessionParams{
		UserID: uuidToPg(session.UserID), OrganizationID: uuidToPg(session.OrganizationID),
		TokenHash: session.Hash, ExpiresAt: pgtype.Timestamptz{Time: session.ExpiresAt, Valid: true},
	})
	if err != nil {
		return domain.Session{}, err
	}
	return toSession(row), nil
}

func (s Sessions) GetByHash(ctx context.Context, hash []byte) (domain.Session, error) {
	row, err := s.Store.q(ctx).GetSessionByHash(ctx, hash)
	if err != nil {
		return domain.Session{}, err
	}
	return toSession(row), nil
}

func (s Sessions) SetOrganization(ctx context.Context, id, organizationID uuid.UUID) error {
	return s.Store.q(ctx).UpdateSessionOrganization(ctx, db.UpdateSessionOrganizationParams{ID: uuidToPg(id), OrganizationID: uuidToPg(organizationID)})
}

func (s Sessions) Delete(ctx context.Context, id uuid.UUID) error {
	return s.Store.q(ctx).DeleteSession(ctx, uuidToPg(id))
}

func (s Sessions) DeleteExpired(ctx context.Context) (int64, error) {
	return s.Store.q(ctx).DeleteExpiredSessions(ctx)
}

// RateLimiter counts requests per key in PostgreSQL so every API replica
// shares one limit. Time is read from the database, so replicas need no
// synchronised clocks.
type RateLimiter struct {
	store  *Store
	window time.Duration
	limit  int32
}

// NewRateLimiter allows limit requests per key in each fixed window.
func (s *Store) NewRateLimiter(window time.Duration, limit int) *RateLimiter {
	return &RateLimiter{store: s, window: window, limit: int32(limit)}
}

// Allow counts one request. The error means the count is unknown, in which
// case callers should let the request through.
func (r *RateLimiter) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	row, err := r.store.q(ctx).HitRateLimit(ctx, db.HitRateLimitParams{WindowSeconds: int32(r.window.Seconds()), Key: key})
	if err != nil {
		return true, 0, err
	}
	if row.Hits <= r.limit {
		return true, 0, nil
	}
	return false, time.Duration(row.SecondsLeft * float64(time.Second)), nil
}

// Prune drops counters of finished windows.
func (r *RateLimiter) Prune(ctx context.Context) (int64, error) {
	return r.store.q(ctx).DeleteOldRateLimitWindows(ctx)
}

func (s *Store) Accounts() Accounts { return Accounts{Store: s} }
func (s *Store) Sessions() Sessions { return Sessions{Store: s} }

var (
	_ application.AccountRepository = Accounts{}
	_ application.SessionRepository = Sessions{}
)
