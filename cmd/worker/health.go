package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// The container healthcheck (`worker healthcheck`) cannot use a shell or HTTP
// (distroless, no server), so it reads a heartbeat file. One goroutine,
// heartbeatWriter, is its only writer: every heartbeatEvery it looks at when
// each slot of the queue loop last reported (worker.SlotBeats) and rewrites
// the file only if every slot reported within the lease. A slot reports
// about every second while it waits for work and after every scan, which
// the scanner's timeout (60 s) keeps under the lease (90 s): a slot silent
// for longer than the lease has lost its scan or is stuck, and a failing
// queue (database down) stops reporting too.
//
// Values:
//   - heartbeatEvery = 5 s: often enough that a healthy file is never older
//     than a few seconds, rare enough to cost nothing.
//   - stale = the worker's lease (90 s).
//   - heartbeatMaxAge = 30 s, the Docker healthcheck interval of the worker
//     (deploy/compose.prod.yaml): six missed writes, so a slow tick never
//     trips it, and a dead writer turns the worker unhealthy at the next run
//     or the one after.
//
// From a slot getting stuck to an unhealthy container: 90 s until the writer
// notices, then up to 30 s until the file is too old, then Docker's retries.
const (
	heartbeatEvery  = 5 * time.Second
	heartbeatMaxAge = 30 * time.Second
)

func heartbeatPath() string {
	if p := os.Getenv("WORKER_HEARTBEAT_FILE"); p != "" {
		return p
	}
	return "/tmp/qr-worker-heartbeat"
}

// writeHeartbeat replaces path atomically with t: a temporary file with a
// unique name in the same directory, then a rename. Concurrent calls never
// share a temporary file.
func writeHeartbeat(path string, t time.Time) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.WriteString(t.UTC().Format(time.RFC3339Nano))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp, path)
	}
	if werr != nil {
		_ = os.Remove(tmp)
	}
	return werr
}

func checkHeartbeat(path string, now time.Time, maxAge time.Duration) error {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("no heartbeat: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(b)))
	if err != nil {
		return fmt.Errorf("bad heartbeat: %w", err)
	}
	if age := now.Sub(t); age > maxAge {
		return fmt.Errorf("heartbeat last written %v ago (max %v): a slot of the queue loop is stuck or the worker is not running", age.Round(time.Second), maxAge)
	}
	return nil
}

// heartbeatWriter is the only writer of the heartbeat file (see above).
type heartbeatWriter struct {
	path  string
	every time.Duration
	stale time.Duration      // a slot silent for longer is stuck (the lease)
	slots func() []time.Time // worker.SlotBeats
	now   func() time.Time
	log   *slog.Logger
	stuck []int // slots reported stuck: warn once per episode, not every tick
	ioErr bool  // a write error was reported: warn once until it works again
}

// tick checks the slots once and writes the file if they are all alive.
func (h *heartbeatWriter) tick() {
	now := h.now()
	var stuck []int
	for i, t := range h.slots() {
		if now.Sub(t) > h.stale {
			stuck = append(stuck, i)
		}
	}
	if len(stuck) > 0 {
		if !slices.Equal(stuck, h.stuck) {
			h.log.Warn("worker: queue loop slot stuck; heartbeat stopped, the healthcheck will fail",
				"stuck_slots", stuck, "slots", len(h.slots()), "silent_for_more_than", h.stale.String())
		}
		h.stuck = stuck
		return
	}
	if h.stuck != nil {
		h.log.Info("worker: every queue loop slot is alive again; heartbeat resumed", "were_stuck", h.stuck)
		h.stuck = nil
	}
	if err := writeHeartbeat(h.path, now); err != nil {
		if !h.ioErr {
			h.log.Warn("worker: cannot write heartbeat", "err", err)
		}
		h.ioErr = true
		return
	}
	h.ioErr = false
}

// run ticks right away and then every h.every until ctx is done.
func (h *heartbeatWriter) run(ctx context.Context) {
	h.tick()
	t := time.NewTicker(h.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.tick()
		}
	}
}
