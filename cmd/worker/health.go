package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The container healthcheck (`worker healthcheck`) cannot use a shell or HTTP
// (distroless, no server). The queue loop itself writes a heartbeat file after
// every iteration that did not fail, idle ones included (worker.Config.Heartbeat),
// holding the oldest last-iteration time across its slots. The check fails if
// the file is missing or older than heartbeatMaxAge.
//
// heartbeatMaxAge = lease (90 s) + 30 s. A healthy iteration lasts at most one
// scan (60 s timeout) plus storing it, always under the lease; an iteration
// longer than the lease has lost its scan to another worker, so the worker is
// not healthy. The 30 s cover the gap between Docker healthcheck runs
// (interval 30 s). An idle loop beats every second; a database outage makes
// every iteration fail, so it also turns the worker unhealthy within 2 minutes.
const heartbeatMaxAge = 120 * time.Second

func heartbeatPath() string {
	if p := os.Getenv("WORKER_HEARTBEAT_FILE"); p != "" {
		return p
	}
	return "/tmp/qr-worker-heartbeat"
}

func writeHeartbeat(path string, t time.Time) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(t.UTC().Format(time.RFC3339Nano)), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
		return fmt.Errorf("queue loop last completed an iteration %v ago (max %v)", age.Round(time.Second), maxAge)
	}
	return nil
}
