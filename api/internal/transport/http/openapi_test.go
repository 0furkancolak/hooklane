package http

import (
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/0furkancolak/hooklane/api/openapi"
)

var specPath = regexp.MustCompile(`(?m)^  (/[^:\s]*):\s*$`)
var specMethod = regexp.MustCompile(`(?m)^    (get|post|patch|put|delete):`)

// Every registered route must be described in openapi.yaml, and the spec
// must not describe routes that do not exist.
func TestOpenAPIMatchesRegisteredRoutes(t *testing.T) {
	spec := string(openapi.Spec)
	documented := map[string]bool{}
	locations := specPath.FindAllStringSubmatchIndex(spec, -1)
	for i, loc := range locations {
		path := spec[loc[2]:loc[3]]
		end := len(spec)
		if i+1 < len(locations) {
			end = locations[i+1][0]
		}
		for _, m := range specMethod.FindAllStringSubmatch(spec[loc[1]:end], -1) {
			documented[strings.ToUpper(m[1])+" "+path] = true
		}
	}

	env := newTestEnv(t, config.AppEnvProduction)
	registered := map[string]bool{}
	for _, route := range env.server.App().GetRoutes(true) {
		if route.Method == stdhttp.MethodHead || route.Path == "/" || route.Path == "/*" {
			continue
		}
		path := regexp.MustCompile(`:(\w+)`).ReplaceAllString(route.Path, "{$1}")
		registered[route.Method+" "+path] = true
	}
	if len(registered) < 20 {
		t.Fatalf("only %d routes registered; route discovery is broken", len(registered))
	}
	for route := range registered {
		if !documented[route] {
			t.Errorf("route %s is missing from openapi.yaml", route)
		}
	}
	for route := range documented {
		if !registered[route] {
			t.Errorf("openapi.yaml documents %s, which is not registered", route)
		}
	}
}

func TestOpenAPIIsServed(t *testing.T) {
	env := newTestEnv(t, config.AppEnvProduction)
	resp, err := env.server.App().Test(httptest.NewRequest(stdhttp.MethodGet, "/openapi.yaml", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != stdhttp.StatusOK || !strings.HasPrefix(string(body), "openapi: 3.1.0") || resp.Header.Get("Content-Type") != "application/yaml" {
		t.Fatalf("%d %q %s", resp.StatusCode, resp.Header.Get("Content-Type"), body[:min(len(body), 40)])
	}
}
