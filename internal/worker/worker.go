// Package worker runs queued scans: it claims a scan from the PostgreSQL
// queue, runs the scanner, scores the result and stores it with its findings.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fizsar/quantumready-saas/internal/scanner"
	"github.com/fizsar/quantumready-saas/internal/scoring"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// Scanner is what the worker needs from internal/scanner.
type Scanner interface {
	Scan(ctx context.Context, host string, port int) (*scanner.Result, error)
	// ScanTimeout bounds one Scan; the lease must outlast it.
	ScanTimeout() time.Duration
}

// Config tunes a Worker. Zero values select the defaults.
type Config struct {
	// Concurrency is the number of scans run in parallel. Default 2.
	Concurrency int
	// Lease is how long a claim lasts. It must exceed the scanner's timeout
	// plus LeaseMargin, so a healthy worker never loses a scan. Default 90 s.
	Lease time.Duration
	// LeaseMargin is the minimum slack between the scanner's timeout and the
	// lease, for scoring and storing the result. Default 15 s.
	LeaseMargin time.Duration
	// RetryDelay is the wait before retrying a scan after a transient error
	// (timeout, connection reset…). Default 30 s.
	RetryDelay time.Duration
	// MaxRetries is how many times a transient error is retried. Default 1;
	// negative disables retries.
	MaxRetries int
	// PollInterval is the wait when the queue is empty. Default 1 s.
	PollInterval time.Duration
	// ShutdownTimeout bounds the wait for in-flight scans on stop. Scans still
	// running then keep their lease and another worker retries them after it
	// expires. Default 50 s.
	ShutdownTimeout time.Duration
	Logger          *slog.Logger
}

// Worker processes the scan queue.
type Worker struct {
	st  *store.Store
	sc  Scanner
	cfg Config
	// Per slot, when its loop last finished an iteration without error
	// (Unix nanoseconds); see SlotBeats. Slots only store their own entry.
	beats []atomic.Int64
}

// New returns a Worker with cfg, filling in defaults. It refuses a lease that
// does not outlast the scanner's timeout plus LeaseMargin: such a worker could
// lose healthy scans to another worker.
func New(st *store.Store, sc Scanner, cfg Config) (*Worker, error) {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
	if cfg.Lease <= 0 {
		cfg.Lease = 90 * time.Second
	}
	if cfg.LeaseMargin <= 0 {
		cfg.LeaseMargin = 15 * time.Second
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 30 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 1
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 50 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if minLease := sc.ScanTimeout() + cfg.LeaseMargin; cfg.Lease <= minLease {
		return nil, fmt.Errorf("worker: lease %v must exceed the scan timeout %v plus a margin of %v (> %v)",
			cfg.Lease, sc.ScanTimeout(), cfg.LeaseMargin, minLease)
	}
	w := &Worker{st: st, sc: sc, cfg: cfg, beats: make([]atomic.Int64, cfg.Concurrency)}
	w.resetBeats()
	return w, nil
}

func (w *Worker) resetBeats() {
	now := time.Now().UnixNano()
	for i := range w.beats {
		w.beats[i].Store(now)
	}
}

// SlotBeats returns, per slot, when its loop last finished an iteration
// without error, idle ones included (New or Run's start before the first).
// A slot beats about every PollInterval while it waits for work, and after
// each scan, which the scanner's timeout keeps under the lease: a slot silent
// for longer than Lease is stuck. Failing iterations (database down) do not
// beat, so a broken queue looks stuck too. It never blocks the slots: the
// caller (the healthcheck writer in cmd/worker) does any I/O.
func (w *Worker) SlotBeats() []time.Time {
	out := make([]time.Time, len(w.beats))
	for i := range w.beats {
		out[i] = time.Unix(0, w.beats[i].Load())
	}
	return out
}

// Lease returns how long a claim lasts.
func (w *Worker) Lease() time.Duration { return w.cfg.Lease }

// Run processes scans until ctx is canceled, then stops claiming and waits up
// to ShutdownTimeout for the scans in flight.
func (w *Worker) Run(ctx context.Context) error {
	// In-flight scans outlive ctx (a stop signal) by up to ShutdownTimeout.
	work, cancelWork := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelWork()

	w.resetBeats()
	var wg sync.WaitGroup
	for slot := range w.cfg.Concurrency {
		wg.Go(func() {
			for ctx.Err() == nil {
				processed, err := w.ProcessOne(work)
				if err != nil {
					w.cfg.Logger.Error("worker: process scan", "err", err)
				} else {
					w.beats[slot].Store(time.Now().UnixNano())
				}
				if processed && err == nil {
					continue // there may be more work: no wait
				}
				select {
				case <-ctx.Done():
				case <-time.After(w.cfg.PollInterval):
				}
			}
		})
	}

	<-ctx.Done()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(w.cfg.ShutdownTimeout):
		w.cfg.Logger.Warn("worker: shutdown timeout, abandoning in-flight scans to lease expiry")
		cancelWork()
		<-done
	}
	return nil
}

