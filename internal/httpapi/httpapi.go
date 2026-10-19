// Package httpapi wires the HTTP routes of the QuantumReady API.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const defaultPingTimeout = 2 * time.Second

// DefaultTargetCooldown is the window in which a new request for the same
// target returns the latest scan instead of starting another.
const DefaultTargetCooldown = 2 * time.Minute

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

	// Scans enables /api/scans; nil leaves only the health endpoint.
	Scans ScanStore
	// Allowlist holds the only hosts that may be scanned (SCAN_ALLOWLIST).
	Allowlist *Allowlist
	// Limiter rate-limits scan creation per client; nil means 5/min and 30/day.
	Limiter *Limiter
	// TrustedProxies are the addresses whose X-Forwarded-For is believed (the
	// Caddy network). Empty: the header is ignored.
	TrustedProxies []netip.Prefix
	// TargetCooldown defaults to DefaultTargetCooldown.
	TargetCooldown time.Duration

	// Usage enables the M1 usage counters (POST /api/e and the 403 count);
	// nil disables them.
	Usage UsageStore
	// UsageLimiter rate-limits POST /api/e per client; nil means 60/min and
	// 1,000/day.
	UsageLimiter *Limiter

	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

type api struct {
	cfg          Config
	usageLimiter *Limiter
}

// NewRouter returns the root handler of the API.
func NewRouter(cfg Config) http.Handler {
	if cfg.PingTimeout <= 0 {
		cfg.PingTimeout = defaultPingTimeout
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Limiter == nil {
		cfg.Limiter = NewLimiter(5, 30)
	}
	if cfg.TargetCooldown <= 0 {
		cfg.TargetCooldown = DefaultTargetCooldown
	}
	if cfg.UsageLimiter == nil {
		cfg.UsageLimiter = NewLimiter(60, 1000)
	}
	a := &api{cfg: cfg, usageLimiter: cfg.UsageLimiter}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	// Serve HEAD with the GET handlers (net/http drops the body).
	r.Use(middleware.GetHead)
	r.Use(apiHeaders)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) { writeError(w, r, http.StatusNotFound, "not_found") })
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusMethodNotAllowed, "method_not_allowed")
	})
	r.Route("/api", func(r chi.Router) {
		r.Get("/health", healthHandler(cfg))
		if cfg.Scans != nil {
			r.Get("/targets", a.listTargets)
			r.Post("/scans", a.createScan)
			r.Get("/scans/{id}", a.getScan)
		}
		if cfg.Usage != nil {
			r.Post("/e", a.usageEvent)
		}
	})
	return r
}

// apiHeaders: JSON only, never framed, no CORS (no Access-Control-* headers:
// browsers block cross-origin reads, and createScan requires a JSON body).
func apiHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Vary", "Accept-Language")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
