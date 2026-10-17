package openapi

import (
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The document must parse, and every local $ref must point at something.
func TestSpecParsesAndRefsResolve(t *testing.T) {
	var doc map[string]any
	if err := yaml.Unmarshal(Spec, &doc); err != nil {
		t.Fatalf("openapi.yaml does not parse: %v", err)
	}
	if doc["openapi"] != "3.1.0" {
		t.Fatalf("openapi = %v", doc["openapi"])
	}
	refs := regexp.MustCompile(`\$ref: "#/([^"]+)"`).FindAllStringSubmatch(string(Spec), -1)
	if len(refs) == 0 {
		t.Fatal("no refs found")
	}
	for _, ref := range refs {
		var node any = doc
		for _, part := range strings.Split(ref[1], "/") {
			m, ok := node.(map[string]any)
			if !ok {
				node = nil
				break
			}
			node = m[part]
		}
		if node == nil {
			t.Errorf("unresolved $ref #/%s", ref[1])
		}
	}
}
