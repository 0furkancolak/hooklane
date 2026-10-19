package http

import (
	"bytes"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/gofiber/fiber/v3"
)

// asToken sends a request with an explicit bearer token (empty = none).
func asToken(t *testing.T, env testEnv, token, method, path, body string) (*stdhttp.Response, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.server.App().Test(req, fiber.TestConfig{Timeout: 15 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp, out
}

func signUp(t *testing.T, env testEnv, email string) string {
	t.Helper()
	resp, body := asToken(t, env, "", "POST", "/v1/auth/signup", `{"email":"`+email+`","password":"correct horse battery","organization_name":"Acme Inc"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("signup: %d %v", resp.StatusCode, body)
	}
	token, _ := body["token"].(string)
	if len(token) != 52 || token[:4] != "hls_" {
		t.Fatalf("session token shape: %q", token)
	}
	return token
}

func TestAccountSignUpSignInAndWorkAsAdmin(t *testing.T) {
	env := newTestEnv(t, config.AppEnvDevelopment)
	token := signUp(t, env, "Ada@Example.com")

	resp, body := asToken(t, env, token, "GET", "/v1/auth/me", "")
	if resp.StatusCode != 200 || body["user"].(map[string]any)["email"] != "ada@example.com" {
		t.Fatalf("me: %d %v", resp.StatusCode, body)
	}
	if _, leaked := body["user"].(map[string]any)["password_hash"]; leaked {
		t.Fatal("password hash returned")
	}
	if org := body["organization"].(map[string]any); org["name"] != "Acme Inc" {
		t.Fatalf("organization: %v", org)
	}

	// A session acts as admin of its organization: it can create a project
	// and cannot see another organization's project.
	resp, body = asToken(t, env, token, "POST", "/v1/projects", `{"name":"billing"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("create project: %d %v", resp.StatusCode, body)
	}
	if resp, _ := asToken(t, env, token, "GET", "/v1/projects/"+env.foreign.ID.String(), ""); resp.StatusCode != 403 {
		t.Fatalf("foreign project via session: %d", resp.StatusCode)
	}

	// Wrong password and unknown email look the same.
	for _, creds := range []string{
		`{"email":"ada@example.com","password":"wrong password here"}`,
		`{"email":"nobody@example.com","password":"correct horse battery"}`,
	} {
		resp, body := asToken(t, env, "", "POST", "/v1/auth/signin", creds)
		if resp.StatusCode != 401 || body["error"].(map[string]any)["code"] != "invalid_credentials" {
			t.Fatalf("signin %s: %d %v", creds, resp.StatusCode, body)
		}
	}
	resp, body = asToken(t, env, "", "POST", "/v1/auth/signin", `{"email":"ADA@example.com","password":"correct horse battery"}`)
	if resp.StatusCode != 200 || body["token"] == nil {
		t.Fatalf("signin: %d %v", resp.StatusCode, body)
	}

	// Sign out ends the session.
	if resp, _ := asToken(t, env, token, "POST", "/v1/auth/signout", ""); resp.StatusCode != 204 {
		t.Fatalf("signout: %d", resp.StatusCode)
	}
	if resp, _ := asToken(t, env, token, "GET", "/v1/auth/me", ""); resp.StatusCode != 401 {
		t.Fatalf("signed-out session still works: %d", resp.StatusCode)
	}
}

func TestAccountValidationAndDuplicates(t *testing.T) {
	env := newTestEnv(t, config.AppEnvDevelopment)
	for _, bad := range []string{
		`{"email":"not-an-email","password":"correct horse battery"}`,
		`{"email":"a@example.com","password":"short"}`,
		`{"email":"Ada <a@example.com>","password":"correct horse battery"}`,
		`{"email":"a@example.com","password":"` + string(bytes.Repeat([]byte("x"), 200)) + `"}`,
	} {
		if resp, body := asToken(t, env, "", "POST", "/v1/auth/signup", bad); resp.StatusCode != 400 {
			t.Fatalf("%s: %d %v, want 400", bad, resp.StatusCode, body)
		}
	}
	signUp(t, env, "a@example.com")
	resp, body := asToken(t, env, "", "POST", "/v1/auth/signup", `{"email":"A@example.com","password":"correct horse battery"}`)
	if resp.StatusCode != 409 || body["error"].(map[string]any)["code"] != "email_taken" {
		t.Fatalf("duplicate: %d %v", resp.StatusCode, body)
	}
}

func TestAccountOrganizations(t *testing.T) {
	env := newTestEnv(t, config.AppEnvDevelopment)
	token := signUp(t, env, "a@example.com")
	other := signUp(t, env, "b@example.com")

	resp, body := asToken(t, env, token, "POST", "/v1/auth/organizations", `{"name":"Second"}`)
	if resp.StatusCode != 201 || body["organization"].(map[string]any)["name"] != "Second" || len(body["organizations"].([]any)) != 2 {
		t.Fatalf("create org: %d %v", resp.StatusCode, body)
	}
	first := body["organizations"].([]any)[0].(map[string]any)["id"].(string)

	// Projects follow the active organization.
	asToken(t, env, token, "POST", "/v1/projects", `{"name":"in-second"}`)
	resp, body = asToken(t, env, token, "POST", "/v1/auth/switch-organization", `{"organization_id":"`+first+`"}`)
	if resp.StatusCode != 200 || body["organization"].(map[string]any)["id"] != first {
		t.Fatalf("switch: %d %v", resp.StatusCode, body)
	}
	_, body = asToken(t, env, token, "GET", "/v1/projects", "")
	if n := len(body["projects"].([]any)); n != 0 {
		t.Fatalf("first org should have no projects, has %d", n)
	}

	// Another user's organization cannot be entered.
	_, otherMe := asToken(t, env, other, "GET", "/v1/auth/me", "")
	foreign := otherMe["organization"].(map[string]any)["id"].(string)
	if resp, _ := asToken(t, env, token, "POST", "/v1/auth/switch-organization", `{"organization_id":"`+foreign+`"}`); resp.StatusCode != 404 {
		t.Fatalf("switch into foreign org: %d", resp.StatusCode)
	}
}

func TestSessionsAndAPIKeysStayInTheirLane(t *testing.T) {
	env := newTestEnv(t, config.AppEnvDevelopment)
	token := signUp(t, env, "a@example.com")

	// API keys have no account.
	if resp, _ := asToken(t, env, env.key, "GET", "/v1/auth/me", ""); resp.StatusCode != 403 {
		t.Fatalf("api key on /auth/me: %d", resp.StatusCode)
	}
	// Sessions are not portal tokens.
	if resp, _ := portalDo(t, env, token, "GET", "/v1/portal/me", ""); resp.StatusCode != 401 {
		t.Fatalf("session on portal: %d", resp.StatusCode)
	}
	// Garbage session tokens are rejected.
	for _, bad := range []string{"hls_", "hls_zz", "hls_" + string(bytes.Repeat([]byte("0"), 48))} {
		if resp, _ := asToken(t, env, bad, "GET", "/v1/projects", ""); resp.StatusCode != 401 {
			t.Fatalf("token %q: %d", bad, resp.StatusCode)
		}
	}
}

func TestSignUpClosedAfterFirstAccount(t *testing.T) {
	env := newTestEnv(t, config.AppEnvProduction)
	// newTestEnv opens sign-up; build a closed service on the same store.
	closed := newClosedSignupEnv(t)
	if resp, _ := asToken(t, closed, "", "POST", "/v1/auth/signup", `{"email":"first@example.com","password":"correct horse battery"}`); resp.StatusCode != 201 {
		t.Fatalf("first account on a closed server: %d", resp.StatusCode)
	}
	resp, body := asToken(t, closed, "", "POST", "/v1/auth/signup", `{"email":"second@example.com","password":"correct horse battery"}`)
	if resp.StatusCode != 403 || body["error"].(map[string]any)["code"] != "signup_disabled" {
		t.Fatalf("second account: %d %v", resp.StatusCode, body)
	}
	_ = env
}

func TestTrustedProxyDecidesClientAddress(t *testing.T) {
	handler := func(c fiber.Ctx) error { return c.SendString(c.IP()) }
	forwarded := func(cfg fiber.Config) string {
		app := fiber.New(cfg)
		app.Get("/", handler)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.9")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if got := forwarded(fiber.Config{}); got == "203.0.113.9" {
		t.Fatal("X-Forwarded-For trusted without configuration")
	}
	trusted := fiber.Config{TrustProxy: true, ProxyHeader: fiber.HeaderXForwardedFor, TrustProxyConfig: fiber.TrustProxyConfig{Proxies: []string{"0.0.0.0/0"}}}
	if got := forwarded(trusted); got != "203.0.113.9" {
		t.Fatalf("trusted proxy address = %q", got)
	}
}
