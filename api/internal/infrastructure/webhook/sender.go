package webhook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
)

type SenderConfig struct {
	// RequestTimeout bounds the whole exchange: connect, TLS, headers and body.
	RequestTimeout time.Duration
	// MaxResponseBytes caps how much of the response body is read and kept.
	MaxResponseBytes int64
	UserAgent        string
	// Destinations is checked against the resolved address of every
	// connection, after DNS and immediately before connect.
	Destinations application.DestinationPolicy
}

type Sender struct {
	client *http.Client
	cfg    SenderConfig
	now    func() time.Time
}

func NewSender(cfg SenderConfig) *Sender {
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 15 * time.Second
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = 4 << 10
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Hooklane-Webhooks/1.0"
	}
	connectTimeout := min(5*time.Second, cfg.RequestTimeout)
	dialer := &net.Dialer{
		Timeout:   connectTimeout,
		KeepAlive: 30 * time.Second,
		// Control sees the exact address the socket is about to connect to,
		// so the check cannot be bypassed by DNS rebinding (a second lookup
		// returning a different address) or by a resolver that understands
		// address forms the URL validation did not.
		Control: func(network, address string, _ syscall.RawConn) error {
			return checkDialAddress(cfg.Destinations, network, address)
		},
	}
	transport := &http.Transport{
		// Destinations are tenant-controlled; never route through an
		// environment-configured proxy.
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   connectTimeout,
		ResponseHeaderTimeout: cfg.RequestTimeout,
		ExpectContinueTimeout: time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &Sender{
		client: &http.Client{
			Transport: transport,
			Timeout:   cfg.RequestTimeout,
			// Redirects are not followed: a 3xx is a non-retryable failure.
			// Receivers must answer at the registered URL, and a redirect
			// cannot steer the worker to another destination.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		cfg: cfg,
		now: time.Now,
	}
}

func (s *Sender) Send(ctx context.Context, req application.WebhookRequest) application.WebhookResult {
	started := s.now()
	signature, err := Sign(req.Secret, req.MessageID, started, req.Body)
	if err != nil {
		return application.WebhookResult{Err: err}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return application.WebhookResult{Err: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", s.cfg.UserAgent)
	httpReq.Header.Set(HeaderID, req.MessageID)
	httpReq.Header.Set(HeaderTimestamp, strconv.FormatInt(started.Unix(), 10))
	httpReq.Header.Set(HeaderSignature, signature)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return application.WebhookResult{Duration: time.Since(started), Err: redactURLError(err)}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, s.cfg.MaxResponseBytes))
	result := application.WebhookResult{
		StatusCode:   resp.StatusCode,
		ResponseBody: sanitizeBody(raw),
		RetryAfter:   parseRetryAfter(resp.Header.Get("Retry-After"), s.now()),
		Duration:     time.Since(started),
	}
	if readErr != nil {
		result.Err = readErr
	}
	return result
}

func checkDialAddress(policy application.DestinationPolicy, network, address string) error {
	if network != "tcp4" && network != "tcp6" {
		return fmt.Errorf("%w: network %s", application.ErrDestinationNotAllowed, network)
	}
	addrPort, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: %w", application.ErrDestinationNotAllowed, err)
	}
	if !policy.AllowAddr(addrPort.Addr()) {
		return fmt.Errorf("%w: %s", application.ErrDestinationNotAllowed, addrPort.Addr())
	}
	return nil
}

// redactURLError drops the query string from errors that quote the request
// URL. Receivers sometimes put tokens there, and the error is stored with the
// attempt.
func redactURLError(err error) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	redacted := *urlErr
	if parsed, parseErr := url.Parse(urlErr.URL); parseErr == nil {
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		parsed.User = nil
		redacted.URL = parsed.String()
	} else {
		redacted.URL = "[redacted]"
	}
	return &redacted
}

// parseRetryAfter accepts delay-seconds or an HTTP date and returns zero for
// anything else. The retry policy caps the result.
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if secs, err := strconv.ParseInt(value, 10, 64); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(min(secs, int64(7*24*time.Hour/time.Second))) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}

// sanitizeBody makes an arbitrary response storable in a PostgreSQL TEXT
// column, which rejects NUL bytes and invalid UTF-8.
func sanitizeBody(raw []byte) string {
	return strings.ReplaceAll(strings.ToValidUTF8(string(raw), "�"), "\x00", "")
}
