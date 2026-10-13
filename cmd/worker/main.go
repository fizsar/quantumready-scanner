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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
	concurrency, err := envInt("WORKER_CONCURRENCY", 2)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("database pool: %w", err)
	}
	defer pool.Close()

	w, err := worker.New(store.NewStore(pool), scanner.New(scanner.Options{}), worker.Config{
		Concurrency: concurrency,
		Logger:      logger,
	})
	if err != nil {
		return err
	}
	logger.Info("worker started", "version", version, "commit", commit, "concurrency", concurrency)
	err = w.Run(ctx)
	logger.Info("worker stopped")
	return err
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
