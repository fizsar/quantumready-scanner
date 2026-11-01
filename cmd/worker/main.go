// Command worker processes the scan queue stored in PostgreSQL.
//
// The godebug settings let crypto/tls complete handshakes with very old
// servers (SHA-1 signatures in TLS 1.2, RSA keys under 1024 bits) so they can
// be analyzed; the scanner only observes those connections and never trusts
// them. They apply to the whole process, so the worker refuses a TLS
// connection to PostgreSQL (requirePlaintextDB): the database is reached over
// the internal Docker network. CI checks that api and migrate never carry
// these settings and that this binary does.
//
//go:debug tlssha1=1
//go:debug rsa1024min=0
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fizsar/quantumready-saas/internal/dnscheck"
	"github.com/fizsar/quantumready-saas/internal/scanner"
	"github.com/fizsar/quantumready-saas/internal/store"
	"github.com/fizsar/quantumready-saas/internal/worker"
)

// Set at build time with -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := checkHeartbeat(heartbeatPath(), time.Now(), heartbeatMaxAge); err != nil {
			fmt.Fprintln(os.Stderr, "unhealthy:", err)
			os.Exit(1)
		}
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("worker stopped", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("DATABASE_URL: %w", err)
	}
	if err := requirePlaintextDB(cfg.ConnConfig); err != nil {
		return err
	}
	// Least privilege: scan queue, findings and the verification columns of
	// domains (docs/security.md).
	if err := store.UseRole(cfg, store.RoleWorker); err != nil {
		return err
	}
	// Never as a superuser or with BYPASSRLS (Apertura step 4).
	store.RequireUnprivileged(cfg)
	concurrency, err := envInt("WORKER_CONCURRENCY", 2)
	if err != nil {
		return err
	}
	perMinute, err := maxScansPerMinute(os.Getenv)
	if err != nil {
		return err
	}
	queueTimeout, err := publicQueueTimeout(os.Getenv)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("database pool: %w", err)
	}
	if reachable, err := store.CheckPoolAtStart(ctx, pool, 30*time.Second); err != nil {
		pool.Close()
		return err
	} else if !reachable {
		logger.Warn("database unreachable at start: its role is checked on every connection")
	}
	// Close the pool on the way out, but never hang on it: a stuck slot may
	// still hold a connection when the worker gives up on it.
	defer func() {
		closed := make(chan struct{})
		go func() { pool.Close(); close(closed) }()
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			logger.Warn("worker: database pool did not close within 5 s")
		}
	}()

	hbPath := heartbeatPath()
	scanOpts, err := scannerOptions(os.Getenv)
	if err != nil {
		return err
	}
	w, err := worker.New(store.NewStore(pool), scanner.New(scanOpts), worker.Config{
		Concurrency:        concurrency,
		MaxPerMinute:       perMinute,
		PublicQueueTimeout: queueTimeout,
		Logger:             logger,
	})
	if err != nil {
		return err
	}
	// Domain verification: TXT records read from the authoritative servers.
	checker := &worker.DomainChecker{Queue: store.NewStore(pool).Queries, DNS: &dnscheck.Checker{}, Logger: logger}
	go checker.Run(ctx)
	// Public scans are kept 30 days (/privacy): purged at start and daily.
	go worker.PurgeLoop(ctx, "expired public scans", store.NewStore(pool).PurgeExpiredPublicScans, 24*time.Hour, logger)

	stuck := make(chan []int, 1)
	hb := &heartbeatWriter{path: hbPath, every: heartbeatEvery, stale: w.Lease(), slots: w.SlotBeats, now: time.Now, log: logger,
		exitAfter: stuckExitAfter, onStuck: func(slots []int) {
			select {
			case stuck <- slots:
			default:
			}
		}}
	go hb.run(ctx)
	logger.Info("worker started", "version", version, "commit", commit, "concurrency", concurrency, "intensity", scanOpts.Intensity,
		"max_scans_per_minute", perMinute, "public_queue_timeout", queueTimeout.String())
	// In-flight scans get the worker's shutdown timeout (50 s) plus a margin.
	err = supervise(ctx, w.Run, stuck, 60*time.Second)
	logger.Info("worker stopped")
	return err
}

// scannerOptions is the scanner configuration. Since milestone 3.1 the
// intensity comes with each queued scan (basic for public scans, full only
// for verified domains of an organization), so nothing is read from the
// environment: the scanner defaults stand, in particular the anti-SSRF
// address policy cannot be changed (only cmd/qrscan -lab can). getenv is kept
// so the test can prove that no variable changes it.
func scannerOptions(_ func(string) string) (scanner.Options, error) {
	return scanner.Options{}, nil
}

// requirePlaintextDB refuses any configuration that would use TLS to reach
// PostgreSQL, including the libpq default sslmode=prefer.
func requirePlaintextDB(c *pgx.ConnConfig) error {
	tls := c.TLSConfig != nil
	for _, fb := range c.Fallbacks {
		tls = tls || fb.TLSConfig != nil
	}
	if tls {
		return errors.New("DATABASE_URL must set sslmode=disable: this binary relaxes crypto/tls for scanning " +
			"(GODEBUG tlssha1=1, rsa1024min=0), which would also weaken a TLS connection to PostgreSQL")
	}
	return nil
}

// maxScansPerMinute is WORKER_MAX_SCANS_PER_MINUTE: scans started per
// minute by all workers together (default 20, 1–600), so the server's
// address never looks like an Internet sweep.
func maxScansPerMinute(getenv func(string) string) (int, error) {
	v := getenv("WORKER_MAX_SCANS_PER_MINUTE")
	if v == "" {
		return 20, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 600 {
		return 0, fmt.Errorf("WORKER_MAX_SCANS_PER_MINUTE must be an integer between 1 and 600, got %q", v)
	}
	return n, nil
}

// publicQueueTimeout is QR_PUBLIC_QUEUE_TIMEOUT: how long a public scan may
// wait in the queue before failing with queue_timeout (default 15m, 1m–24h).
func publicQueueTimeout(getenv func(string) string) (time.Duration, error) {
	v := getenv("QR_PUBLIC_QUEUE_TIMEOUT")
	if v == "" {
		return 15 * time.Minute, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < time.Minute || d > 24*time.Hour {
		return 0, fmt.Errorf("QR_PUBLIC_QUEUE_TIMEOUT must be a duration between 1m and 24h (e.g. 15m), got %q", v)
	}
	return d, nil
}

func envInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 32 {
		return 0, fmt.Errorf("%s must be an integer between 1 and 32", key)
	}
	return n, nil
}
