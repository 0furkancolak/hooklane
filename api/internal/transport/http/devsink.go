package http

import (
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/gofiber/fiber/v3"
)

const (
	sinkMaxTokens   = 100
	sinkMaxPerToken = 50
	sinkMaxBody     = 16 << 10
)

var sinkTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// sinkRequest is one webhook the playground receiver captured.
type sinkRequest struct {
	ReceivedAt string            `json:"received_at"`
	Method     string            `json:"method"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	Truncated  bool              `json:"truncated"`
	Status     int               `json:"status"`
}

// devSink is the dashboard playground's built-in webhook receiver: it records
// what Hooklane delivers so the effect of an event can be inspected without
// any outside service. It is registered only when APP_ENV=development and
// keeps a bounded amount of data in memory.
type devSink struct {
	baseURL string

	mu       sync.Mutex
	requests map[string][]sinkRequest
	order    []string
}

func newDevSink(baseURL string) *devSink {
	return &devSink{baseURL: baseURL, requests: map[string][]sinkRequest{}}
}

func (s *devSink) status(c fiber.Ctx) error {
	return c.JSON(fiber.Map{"development": true, "sink_url": s.baseURL})
}

// receive accepts any delivery. ?status=NNN (200–599) makes it answer with
// that status so retries and dead letters can be demonstrated.
func (s *devSink) receive(c fiber.Ctx) error {
	// Fiber reuses request buffers, so a value that outlives the request, such
	// as a map key, must be copied.
	token := strings.Clone(c.Params("token"))
	if !sinkTokenPattern.MatchString(token) {
		return writeError(c, application.ErrInvalidInput)
	}
	status := fiber.StatusOK
	if raw := c.Query("status"); raw != "" {
		code, err := strconv.Atoi(raw)
		if err != nil || code < 200 || code > 599 {
			return writeError(c, application.ErrInvalidInput)
		}
		status = code
	}

	body := c.Body()
	truncated := len(body) > sinkMaxBody
	if truncated {
		body = body[:sinkMaxBody]
	}
	headers := map[string]string{}
	for key, value := range c.Request().Header.All() {
		name := string(key)
		if name == "Authorization" || name == "Cookie" {
			continue
		}
		headers[name] = string(value)
	}

	s.mu.Lock()
	if _, known := s.requests[token]; !known {
		if len(s.order) >= sinkMaxTokens {
			delete(s.requests, s.order[0])
			s.order = s.order[1:]
		}
		s.order = append(s.order, token)
	}
	list := append([]sinkRequest{{
		ReceivedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Method:     c.Method(),
		Headers:    headers,
		Body:       string(body),
		Truncated:  truncated,
		Status:     status,
	}}, s.requests[token]...)
	if len(list) > sinkMaxPerToken {
		list = list[:sinkMaxPerToken]
	}
	s.requests[token] = list
	s.mu.Unlock()

	return c.Status(status).JSON(fiber.Map{"received": true})
}

// list returns captured requests, newest first. Only this machine may read
// them, since they include signatures and payloads.
func (s *devSink) list(c fiber.Ctx) error {
	if ip := net.ParseIP(c.IP()); ip == nil || !ip.IsLoopback() {
		return writeError(c, application.ErrForbidden)
	}
	token := c.Params("token")
	if !sinkTokenPattern.MatchString(token) {
		return writeError(c, application.ErrInvalidInput)
	}
	s.mu.Lock()
	list := append([]sinkRequest{}, s.requests[token]...)
	s.mu.Unlock()
	return c.JSON(fiber.Map{"requests": list})
}
