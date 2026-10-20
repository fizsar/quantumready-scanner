package main

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

// The purge runs at start and then on every tick, with the 30 days.
func TestPurgeLoop(t *testing.T) {
	var calls atomic.Int32
	var keep atomic.Int32
	purge := func(_ context.Context, k int32) (int64, error) {
		keep.Store(k)
		calls.Add(1)
		return 0, nil
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		purgeLoop(ctx, purge, blockedTargetsKeepDays, 20*time.Millisecond, slog.New(slog.DiscardHandler))
		close(done)
	}()
	time.Sleep(110 * time.Millisecond)
	stop()
	<-done
	if n := calls.Load(); n < 3 {
		t.Errorf("purged %d times in 110 ms with a 20 ms interval, want at start and on ticks", n)
	}
	if keep.Load() != 30 {
		t.Errorf("keep days %d, want 30", keep.Load())
	}
}
