package http

import (
	"encoding/json"
	"io"
	"net"
	stdhttp "net/http"
	"strings"
	"testing"

	"github.com/0furkancolak/hooklane/api/internal/config"
	"github.com/gofiber/fiber/v3"
)

func TestDevSinkCapturesDeliveriesAndSimulatesFailures(t *testing.T) {
	e := newDevKeyEnv(t, config.AppEnvDevelopment)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = e.server.App().Listener(ln, fiber.ListenConfig{DisableStartupMessage: true}) }()
	t.Cleanup(func() { _ = e.server.App().Shutdown() })
	base := "http://" + ln.Addr().String() + "/v1/dev/sink/playground-token"

	post := func(url string) int {
		req, _ := stdhttp.NewRequest(stdhttp.MethodPost, url, strings.NewReader(`{"hello":"world"}`))
		req.Header.Set("Webhook-Signature", "v1,abc")
		req.Header.Set("Authorization", "Bearer secret")
		resp, err := stdhttp.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := post(base); got != 200 {
		t.Fatalf("receive = %d", got)
	}
	if got := post(base + "?status=503"); got != 503 {
		t.Fatalf("simulated failure = %d", got)
	}
	if got := post(base + "?status=42"); got != 400 {
		t.Fatalf("bad status = %d", got)
	}

	resp, err := stdhttp.Get(base)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out struct {
		Requests []sinkRequest `json:"requests"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Requests) != 2 || out.Requests[0].Status != 503 || out.Requests[1].Body != `{"hello":"world"}` {
		t.Fatalf("requests = %+v", out.Requests)
	}
	if out.Requests[1].Headers["Webhook-Signature"] != "v1,abc" {
		t.Fatal("signature header was not captured")
	}
	if _, leaked := out.Requests[1].Headers["Authorization"]; leaked {
		t.Fatal("Authorization header was stored")
	}
}

func TestDevSinkIsAbsentOutsideDevelopment(t *testing.T) {
	e := newDevKeyEnv(t, config.AppEnvProduction)
	if resp := devRequest(t, e, stdhttp.MethodPost, "/v1/dev/sink/playground-token"); resp.StatusCode != stdhttp.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// Fiber reuses request buffers; tokens stored as map keys must survive later
// requests with other paths.
func TestDevSinkKeepsTokensAcrossRequests(t *testing.T) {
	e := newDevKeyEnv(t, config.AppEnvDevelopment)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = e.server.App().Listener(ln, fiber.ListenConfig{DisableStartupMessage: true}) }()
	t.Cleanup(func() { _ = e.server.App().Shutdown() })
	base := "http://" + ln.Addr().String() + "/v1/dev/sink/"

	for _, token := range []string{"aaaaaaaa-one", "bbbbbbbb-two", "cccccccc-three"} {
		resp, err := stdhttp.Post(base+token, "application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	for _, token := range []string{"aaaaaaaa-one", "bbbbbbbb-two", "cccccccc-three"} {
		resp, err := stdhttp.Get(base + token)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(body), `"method":"POST"`) {
			t.Fatalf("token %s lost its request: %s", token, body)
		}
	}
}
