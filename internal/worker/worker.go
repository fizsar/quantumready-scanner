// Package worker runs queued scans: it claims a scan from the PostgreSQL
// queue, runs the scanner, scores the result and stores it with its findings.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fizsar/quantumready-saas/internal/scanner"
	"github.com/fizsar/quantumready-saas/internal/scoring"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// Scanner is what the worker needs from internal/scanner.
type Scanner interface {
	Scan(ctx context.Context, host string, port int) (*scanner.Result, error)
}

// Config tunes a Worker. Zero values select the defaults.
type Config struct {
	// Concurrency is the number of scans run in parallel. Default 2.
	Concurrency int
	// Lease is how long a claim lasts. It must exceed the scanner's own
	// timeout (60 s), so a healthy worker never loses a scan. Default 90 s.
	Lease time.Duration
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
}

// New returns a Worker with cfg, filling in defaults.
func New(st *store.Store, sc Scanner, cfg Config) *Worker {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
	if cfg.Lease <= 0 {
		cfg.Lease = 90 * time.Second
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
	return &Worker{st: st, sc: sc, cfg: cfg}
}

// Run processes scans until ctx is canceled, then stops claiming and waits up
// to ShutdownTimeout for the scans in flight.
func (w *Worker) Run(ctx context.Context) error {
	// In-flight scans outlive ctx (a stop signal) by up to ShutdownTimeout.
	work, cancelWork := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelWork()

	var wg sync.WaitGroup
	for range w.cfg.Concurrency {
		wg.Go(func() {
			for ctx.Err() == nil {
				processed, err := w.ProcessOne(work)
				if err != nil {
					w.cfg.Logger.Error("worker: process scan", "err", err)
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
		code := FailureCode(err)
		log.Info("worker: scan failed", "code", code, "err", err)
		if ferr := w.st.Fail(ctx, sc.ID, *sc.LeaseToken, code); ferr != nil && !errors.Is(ferr, store.ErrLeaseLost) {
			return true, fmt.Errorf("store failure: %w", ferr)
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

// FailureCode maps a scanner error to the stable code stored in scans.error.
func FailureCode(err error) string {
	switch {
	case errors.Is(err, scanner.ErrBlockedAddress):
		return "blocked_address"
	case errors.Is(err, scanner.ErrInvalidTarget):
		return "invalid_target"
	case errors.Is(err, scanner.ErrResolve):
		return "dns"
	case errors.Is(err, scanner.ErrUnreachable):
		return "unreachable"
	}
	return "scan_error"
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
