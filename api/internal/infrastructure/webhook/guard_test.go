package webhook

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
)

func countingServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server, &hits
}

func send(cfg SenderConfig, url string) application.WebhookResult {
	return NewSender(cfg).Send(context.Background(), application.WebhookRequest{
		URL: url, Secret: testSecret, MessageID: "msg_1", Body: []byte(`{}`),
	})
}

func TestProductionPolicyBlocksLoopbackLiteral(t *testing.T) {
	server, hits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	result := send(SenderConfig{}, server.URL)
	if !errors.Is(result.Err, application.ErrDestinationNotAllowed) {
		t.Fatalf("err = %v, want ErrDestinationNotAllowed", result.Err)
	}
	if hits.Load() != 0 {
		t.Fatal("blocked destination received a request")
	}
	if succeeded, retryable := application.Classify(result); succeeded || retryable {
		t.Fatalf("classify = %v/%v, want permanent failure", succeeded, retryable)
	}
}

// A hostname is only judged by the address it resolves to at connect time, so
// a name that passed URL validation (or that later rebinds) cannot reach a
// blocked address.
func TestProductionPolicyChecksResolvedAddress(t *testing.T) {
	server, hits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))

	result := send(SenderConfig{RequestTimeout: 2 * time.Second}, "http://localhost:"+port+"/hook")
	if !errors.Is(result.Err, application.ErrDestinationNotAllowed) {
		t.Fatalf("err = %v, want ErrDestinationNotAllowed", result.Err)
	}
	if hits.Load() != 0 {
		t.Fatal("blocked destination received a request")
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	target, targetHits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	origin, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/internal", http.StatusTemporaryRedirect)
	})

	result := send(SenderConfig{Destinations: loopbackOK}, origin.URL)
	if result.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, err = %v", result.StatusCode, result.Err)
	}
	if targetHits.Load() != 0 {
		t.Fatal("redirect target was contacted")
	}
	if succeeded, retryable := application.Classify(result); succeeded || retryable {
		t.Fatalf("classify = %v/%v, want permanent failure", succeeded, retryable)
	}
}

func TestDevelopmentPolicyStillBlocksMetadata(t *testing.T) {
	result := send(SenderConfig{Destinations: loopbackOK, RequestTimeout: time.Second}, "http://169.254.169.254/latest/meta-data/")
	if !errors.Is(result.Err, application.ErrDestinationNotAllowed) {
		t.Fatalf("err = %v, want ErrDestinationNotAllowed", result.Err)
	}
}

func TestTransportErrorsDropQueryString(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close() // nothing listens here any more: connection refused

	result := send(SenderConfig{Destinations: loopbackOK, RequestTimeout: time.Second}, "http://"+addr+"/hook?token=s3cr3t-value")
	if result.Err == nil {
		t.Fatal("expected a connection error")
	}
	if strings.Contains(result.Err.Error(), "s3cr3t-value") {
		t.Fatalf("error leaks the query string: %v", result.Err)
	}
	if !strings.Contains(result.Err.Error(), addr+"/hook") {
		t.Fatalf("error lost the destination: %v", result.Err)
	}
}

func TestCheckDialAddressRejectsNonTCP(t *testing.T) {
	if err := checkDialAddress(loopbackOK, "udp4", "8.8.8.8:53"); !errors.Is(err, application.ErrDestinationNotAllowed) {
		t.Fatalf("udp: %v", err)
	}
	if err := checkDialAddress(application.DestinationPolicy{}, "tcp4", "93.184.215.14:443"); err != nil {
		t.Fatalf("public address: %v", err)
	}
	if err := checkDialAddress(application.DestinationPolicy{}, "tcp6", "[::ffff:10.0.0.1]:443"); !errors.Is(err, application.ErrDestinationNotAllowed) {
		t.Fatalf("mapped private: %v", err)
	}
}
