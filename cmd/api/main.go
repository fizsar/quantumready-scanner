// Command api serves the QuantumReady HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
	demoOn, demoProof, err := demoConfig(os.Getenv)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The pool connects lazily: the API starts even if PostgreSQL is down and
	// /api/health reports it.
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("configure database pool: %w", err)
	}
	defer pool.Close()

	st := store.NewStore(pool)
	srv := &http.Server{
		Addr: addr,
		Handler: httpapi.NewRouter(httpapi.Config{
			Version:        version,
			DB:             pool,
			Scans:          st,
			Usage:          st,
			Demos:          st,
			DemoAltcha:     demoProof,
			DemoEnabled:    demoOn,
			Allowlist:      allow,
			TrustedProxies: trusted,
			Limiter:        httpapi.NewLimiter(lim.perMinute, lim.perDay),
			TargetCooldown: lim.cooldown,
			Logger:         logger,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// What /privacy says is deleted: the domains of blocked scan requests (M1)
	// after 30 days, demo requests after 12 months (also with DEMO_ENABLED off:
	// rows stored while it was on must still go).
	go purgeLoop(ctx, "blocked targets", func(ctx context.Context) (int64, error) {
		return st.PurgeBlockedTargets(ctx, blockedTargetsKeepDays)
	}, 24*time.Hour, logger)
	go purgeLoop(ctx, "demo requests", func(ctx context.Context) (int64, error) {
		return st.PurgeDemoRequests(ctx, demoRequestsKeepMonths)
	}, 24*time.Hour, logger)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	logger.Info("api listening", "addr", addr, "version", version, "commit", commit,
		"allowlist", allow.Len(), "trusted_proxies", len(trusted),
		"rate_per_minute", lim.perMinute, "rate_per_day", lim.perDay, "target_cooldown", lim.cooldown.String(),
		"demo_enabled", demoOn, "demo_proof_of_work", demoProof != nil)

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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
