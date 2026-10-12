// Package httpapi wires the HTTP routes of the QuantumReady API.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const defaultPingTimeout = 2 * time.Second

// Pinger reports whether a dependency (e.g. the PostgreSQL pool) is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Config holds the dependencies of the HTTP API.
type Config struct {
	// Version is reported by GET /api/health.
	Version string
	// DB is checked by GET /api/health.
	DB Pinger
	// PingTimeout bounds the database check. Zero means defaultPingTimeout.
	PingTimeout time.Duration
	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

// NewRouter returns the root handler of the API.
func NewRouter(cfg Config) http.Handler {
	if cfg.PingTimeout <= 0 {
		cfg.PingTimeout = defaultPingTimeout
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Route("/api", func(r chi.Router) {
		r.Get("/health", healthHandler(cfg))
	})
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
