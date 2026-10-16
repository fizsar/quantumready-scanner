package worker

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/domains"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// DomainQueue is what the domain checker needs from the store (role
// qr_worker: the verification columns of domains and nothing else).
type DomainQueue interface {
	ClaimDomainChecks(ctx context.Context, arg store.ClaimDomainChecksParams) ([]store.ClaimDomainChecksRow, error)
	MarkDomainFound(ctx context.Context, id uuid.UUID) error
	MarkDomainAbsent(ctx context.Context, arg store.MarkDomainAbsentParams) error
	MarkDomainCheckError(ctx context.Context, arg store.MarkDomainCheckErrorParams) error
}

// TXTLookup reads TXT records (dnscheck.Checker: authoritative servers only).
type TXTLookup interface {
	TXT(ctx context.Context, name string) ([]string, error)
}

// DomainChecker verifies domains: the first check a member asks for, then
// weekly (daily while the record is missing, until the grace period ends).
type DomainChecker struct {
	Queue   DomainQueue
	DNS     TXTLookup
	Logger  *slog.Logger
	Poll    time.Duration // default 5 s
	Batch   int           // default 5
	Timeout time.Duration // per domain; default 20 s
	Backoff time.Duration // after a DNS failure; default 5 min
}

func (d *DomainChecker) defaults() {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Poll <= 0 {
		d.Poll = 5 * time.Second
	}
	if d.Batch <= 0 {
		d.Batch = 5
	}
	if d.Timeout <= 0 {
		d.Timeout = 20 * time.Second
	}
	if d.Backoff <= 0 {
		d.Backoff = 5 * time.Minute
	}
}

// Run checks until ctx is done.
func (d *DomainChecker) Run(ctx context.Context) {
	d.defaults()
	t := time.NewTicker(d.Poll)
	defer t.Stop()
	for {
		if _, err := d.Once(ctx); err != nil && ctx.Err() == nil {
			d.Logger.Warn("domains: claim failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Once checks one batch and returns how many domains it checked.
func (d *DomainChecker) Once(ctx context.Context) (int, error) {
	d.defaults()
	rows, err := d.Queue.ClaimDomainChecks(ctx, store.ClaimDomainChecksParams{
		LeaseSeconds:       (d.Timeout + time.Minute).Seconds(),
		RecheckSeconds:     store.DomainRecheck.Seconds(),
		LostRecheckSeconds: store.DomainLostRecheck.Seconds(),
		MaxRows:            int32(d.Batch), //nolint:gosec // small
	})
	if err != nil {
		return 0, err
	}
	for _, r := range rows {
		d.check(ctx, r)
	}
	return len(rows), nil
}

// Outcome of a check, for logs and tests.
const (
	OutcomeFound  = "found"
	OutcomeAbsent = "absent"
	OutcomeError  = "dns_unavailable"
)

func (d *DomainChecker) check(ctx context.Context, r store.ClaimDomainChecksRow) {
	cctx, cancel := context.WithTimeout(ctx, d.Timeout)
	txt, err := d.DNS.TXT(cctx, domains.RecordName(r.Name))
	cancel()
	outcome := OutcomeAbsent
	switch {
	case err != nil:
		outcome = OutcomeError
	case matches(txt, domains.RecordValue(r.Token)):
		outcome = OutcomeFound
	}
	switch outcome {
	case OutcomeFound:
		err = d.Queue.MarkDomainFound(ctx, r.ID)
	case OutcomeAbsent:
		err = d.Queue.MarkDomainAbsent(ctx, store.MarkDomainAbsentParams{ID: r.ID, GraceSeconds: store.DomainGrace.Seconds()})
	default:
		code := OutcomeError
		err = d.Queue.MarkDomainCheckError(ctx, store.MarkDomainCheckErrorParams{ID: r.ID, Error: &code, BackoffSeconds: d.Backoff.Seconds()})
	}
	log := d.Logger.With("domain", r.ID, "outcome", outcome, "was", r.Status)
	if err != nil {
		log.Error("domains: cannot record the check", "err", err)
		return
	}
	log.Info("domains: checked")
}

func matches(txt []string, want string) bool {
	for _, s := range txt {
		if strings.TrimSpace(s) == want {
			return true
		}
	}
	return false
}
