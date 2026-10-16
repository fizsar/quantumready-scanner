package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNoScan means there is nothing to claim.
var ErrNoScan = errors.New("store: no scan to claim")

// ErrLeaseLost means the caller no longer holds the scan's lease: it expired
// and another worker claimed the scan. The caller must discard its work.
var ErrLeaseLost = errors.New("store: scan lease lost")

// Store wraps the generated queries with the queue operations.
type Store struct {
	pool *pgxpool.Pool
	*Queries
}

// NewStore returns a Store backed by pool (New, from sqlc, returns bare Queries).
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, Queries: New(pool)}
}

// Enqueue adds a public basic scan to the queue, as the connecting role (tests
// and tools; requests go through EnqueueOrReuse).
func (s *Store) Enqueue(ctx context.Context, host string, port int32) (Scan, error) {
	return s.EnqueueScan(ctx, EnqueueScanParams{Host: host, Port: port, Intensity: IntensityBasic})
}

// Claim leases the next scan for lease. The returned Scan carries the lease
// token needed to complete or fail it.
func (s *Store) Claim(ctx context.Context, lease time.Duration) (Scan, error) {
	return s.ClaimCapped(ctx, lease, 0)
}

// ClaimCapped is Claim with a global cap: ErrNoScan while maxPerMinute scans
// have started in the last minute (0: no cap).
func (s *Store) ClaimCapped(ctx context.Context, lease time.Duration, maxPerMinute int) (Scan, error) {
	token := uuid.New()
	sc, err := s.ClaimScan(ctx, ClaimScanParams{LeaseToken: &token, LeaseSeconds: lease.Seconds(), MaxPerMinute: int32(maxPerMinute)}) //nolint:gosec // validated small
	if errors.Is(err, pgx.ErrNoRows) {
		return Scan{}, ErrNoScan
	}
	return sc, err
}

// Complete stores the result and its findings atomically, if the caller still
// holds the lease.
func (s *Store) Complete(ctx context.Context, p CompleteScanParams, findings []InsertFindingsParams) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.WithTx(tx)
		n, err := q.CompleteScan(ctx, p)
		if err != nil {
			return fmt.Errorf("complete scan: %w", err)
		}
		if n == 0 {
			return ErrLeaseLost
		}
		if err := q.InsertFindings(ctx, findings); err != nil {
			return fmt.Errorf("insert findings: %w", err)
		}
		return nil
	})
}

// Retry puts the scan back in the queue after a transient error, due after
// delay, if the caller still holds the lease.
func (s *Store) Retry(ctx context.Context, id, token uuid.UUID, code string, delay time.Duration) error {
	n, err := s.RetryScan(ctx, RetryScanParams{ID: id, LeaseToken: &token, Error: &code, DelaySeconds: delay.Seconds()})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrLeaseLost
	}
	return nil
}

// Fail marks the scan as failed with a stable reason code, if the caller
// still holds the lease.
func (s *Store) Fail(ctx context.Context, id, token uuid.UUID, code string) error {
	n, err := s.FailScan(ctx, FailScanParams{ID: id, LeaseToken: &token, Error: &code})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrLeaseLost
	}
	return nil
}
