package main

import (
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

func TestPublicQueueTimeout(t *testing.T) {
	for v, want := range map[string]time.Duration{"": 15 * time.Minute, "1m": time.Minute, "2h": 2 * time.Hour} {
		if d, err := publicQueueTimeout(func(string) string { return v }); err != nil || d != want {
			t.Errorf("%q: %v %v", v, d, err)
		}
	}
	for _, v := range []string{"30s", "25h", "x"} {
		if _, err := publicQueueTimeout(func(string) string { return v }); err == nil {
			t.Errorf("%q accepted", v)
		}
	}
}

func TestMaxScansPerMinute(t *testing.T) {
	for v, want := range map[string]int{"": 20, "1": 1, "600": 600} {
		if n, err := maxScansPerMinute(func(string) string { return v }); err != nil || n != want {
			t.Errorf("%q: %d %v", v, n, err)
		}
	}
	for _, v := range []string{"0", "601", "x", "-3"} {
		if _, err := maxScansPerMinute(func(string) string { return v }); err == nil {
			t.Errorf("%q accepted", v)
		}
	}
}
