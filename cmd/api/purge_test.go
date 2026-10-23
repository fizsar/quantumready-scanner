package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The purge runs at start and then on every tick, and a failure only logs.
func TestPurgeLoop(t *testing.T) {
	var calls atomic.Int32
	purge := func(context.Context) (int64, error) {
		if calls.Add(1) == 2 {
			return 0, errors.New("database away")
		}
		return 1, nil
	}
	var logs strings.Builder
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		purgeLoop(ctx, "demo requests", purge, 20*time.Millisecond, slog.New(slog.NewTextHandler(&logs, nil)))
		close(done)
	}()
	time.Sleep(110 * time.Millisecond)
	stop()
	<-done
	if n := calls.Load(); n < 3 {
		t.Errorf("purged %d times in 110 ms with a 20 ms interval, want at start and on ticks", n)
	}
	if out := logs.String(); !strings.Contains(out, `msg="purge failed" what="demo requests"`) || !strings.Contains(out, `msg=purged what="demo requests" rows=1`) {
		t.Errorf("logs:\n%s", out)
	}
}

// What /privacy promises.
func TestRetention(t *testing.T) {
	if blockedTargetsKeepDays != 30 || demoRequestsKeepMonths != 12 {
		t.Errorf("retention: 403 domains %d days, demo requests %d months; /privacy says 30 and 12", blockedTargetsKeepDays, demoRequestsKeepMonths)
	}
}