// ProcessOne claims and processes one scan. It reports whether a scan was
// claimed. If ctx is canceled mid-scan the scan is left running: its lease
// expires and another worker retries it.
func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	if ids, err := w.st.FailExpiredScans(ctx); err != nil {
		return false, fmt.Errorf("fail expired scans: %w", err)
	} else if len(ids) > 0 {
		w.cfg.Logger.Warn("worker: scans failed after exhausting their attempts", "ids", ids)
	}

	sc, err := w.st.Claim(ctx, w.cfg.Lease)
	if errors.Is(err, store.ErrNoScan) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim: %w", err)
	}
	log := w.cfg.Logger.With("scan", sc.ID, "host", sc.Host, "port", sc.Port, "attempt", sc.Attempts)
	log.Info("worker: scan claimed")

	start := time.Now()
	res, err := w.sc.Scan(ctx, sc.Host, int(sc.Port))
	if ctx.Err() != nil {
		log.Warn("worker: stopped mid-scan; the lease will expire and the scan will be retried")
		return true, nil
	}
	if err != nil {
		f := Classify(err)
		var serr error
		if f.Transient && int(sc.Retries) < w.cfg.MaxRetries {
			log.Info("worker: transient scan error, retrying later", "code", f.Code, "err", err, "in", w.cfg.RetryDelay)
			serr = w.st.Retry(ctx, sc.ID, *sc.LeaseToken, f.Code, w.cfg.RetryDelay)
		} else {
			log.Info("worker: scan failed", "code", f.Code, "transient", f.Transient, "err", err)
			serr = w.st.Fail(ctx, sc.ID, *sc.LeaseToken, f.Code)
		}
		if serr != nil && !errors.Is(serr, store.ErrLeaseLost) {
			return true, fmt.Errorf("store failure: %w", serr)
		}
		return true, nil
	}

	params, findings, err := completion(sc, res, scoring.Evaluate(res))
	if err != nil {
		return true, err
	}
	switch err := w.st.Complete(ctx, params, findings); {
	case errors.Is(err, store.ErrLeaseLost):
		log.Warn("worker: lease lost before completion; result discarded")
	case err != nil:
		return true, fmt.Errorf("store result: %w", err)
	default:
		log.Info("worker: scan done", "grade", *params.Grade, "score", *params.Score, "duration", time.Since(start))
	}
	return true, nil
}

// Failure is a scanner error classified for the queue: Code is stored in
// scans.error; Transient errors are worth one more try after a delay.
type Failure struct {
	Code      string
	Transient bool
}

// Classify maps a scanner error to a stable code and decides whether it is
// transient (a timeout or a network hiccup may not happen again) or definitive
// (the target does not exist, refuses connections or must not be scanned).
func Classify(err error) Failure {
	var dnsErr *net.DNSError
	switch {
	case errors.Is(err, scanner.ErrBlockedAddress):
		return Failure{Code: "blocked_address"}
	case errors.Is(err, scanner.ErrInvalidTarget):
		return Failure{Code: "invalid_target"}
	case errors.As(err, &dnsErr):
		if dnsErr.IsNotFound {
			return Failure{Code: "dns_not_found"} // NXDOMAIN or no address
		}
		if dnsErr.IsTimeout || dnsErr.IsTemporary {
			return Failure{Code: "dns_temporary", Transient: true}
		}
		return Failure{Code: "dns"}
	case errors.Is(err, scanner.ErrResolve):
		return Failure{Code: "dns"}
	case errors.Is(err, syscall.ECONNREFUSED):
		return Failure{Code: "connection_refused"}
	case isTimeout(err):
		return Failure{Code: "timeout", Transient: true}
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.ECONNABORTED),
		errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH),
		errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return Failure{Code: "network", Transient: true}
	case errors.Is(err, scanner.ErrUnreachable):
		return Failure{Code: "unreachable"}
	}
	return Failure{Code: "scan_error"}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout())
}

func completion(sc store.Scan, res *scanner.Result, s scoring.Result) (store.CompleteScanParams, []store.InsertFindingsParams, error) {
	resultJSON, err := json.Marshal(res)
	if err != nil {
		return store.CompleteScanParams{}, nil, fmt.Errorf("marshal result: %w", err)
	}
	findings := s.Findings
	s.Findings = nil // stored as rows
	scoringJSON, err := json.Marshal(s)
	if err != nil {
		return store.CompleteScanParams{}, nil, fmt.Errorf("marshal scoring: %w", err)
	}

	rows := make([]store.InsertFindingsParams, len(findings))
	for i, f := range findings {
		params := f.Params
		if params == nil {
			params = map[string]string{}
		}
		pj, err := json.Marshal(params)
		if err != nil {
			return store.CompleteScanParams{}, nil, err
		}
		rows[i] = store.InsertFindingsParams{
			ScanID: sc.ID, Position: int32(i), Code: f.Code, Axis: string(f.Axis),
			Severity: string(f.Severity), Category: string(f.Category), Params: pj,
		}
	}

	schema := int32(res.SchemaVersion) //nolint:gosec // small constant
	score := int32(s.Score)            //nolint:gosec // 0–100
	grade, version, pqc := s.Grade, s.ScoringVersion, s.PQC
	return store.CompleteScanParams{
		ID: sc.ID, LeaseToken: sc.LeaseToken,
		Result: resultJSON, ScannerSchema: &schema,
		Scoring: scoringJSON, ScoringVersion: &version,
		Score: &score, Grade: &grade, Pqc: &pqc,
	}, rows, nil
}
