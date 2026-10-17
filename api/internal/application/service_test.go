package application

import (
	"context"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type memOrgs struct {
	items []domain.Organization
}

func (m *memOrgs) Create(_ context.Context, org domain.Organization) (domain.Organization, error) {
	org.ID = uuid.New()
	org.CreatedAt = time.Now().UTC()
	m.items = append(m.items, org)
	return org, nil
}

func (m *memOrgs) Get(_ context.Context, id uuid.UUID) (domain.Organization, error) {
	for _, item := range m.items {
		if item.ID == id {
			return item, nil
		}
	}
	return domain.Organization{}, pgx.ErrNoRows
}

func (m *memOrgs) Count(context.Context) (int64, error) {
	return int64(len(m.items)), nil
}

type memProjects struct {
	items []domain.Project
}

func (m *memProjects) Create(_ context.Context, project domain.Project) (domain.Project, error) {
	project.ID = uuid.New()
	project.CreatedAt = time.Now().UTC()
	m.items = append(m.items, project)
	return project, nil
}

func (m *memProjects) Get(_ context.Context, id uuid.UUID) (domain.Project, error) {
	for _, item := range m.items {
		if item.ID == id {
			return item, nil
		}
	}
	return domain.Project{}, pgx.ErrNoRows
}

func (m *memProjects) ListByOrganization(_ context.Context, organizationID uuid.UUID, _, _ int32) ([]domain.Project, error) {
	var out []domain.Project
	for _, item := range m.items {
		if item.OrganizationID == organizationID {
			out = append(out, item)
		}
	}
	return out, nil
}

type memKeys struct {
	items []domain.APIKey
}

func (m *memKeys) Create(_ context.Context, key domain.APIKey) (domain.APIKey, error) {
	key.ID = uuid.New()
	key.CreatedAt = time.Now().UTC()
	m.items = append(m.items, key)
	return key, nil
}

func (m *memKeys) GetByHash(_ context.Context, hash []byte) (domain.APIKey, error) {
	for _, item := range m.items {
		if EqualHash(item.Hash, hash) {
			return item, nil
		}
	}
	return domain.APIKey{}, pgx.ErrNoRows
}

func (m *memKeys) Get(_ context.Context, id uuid.UUID) (domain.APIKey, error) {
	for _, item := range m.items {
		if item.ID == id {
			return item, nil
		}
	}
	return domain.APIKey{}, pgx.ErrNoRows
}

func (m *memKeys) ListByProject(_ context.Context, projectID uuid.UUID) ([]domain.APIKey, error) {
	var out []domain.APIKey
	for _, item := range m.items {
		if item.ProjectID == projectID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (m *memKeys) Revoke(_ context.Context, id uuid.UUID) (domain.APIKey, error) {
	now := time.Now().UTC()
	for i, item := range m.items {
		if item.ID == id {
			if item.RevokedAt != nil {
				return domain.APIKey{}, pgx.ErrNoRows
			}
			m.items[i].RevokedAt = &now
			return m.items[i], nil
		}
	}
	return domain.APIKey{}, pgx.ErrNoRows
}

func (m *memKeys) TouchLastUsed(context.Context, uuid.UUID) error {
	return nil
}

type nopTx struct{}

func (nopTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func TestBootstrapAndAuth(t *testing.T) {
	orgs := &memOrgs{}
	projects := &memProjects{}
	keys := &memKeys{}
	svc := NewBootstrapService(orgs, projects, keys, nopTx{})

	result, err := svc.Bootstrap(context.Background(), BootstrapInput{
		OrganizationName: "Acme",
		OrganizationSlug: "acme",
		ProjectName:      "default",
		KeyName:          "root",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RawAPIKey == "" {
		t.Fatal("raw api key must be returned once")
	}

	if _, err := svc.Bootstrap(context.Background(), BootstrapInput{
		OrganizationName: "Other",
		OrganizationSlug: "other",
		ProjectName:      "default",
	}); err != ErrAlreadyBootstrapped {
		t.Fatalf("second bootstrap = %v", err)
	}

	auth := NewAuthenticator(keys)
	principal, err := auth.Authenticate(context.Background(), result.RawAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if principal.OrganizationID != result.Organization.ID {
		t.Fatal("principal org mismatch")
	}

	if _, err := auth.Authenticate(context.Background(), "hlk_invalid"); err != ErrUnauthorized {
		t.Fatalf("invalid key = %v", err)
	}
}

func TestCrossTenantProjectAccess(t *testing.T) {
	projects := &memProjects{}
	keys := &memKeys{}
	svc := NewProjectService(projects, keys)

	orgA := uuid.New()
	orgB := uuid.New()
	projectB, err := projects.Create(context.Background(), domain.Project{
		OrganizationID: orgB,
		Name:           "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Get(context.Background(), Principal{OrganizationID: orgA}, projectB.ID)
	if err != ErrForbidden {
		t.Fatalf("cross-tenant get = %v", err)
	}
}

func TestRevokedKeyRejected(t *testing.T) {
	keys := &memKeys{}
	raw, prefix, hash, err := GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	key, err := keys.Create(context.Background(), domain.APIKey{
		ProjectID:      uuid.New(),
		OrganizationID: uuid.New(),
		Name:           "ci",
		Prefix:         prefix,
		Hash:           hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Revoke(context.Background(), key.ID); err != nil {
		t.Fatal(err)
	}

	auth := NewAuthenticator(keys)
	if _, err := auth.Authenticate(context.Background(), raw); err != ErrRevoked {
		t.Fatalf("revoked key = %v", err)
	}
}

func TestHashAPIKeyIsDeterministicAndHidden(t *testing.T) {
	raw, _, hash, err := GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if string(hash) == raw {
		t.Fatal("hash must not equal raw key")
	}
	if !EqualHash(hash, HashAPIKey(raw)) {
		t.Fatal("hash mismatch")
	}
}
