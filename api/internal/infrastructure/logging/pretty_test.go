package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrettyRequestLine(t *testing.T) {
	var out bytes.Buffer
	log := New(&out, FormatPretty, false)
	log.Info("request", "method", "GET", "route", "/v1/projects/:id", "path", "/v1/projects/abc", "status", 200, "duration_ms", 3, "request_id", "0123456789abcdef", "api_key_id", "k1")
	line := out.String()
	for _, want := range []string{"INFO", "GET", "/v1/projects/:id", "200", "3ms", "api_key_id=k1", "req=01234567"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q is missing %q", line, want)
		}
	}
	if strings.Contains(line, "\x1b[") {
		t.Error("colour codes were written with colour disabled")
	}
	if strings.Count(line, "\n") != 1 {
		t.Errorf("expected exactly one line, got %q", line)
	}
}

func TestPrettyOtherMessagesAndColour(t *testing.T) {
	var out bytes.Buffer
	log := New(&out, FormatPretty, true).With("component", "worker")
	log.Warn("delivery attempted", "attempt", 2, "error", "endpoint responded with status 503")
	line := out.String()
	for _, want := range []string{"WARN", "delivery attempted", "component=", "attempt=", `"endpoint responded with status 503"`, "\x1b["} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q is missing %q", line, want)
		}
	}
}

func TestJSONFormat(t *testing.T) {
	var out bytes.Buffer
	New(&out, FormatJSON, false).Info("hello", "n", 1)
	if !strings.HasPrefix(out.String(), `{"time":`) {
		t.Fatalf("not JSON: %s", out.String())
	}
}
