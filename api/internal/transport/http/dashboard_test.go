package http

import (
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/config"
)

func TestDashboardIsServedWithoutShadowingTheAPI(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>Hooklane</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "index-abc123.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := newTestEnv(t, config.AppEnvProduction, WithDashboard(dir))

	get := func(path string) *stdhttp.Response {
		resp, err := env.server.App().Test(httptest.NewRequest(stdhttp.MethodGet, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	index := get("/")
	body, _ := io.ReadAll(index.Body)
	if index.StatusCode != stdhttp.StatusOK || !strings.Contains(string(body), "<title>Hooklane</title>") {
		t.Fatalf("index: %d %s", index.StatusCode, body)
	}
	if csp := index.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("csp = %q", csp)
	}
	if index.Header.Get("Cache-Control") != "no-store" || index.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("index headers = %v", index.Header)
	}
	asset := get("/assets/index-abc123.js")
	if asset.StatusCode != stdhttp.StatusOK || !strings.Contains(asset.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d %v", asset.StatusCode, asset.Header)
	}

	// API routes still answer with JSON, including unknown /v1 paths.
	if resp := get("/v1/projects"); resp.StatusCode != stdhttp.StatusUnauthorized {
		t.Fatalf("/v1/projects: %d", resp.StatusCode)
	}
	resp := get("/v1/nope")
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != stdhttp.StatusNotFound || !strings.Contains(string(b), `"not_found"`) {
		t.Fatalf("/v1/nope: %d %s", resp.StatusCode, b)
	}
	if resp := get("/healthz"); resp.StatusCode != stdhttp.StatusOK {
		t.Fatalf("/healthz: %d", resp.StatusCode)
	}
	if resp := get("/../go.mod"); resp.StatusCode == stdhttp.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(b), "module ") {
			t.Fatal("path traversal escaped the dashboard directory")
		}
	}
}
