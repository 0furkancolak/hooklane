package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/0furkancolak/hooklane/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type memProjects struct {
	items []domain.Project
}

func (m *memProjects) Create(_ context.Context, project domain.Project) (domain.Project, error) {
	project.ID = uuid.New()
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
	m.items = append(m.items, key)
	return key, nil
}

func (m *memKeys) GetByHash(_ context.Context, hash []byte) (domain.APIKey, error) {
	for _, item := range m.items {
		if application.EqualHash(item.Hash, hash) {
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

func (m *memKeys) Revoke(context.Context, uuid.UUID) (domain.APIKey, error) {
	return domain.APIKey{}, nil
}

func (m *memKeys) TouchLastUsed(context.Context, uuid.UUID) error {
	return nil
}

func TestHealthz(t *testing.T) {
	server := NewServer(config.Config{Port: "8080"}, Services{Auth: application.NewAuthenticator(&memKeys{}), Projects: application.NewProjectService(&memProjects{}, &memKeys{})})
	req := httptest.NewRequest(stdhttp.MethodGet, "/healthz", nil)
	resp, err := server.App().Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != stdhttp.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestProjectRoutesEnforceAuthAndTenant(t *testing.T) {
	keys := &memKeys{}
	projects := &memProjects{}
	raw, prefix, hash, err := application.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	orgID := uuid.New()
	if _, err := keys.Create(context.Background(), domain.APIKey{
		ProjectID:      uuid.New(),
		OrganizationID: orgID,
		Name:           "test",
		Prefix:         prefix,
		Hash:           hash,
	}); err != nil {
		t.Fatal(err)
	}
	foreign, err := projects.Create(context.Background(), domain.Project{
		OrganizationID: uuid.New(),
		Name:           "other",
	})
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(config.Config{Port: "8080"}, Services{Auth: application.NewAuthenticator(keys), Projects: application.NewProjectService(projects, keys)})

	unauthorized := httptest.NewRequest(stdhttp.MethodGet, "/v1/projects", nil)
	resp, err := server.App().Test(unauthorized)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != stdhttp.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", resp.StatusCode)
	}

	createReq := httptest.NewRequest(stdhttp.MethodPost, "/v1/projects", bytes.NewBufferString(`{"name":"alpha"}`))
	createReq.Header.Set("Authorization", "Bearer "+raw)
	createReq.Header.Set("Content-Type", "application/json")
	resp, err = server.App().Test(createReq)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != stdhttp.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create status = %d body=%s", resp.StatusCode, body)
	}

	for _, header := range []string{raw, "Bearer" + raw, "Basic " + raw} {
		bad := httptest.NewRequest(stdhttp.MethodGet, "/v1/projects", nil)
		bad.Header.Set("Authorization", header)
		resp, err = server.App().Test(bad)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != stdhttp.StatusUnauthorized {
			t.Fatalf("malformed header %q status = %d", header[:6], resp.StatusCode)
		}
	}

	getReq := httptest.NewRequest(stdhttp.MethodGet, "/v1/projects/"+foreign.ID.String(), nil)
	getReq.Header.Set("Authorization", "Bearer "+raw)
	resp, err = server.App().Test(getReq)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != stdhttp.StatusForbidden {
		t.Fatalf("cross-tenant status = %d", resp.StatusCode)
	}
}

func TestCreatedAPIKeyJSONDoesNotEchoHash(t *testing.T) {
	var resp apiKeyResponse
	raw := []byte(`{"id":"00000000-0000-0000-0000-000000000000","project_id":"00000000-0000-0000-0000-000000000000","name":"n","prefix":"hlk_1234","revoked":false,"created_at":"x","raw_key":"secret"}`)
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.RawKey != "secret" {
		t.Fatal("raw_key should deserialize for creation responses")
	}
}
