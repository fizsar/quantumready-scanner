package main

import (
	"context"
	"log/slog"
	"time"
)

// blockedTargetsKeepDays: how long the registrable domains of scan requests
// refused with 403 are kept (M1; /privacy says so).
const blockedTargetsKeepDays = 30

// purgeLoop deletes the 403 domains older than keepDays, at start and then
// every interval, until ctx is done. A failure is logged and retried at the
// next tick.
func purgeLoop(ctx context.Context, purge func(context.Context, int32) (int64, error), keepDays int32, every time.Duration, log *slog.Logger) {
	run := func() {
		n, err := purge(ctx, keepDays)
		switch {
		case err != nil && ctx.Err() == nil:
			log.Warn("usage: purge blocked targets", "err", err)
		case n > 0:
			log.Info("usage: purged blocked targets", "rows", n, "older_than_days", keepDays)
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
