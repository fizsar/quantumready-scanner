// Package httpapi wires the HTTP routes of the QuantumReady API.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"net/netip"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fizsar/quantumready-saas/internal/altcha"
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
	// PublicScanOpen lets anyone scan any domain (basic). False: only the
	// Allowlist (PUBLIC_SCAN_OPEN; opened after the security review).
	PublicScanOpen bool
	// Altcha issues and checks the proof of work of open public scans (GET
	// /api/altcha); nil: none required (cmd/api refuses to open without it).
	Altcha *altcha.Verifier
	// PublicLimits apply to open public scans, on top of Limiter (per IP).
	PublicLimits PublicLimits
	// PublicQueueMax: queued public scans above which new ones get 429 busy
	// (QR_PUBLIC_QUEUE_MAX; default 100, about five minutes of work).
	PublicQueueMax int
	// ScansPerMinute is the worker's pace (WORKER_MAX_SCANS_PER_MINUTE,
	// default 20), for the queue wait shown to visitors.
	ScansPerMinute int

	// Accounts enables /api/auth; it needs PublicOrigin.
	Accounts AccountStore
	// Orgs (with Accounts) enables /api/org.
	Orgs OrgStore
	// Domains (with Orgs) enables /api/org/domains.
	Domains DomainStore
	// InvitedCanCreateOrgs lets accounts created by invitation create their
	// own organizations (false during the closed beta).
	InvitedCanCreateOrgs bool
	// PublicOrigin is the site's origin ("https://quantumready.fizsar.com"):
	// links in emails, and state-changing requests from browsers must come
	// from it.
	PublicOrigin string
	// Support is the address shown in emails (default soporte@fizsar.com).
	Support    string
	AuthLimits AuthLimits
	// Async runs work after the response (default: a goroutine). Tests make
	// it synchronous.
	Async func(func())

	// Usage enables the M1 usage counters (POST /api/e and the 403 count);
	// nil disables them.
	Usage UsageStore
	// UsageLimiter rate-limits POST /api/e per client; nil means 60/min and
	// 1,000/day.
	UsageLimiter *Limiter

	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

// PublicLimits of open public scanning. Nil fields take the defaults.
type PublicLimits struct {
	PerDomain *Limiter // new scans per registrable domain: 10/day
	Global    *Limiter // new public scans in all: 30/min, 1000/day
	Challenge *Limiter // ALTCHA challenges per client: 60/min, 2000/day
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
	if cfg.PublicQueueMax <= 0 {
		cfg.PublicQueueMax = 100
	}
	if cfg.ScansPerMinute <= 0 {
		cfg.ScansPerMinute = 20
	}
	if cfg.PublicLimits.PerDomain == nil {
		cfg.PublicLimits.PerDomain = NewWindowLimiter(10, 24*time.Hour)
	}
	if cfg.PublicLimits.Global == nil {
		cfg.PublicLimits.Global = NewLimiter(30, 1000)
	}
	if cfg.PublicLimits.Challenge == nil {
		cfg.PublicLimits.Challenge = NewLimiter(60, 2000)
	}
	if cfg.UsageLimiter == nil {
		cfg.UsageLimiter = NewLimiter(60, 1000)
	}
	a := &api{cfg: cfg, usageLimiter: cfg.UsageLimiter}
	a.authDefaults()

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	// Serve HEAD with the GET handlers (net/http drops the body).
	r.Use(middleware.GetHead)
	r.Use(apiHeaders)
	if cfg.PublicOrigin != "" {
		r.Use(checkOrigin(cfg.PublicOrigin))
	}
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
			if cfg.Altcha != nil {
				r.Get("/altcha", a.altchaChallenge)
			}
		}
		if cfg.Accounts != nil && cfg.Orgs != nil {
			r.Route("/org", func(r chi.Router) {
				r.Use(a.loadSession, requireSession, a.requireOrg)
				r.Get("/", a.getOrg)
				r.Patch("/", a.renameOrg)
				r.Get("/members", a.listMembers)
				r.Patch("/members/{userID}", a.changeRole)
				r.Delete("/members/{userID}", a.removeMember)
				r.Post("/leave", a.leaveOrg)
				r.Post("/invitations", a.createInvitation)
				r.Get("/invitations", a.listInvitations)
				r.Delete("/invitations/{invitationID}", a.revokeInvitation)
				if cfg.Domains != nil {
					r.Get("/domains", a.listDomains)
					r.Post("/domains", a.addDomain)
					r.Get("/domains/{domainID}", a.getDomain)
					r.Delete("/domains/{domainID}", a.deleteDomain)
					r.Patch("/domains/{domainID}", a.patchDomain)
					r.Post("/domains/{domainID}/verify", a.verifyDomain)
					if cfg.Scans != nil {
						r.Get("/scans", a.listOrgScans)
						r.Post("/scans", a.createOrgScan)
						r.Get("/scans/{scanID}", a.getOrgScan)
					}
				}
			})
			r.Route("/organizations", func(r chi.Router) {
				r.Use(a.loadSession, requireSession)
				r.Post("/", a.createOrganization)
				r.Post("/{orgID}/switch", a.switchOrganization)
			})
			r.Route("/invitations", func(r chi.Router) {
				r.Use(a.loadSession)
				r.Post("/preview", a.previewInvitation)
				r.With(requireSession).Post("/accept", a.acceptInvitation)
			})
		}
		if cfg.Accounts != nil {
			r.Route("/auth", func(r chi.Router) {
				r.Use(a.loadSession)
				r.Post("/register", a.register)
				r.Post("/login", a.login)
				r.Post("/verify-email", a.verifyEmail)
				r.Post("/resend-verification", a.resendVerification)
				r.Post("/forgot-password", a.forgotPassword)
				r.Post("/reset-password", a.resetPassword)
				r.Group(func(r chi.Router) {
					r.Use(requireSession)
					r.Get("/me", a.me)
					r.Post("/logout", a.logout)
					r.Post("/change-password", a.changePassword)
				})
			})
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

// decodeJSON reads a JSON body of at most limit bytes into v. On failure it
// writes the error and returns false.
func decodeJSON(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		// Also keeps cross-site HTML forms out: they cannot send JSON.
		writeError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, r, http.StatusRequestEntityTooLarge, "body_too_large")
		} else {
			writeError(w, r, http.StatusBadRequest, "invalid_json")
		}
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
