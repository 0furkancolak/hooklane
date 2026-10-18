package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	sessionTokenPrefix    = "hls_"
	sessionTokenHexLength = 48

	// SessionTTL is how long a sign-in lasts.
	SessionTTL = 7 * 24 * time.Hour

	minPasswordLength = 10
	maxPasswordBytes  = 128
	maxEmailLength    = 254
)

// Account is what the dashboard shows about the signed-in user.
type Account struct {
	User          domain.User
	Organization  domain.Organization
	Organizations []domain.Organization
}

// SignedIn is returned by sign-up and sign-in. Token is shown once.
type SignedIn struct {
	Token     string
	ExpiresAt time.Time
	Account   Account
}

// AccountService registers users and signs them in. A session is a bearer
// token that acts as an admin of the organization it is working in.
type AccountService struct {
	users        AccountRepository
	sessions     SessionRepository
	orgs         OrganizationRepository
	tx           Transactor
	signupOpen   bool
	now          func() time.Time
	slugSuffixes func() string
}

// NewAccountService builds the service. When signupOpen is false only the very
// first account can be created, so a fresh install can be claimed but a
// running one cannot be joined by strangers.
func NewAccountService(users AccountRepository, sessions SessionRepository, orgs OrganizationRepository, tx Transactor, signupOpen bool) *AccountService {
	return &AccountService{
		users: users, sessions: sessions, orgs: orgs, tx: tx,
		signupOpen: signupOpen, now: time.Now, slugSuffixes: randomSuffix,
	}
}

// SetClock replaces the clock; tests use it to expire sessions.
func (s *AccountService) SetClock(now func() time.Time) { s.now = now }

// SignUp creates a user and an organization they own, and signs them in.
func (s *AccountService) SignUp(ctx context.Context, email, password, organizationName string) (SignedIn, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return SignedIn{}, err
	}
	if err := checkPassword(password); err != nil {
		return SignedIn{}, err
	}
	organizationName = strings.TrimSpace(organizationName)
	if organizationName == "" {
		organizationName = strings.SplitN(email, "@", 2)[0]
	}
	if len(organizationName) > maxNameLength {
		return SignedIn{}, ErrInvalidInput
	}
	if !s.signupOpen {
		count, err := s.users.CountUsers(ctx)
		if err != nil {
			return SignedIn{}, err
		}
		if count > 0 {
			return SignedIn{}, ErrSignupDisabled
		}
	}
	hash, err := HashPassword(password)
	if err != nil {
		return SignedIn{}, err
	}

	var user domain.User
	var org domain.Organization
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if user, err = s.users.CreateUser(ctx, email, hash); err != nil {
			return err
		}
		org, err = s.createOrganization(ctx, user.ID, organizationName)
		return err
	})
	if err != nil {
		return SignedIn{}, err
	}
	return s.startSession(ctx, user, org)
}

// SignIn checks the password and starts a session in the user's first
// organization. The same error covers an unknown email and a wrong password.
func (s *AccountService) SignIn(ctx context.Context, email, password string) (SignedIn, error) {
	email, err := normalizeEmail(email)
	if err != nil || len(password) > maxPasswordBytes {
		return SignedIn{}, ErrInvalidCredentials
	}
	user, err := s.users.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			VerifyPassword(password, dummyHash) // spend the same time as a real check
			return SignedIn{}, ErrInvalidCredentials
		}
		return SignedIn{}, err
	}
	if !VerifyPassword(password, user.PasswordHash) {
		return SignedIn{}, ErrInvalidCredentials
	}
	orgs, err := s.users.ListOrganizations(ctx, user.ID)
	if err != nil {
		return SignedIn{}, err
	}
	if len(orgs) == 0 {
		return SignedIn{}, ErrInvalidCredentials
	}
	return s.startSession(ctx, user, orgs[0])
}

// Me describes the signed-in user and their organizations.
func (s *AccountService) Me(ctx context.Context, principal Principal) (Account, error) {
	if !principal.IsSession() {
		return Account{}, ErrForbidden
	}
	user, err := s.users.GetUser(ctx, principal.UserID)
	if err != nil {
		return Account{}, mapNoRows(err)
	}
	return s.account(ctx, user, principal.OrganizationID)
}

// CreateOrganization adds an organization owned by the user and moves the
// session into it.
func (s *AccountService) CreateOrganization(ctx context.Context, principal Principal, name string) (Account, error) {
	if !principal.IsSession() {
		return Account{}, ErrForbidden
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxNameLength {
		return Account{}, ErrInvalidInput
	}
	var org domain.Organization
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if org, err = s.createOrganization(ctx, principal.UserID, name); err != nil {
			return err
		}
		return s.sessions.SetOrganization(ctx, principal.SessionID, org.ID)
	})
	if err != nil {
		return Account{}, err
	}
	return s.Me(ctx, Principal{UserID: principal.UserID, SessionID: principal.SessionID, OrganizationID: org.ID})
}

