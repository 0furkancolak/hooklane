package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	portalTokenPrefix    = "hlp_"
	portalTokenHexLength = 48

	// DefaultPortalLinkTTL and MaxPortalLinkTTL bound how long a portal link works.
	DefaultPortalLinkTTL = 24 * time.Hour
	MaxPortalLinkTTL     = 7 * 24 * time.Hour
	minPortalLinkTTL     = time.Minute
)

// PortalLink is a freshly created portal credential. Token is shown once.
type PortalLink struct {
	Token     string
	Customer  string
	ExpiresAt time.Time
}

// PortalService issues portal links. Links are for customers of the
// integrator: they open a small page where the customer registers and
// inspects their own webhook endpoints.
type PortalService struct {
	projects ProjectRepository
	tokens   PortalTokenRepository
	now      func() time.Time
}

func NewPortalService(projects ProjectRepository, tokens PortalTokenRepository) *PortalService {
	return &PortalService{projects: projects, tokens: tokens, now: time.Now}
}

// CreateLink issues a link for customer in the project. A zero ttl means the
// default; anything outside the allowed range is rejected.
func (s *PortalService) CreateLink(ctx context.Context, principal Principal, projectID uuid.UUID, customer string, ttl time.Duration) (PortalLink, error) {
	if principal.IsPortal() {
		return PortalLink{}, ErrForbidden
	}
	customer = strings.TrimSpace(customer)
	if !customerPattern.MatchString(customer) {
		return PortalLink{}, ErrInvalidInput
	}
	if ttl == 0 {
		ttl = DefaultPortalLinkTTL
	}
	if ttl < minPortalLinkTTL || ttl > MaxPortalLinkTTL {
		return PortalLink{}, ErrInvalidInput
	}
	project, err := ownedProject(ctx, s.projects, principal, projectID)
	if err != nil {
		return PortalLink{}, err
	}
	raw, hash, err := generatePortalToken()
	if err != nil {
		return PortalLink{}, err
	}
	expires := s.now().UTC().Add(ttl)
	if _, err := s.tokens.Create(ctx, domain.PortalToken{
		ProjectID:      project.ID,
		OrganizationID: project.OrganizationID,
		Customer:       customer,
		Hash:           hash,
		ExpiresAt:      expires,
	}); err != nil {
		return PortalLink{}, err
	}
	return PortalLink{Token: raw, Customer: customer, ExpiresAt: expires}, nil
}

// AuthenticatePortal resolves a portal token to a principal limited to one
// customer of one project. Expired and unknown tokens look the same.
func (a *Authenticator) AuthenticatePortal(ctx context.Context, rawToken string) (Principal, domain.PortalToken, error) {
	rawToken = strings.TrimSpace(rawToken)
	if a.portal == nil || !wellFormedPortalToken(rawToken) {
		return Principal{}, domain.PortalToken{}, ErrUnauthorized
	}
	token, err := a.portal.GetByHash(ctx, hashPortalToken(rawToken))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, domain.PortalToken{}, ErrUnauthorized
		}
		return Principal{}, domain.PortalToken{}, err
	}
	if !a.now().Before(token.ExpiresAt) {
		return Principal{}, domain.PortalToken{}, ErrUnauthorized
	}
	customer := token.Customer
	return Principal{
		ProjectID:      token.ProjectID,
		OrganizationID: token.OrganizationID,
		Customer:       &customer,
	}, token, nil
}

// SetClock replaces the clock used to check expiry; tests use it.
func (a *Authenticator) SetClock(now func() time.Time) { a.now = now }

// WithPortalTokens enables portal authentication.
func (a *Authenticator) WithPortalTokens(tokens PortalTokenRepository) *Authenticator {
	a.portal = tokens
	return a
}

func generatePortalToken() (raw string, hash []byte, err error) {
	secret := make([]byte, portalTokenHexLength/2)
	if _, err := rand.Read(secret); err != nil {
		return "", nil, fmt.Errorf("generate portal token: %w", err)
	}
	raw = portalTokenPrefix + hex.EncodeToString(secret)
	return raw, hashPortalToken(raw), nil
}

func hashPortalToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func wellFormedPortalToken(raw string) bool {
	if len(raw) != len(portalTokenPrefix)+portalTokenHexLength || !strings.HasPrefix(raw, portalTokenPrefix) {
		return false
	}
	for i := len(portalTokenPrefix); i < len(raw); i++ {
		if c := raw[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
