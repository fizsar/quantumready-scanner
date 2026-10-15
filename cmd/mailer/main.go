// Command mailer sends the queued transactional emails (email_outbox) over
// SMTP. It is separate from the worker on purpose: the worker relaxes
// crypto/tls for scanning (GODEBUG), which must not weaken the SMTP
// connection, and the mailer's database role (qr_mailer) can only touch the
// email outbox, whose rows are already rendered (docs/security.md).
//
// Environment: DATABASE_URL; SMTP_HOST, SMTP_PORT, SMTP_SECURITY
// (starttls | tls | none, the last only for a local catcher such as
// Mailpit), SMTP_USERNAME, SMTP_PASSWORD; MAIL_FROM, MAIL_REPLY_TO.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fizsar/quantumready-saas/internal/mail"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// Set at build time with -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "dev"
	commit  = "unknown"
)

// heartbeatMaxAge: a batch is at most 10 emails × 30 s SMTP timeout; the loop
// beats after every batch, idle ones included.
const heartbeatMaxAge = 6 * time.Minute

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
		logger.Error("mailer stopped", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	smtpCfg, err := smtpConfig(os.Getenv)
	if err != nil {
		return err
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("DATABASE_URL: %w", err)
	}
	if err := store.UseRole(cfg, store.RoleMailer); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("database pool: %w", err)
	}
	defer pool.Close()

	hb := heartbeatPath()
	m := &mail.Mailer{
		Outbox: store.NewStore(pool).Queries,
		SMTP:   smtpCfg,
		Logger: logger,
		Heartbeat: func() {
			if err := writeHeartbeat(hb, time.Now()); err != nil {
				logger.Warn("mailer: cannot write heartbeat", "err", err)
			}
		},
	}
	logger.Info("mailer started", "version", version, "commit", commit, "smtp_host", smtpCfg.Host, "security", smtpCfg.Security)
	return m.Run(ctx)
}

func smtpConfig(getenv func(string) string) (mail.SMTPConfig, error) {
	port, err := strconv.Atoi(getenv("SMTP_PORT"))
	if err != nil {
		return mail.SMTPConfig{}, fmt.Errorf("SMTP_PORT: %w", err)
	}
	c := mail.SMTPConfig{
		Host: getenv("SMTP_HOST"), Port: port, Security: getenv("SMTP_SECURITY"),
		Username: getenv("SMTP_USERNAME"), Password: getenv("SMTP_PASSWORD"),
		From: getenv("MAIL_FROM"), ReplyTo: getenv("MAIL_REPLY_TO"),
	}
	return c, c.Validate()
}

func heartbeatPath() string {
	if p := os.Getenv("MAILER_HEARTBEAT_FILE"); p != "" {
		return p
	}
	return "/tmp/qr-mailer-heartbeat"
}

func writeHeartbeat(path string, t time.Time) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(t.UTC().Format(time.RFC3339Nano)), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func checkHeartbeat(path string, now time.Time, maxAge time.Duration) error {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("no heartbeat: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(b)))
	if err != nil {
		return fmt.Errorf("bad heartbeat: %w", err)
	}
	if age := now.Sub(t); age > maxAge {
		return fmt.Errorf("send loop last completed %v ago (max %v)", age.Round(time.Second), maxAge)
	}
	return nil
}
