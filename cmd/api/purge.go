package main

import (
	"context"
	"log/slog"
	"time"
)

// What the API deletes on its own, as /privacy says.
const (
	// blockedTargetsKeepDays: the registrable domains of scan requests
	// refused with 403 (M1).
	blockedTargetsKeepDays = 30
	// demoRequestsKeepMonths: the "Request a free demo" submissions.
	demoRequestsKeepMonths = 12
)

// purgeLoop runs purge at start and then every interval, until ctx is done.
// A failure is logged and retried at the next tick.
func purgeLoop(ctx context.Context, what string, purge func(context.Context) (int64, error), every time.Duration, log *slog.Logger) {
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
