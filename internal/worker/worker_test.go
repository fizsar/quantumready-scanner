package worker

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/scanner"
	"github.com/fizsar/quantumready-saas/internal/scoring"
	"github.com/fizsar/quantumready-saas/internal/store"
	"github.com/fizsar/quantumready-saas/internal/testdb"
	"github.com/fizsar/quantumready-saas/migrations"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	return store.NewStore(testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	}))
}

type scanFunc func(ctx context.Context, host string, port int) (*scanner.Result, error)

func (f scanFunc) Scan(ctx context.Context, host string, port int) (*scanner.Result, error) {
	return f(ctx, host, port)
}

// canned returns a fixed scan result: PQC preferred, valid certificate.
func canned(host string, port int, selected string) *scanner.Result {
	hybrid := selected == "X25519MLKEM768"
	return &scanner.Result{
		SchemaVersion: scanner.SchemaVersion, Host: host, Port: port,
		Default:             scanner.Handshake{Status: scanner.Supported, Version: "TLS 1.3", Group: selected},
		KeyExchange:         scanner.KeyExchange{Selected: selected, SelectedHybrid: hybrid},
		Versions:            []scanner.VersionProbe{{Version: "TLS 1.3", Status: scanner.Supported}},
		TLS12SuitesComplete: true,
		Certificate: &scanner.Certificate{
			Trusted: true, HostnameMatch: true, KeyAlgorithm: "ECDSA", KeySize: 256, Curve: "P-256",
			SignatureAlgorithm: "ECDSA-SHA256", ValidityDays: 90, DaysToExpiry: 60, IssuerOrg: []string{"Let's Encrypt"},
		},
	}
}

func fastScanner(selected string) Scanner {
	return scanFunc(func(_ context.Context, host string, port int) (*scanner.Result, error) {
		return canned(host, port, selected), nil
	})
}

