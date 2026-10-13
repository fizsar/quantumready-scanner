package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The container healthcheck (`worker healthcheck`) cannot use a shell or HTTP
// (distroless, no server). The running worker writes a heartbeat file every
// heartbeatEvery while PostgreSQL answers; the check fails if the file is
// missing or older than heartbeatMaxAge.
const (
	heartbeatEvery  = 10 * time.Second
	heartbeatMaxAge = 45 * time.Second
)

func heartbeatPath() string {
	if p := os.Getenv("WORKER_HEARTBEAT_FILE"); p != "" {
		return p
	}
	return "/tmp/qr-worker-heartbeat"
}

func writeHeartbeat(path string, now time.Time) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(now.UTC().Format(time.RFC3339Nano)), 0o600); err != nil {
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
		return fmt.Errorf("heartbeat is %v old (max %v)", age.Round(time.Second), maxAge)
	}
	return nil
}

// heartbeat writes the file now and every heartbeatEvery while ping succeeds,
// until ctx ends.
func heartbeat(ctx context.Context, path string, ping func(context.Context) error, logger *slog.Logger) {
	beat := func() {
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if err := ping(pctx); err != nil {
			logger.Warn("worker: database unreachable, heartbeat skipped", "err", err)
			return
		}
		if err := writeHeartbeat(path, time.Now()); err != nil {
			logger.Warn("worker: cannot write heartbeat", "err", err)
		}
	}
	beat()
	t := time.NewTicker(heartbeatEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			beat()
		}
	}
}
