package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestRequirePlaintextDB(t *testing.T) {
	tests := []struct {
		dsn    string
		wantOK bool
	}{
		{"postgres://u:p@db:5432/q?sslmode=disable", true},
		{"postgres://u:p@db:5432/q", false}, // libpq default: prefer (tries TLS)
		{"postgres://u:p@db:5432/q?sslmode=prefer", false},
		{"postgres://u:p@db:5432/q?sslmode=allow", false},
		{"postgres://u:p@db:5432/q?sslmode=require", false},
		{"postgres://u:p@db:5432/q?sslmode=verify-full", false},
	}
	for _, tt := range tests {
		cfg, err := pgx.ParseConfig(tt.dsn)
		if err != nil {
			t.Fatalf("%s: %v", tt.dsn, err)
		}
		if err := requirePlaintextDB(cfg); (err == nil) != tt.wantOK {
			t.Errorf("%s: requirePlaintextDB = %v, want ok=%v", tt.dsn, err, tt.wantOK)
		}
	}
}

func TestHeartbeat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hb")
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if err := checkHeartbeat(path, now, heartbeatMaxAge); err == nil {
		t.Error("missing heartbeat accepted")
	}
	if err := writeHeartbeat(path, now); err != nil {
		t.Fatal(err)
	}
	if err := checkHeartbeat(path, now.Add(30*time.Second), heartbeatMaxAge); err != nil {
		t.Errorf("fresh heartbeat: %v", err)
	}
	if err := checkHeartbeat(path, now.Add(time.Minute), heartbeatMaxAge); err == nil {
		t.Error("stale heartbeat accepted")
	}
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkHeartbeat(path, now, heartbeatMaxAge); err == nil {
		t.Error("garbage heartbeat accepted")
	}
}

// The heartbeat is only written while the database answers.
func TestHeartbeatNeedsTheDatabase(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	down := filepath.Join(t.TempDir(), "down")
	go heartbeat(ctx, down, func(context.Context) error { return errors.New("db down") }, logger)
	up := filepath.Join(t.TempDir(), "up")
	go heartbeat(ctx, up, func(context.Context) error { return nil }, logger)

	time.Sleep(200 * time.Millisecond)
	if err := checkHeartbeat(up, time.Now(), heartbeatMaxAge); err != nil {
		t.Errorf("db up: %v", err)
	}
	if err := checkHeartbeat(down, time.Now(), heartbeatMaxAge); err == nil {
		t.Error("db down: heartbeat written")
	}
}

func TestEnvInt(t *testing.T) {
	t.Setenv("QR_N", "")
	if n, err := envInt("QR_N", 2); n != 2 || err != nil {
		t.Errorf("default = %d, %v", n, err)
	}
	t.Setenv("QR_N", "4")
	if n, err := envInt("QR_N", 2); n != 4 || err != nil {
		t.Errorf("4 = %d, %v", n, err)
	}
	for _, bad := range []string{"0", "-1", "33", "x"} {
		t.Setenv("QR_N", bad)
		if _, err := envInt("QR_N", 2); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