func mustGet(t *testing.T, st *store.Store, id uuid.UUID) store.Scan {
	t.Helper()
	sc, err := st.GetScan(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return sc
}

func enqueue(t *testing.T, st *store.Store, host string) store.Scan {
	t.Helper()
	sc, err := st.Enqueue(context.Background(), host, 443)
	if err != nil {
		t.Fatal(err)
	}
	return sc
}

// The real scanner against a local TLS server, through the queue, to stored
// scoring and findings.
func TestEndToEndWithRealScanner(t *testing.T) {
	st := newTestStore(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(srv.Close)
	port := srv.Listener.Addr().(*net.TCPAddr).Port

	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	sc := scanner.New(scanner.Options{
		Resolver:         staticResolver{"example.com": netip.MustParseAddr("127.0.0.1")},
		AddrPolicy:       func(a netip.Addr) error { return nil }, // test server on loopback
		RootCAs:          roots,
		HandshakeTimeout: 2 * time.Second,
	})

	queued, err := st.Enqueue(context.Background(), "example.com", int32(port))
	if err != nil {
		t.Fatal(err)
	}
	w := New(st, sc, Config{Logger: quiet})
	if ok, err := w.ProcessOne(context.Background()); !ok || err != nil {
		t.Fatalf("ProcessOne = %v, %v", ok, err)
	}

	got := mustGet(t, st, queued.ID)
	if got.Status != "done" || got.Attempts != 1 || *got.ScoringVersion != scoring.Version || got.Grade == nil || !*got.Pqc {
		t.Fatalf("scan = %+v", got)
	}
	var res scanner.Result
	if err := json.Unmarshal(got.Result, &res); err != nil || res.Host != "example.com" || res.Port != port {
		t.Fatalf("stored result: %v %+v", err, res)
	}
	var s scoring.Result
	if err := json.Unmarshal(got.Scoring, &s); err != nil || s.Score != int(*got.Score) || len(s.Blocks) != 5 || s.Findings != nil {
		t.Fatalf("stored scoring: %v %+v", err, s)
	}
	fs, err := st.ListFindings(context.Background(), queued.ID)
	if err != nil || len(fs) == 0 {
		t.Fatalf("findings: %v (%d)", err, len(fs))
	}
	for i, f := range fs {
		if f.Position != int32(i) {
			t.Errorf("finding %d at position %d", i, f.Position)
		}
	}
	if fs[0].Severity == "pass" && len(fs) > 1 && fs[len(fs)-1].Severity != "pass" {
		t.Errorf("findings not in severity order: %v first", fs[0].Code)
	}
}

type staticResolver map[string]netip.Addr

func (r staticResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := r[host]; ok {
		return []netip.Addr{a}, nil
	}
	return nil, errors.New("no such host")
}

func TestFailureCodes(t *testing.T) {
	st := newTestStore(t)
	for _, tt := range []struct {
		err  error
		code string
	}{
		{scanner.ErrBlockedAddress, "blocked_address"},
		{scanner.ErrInvalidTarget, "invalid_target"},
		{scanner.ErrResolve, "dns"},
		{scanner.ErrUnreachable, "unreachable"},
		{errors.New("boom"), "scan_error"},
	} {
		q := enqueue(t, st, "example.com")
		w := New(st, scanFunc(func(context.Context, string, int) (*scanner.Result, error) { return nil, tt.err }), Config{Logger: quiet})
		if ok, err := w.ProcessOne(context.Background()); !ok || err != nil {
			t.Fatalf("ProcessOne = %v, %v", ok, err)
		}
		got := mustGet(t, st, q.ID)
		if got.Status != "failed" || got.Error == nil || *got.Error != tt.code {
			t.Errorf("%v: scan = status %s error %v, want failed %s", tt.err, got.Status, got.Error, tt.code)
		}
	}
}

// Worker A dies in the middle of a scan. Its lease protects the scan while
// valid; once it expires, worker B claims it and completes it.
func TestWorkerDiesMidScanAndAnotherRecovers(t *testing.T) {
	st := newTestStore(t)
	q := enqueue(t, st, "example.com")

	started := make(chan struct{})
	blocking := scanFunc(func(ctx context.Context, _ string, _ int) (*scanner.Result, error) {
		close(started)
		<-ctx.Done() // until the worker "dies"
		return nil, ctx.Err()
	})
	a := New(st, blocking, Config{Lease: 500 * time.Millisecond, Logger: quiet})
	ctxA, kill := context.WithCancel(context.Background())
	doneA := make(chan error, 1)
	go func() { _, err := a.ProcessOne(ctxA); doneA <- err }()
	<-started
	kill()
	if err := <-doneA; err != nil {
		t.Fatalf("dying worker returned %v", err)
	}
	if got := mustGet(t, st, q.ID); got.Status != "running" || got.Attempts != 1 {
		t.Fatalf("after A died: status %s attempts %d, want running 1 (not failed)", got.Status, got.Attempts)
	}

	b := New(st, fastScanner("X25519MLKEM768"), Config{Lease: time.Minute, Logger: quiet})
	if ok, err := b.ProcessOne(context.Background()); ok || err != nil {
		t.Fatalf("B claimed while A's lease was valid: %v, %v", ok, err)
	}

	time.Sleep(600 * time.Millisecond) // A's lease expires
	if ok, err := b.ProcessOne(context.Background()); !ok || err != nil {
		t.Fatalf("B ProcessOne = %v, %v", ok, err)
	}
	got := mustGet(t, st, q.ID)
	if got.Status != "done" || got.Attempts != 2 || *got.Grade != "A" {
		t.Fatalf("after B: %+v", got)
	}
}

// Worker A stalls (it is not dead) past its lease; B takes over and completes.
// When A finally finishes, its late result is discarded.
func TestStalledWorkerResultIsDiscarded(t *testing.T) {
	st := newTestStore(t)
	q := enqueue(t, st, "example.com")

	started, release := make(chan struct{}), make(chan struct{})
	stalled := scanFunc(func(_ context.Context, host string, port int) (*scanner.Result, error) {
		close(started)
		<-release
		return canned(host, port, "x25519"), nil // would score worse than B's
	})
	a := New(st, stalled, Config{Lease: 300 * time.Millisecond, Logger: quiet})
	doneA := make(chan error, 1)
	go func() { _, err := a.ProcessOne(context.Background()); doneA <- err }()
	<-started

	time.Sleep(400 * time.Millisecond)
	b := New(st, fastScanner("X25519MLKEM768"), Config{Logger: quiet})
	if ok, err := b.ProcessOne(context.Background()); !ok || err != nil {
		t.Fatalf("B ProcessOne = %v, %v", ok, err)
	}
	close(release)
	if err := <-doneA; err != nil {
		t.Fatalf("stalled worker returned %v, want nil (lease lost is logged, not an error)", err)
	}

	got := mustGet(t, st, q.ID)
	if got.Status != "done" || !*got.Pqc || *got.Grade != "A" {
		t.Fatalf("final scan = %+v, want B's result", got)
	}
	fs, _ := st.ListFindings(context.Background(), q.ID)
	for _, f := range fs {
		if f.Code == "KEX_NO_HYBRID" {
			t.Error("the stalled worker's findings were stored")
		}
	}
}

func TestRunProcessesTheQueue(t *testing.T) {
	st := newTestStore(t)
	var ids []uuid.UUID
	for range 5 {
		ids = append(ids, enqueue(t, st, "example.com").ID)
	}
	slow := scanFunc(func(_ context.Context, host string, port int) (*scanner.Result, error) {
		time.Sleep(50 * time.Millisecond)
		return canned(host, port, "X25519MLKEM768"), nil
	})
	w := New(st, slow, Config{Concurrency: 2, PollInterval: 20 * time.Millisecond, Logger: quiet})
	ctx, stop := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- w.Run(ctx) }()

	deadline := time.Now().Add(10 * time.Second)
	for _, id := range ids {
		for mustGet(t, st, id).Status != "done" {
			if time.Now().After(deadline) {
				t.Fatalf("scan %s not done in time", id)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	stop()
	if err := <-runDone; err != nil {
		t.Fatal(err)
	}
}

// On stop, Run waits for in-flight scans up to ShutdownTimeout, then abandons
// them to lease expiry instead of failing them.
func TestRunShutdownLeavesInFlightScansToTheLease(t *testing.T) {
	st := newTestStore(t)
	q := enqueue(t, st, "example.com")
	started := make(chan struct{})
	blocking := scanFunc(func(ctx context.Context, _ string, _ int) (*scanner.Result, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	w := New(st, blocking, Config{Concurrency: 1, ShutdownTimeout: 200 * time.Millisecond, Logger: quiet})
	ctx, stop := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- w.Run(ctx) }()
	<-started
	stop()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after ShutdownTimeout")
	}
	if got := mustGet(t, st, q.ID); got.Status != "running" || got.LeaseToken == nil {
		t.Fatalf("abandoned scan = status %s, want running with its lease", got.Status)
	}
}
