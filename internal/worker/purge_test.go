package worker

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// PurgeLoop runs at once (the first purge after a deploy does not wait a
// day), then on every tick, logs what it deleted and survives a failure.
func TestPurgeLoop(t *testing.T) {
	var calls atomic.Int32
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		PurgeLoop(ctx, "public scans", func(context.Context) (int64, error) {
			switch calls.Add(1) {
			case 1:
				return 3, nil
			case 2:
				return 0, errors.New("db down")
			}
			return 0, nil
		}, 20*time.Millisecond, log)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	out := buf.String()
	if calls.Load() < 3 || !strings.Contains(out, "rows=3") || !strings.Contains(out, "purge failed") {
		t.Errorf("calls %d, log:\n%s", calls.Load(), out)
	}

	// With a daily interval, the first purge still comes at once.
	var first atomic.Int32
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go PurgeLoop(ctx2, "public scans", func(context.Context) (int64, error) { first.Add(1); return 0, nil }, time.Hour, log)
	for i := 0; i < 200 && first.Load() == 0; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if first.Load() != 1 {
		t.Errorf("daily loop: %d purges at start, want 1", first.Load())
	}
}
