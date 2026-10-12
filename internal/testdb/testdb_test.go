package testdb

import "testing"

// recorder captures Skip/Fatal instead of stopping the test goroutine.
type recorder struct {
	testing.TB
	skipped, failed bool
}

func (r *recorder) Helper()      {}
func (r *recorder) Skip(...any)  { r.skipped = true }
func (r *recorder) Fatal(...any) { r.failed = true }

func TestDSN(t *testing.T) {
	tests := []struct {
		name, dsn, require   string
		wantDSN              string
		wantSkip, wantFailed bool
	}{
		{name: "set", dsn: "postgres://x", wantDSN: "postgres://x"},
		{name: "set and required", dsn: "postgres://x", require: "1", wantDSN: "postgres://x"},
		{name: "unset skips", wantSkip: true},
		{name: "unset and required fails", require: "1", wantFailed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("QR_TEST_DATABASE_URL", tt.dsn)
			t.Setenv("REQUIRE_DB", tt.require)
			r := &recorder{}
			if got := DSN(r); got != tt.wantDSN {
				t.Errorf("DSN = %q, want %q", got, tt.wantDSN)
			}
			if r.skipped != tt.wantSkip || r.failed != tt.wantFailed {
				t.Errorf("skipped=%v failed=%v, want skipped=%v failed=%v", r.skipped, r.failed, tt.wantSkip, tt.wantFailed)
			}
		})
	}
}
