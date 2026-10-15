// Command api serves the QuantumReady HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fizsar/quantumready-saas/internal/httpapi"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// Set at build time with -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := healthcheck(envOr("API_ADDR", "127.0.0.1:8080")); err != nil {
			fmt.Fprintln(os.Stderr, "unhealthy:", err)
			os.Exit(1)
		}
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("api stopped", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	addr := envOr("API_ADDR", "127.0.0.1:8080")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	allow, err := httpapi.ParseAllowlist(os.Getenv("SCAN_ALLOWLIST"))
	if err != nil {
		return err
	}
	if allow.Len() == 0 {
		logger.Warn("SCAN_ALLOWLIST is empty: every scan request will be refused")
	}
	trusted, err := httpapi.ParsePrefixes(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}
	lim, err := loadLimits(os.Getenv)
	if err != nil {
		return err
	}
	origin, err := publicOrigin(os.Getenv("PUBLIC_ORIGIN"))
	if err != nil {
		return err
	}
	if origin == "" {
		logger.Warn("PUBLIC_ORIGIN is empty: accounts (/api/auth) are disabled")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The pool connects lazily: the API starts even if PostgreSQL is down and
	// /api/health reports it.
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("DATABASE_URL: %w", err)
	}
	// Every query runs as qr_app, under row-level security (docs/security.md).
	if err := store.UseRole(poolCfg, store.RoleApp); err != nil {
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("configure database pool: %w", err)
	}
	defer pool.Close()

	st := store.NewStore(pool)
	cfg := httpapi.Config{
		Version:        version,
		DB:             pool,
		Scans:          st,
		Allowlist:      allow,
		TrustedProxies: trusted,
		Limiter:        httpapi.NewLimiter(lim.perMinute, lim.perDay),
		TargetCooldown: lim.cooldown,
		PublicOrigin:   origin,
		Support:        os.Getenv("MAIL_SUPPORT"),
		Logger:         logger,
	}
	if origin != "" {
		cfg.Accounts = st
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewRouter(cfg),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	logger.Info("api listening", "addr", addr, "version", version, "commit", commit,
		"allowlist", allow.Len(), "trusted_proxies", len(trusted),
		"rate_per_minute", lim.perMinute, "rate_per_day", lim.perDay, "target_cooldown", lim.cooldown.String())

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("api shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// publicOrigin checks PUBLIC_ORIGIN: scheme and host only, https except for
// localhost (development).
func publicOrigin(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("PUBLIC_ORIGIN %q: want scheme://host[:port]", s)
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"
	if u.Scheme != "https" && (u.Scheme != "http" || !local) {
		return "", fmt.Errorf("PUBLIC_ORIGIN %q: must be https (http only for localhost)", s)
	}
	return u.Scheme + "://" + strings.ToLower(u.Host), nil // as browsers send it
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
