package mail

import (
	"context"
	"log/slog"
	"time"

	"github.com/fizsar/quantumready-saas/internal/store"
)

// Outbox is what the mailer needs from the store (role qr_mailer: the email
// outbox and nothing else).
type Outbox interface {
	ClaimEmails(ctx context.Context, arg store.ClaimEmailsParams) ([]store.EmailOutbox, error)
	MarkEmailSent(ctx context.Context, id int64) error
	MarkEmailRetry(ctx context.Context, arg store.MarkEmailRetryParams) error
	MarkEmailFailed(ctx context.Context, arg store.MarkEmailFailedParams) error
	PurgeEmails(ctx context.Context, olderThanSeconds float64) (int64, error)
}

// Mailer sends the queued emails.
type Mailer struct {
	Outbox    Outbox
	SMTP      SMTPConfig
	Send      func(ctx context.Context, cfg SMTPConfig, to string, msg []byte) error // default Send
	Logger    *slog.Logger
	Poll      time.Duration // default 5 s
	Batch     int           // default 10
	Lease     time.Duration // a claimed email is retried after this if the mailer dies; default 5 min
	Keep      time.Duration // sent and failed emails are purged after this; default 7 days
	Heartbeat func()        // called after every loop iteration without a queue error
}

// Retry delays after a temporary failure; attempt 5 gives up.
var retryDelays = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}

const maxAttempts = 5

// Run sends until ctx is done.
func (m *Mailer) Run(ctx context.Context) error {
	m.defaults()
	t := time.NewTicker(m.Poll)
	defer t.Stop()
	lastPurge := time.Time{}
	for {
		ok := m.Once(ctx)
		if time.Since(lastPurge) > time.Hour {
			if n, err := m.Outbox.PurgeEmails(ctx, m.Keep.Seconds()); err != nil {
				m.Logger.Warn("mailer: purge failed", "err", err)
				ok = false
			} else {
				lastPurge = time.Now()
				if n > 0 {
					m.Logger.Info("mailer: purged old emails", "count", n)
				}
			}
		}
		if ok && m.Heartbeat != nil {
			m.Heartbeat()
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

func (m *Mailer) defaults() {
	if m.Send == nil {
		m.Send = Send
	}
	if m.Logger == nil {
		m.Logger = slog.Default()
	}
	if m.Poll <= 0 {
		m.Poll = 5 * time.Second
	}
	if m.Batch <= 0 {
		m.Batch = 10
	}
	if m.Lease <= 0 {
		m.Lease = 5 * time.Minute
	}
	if m.Keep <= 0 {
		m.Keep = 7 * 24 * time.Hour
	}
}

// Once sends one batch; it returns false if the queue could not be read.
// Addresses and contents are never logged: only the email ID and template.
func (m *Mailer) Once(ctx context.Context) bool {
	m.defaults()
	emails, err := m.Outbox.ClaimEmails(ctx, store.ClaimEmailsParams{MaxRows: int32(m.Batch), LeaseSeconds: m.Lease.Seconds()}) //nolint:gosec // small
	if err != nil {
		m.Logger.Warn("mailer: claim failed", "err", err)
		return false
	}
	for _, e := range emails {
		log := m.Logger.With("email", e.ID, "template", e.Template, "attempt", e.Attempts)
		msg, err := Message(m.SMTP, e.ToEmail, e.Subject, e.BodyText, time.Now())
		if err == nil {
			err = m.Send(ctx, m.SMTP, e.ToEmail, msg)
		}
		switch {
		case err == nil:
			if err := m.Outbox.MarkEmailSent(ctx, e.ID); err != nil {
				log.Error("mailer: sent but not marked (it may be sent twice)", "err", err)
			} else {
				log.Info("mailer: sent")
			}
		case Permanent(err) || int(e.Attempts) >= maxAttempts:
			msg := err.Error()
			log.Warn("mailer: giving up", "err", msg)
			if err := m.Outbox.MarkEmailFailed(ctx, store.MarkEmailFailedParams{ID: e.ID, LastError: &msg}); err != nil {
				log.Error("mailer: cannot mark failed", "err", err)
			}
		default:
			msg := err.Error()
			delay := retryDelays[min(int(e.Attempts)-1, len(retryDelays)-1)]
			log.Warn("mailer: will retry", "err", msg, "in", delay)
			if err := m.Outbox.MarkEmailRetry(ctx, store.MarkEmailRetryParams{ID: e.ID, DelaySeconds: delay.Seconds(), LastError: &msg}); err != nil {
				log.Error("mailer: cannot schedule retry", "err", err)
			}
		}
	}
	return true
}
