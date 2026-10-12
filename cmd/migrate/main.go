// Command migrate applies pending database migrations and exits. In production
// it runs as a one-shot compose service before the API starts.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/fizsar/quantumready-saas/migrations"
)

// Set at build time with -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("migrate failed", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	logger.Info("migrate starting", "version", version, "commit", commit)
	results, err := migrations.Up(ctx, db)
	for _, r := range results {
		logger.Info("migration applied", "version", r.Source.Version, "file", path.Base(r.Source.Path), "duration", r.Duration)
	}
	if err != nil {
		return err
	}
	logger.Info("migrations up to date", "applied", len(results))
	return nil
}
