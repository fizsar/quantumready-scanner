package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fizsar/quantumready-saas/internal/altcha"
)

// Every kind of refused public scan request is counted once, as that kind
// and no other, and the client gets the same answer as before (status, code
// and the catalogue's message).
func TestScanRefusalsCounted(t *testing.T) {
	proof, _ := altcha.New([]byte(strings.Repeat("k", 32)), 300, time.Minute)
	for _, tc := range []struct {
		name   string
		opts   []func(*Config)
		setup  func(t *testing.T, a *authAPI)
		before []string // hosts scanned first (accepted)
		host   string
		status int
		code   string
		kind   string
	}{
		{name: "per client", opts: []func(*Config){func(c *Config) { c.Limiter = NewLimiter(1, 100) }},
			before: []string{"a.example.org"}, host: "b.example.net", status: http.StatusTooManyRequests, code: "rate_limited", kind: RefusedClientLimit},
		{name: "per domain", opts: []func(*Config){func(c *Config) { c.PublicLimits.PerDomain = NewWindowLimiter(1, time.Hour) }},
			before: []string{"a.example.org"}, host: "b.example.org", status: http.StatusTooManyRequests, code: "domain_rate_limited", kind: RefusedDomainLimit},
		{name: "global", opts: []func(*Config){func(c *Config) { c.PublicLimits.Global = NewLimiter(1, 100) }},
			before: []string{"a.example.org"}, host: "b.example.net", status: http.StatusTooManyRequests, code: "busy", kind: RefusedGlobalLimit},
		{name: "queue full", opts: []func(*Config){func(c *Config) { c.PublicQueueMax = 1 }},
			before: []string{"a.example.org"}, host: "b.example.net", status: http.StatusTooManyRequests, code: "busy", kind: RefusedQueueFull},
		{name: "excluded", setup: func(t *testing.T, a *authAPI) {
			if _, err := a.owner.Exec(context.Background(), "INSERT INTO scan_exclusions (name) VALUES ('excluded.example.net')"); err != nil {
				t.Fatal(err)
			}
		}, host: "www.excluded.example.net", status: http.StatusForbidden, code: "scan_not_allowed", kind: RefusedExcluded},
		{name: "proof of work", opts: []func(*Config){func(c *Config) { c.Altcha = proof }},
			host: "example.org", status: http.StatusBadRequest, code: "proof_failed", kind: RefusedProofFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refusals := NewRefusals()
			opts := append([]func(*Config){openPublic(PublicLimits{}), func(c *Config) { c.Refusals = refusals }}, tc.opts...)
			a := newAuthAPI(t, AuthLimits{}, opts...)
			if tc.setup != nil {
				tc.setup(t, a)
			}
			c := a.client("198.51.100.20")
			for _, h := range tc.before {
				expect(t, c.do(t, "POST", "/api/scans", js(map[string]string{"host": h})), http.StatusAccepted, "")
			}
			if got := refusals.take(); len(got) != 0 {
				t.Fatalf("counted before any refusal: %v", got)
			}
			w := c.do(t, "POST", "/api/scans", js(map[string]string{"host": tc.host}))
			expect(t, w, tc.status, tc.code)
			// The same answer as without counting: the code and the catalogue's message only.
			var body map[string]apiError
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body) != 1 ||
				body["error"] != (apiError{Code: tc.code, Message: message(tc.code, w.Header().Get("Content-Language"))}) {
				t.Errorf("answer changed: %s", w.Body)
			}
			if got := refusals.take(); !maps.Equal(got, map[string]int64{tc.kind: 1}) {
				t.Errorf("counted %v, want one %s", got, tc.kind)
			}
		})
	}
}

// Accepted and reused scans, the closed scanner's 403 (counted by M1) and
// requests refused before any scan logic (bad JSON, bad host) count nothing.
func TestScanRefusalsNotCounted(t *testing.T) {
	refusals := NewRefusals()
	demo, _ := ParseAllowlist("fizsar.com")
	a := newAuthAPI(t, AuthLimits{}, func(c *Config) { c.Refusals = refusals; c.Allowlist = demo })
	c := a.client("198.51.100.21")
	expect(t, c.do(t, "POST", "/api/scans", js(map[string]string{"host": "fizsar.com"})), http.StatusAccepted, "")
	expect(t, c.do(t, "POST", "/api/scans", js(map[string]string{"host": "fizsar.com"})), http.StatusOK, "")
	expect(t, c.do(t, "POST", "/api/scans", js(map[string]string{"host": "example.com"})), http.StatusForbidden, "host_not_allowed")
	expect(t, c.do(t, "POST", "/api/scans", js(map[string]string{"host": "fizsar.com:8443"})), http.StatusBadRequest, "port_not_allowed")
	if got := refusals.take(); len(got) != 0 {
		t.Errorf("counted %v", got)
	}
}

func TestRefusalsFlush(t *testing.T) {
	r := NewRefusals()
	r.add(RefusedClientLimit, 1)
	r.add(RefusedClientLimit, 1)
	r.add(RefusedExcluded, 1)
	r.add(RefusedQueueFull, 0) // nothing
	written := map[string]int64{}
	failing := RefusedExcluded
	add := func(_ context.Context, kind string, n int64) error {
		if kind == failing {
			return errors.New("database down")
		}
		written[kind] += n
		return nil
	}
	if err := r.Flush(context.Background(), add); err == nil {
		t.Error("a failed write was not reported")
	}
	if !maps.Equal(written, map[string]int64{RefusedClientLimit: 2}) {
		t.Errorf("written %v", written)
	}
	// What failed is kept, and written by the next flush; nothing twice.
	failing = ""
	if err := r.Flush(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(written, map[string]int64{RefusedClientLimit: 2, RefusedExcluded: 1}) {
		t.Errorf("after the retry: %v", written)
	}
	if got := r.take(); len(got) != 0 {
		t.Errorf("left after flushing: %v", got)
	}
	var none *Refusals
	none.add(RefusedExcluded, 1) // a nil counter counts nothing, and flushes nothing
	if err := none.Flush(context.Background(), add); err != nil {
		t.Error(err)
	}
}

func TestFlushRefusalsLoop(t *testing.T) {
	r := NewRefusals()
	r.add(RefusedProofFailed, 3)
	flushed := make(chan int64, 4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		FlushRefusalsLoop(ctx, r, func(_ context.Context, _ string, n int64) error { flushed <- n; return nil }, 10*time.Millisecond, slog.New(slog.DiscardHandler))
		close(done)
	}()
	select {
	case n := <-flushed:
		if n != 3 {
			t.Errorf("flushed %d", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("never flushed")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the loop did not stop with its context")
	}
}
