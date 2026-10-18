package http

import (
	"context"
	stdhttp "net/http"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/0furkancolak/hooklane/api/internal/domain"
)

// withScopedKey returns a copy of env that authenticates with a new key that
// has only the given scopes.
func withScopedKey(t *testing.T, env testEnv, scopes ...domain.Scope) testEnv {
	t.Helper()
	raw, prefix, hash, err := application.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.Keys().Create(context.Background(), domain.APIKey{ProjectID: env.project.ID, OrganizationID: env.project.OrganizationID, Name: "scoped", Prefix: prefix, Hash: hash, Scopes: scopes}); err != nil {
		t.Fatal(err)
	}
	env.key = raw
	return env
}

func TestScopedKeysAreLimited(t *testing.T) {
	admin := newTestEnv(t, config.AppEnvProduction)
	projectPath := "/v1/projects/" + admin.project.ID.String()
	if _, err := admin.store.Endpoints().Create(context.Background(), domain.Endpoint{ProjectID: admin.project.ID, URL: "https://example.com", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	resp, body := admin.do(t, "POST", projectPath+"/endpoints", `{"url":"https://example.com/two"}`)
	if resp.StatusCode != stdhttp.StatusCreated {
		t.Fatalf("setup: %d %v", resp.StatusCode, body)
	}
	endpointID := body["id"].(string)

	read := withScopedKey(t, admin, domain.ScopeRead)
	ingest := withScopedKey(t, admin, domain.ScopeIngest)

	cases := []struct {
		name         string
		env          testEnv
		method, path string
		body         string
		want         int
	}{
		{"read lists endpoints", read, "GET", projectPath + "/endpoints", "", 200},
		{"read cannot reveal secrets", read, "GET", "/v1/endpoints/" + endpointID + "/secret", "", 403},
		{"read cannot create endpoints", read, "POST", projectPath + "/endpoints", `{"url":"https://example.com/x"}`, 403},
		{"read cannot ingest", read, "POST", projectPath + "/events", `{"event_type":"a.b","payload":{}}`, 403},
		{"read cannot create keys", read, "POST", projectPath + "/api-keys", `{"name":"x"}`, 403},
		{"ingest can ingest", ingest, "POST", projectPath + "/events", `{"event_type":"a.b","payload":{}}`, 202},
		{"ingest cannot list events", ingest, "GET", projectPath + "/events", "", 403},
		{"ingest cannot list endpoints", ingest, "GET", projectPath + "/endpoints", "", 403},
		{"admin can do both", admin, "POST", projectPath + "/events", `{"event_type":"a.b","payload":{}}`, 202},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := tc.env.do(t, tc.method, tc.path, tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("%s %s = %d %v, want %d", tc.method, tc.path, resp.StatusCode, body, tc.want)
			}
			if tc.want == 403 {
				if detail, _ := body["error"].(map[string]any); detail["code"] != "insufficient_scope" {
					t.Fatalf("error code = %v", detail["code"])
				}
			}
		})
	}
}

func TestCreateKeyScopes(t *testing.T) {
	env := newTestEnv(t, config.AppEnvProduction)
	path := "/v1/projects/" + env.project.ID.String() + "/api-keys"

	resp, body := env.do(t, "POST", path, `{"name":"default"}`)
	if resp.StatusCode != 201 || len(body["scopes"].([]any)) != 1 || body["scopes"].([]any)[0] != "admin" {
		t.Fatalf("default scopes: %d %v", resp.StatusCode, body)
	}
	resp, body = env.do(t, "POST", path, `{"name":"sender","scopes":["ingest","ingest"]}`)
	if resp.StatusCode != 201 || len(body["scopes"].([]any)) != 1 || body["scopes"].([]any)[0] != "ingest" {
		t.Fatalf("scoped key: %d %v", resp.StatusCode, body)
	}
	resp, _ = env.do(t, "POST", path, `{"name":"bad","scopes":["root"]}`)
	if resp.StatusCode != 400 {
		t.Fatalf("unknown scope: %d", resp.StatusCode)
	}
}
