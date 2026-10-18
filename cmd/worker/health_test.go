package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCheckHeartbeat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hb")
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if err := checkHeartbeat(path, now, heartbeatMaxAge); err == nil {
		t.Error("missing heartbeat accepted")
	}
	if err := writeHeartbeat(path, now); err != nil {
		t.Fatal(err)
	}
	if err := checkHeartbeat(path, now.Add(heartbeatMaxAge), heartbeatMaxAge); err != nil {
		t.Errorf("heartbeat exactly maxAge old: %v", err)
	}
	if err := checkHeartbeat(path, now.Add(heartbeatMaxAge+time.Second), heartbeatMaxAge); err == nil {
		t.Error("stale heartbeat accepted")
	}
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkHeartbeat(path, now, heartbeatMaxAge); err == nil {
		t.Error("garbage heartbeat accepted")
	}
}

// The bug in production (WORKER_CONCURRENCY=2): two slots wrote the same
// temporary file and one rename failed with ENOENT, about once a second.
// Unique temporary files make concurrent writes safe (run with -race).
func TestWriteHeartbeatConcurrently(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hb")
	var wg sync.WaitGroup
	errs := make(chan error, 16*100)
	for g := range 16 {
		wg.Go(func() {
			for i := range 100 {
				if err := writeHeartbeat(path, time.Unix(int64(g*1000+i), 0)); err != nil {
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent write: %v", err)
	}
	if err := checkHeartbeat(path, time.Unix(16*1000, 0), 24*time.Hour); err != nil {
		t.Errorf("file after concurrent writes: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(left) > 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

// fakeSlots is a worker whose slots report when the test says.
type fakeSlots struct {
	mu    sync.Mutex
	beats []time.Time
}

func (f *fakeSlots) set(i int, t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.beats[i] = t
}

func (f *fakeSlots) get() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Time{}, f.beats...)
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newWriter(t *testing.T, slots *fakeSlots, c *clock) (*heartbeatWriter, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	return &heartbeatWriter{
		path: filepath.Join(t.TempDir(), "hb"), every: heartbeatEvery, stale: 90 * time.Second,
		slots: slots.get, now: c.now, log: slog.New(slog.NewJSONHandler(&logs, nil)),
	}, &logs
}

func readTime(t *testing.T, path string) time.Time {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := time.Parse(time.RFC3339Nano, string(b))
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

// A stuck slot stops the heartbeat (so the healthcheck fails) with one warning
// naming the slot; when it reports again, the heartbeat resumes.
func TestHeartbeatStopsWhileASlotIsStuck(t *testing.T) {
	t0 := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	c := &clock{t0}
	slots := &fakeSlots{beats: []time.Time{t0, t0}}
	h, logs := newWriter(t, slots, c)

	h.tick()
	if got := readTime(t, h.path); !got.Equal(t0) {
		t.Fatalf("first write: %v", got)
	}

	// Slot 1 stops reporting; slot 0 keeps going.
	for step := range 40 { // 40 ticks of 5 s: 200 s
		c.t = t0.Add(time.Duration(step+1) * heartbeatEvery)
		slots.set(0, c.t)
		h.tick()
	}
	last := readTime(t, h.path)
	if want := t0.Add(90 * time.Second); last.After(want) {
		t.Errorf("heartbeat written at %v although slot 1 has been silent since %v (stale after 90 s)", last, t0)
	}
	if err := checkHeartbeat(h.path, c.t, heartbeatMaxAge); err == nil {
		t.Error("healthcheck passes with a slot stuck for 200 s")
	}
	if n := strings.Count(logs.String(), "slot stuck"); n != 1 {
		t.Errorf("%d stuck warnings over 40 ticks, want exactly 1:\n%s", n, logs.String())
	}
	if !strings.Contains(logs.String(), `"stuck_slots":[1]`) {
		t.Errorf("the warning does not name slot 1:\n%s", logs.String())
	}

	// Slot 1 recovers.
	c.t = c.t.Add(heartbeatEvery)
	slots.set(0, c.t)
	slots.set(1, c.t)
	h.tick()
	if got := readTime(t, h.path); !got.Equal(c.t) {
		t.Errorf("heartbeat not resumed: %v, want %v", got, c.t)
	}
	if err := checkHeartbeat(h.path, c.t, heartbeatMaxAge); err != nil {
		t.Errorf("healthcheck after recovery: %v", err)
	}
	if !strings.Contains(logs.String(), "alive again") {
		t.Errorf("no recovery message:\n%s", logs.String())
	}
}

// A slot is stuck only after the stale threshold: a long scan is fine.
func TestHeartbeatToleratesALongScan(t *testing.T) {
	t0 := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	c := &clock{t0.Add(89 * time.Second)} // a 60 s scan plus storing
	slots := &fakeSlots{beats: []time.Time{t0, c.t}}
	h, logs := newWriter(t, slots, c)
	h.tick()
	if got := readTime(t, h.path); !got.Equal(c.t) {
		t.Errorf("no write during a long but healthy scan: %v", got)
	}
	if logs.Len() != 0 {
		t.Errorf("unexpected log: %s", logs.String())
	}
}

// The healthcheck reads the same file as before: /tmp/qr-worker-heartbeat
// (tmpfs in deploy/compose.prod.yaml), unless WORKER_HEARTBEAT_FILE says
// otherwise, and the running writer keeps it fresh.
func TestHealthcheckReadsTheWritersFile(t *testing.T) {
	t.Setenv("WORKER_HEARTBEAT_FILE", "")
	if p := heartbeatPath(); p != "/tmp/qr-worker-heartbeat" {
		t.Errorf("default path %q changed: compose mounts /tmp for it", p)
	}
	path := filepath.Join(t.TempDir(), "hb")
	t.Setenv("WORKER_HEARTBEAT_FILE", path)
	slots := &fakeSlots{beats: []time.Time{time.Now(), time.Now()}}
	h := &heartbeatWriter{path: heartbeatPath(), every: 10 * time.Millisecond, stale: time.Minute,
		slots: func() []time.Time { n := time.Now(); slots.set(0, n); slots.set(1, n); return slots.get() },
		now:   time.Now, log: slog.New(slog.DiscardHandler)}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { h.run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond)
	stop()
	<-done
	if err := checkHeartbeat(heartbeatPath(), time.Now(), heartbeatMaxAge); err != nil {
		t.Errorf("healthcheck on the writer's file: %v", err)
	}
}

// The values relate as health.go explains.
func TestHeartbeatValues(t *testing.T) {
	if heartbeatMaxAge < 3*heartbeatEvery {
		t.Errorf("heartbeatMaxAge %v must allow a few missed writes every %v", heartbeatMaxAge, heartbeatEvery)
	}
	if heartbeatMaxAge > 30*time.Second {
		t.Errorf("heartbeatMaxAge %v longer than the worker's healthcheck interval (30 s)", heartbeatMaxAge)
	}
}
