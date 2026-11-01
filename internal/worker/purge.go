package worker

import (
	"context"
	"log/slog"
	"time"
)

// PurgeLoop runs purge now and then every interval until ctx ends, logging
// what it deleted. The worker uses it for the public scans past their
// expiry (store.Queries.PurgeExpiredPublicScans, daily): what /privacy
// promises, 30 days. A failed run is logged and retried at the next tick.
func PurgeLoop(ctx context.Context, what string, purge func(context.Context) (int64, error), every time.Duration, log *slog.Logger) {
	run := func() {
		n, err := purge(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			log.Warn("purge failed", "what", what, "err", err)
		case n > 0:
			log.Info("purged", "what", what, "rows", n)
		}
	}
	run()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
