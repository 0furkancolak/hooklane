package http

import (
	"encoding/json"
	"net"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/gofiber/fiber/v3"
)

func newDevKeyEnv(t *testing.T, env config.AppEnv) testEnv {
	t.Helper()
	e := newTestEnv(t, env)
	projects := application.NewProjectService(e.store.Projects(), e.store.Keys())
	devKeys := application.NewDevelopmentKeyService(nil, e.store, projects)
	e.server = NewServer(config.Config{Port: "8080", AppEnv: env}, Services{
		Auth:     application.NewAuthenticator(e.store.Keys()),
		Projects: projects,
		DevKeys:  devKeys,
	})
	return e
}

func devRequest(t *testing.T, e testEnv, method, path string) *stdhttp.Response {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	resp, err := e.server.App().Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// app.Test always reports the client as 0.0.0.0, so this test serves the app
// on a real loopback listener.
func TestDevKeyRoutesIssueAWorkingKeyFromLoopback(t *testing.T) {
	e := newDevKeyEnv(t, config.AppEnvDevelopment)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = e.server.App().Listener(ln, fiber.ListenConfig{DisableStartupMessage: true}) }()
	t.Cleanup(func() { _ = e.server.App().Shutdown() })
	base := "http://" + ln.Addr().String()

	resp, err := stdhttp.Get(base + "/v1/dev")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != stdhttp.StatusOK {
		t.Fatalf("GET /v1/dev = %d", resp.StatusCode)
	}
	resp, err = stdhttp.Post(base+"/v1/dev/api-keys", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != stdhttp.StatusCreated {
		t.Fatalf("POST /v1/dev/api-keys = %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	raw, _ := body["raw_key"].(string)
	if body["project_id"] != e.project.ID.String() || raw == "" {
		t.Fatalf("body = %v", body)
	}
	e.key = raw
	if resp, _ := e.do(t, "GET", "/v1/projects", ""); resp.StatusCode != stdhttp.StatusOK {
		t.Fatalf("issued key rejected: %d", resp.StatusCode)
	}
}

// app.Test's client address, 0.0.0.0, is not loopback.
func TestDevKeyRoutesRejectRemoteClients(t *testing.T) {
	e := newDevKeyEnv(t, config.AppEnvDevelopment)
	if resp := devRequest(t, e, "POST", "/v1/dev/api-keys"); resp.StatusCode != stdhttp.StatusForbidden {
		t.Fatalf("remote POST = %d", resp.StatusCode)
	}
}

func TestDevKeyRoutesAbsentOutsideDevelopment(t *testing.T) {
	for _, env := range []config.AppEnv{config.AppEnvProduction, config.AppEnvTest} {
		e := newDevKeyEnv(t, env)
		for _, route := range [][2]string{{"GET", "/v1/dev"}, {"POST", "/v1/dev/api-keys"}} {
			if resp := devRequest(t, e, route[0], route[1]); resp.StatusCode != stdhttp.StatusNotFound {
				t.Fatalf("%s %s in %s = %d", route[0], route[1], env, resp.StatusCode)
			}
		}
	}
}
