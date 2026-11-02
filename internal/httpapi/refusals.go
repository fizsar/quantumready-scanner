package httpapi

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/fizsar/quantumready-saas/internal/store"
)

// Refused public scan requests, by kind (store.RefusalKinds, migration
// 00022): the closed list, and nothing else. They are counted in memory and
// written every few seconds (FlushRefusalsLoop), so that a flood of refused
// requests is not a flood of database writes. The answer to the client does
// not change.
const (
	RefusedClientLimit = store.RefusedClientLimit
	RefusedDomainLimit = store.RefusedDomainLimit
	RefusedGlobalLimit = store.RefusedGlobalLimit
	RefusedQueueFull   = store.RefusedQueueFull
	RefusedExcluded    = store.RefusedExcluded
	RefusedProofFailed = store.RefusedProofFailed
)

// Refusals holds the counts not yet written. The zero value is not usable;
// a nil *Refusals counts nothing.
type Refusals struct {
	mu sync.Mutex
	n  map[string]int64
}

// NewRefusals returns an empty counter.
func NewRefusals() *Refusals { return &Refusals{n: map[string]int64{}} }

func (r *Refusals) add(kind string, n int64) {
	if r == nil || n <= 0 {
		return
	}
	r.mu.Lock()
	r.n[kind] += n
	r.mu.Unlock()
}

// take returns the counts and starts again from zero.
func (r *Refusals) take() map[string]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.n
	r.n = map[string]int64{}
	return out
}

// Flush writes the counts with add, one call per kind. Counts that could not
// be written are kept for the next flush.
func (r *Refusals) Flush(ctx context.Context, add func(ctx context.Context, kind string, n int64) error) error {
	if r == nil {
		return nil
	}
	var first error
	for kind, n := range r.take() {
		if err := add(ctx, kind, n); err != nil {
			r.add(kind, n)
			if first == nil {
				first = err
			}
		}
	}
	return first
}

// FlushRefusalsLoop flushes every interval until ctx ends. The caller flushes
// once more after the server has stopped (what arrived in the meantime).
func FlushRefusalsLoop(ctx context.Context, r *Refusals, add func(ctx context.Context, kind string, n int64) error, every time.Duration, log *slog.Logger) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := r.Flush(ctx, add); err != nil && ctx.Err() == nil {
				log.Warn("refusals: flush failed; kept for the next one", "err", err)
			}
		}
	}
}

// refused counts one refused public scan request.
func (a *api) refused(kind string) { a.cfg.Refusals.add(kind, 1) }
