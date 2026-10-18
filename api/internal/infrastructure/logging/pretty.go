// Package logging builds the process logger: JSON for machines, a compact
// coloured line for people reading a terminal during development.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Format selects the log encoding.
type Format string

const (
	FormatJSON   Format = "json"
	FormatPretty Format = "pretty"
)

// New returns a logger writing to w. Pretty output uses colour when color is
// true.
func New(w io.Writer, format Format, color bool) *slog.Logger {
	if format == FormatPretty {
		return slog.New(&prettyHandler{w: w, color: color, mu: &sync.Mutex{}})
	}
	return slog.New(slog.NewJSONHandler(w, nil))
}

// ColorEnabled reports whether coloured output is appropriate: LOG_COLOR=1
// forces it, NO_COLOR disables it, otherwise it follows whether f is a terminal.
func ColorEnabled(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if v := os.Getenv("LOG_COLOR"); v != "" {
		return v == "1" || strings.EqualFold(v, "true")
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

const (
	reset  = "\x1b[0m"
	dim    = "\x1b[2m"
	bold   = "\x1b[1m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	blue   = "\x1b[34m"
	cyan   = "\x1b[36m"
)

type prettyHandler struct {
	w     io.Writer
	color bool
	mu    *sync.Mutex
	attrs []slog.Attr
}

func (h *prettyHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *prettyHandler) WithGroup(string) slog.Handler { return h }

func (h *prettyHandler) paint(color, text string) string {
	if !h.color {
		return text
	}
	return color + text + reset
}

func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]string{}
	var order []string
	add := func(a slog.Attr) bool {
		if _, seen := attrs[a.Key]; !seen {
			order = append(order, a.Key)
		}
		attrs[a.Key] = a.Value.Resolve().String()
		return true
	}
	for _, a := range h.attrs {
		add(a)
	}
	r.Attrs(add)

	var b strings.Builder
	b.WriteString(h.paint(dim, r.Time.Format("15:04:05")))
	b.WriteByte(' ')
	b.WriteString(h.level(r.Level))
	b.WriteByte(' ')

	used := map[string]bool{}
	if r.Message == "request" && attrs["method"] != "" {
		b.WriteString(h.requestLine(attrs, used))
	} else {
		b.WriteString(r.Message)
	}

	for _, key := range order {
		if used[key] || key == "request_id" {
			continue
		}
		b.WriteString("  ")
		b.WriteString(h.paint(dim, key+"="))
		b.WriteString(quote(attrs[key]))
	}
	if id := attrs["request_id"]; id != "" {
		b.WriteString("  ")
		b.WriteString(h.paint(dim, "req="+shorten(id)))
	}
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *prettyHandler) level(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return h.paint(bold+red, "ERR ")
	case l >= slog.LevelWarn:
		return h.paint(yellow, "WARN")
	case l >= slog.LevelInfo:
		return h.paint(blue, "INFO")
	default:
		return h.paint(dim, "DBG ")
	}
}

// requestLine renders "GET  /v1/projects  200  3ms" with the status coloured.
func (h *prettyHandler) requestLine(attrs map[string]string, used map[string]bool) string {
	for _, k := range []string{"method", "path", "route", "status", "duration_ms"} {
		used[k] = true
	}
	status := attrs["status"]
	color := green
	switch {
	case strings.HasPrefix(status, "5"):
		color = red
	case strings.HasPrefix(status, "4"):
		color = yellow
	case strings.HasPrefix(status, "3"):
		color = cyan
	}
	path := attrs["path"]
	if route := attrs["route"]; route != "" && route != "unmatched" && route != path {
		path = route
	}
	return fmt.Sprintf("%s %s %s %s",
		h.paint(bold, fmt.Sprintf("%-6s", attrs["method"])),
		fmt.Sprintf("%-38s", path),
		h.paint(color, status),
		h.paint(dim, attrs["duration_ms"]+"ms"))
}

func shorten(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func quote(v string) string {
	if v == "" || strings.ContainsAny(v, " \t\"") {
		return fmt.Sprintf("%q", v)
	}
	return v
}

var _ slog.Handler = (*prettyHandler)(nil)
