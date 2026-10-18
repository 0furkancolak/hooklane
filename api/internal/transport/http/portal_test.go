package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/config"
)

// portalDo sends a request authenticated with a portal token.
func portalDo(t *testing.T, env testEnv, token, method, path, body string) (*stdhttp.Response, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.server.App().Test(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp, out
}

func portalLink(t *testing.T, env testEnv, customer string) string {
	t.Helper()
	resp, body := env.do(t, "POST", "/v1/projects/"+env.project.ID.String()+"/portal-links", fmt.Sprintf(`{"customer":%q}`, customer))
	if resp.StatusCode != 201 {
		t.Fatalf("portal link: %d %v", resp.StatusCode, body)
	}
	token, _ := body["token"].(string)
	if len(token) != 52 || token[:4] != "hlp_" {
		t.Fatalf("token shape: %q", token)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("portal link response must not be cached")
	}
	return token
}

func TestPortalCustomersAreIsolated(t *testing.T) {
	env := newTestEnv(t, config.AppEnvDevelopment)
	project := "/v1/projects/" + env.project.ID.String()

	acme := portalLink(t, env, "acme")
	globex := portalLink(t, env, "globex")

	resp, body := portalDo(t, env, acme, "GET", "/v1/portal/me", "")
	if resp.StatusCode != 200 || body["customer"] != "acme" || body["project_name"] != "mine" {
		t.Fatalf("me: %d %v", resp.StatusCode, body)
	}

	// Customers create their own endpoints; the customer cannot be chosen.
	resp, body = portalDo(t, env, acme, "POST", "/v1/portal/endpoints", `{"url":"https://acme.example/hook","customer":"globex"}`)
	if resp.StatusCode != 201 || body["customer"] != "acme" || body["secret"] == nil {
		t.Fatalf("acme create: %d %v", resp.StatusCode, body)
	}
	acmeEndpoint := body["id"].(string)
	resp, body = portalDo(t, env, globex, "POST", "/v1/portal/endpoints", `{"url":"https://globex.example/hook"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("globex create: %d %v", resp.StatusCode, body)
	}
	globexEndpoint := body["id"].(string)
	// The integrator's own, project-wide endpoint.
	resp, body = env.do(t, "POST", project+"/endpoints", `{"url":"https://own.example/hook"}`)
	if resp.StatusCode != 201 || body["customer"] != nil {
		t.Fatalf("own endpoint: %d %v", resp.StatusCode, body)
	}

	// Each customer sees only their endpoint.
	resp, body = portalDo(t, env, acme, "GET", "/v1/portal/endpoints", "")
	list := body["endpoints"].([]any)
	if resp.StatusCode != 200 || len(list) != 1 || list[0].(map[string]any)["id"] != acmeEndpoint {
		t.Fatalf("acme list: %d %v", resp.StatusCode, body)
	}
	// Another customer's endpoint looks missing for read, update and secret.
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/v1/portal/endpoints/" + globexEndpoint, ""},
		{"PATCH", "/v1/portal/endpoints/" + globexEndpoint, `{"enabled":false}`},
		{"GET", "/v1/portal/endpoints/" + globexEndpoint + "/secret", ""},
	} {
		if resp, body := portalDo(t, env, acme, tc.method, tc.path, tc.body); resp.StatusCode != 404 {
			t.Fatalf("%s %s: %d %v, want 404", tc.method, tc.path, resp.StatusCode, body)
		}
	}
	// A customer can manage and reveal their own.
	if resp, body := portalDo(t, env, acme, "PATCH", "/v1/portal/endpoints/"+acmeEndpoint, `{"enabled":false}`); resp.StatusCode != 200 || body["enabled"] != false {
		t.Fatalf("own patch: %d %v", resp.StatusCode, body)
	}
	if resp, _ := portalDo(t, env, acme, "GET", "/v1/portal/endpoints/"+acmeEndpoint+"/secret", ""); resp.StatusCode != 200 {
		t.Fatalf("own secret: %d", resp.StatusCode)
	}
	portalDo(t, env, acme, "PATCH", "/v1/portal/endpoints/"+acmeEndpoint, `{"enabled":true}`)

	// Events go only to endpoints of the same customer.
	deliveriesFor := func(body string) int {
		resp, out := env.do(t, "POST", project+"/events", body)
		if resp.StatusCode != 202 {
			t.Fatalf("ingest: %d %v", resp.StatusCode, out)
		}
		return int(out["delivery_count"].(float64))
	}
	if n := deliveriesFor(`{"event_type":"a.b","payload":{},"customer":"acme"}`); n != 1 {
		t.Fatalf("acme event fan-out = %d, want 1", n)
	}
	if n := deliveriesFor(`{"event_type":"a.b","payload":{}}`); n != 1 {
		t.Fatalf("customer-less event fan-out = %d, want 1 (the project's own endpoint)", n)
	}
	if n := deliveriesFor(`{"event_type":"a.b","payload":{},"customer":"nobody"}`); n != 0 {
		t.Fatalf("unknown customer fan-out = %d, want 0", n)
	}

	// Deliveries are limited the same way, including attempts and replay.
	resp, body = portalDo(t, env, acme, "GET", "/v1/portal/deliveries", "")
	deliveries := body["deliveries"].([]any)
	if resp.StatusCode != 200 || len(deliveries) != 1 {
		t.Fatalf("acme deliveries: %d %v", resp.StatusCode, body)
	}
	resp, body = portalDo(t, env, globex, "GET", "/v1/portal/deliveries", "")
	if len(body["deliveries"].([]any)) != 0 {
		t.Fatalf("globex deliveries leaked: %v", body)
	}
	acmeDelivery := deliveries[0].(map[string]any)["id"].(string)
	if resp, _ := portalDo(t, env, globex, "GET", "/v1/portal/deliveries/"+acmeDelivery+"/attempts", ""); resp.StatusCode != 404 {
		t.Fatalf("other customer's attempts: %d", resp.StatusCode)
	}
	if resp, _ := portalDo(t, env, globex, "POST", "/v1/portal/deliveries/"+acmeDelivery+"/replay", ""); resp.StatusCode != 404 {
		t.Fatalf("other customer's replay: %d", resp.StatusCode)
	}
}

func TestPortalAndAPIKeysDoNotCrossOver(t *testing.T) {
	env := newTestEnv(t, config.AppEnvProduction)
	token := portalLink(t, env, "acme")
	project := "/v1/projects/" + env.project.ID.String()

	// A portal token cannot call the management API...
	for _, path := range []string{project, project + "/endpoints", project + "/events", project + "/api-keys", project + "/delivery-stats"} {
		if resp, _ := portalDo(t, env, token, "GET", path, ""); resp.StatusCode != 401 {
			t.Fatalf("portal token on %s: %d, want 401", path, resp.StatusCode)
		}
	}
	// ...and an API key cannot use portal routes.
	if resp, _ := portalDo(t, env, env.key, "GET", "/v1/portal/endpoints", ""); resp.StatusCode != 401 {
		t.Fatalf("api key on portal route: %d, want 401", resp.StatusCode)
	}
	if resp, _ := portalDo(t, env, "hlp_"+string(bytes.Repeat([]byte("0"), 48)), "GET", "/v1/portal/me", ""); resp.StatusCode != 401 {
		t.Fatalf("unknown token: %d, want 401", resp.StatusCode)
	}
	// Only admin keys may issue links.
	read := withScopedKey(t, env, "read")
	if resp, _ := read.do(t, "POST", project+"/portal-links", `{"customer":"x"}`); resp.StatusCode != 403 {
		t.Fatalf("read key issuing a link: %d, want 403", resp.StatusCode)
	}
	if resp, _ := env.do(t, "POST", project+"/portal-links", `{"customer":"bad customer!"}`); resp.StatusCode != 400 {
		t.Fatalf("invalid customer: %d, want 400", resp.StatusCode)
	}
	if resp, _ := env.do(t, "POST", project+"/portal-links", `{"customer":"x","ttl_seconds":99999999}`); resp.StatusCode != 400 {
		t.Fatalf("too long ttl: %d, want 400", resp.StatusCode)
	}
	if resp, _ := env.do(t, "POST", "/v1/projects/"+env.foreign.ID.String()+"/portal-links", `{"customer":"x"}`); resp.StatusCode != 403 {
		t.Fatalf("foreign project link: %d, want 403", resp.StatusCode)
	}
}

func TestPortalEndpointLimit(t *testing.T) {
	env := newTestEnv(t, config.AppEnvDevelopment)
	token := portalLink(t, env, "busy")
	for i := 0; i < 25; i++ {
		if resp, body := portalDo(t, env, token, "POST", "/v1/portal/endpoints", fmt.Sprintf(`{"url":"https://busy.example/%d"}`, i)); resp.StatusCode != 201 {
			t.Fatalf("endpoint %d: %d %v", i, resp.StatusCode, body)
		}
	}
	resp, body := portalDo(t, env, token, "POST", "/v1/portal/endpoints", `{"url":"https://busy.example/extra"}`)
	if resp.StatusCode != 409 {
		t.Fatalf("26th endpoint: %d %v, want 409", resp.StatusCode, body)
	}
}
