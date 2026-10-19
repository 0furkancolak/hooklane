package memstore

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// accountState holds users, organizations and sessions. It is not part of the
// transaction snapshot: tests of those flows do not depend on rollback.
type accountState struct {
	mu          sync.Mutex
	users       []domain.User
	orgs        []domain.Organization
	memberships map[uuid.UUID][]uuid.UUID
	sessions    []domain.Session
}

func (s *Store) accounts() *accountState {
	s.accountsOnce.Do(func() { s.acct = &accountState{memberships: map[uuid.UUID][]uuid.UUID{}} })
	return s.acct
}

type Orgs struct{ s *Store }
type Accounts struct{ s *Store }
type Sessions struct{ s *Store }

func (s *Store) Orgs() Orgs         { return Orgs{s} }
func (s *Store) Accounts() Accounts { return Accounts{s} }
func (s *Store) Sessions() Sessions { return Sessions{s} }

func (o Orgs) Create(_ context.Context, org domain.Organization) (domain.Organization, error) {
	a := o.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	org.ID = uuid.New()
	org.CreatedAt = time.Now()
	a.orgs = append(a.orgs, org)
	return org, nil
}

func (o Orgs) Get(_ context.Context, id uuid.UUID) (domain.Organization, error) {
	a := o.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, org := range a.orgs {
		if org.ID == id {
			return org, nil
		}
	}
	return domain.Organization{}, pgx.ErrNoRows
}

func (o Orgs) Count(context.Context) (int64, error) {
	a := o.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	return int64(len(a.orgs)), nil
}

func (c Accounts) CreateUser(_ context.Context, email, hash string) (domain.User, error) {
	a := c.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, u := range a.users {
		if u.Email == email {
			return domain.User{}, application.ErrEmailTaken
		}
	}
	u := domain.User{ID: uuid.New(), Email: email, PasswordHash: hash, CreatedAt: time.Now()}
	a.users = append(a.users, u)
	return u, nil
}

func (c Accounts) GetUserByEmail(_ context.Context, email string) (domain.User, error) {
	a := c.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, u := range a.users {
		if u.Email == email {
			return u, nil
		}
	}
	return domain.User{}, pgx.ErrNoRows
}

func (c Accounts) GetUser(_ context.Context, id uuid.UUID) (domain.User, error) {
	a := c.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, u := range a.users {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.User{}, pgx.ErrNoRows
}

func (c Accounts) CountUsers(context.Context) (int64, error) {
	a := c.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	return int64(len(a.users)), nil
}

func (c Accounts) AddMembership(_ context.Context, userID, organizationID uuid.UUID) error {
	a := c.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.memberships[userID] = append(a.memberships[userID], organizationID)
	return nil
}

func (c Accounts) ListOrganizations(_ context.Context, userID uuid.UUID) ([]domain.Organization, error) {
	a := c.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []domain.Organization
	for _, id := range a.memberships[userID] {
		for _, org := range a.orgs {
			if org.ID == id {
				out = append(out, org)
			}
		}
	}
	return out, nil
}

func (c Accounts) IsMember(_ context.Context, userID, organizationID uuid.UUID) (bool, error) {
	a := c.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, id := range a.memberships[userID] {
		if id == organizationID {
			return true, nil
		}
	}
	return false, nil
}

func (r Sessions) Create(_ context.Context, session domain.Session) (domain.Session, error) {
	a := r.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	session.ID = uuid.New()
	session.CreatedAt = time.Now()
	a.sessions = append(a.sessions, session)
	return session, nil
}

func (r Sessions) GetByHash(_ context.Context, hash []byte) (domain.Session, error) {
	a := r.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range a.sessions {
		if bytes.Equal(s.Hash, hash) {
			return s, nil
		}
	}
	return domain.Session{}, pgx.ErrNoRows
}

func (r Sessions) SetOrganization(_ context.Context, id, organizationID uuid.UUID) error {
	a := r.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.sessions {
		if a.sessions[i].ID == id {
			a.sessions[i].OrganizationID = organizationID
		}
	}
	return nil
}

func (r Sessions) Delete(_ context.Context, id uuid.UUID) error {
	a := r.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.sessions {
		if a.sessions[i].ID == id {
			a.sessions = append(a.sessions[:i], a.sessions[i+1:]...)
			return nil
		}
	}
	return nil
}

func (r Sessions) DeleteExpired(context.Context) (int64, error) {
	a := r.s.accounts()
	a.mu.Lock()
	defer a.mu.Unlock()
	var kept []domain.Session
	for _, s := range a.sessions {
		if s.ExpiresAt.After(time.Now()) {
			kept = append(kept, s)
		}
	}
	n := int64(len(a.sessions) - len(kept))
	a.sessions = kept
	return n, nil
}

var (
	_ application.OrganizationRepository = Orgs{}
	_ application.AccountRepository      = Accounts{}
	_ application.SessionRepository      = Sessions{}
)