// SwitchOrganization moves the session into another organization the user
// belongs to.
func (s *AccountService) SwitchOrganization(ctx context.Context, principal Principal, organizationID uuid.UUID) (Account, error) {
	if !principal.IsSession() {
		return Account{}, ErrForbidden
	}
	member, err := s.users.IsMember(ctx, principal.UserID, organizationID)
	if err != nil {
		return Account{}, err
	}
	if !member {
		return Account{}, ErrNotFound
	}
	if err := s.sessions.SetOrganization(ctx, principal.SessionID, organizationID); err != nil {
		return Account{}, err
	}
	return s.Me(ctx, Principal{UserID: principal.UserID, SessionID: principal.SessionID, OrganizationID: organizationID})
}

// SignOut ends the session.
func (s *AccountService) SignOut(ctx context.Context, principal Principal) error {
	if !principal.IsSession() {
		return ErrForbidden
	}
	return s.sessions.Delete(ctx, principal.SessionID)
}

// DeleteExpiredSessions removes sessions that can no longer be used.
func (s *AccountService) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	return s.sessions.DeleteExpired(ctx)
}

func (s *AccountService) createOrganization(ctx context.Context, userID uuid.UUID, name string) (domain.Organization, error) {
	org, err := s.orgs.Create(ctx, domain.Organization{Name: name, Slug: slugify(name) + "-" + s.slugSuffixes()})
	if err != nil {
		return domain.Organization{}, err
	}
	if err := s.users.AddMembership(ctx, userID, org.ID); err != nil {
		return domain.Organization{}, err
	}
	return org, nil
}

func (s *AccountService) startSession(ctx context.Context, user domain.User, org domain.Organization) (SignedIn, error) {
	raw, hash, err := generateSessionToken()
	if err != nil {
		return SignedIn{}, err
	}
	expires := s.now().UTC().Add(SessionTTL)
	if _, err := s.sessions.Create(ctx, domain.Session{
		UserID: user.ID, OrganizationID: org.ID, Hash: hash, ExpiresAt: expires,
	}); err != nil {
		return SignedIn{}, err
	}
	account, err := s.account(ctx, user, org.ID)
	if err != nil {
		return SignedIn{}, err
	}
	return SignedIn{Token: raw, ExpiresAt: expires, Account: account}, nil
}

func (s *AccountService) account(ctx context.Context, user domain.User, current uuid.UUID) (Account, error) {
	orgs, err := s.users.ListOrganizations(ctx, user.ID)
	if err != nil {
		return Account{}, err
	}
	account := Account{User: user, Organizations: orgs}
	for _, o := range orgs {
		if o.ID == current {
			account.Organization = o
		}
	}
	if account.Organization.ID == uuid.Nil {
		return Account{}, ErrForbidden
	}
	user.PasswordHash = ""
	account.User = user
	return account, nil
}

// AuthenticateSession resolves a session token to a principal that is an
// admin of the session's organization.
func (a *Authenticator) AuthenticateSession(ctx context.Context, raw string) (Principal, error) {
	raw = strings.TrimSpace(raw)
	if a.sessions == nil || !wellFormedToken(raw, sessionTokenPrefix, sessionTokenHexLength) {
		return Principal{}, ErrUnauthorized
	}
	session, err := a.sessions.GetByHash(ctx, hashToken(raw))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, ErrUnauthorized
		}
		return Principal{}, err
	}
	if !session.ExpiresAt.After(a.now()) {
		return Principal{}, ErrUnauthorized
	}
	return Principal{
		UserID:         session.UserID,
		SessionID:      session.ID,
		OrganizationID: session.OrganizationID,
		Scopes:         []domain.Scope{domain.ScopeAdmin},
	}, nil
}

func generateSessionToken() (raw string, hash []byte, err error) {
	secret := make([]byte, sessionTokenHexLength/2)
	if _, err := rand.Read(secret); err != nil {
		return "", nil, err
	}
	raw = sessionTokenPrefix + hex.EncodeToString(secret)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func wellFormedToken(raw, prefix string, hexLength int) bool {
	if len(raw) != len(prefix)+hexLength || !strings.HasPrefix(raw, prefix) {
		return false
	}
	for i := len(prefix); i < len(raw); i++ {
		if c := raw[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func normalizeEmail(raw string) (string, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if len(raw) < 3 || len(raw) > maxEmailLength {
		return "", ErrInvalidInput
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil || addr.Address != raw || !strings.Contains(raw, "@") {
		return "", ErrInvalidInput
	}
	return raw, nil
}

func checkPassword(password string) error {
	if len(password) > maxPasswordBytes {
		return ErrInvalidInput
	}
	runes := 0
	for range password {
		runes++
	}
	if runes < minPasswordLength {
		return ErrInvalidInput
	}
	return nil
}

func slugify(name string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		case !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	if slug == "" {
		slug = "org"
	}
	return slug
}

func randomSuffix() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func mapNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
