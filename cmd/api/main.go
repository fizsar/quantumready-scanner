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
	full, err := httpapi.ParseAllowlist(os.Getenv("SCAN_FULL_DOMAINS"))
	if err != nil {
		return fmt.Errorf("SCAN_FULL_DOMAINS: %w", err)
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
	accountsOn, err := accountsConfig(os.Getenv, origin)
	if err != nil {
		return err
	}
	openScans, proof, err := publicScanConfig(os.Getenv)
	if err != nil {
		return err
	}
	queueMax, perMinute, err := queueConfig(os.Getenv)
	if err != nil {
		return err
	}
	source, err := sourceConfig(os.Getenv)
	if err != nil {
		return err
	}
	if openScans {
		logger.Warn("PUBLIC_SCAN_OPEN: anyone can scan any domain (basic), with a proof of work")
	}
	if origin == "" {
		logger.Warn("PUBLIC_ORIGIN is empty: no Origin check on POST requests")
	}
	demoOn, _, err := demoConfig(os.Getenv) // the proof of work shares main's verifier (proof)
	if err != nil {
		return err
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
	// Never as a superuser or with BYPASSRLS: every connection is checked,
	// and the API does not start on a privileged one (Apertura step 4).
	store.RequireUnprivileged(poolCfg)
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("configure database pool: %w", err)
	}
	defer pool.Close()
	if reachable, err := store.CheckPoolAtStart(ctx, pool, 30*time.Second); err != nil {
		return err
	} else if !reachable {
		logger.Warn("database unreachable at start: its role is checked on every connection")
	}

	st := store.NewStore(pool)
	refusals := httpapi.NewRefusals()
	cfg := httpapi.Config{
		Version:        version,
		SourceURL:      source,
		DB:             pool,
		Scans:          st,
		Usage:          st,
		Refusals:       refusals,
		Demos:          st,
		DemoAltcha:     proof, // one ALTCHA key and one replay cache for scans and demos
		DemoEnabled:    demoOn,
		Allowlist:      allow,
		FullScanHosts:  full,
		TrustedProxies: trusted,
		Limiter:        httpapi.NewLimiter(lim.perMinute, lim.perDay),
		TargetCooldown: lim.cooldown,
		PublicOrigin:   origin,
		PublicScanOpen: openScans,
		Altcha:         proof,
		PublicQueueMax: queueMax,
		ScansPerMinute: perMinute,
		Support:        os.Getenv("MAIL_SUPPORT"),
		Logger:         logger,
	}
	if accountsOn {
		cfg.Accounts, cfg.Orgs, cfg.Domains = st, st, st
	}
	// Closed beta: invited accounts cannot create organizations unless this is "true".
	cfg.InvitedCanCreateOrgs = os.Getenv("BETA_INVITED_CAN_CREATE_ORGS") == "true"
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewRouter(cfg),
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

	// Refused scan requests (migration 00022), written every 30 s, and once
	// more after the server has stopped.
	go httpapi.FlushRefusalsLoop(ctx, refusals, st.AddScanRefusals, refusalsFlushEvery, logger)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	logger.Info("api listening", "addr", addr, "version", version, "commit", commit,
		"allowlist", allow.Len(), "full_scan_domains", full.Len(), "trusted_proxies", len(trusted),
		"rate_per_minute", lim.perMinute, "rate_per_day", lim.perDay, "target_cooldown", lim.cooldown.String(),
		"demo_enabled", demoOn, "demo_proof_of_work", demoOn && proof != nil, "accounts_enabled", accountsOn,
		"public_scan_open", openScans, "source_url", source)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("api shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	if ferr := refusals.Flush(shutdownCtx, st.AddScanRefusals); ferr != nil {
		logger.Warn("refusals: last flush failed", "err", ferr)
	}
	return err
}

// refusalsFlushEvery: how often the refused scan requests counted in memory
// are written (a crash loses at most this much).
const refusalsFlushEvery = 30 * time.Second

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
