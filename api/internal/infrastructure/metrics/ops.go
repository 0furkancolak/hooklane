package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ReadyFunc reports whether the process can do useful work.
type ReadyFunc func(ctx context.Context) error

// OpsServer serves /metrics, /healthz and /readyz on a separate address so
// metrics never share the public API port.
type OpsServer struct {
	srv *http.Server
	log *slog.Logger
}

func NewOpsServer(addr string, reg *prometheus.Registry, ready ReadyFunc, log *slog.Logger) *OpsServer {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", Handler(reg))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeStatus(w, http.StatusOK, "ok", "")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			writeStatus(w, http.StatusServiceUnavailable, "unavailable", err.Error())
			return
		}
		writeStatus(w, http.StatusOK, "ready", "")
	})
	return &OpsServer{
		srv: &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second},
		log: log,
	}
}

// Start listens in the background; a listen failure is logged, not fatal,
// because losing metrics must not stop deliveries.
func (s *OpsServer) Start() {
	go func() {
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("ops server stopped", "error", err)
		}
	}()
}

func (s *OpsServer) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func writeStatus(w http.ResponseWriter, code int, status, reason string) {
	body := map[string]string{"status": status}
	if reason != "" {
		body["reason"] = reason
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
