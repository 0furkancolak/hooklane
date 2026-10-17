package webhook

import (
	"context"
	"github.com/0furkancolak/hooklane/api/internal/application"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

// loopbackOK lets tests reach httptest servers, which listen on 127.0.0.1.
var loopbackOK = application.DestinationPolicy{AllowPrivateNetworks: true}

func TestSendProducesVerifiableRequest(t *testing.T) {
	var verifyErr error
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		verifyErr = Verify(testSecret, r.Header.Get(HeaderID), r.Header.Get(HeaderTimestamp), r.Header.Get(HeaderSignature), body, time.Now(), DefaultTolerance)
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	result := NewSender(SenderConfig{Destinations: loopbackOK}).Send(context.Background(), application.WebhookRequest{
		URL: server.URL, Secret: testSecret, MessageID: "evt_1", Body: []byte(`{"x":1}`),
	})
	if result.Err != nil || result.StatusCode != http.StatusNoContent {
		t.Fatalf("result = %+v", result)
	}
	if verifyErr != nil {
		t.Fatalf("receiver could not verify: %v", verifyErr)
	}
	if gotBody != `{"x":1}` {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestSendTimesOut(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)

	started := time.Now()
	result := NewSender(SenderConfig{Destinations: loopbackOK, RequestTimeout: 200 * time.Millisecond}).Send(context.Background(), application.WebhookRequest{
		URL: server.URL, Secret: testSecret, MessageID: "m", Body: []byte(`{}`),
	})
	if result.Err == nil || result.StatusCode != 0 {
		t.Fatalf("expected timeout error, got %+v", result)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("timeout not enforced: %v", elapsed)
	}
}

func TestSendTimesOutOnSlowBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		for i := 0; i < 50; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(50 * time.Millisecond):
				_, _ = w.Write([]byte("x"))
				w.(http.Flusher).Flush()
			}
		}
	}))
	defer server.Close()

	started := time.Now()
	result := NewSender(SenderConfig{Destinations: loopbackOK, RequestTimeout: 300 * time.Millisecond}).Send(context.Background(), application.WebhookRequest{
		URL: server.URL, Secret: testSecret, MessageID: "m", Body: []byte(`{}`),
	})
	if result.Err == nil {
		t.Fatalf("expected body read timeout, got %+v", result)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("overall timeout not enforced: %v", elapsed)
	}
}

func TestSendDoesNotFollowRedirects(t *testing.T) {
	followed := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { followed = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	result := NewSender(SenderConfig{Destinations: loopbackOK}).Send(context.Background(), application.WebhookRequest{
		URL: server.URL, Secret: testSecret, MessageID: "m", Body: []byte(`{}`),
	})
	if followed || result.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("redirect followed=%v result=%+v", followed, result)
	}
}

func TestSendBoundsAndSanitizesResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("ok\x00\xff"))
		_, _ = w.Write([]byte(strings.Repeat("z", 1<<20)))
	}))
	defer server.Close()

	result := NewSender(SenderConfig{Destinations: loopbackOK, MaxResponseBytes: 64}).Send(context.Background(), application.WebhookRequest{
		URL: server.URL, Secret: testSecret, MessageID: "m", Body: []byte(`{}`),
	})
	if result.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d", result.StatusCode)
	}
	if strings.Contains(result.ResponseBody, "\x00") || !strings.HasPrefix(result.ResponseBody, "ok�") {
		t.Fatalf("body not sanitized: %q", result.ResponseBody[:8])
	}
	if len(result.ResponseBody) > 70 {
		t.Fatalf("body not bounded: %d bytes", len(result.ResponseBody))
	}
}

func TestSendReportsConnectionFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()

	result := NewSender(SenderConfig{Destinations: loopbackOK, RequestTimeout: time.Second}).Send(context.Background(), application.WebhookRequest{
		URL: "http://" + addr, Secret: testSecret, MessageID: "m", Body: []byte(`{}`),
	})
	if result.Err == nil || result.StatusCode != 0 {
		t.Fatalf("expected connection error, got %+v", result)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"":                              0,
		"120":                           2 * time.Minute,
		"0":                             0,
		"-5":                            0,
		"soon":                          0,
		"Sat, 17 Oct 2026 12:05:00 GMT": 5 * time.Minute,
		"Sat, 17 Oct 2026 11:00:00 GMT": 0,
		"99999999999":                   7 * 24 * time.Hour,
	}
	for in, want := range cases {
		if got := parseRetryAfter(in, now); got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}

func TestSendReportsRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "90")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	result := NewSender(SenderConfig{Destinations: loopbackOK}).Send(context.Background(), application.WebhookRequest{
		URL: server.URL, Secret: testSecret, MessageID: "m", Body: []byte(`{}`),
	})
	if result.StatusCode != http.StatusTooManyRequests || result.RetryAfter != 90*time.Second {
		t.Fatalf("result = %+v", result)
	}
}
