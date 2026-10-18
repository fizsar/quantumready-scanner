package worker

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"syscall"
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

// ScanTimeout is short so tests can use leases of a few hundred milliseconds.
func (f scanFunc) ScanTimeout() time.Duration { return 100 * time.Millisecond }

func mustNew(t *testing.T, st *store.Store, sc Scanner, cfg Config) *Worker {
	t.Helper()
	if cfg.LeaseMargin == 0 {
		cfg.LeaseMargin = 50 * time.Millisecond
	}
	w, err := New(st, sc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// canned returns a fixed scan result: PQC preferred, valid certificate.
func canned(host string, port int, selected string) *scanner.Result {
	hybrid := selected == "X25519MLKEM768"
	return &scanner.Result{
		SchemaVersion: scanner.SchemaVersion, Host: host, Port: port,
		Default:             scanner.Handshake{Status: scanner.Supported, Version: "TLS 1.3", Group: selected},
		KeyExchange:         scanner.KeyExchange{Selected: selected, SelectedHybrid: hybrid},
		Versions:            []scanner.VersionProbe{{Version: "TLS 1.3", Status: scanner.Supported}},
		TLS12SuitesComplete: true, LegacySuitesComplete: true,
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
	w := mustNew(t, st, sc, Config{Logger: quiet})
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

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

// unreachable wraps a dial-level cause the way the scanner reports it.
func unreachable(cause error) error {
	return fmt.Errorf("%w: connect: %w", scanner.ErrUnreachable,
		&net.OpError{Op: "dial", Net: "tcp", Err: cause})
}

func resolveErr(d *net.DNSError) error {
	return fmt.Errorf("%w: example.com: %w", scanner.ErrResolve, d)
}

func TestClassify(t *testing.T) {
	for _, tt := range []struct {
		name      string
		err       error
		code      string
		transient bool
	}{
		{"anti-SSRF block", fmt.Errorf("%w: 10.0.0.1 is in 10.0.0.0/8", scanner.ErrBlockedAddress), "blocked_address", false},
		{"invalid target", fmt.Errorf("%w: port 70000", scanner.ErrInvalidTarget), "invalid_target", false},
		{"NXDOMAIN", resolveErr(&net.DNSError{Err: "no such host", Name: "example.com", IsNotFound: true}), "dns_not_found", false},
		{"DNS timeout", resolveErr(&net.DNSError{Err: "i/o timeout", IsTimeout: true}), "dns_temporary", true},
		{"DNS SERVFAIL", resolveErr(&net.DNSError{Err: "server misbehaving", IsTemporary: true}), "dns_temporary", true},
		{"DNS other", fmt.Errorf("%w: no addresses", scanner.ErrResolve), "dns", false},
		{"connection refused", unreachable(os.NewSyscallError("connect", syscall.ECONNREFUSED)), "connection_refused", false},
		{"connect timeout", unreachable(timeoutErr{}), "timeout", true},
		{"scan deadline", fmt.Errorf("%w: %w", scanner.ErrUnreachable, context.DeadlineExceeded), "timeout", true},
		{"connection reset", unreachable(os.NewSyscallError("read", syscall.ECONNRESET)), "network", true},
		{"network unreachable", unreachable(os.NewSyscallError("connect", syscall.ENETUNREACH)), "network", true},
		{"closed mid-handshake", fmt.Errorf("%w: %w", scanner.ErrUnreachable, io.EOF), "network", true},
		{"not TLS", fmt.Errorf("%w: tls: first record does not look like a TLS handshake", scanner.ErrUnreachable), "unreachable", false},
		{"unknown", errors.New("boom"), "scan_error", false},
	} {
		if got := Classify(tt.err); got.Code != tt.code || got.Transient != tt.transient {
			t.Errorf("%s: Classify = %+v, want %s transient=%v", tt.name, got, tt.code, tt.transient)
		}
	}
}

func failingScanner(err error) Scanner {
	return scanFunc(func(context.Context, string, int) (*scanner.Result, error) { return nil, err })
}

func TestDefinitiveErrorsFailDirectly(t *testing.T) {
	st := newTestStore(t)
	for _, err := range []error{
		fmt.Errorf("%w: x", scanner.ErrBlockedAddress),
		resolveErr(&net.DNSError{Err: "no such host", IsNotFound: true}),
		unreachable(os.NewSyscallError("connect", syscall.ECONNREFUSED)),
	} {
		q := enqueue(t, st, "example.com")
		w := mustNew(t, st, failingScanner(err), Config{Logger: quiet})
		if ok, perr := w.ProcessOne(context.Background()); !ok || perr != nil {
			t.Fatalf("ProcessOne = %v, %v", ok, perr)
		}
		got := mustGet(t, st, q.ID)
		if got.Status != "failed" || got.Retries != 0 || got.Attempts != 1 || *got.Error != Classify(err).Code {
			t.Errorf("%v: status %s retries %d attempts %d error %v, want failed without retry", err, got.Status, got.Retries, got.Attempts, *got.Error)
		}
	}
}

// A transient error sends the scan back to the queue once, not before the
// retry delay; a second transient error fails it.
func TestTransientErrorIsRetriedOnce(t *testing.T) {
	st := newTestStore(t)
	q := enqueue(t, st, "example.com")
	w := mustNew(t, st, failingScanner(unreachable(timeoutErr{})), Config{RetryDelay: 400 * time.Millisecond, Logger: quiet})

	if ok, err := w.ProcessOne(context.Background()); !ok || err != nil {
		t.Fatalf("first ProcessOne = %v, %v", ok, err)
	}
	got := mustGet(t, st, q.ID)
	if got.Status != "queued" || got.Retries != 1 || got.Attempts != 1 || *got.Error != "timeout" || got.NotBefore == nil || got.LeaseToken != nil {
		t.Fatalf("after a transient error = %+v, want queued for a retry", got)
	}
	if d := time.Until(*got.NotBefore); d < 200*time.Millisecond || d > 500*time.Millisecond {
		t.Errorf("retry due in %v, want about 400ms", d)
	}
	if ok, err := w.ProcessOne(context.Background()); ok || err != nil {
		t.Fatalf("claimed before the retry delay: %v, %v", ok, err)
	}

	time.Sleep(500 * time.Millisecond)
	if ok, err := w.ProcessOne(context.Background()); !ok || err != nil {
		t.Fatalf("retry ProcessOne = %v, %v", ok, err)
	}
	got = mustGet(t, st, q.ID)
	if got.Status != "failed" || got.Retries != 1 || got.Attempts != 2 || *got.Error != "timeout" {
		t.Fatalf("after the second transient error = %+v, want failed", got)
	}
}

func TestTransientErrorThenSuccess(t *testing.T) {
	st := newTestStore(t)
	q := enqueue(t, st, "example.com")
	calls := 0
	flaky := scanFunc(func(_ context.Context, host string, port int) (*scanner.Result, error) {
		calls++
		if calls == 1 {
			return nil, unreachable(os.NewSyscallError("read", syscall.ECONNRESET))
		}
		return canned(host, port, "X25519MLKEM768"), nil
	})
	w := mustNew(t, st, flaky, Config{RetryDelay: 100 * time.Millisecond, Logger: quiet})
	_, _ = w.ProcessOne(context.Background())
	time.Sleep(200 * time.Millisecond)
	if ok, err := w.ProcessOne(context.Background()); !ok || err != nil {
		t.Fatalf("retry ProcessOne = %v, %v", ok, err)
	}
	if got := mustGet(t, st, q.ID); got.Status != "done" || got.Retries != 1 || got.Error != nil {
		t.Fatalf("after a successful retry = status %s retries %d error %v", got.Status, got.Retries, got.Error)
	}
}

// Real network failures from the real scanner are classified as expected.
func TestRealNetworkErrors(t *testing.T) {
	st := newTestStore(t)
	sc := scanner.New(scanner.Options{
		Resolver:         staticResolver{"example.com": netip.MustParseAddr("127.0.0.1")},
		AddrPolicy:       func(netip.Addr) error { return nil },
		HandshakeTimeout: 300 * time.Millisecond,
	})
	w := mustNew(t, st, sc, Config{Logger: quiet})

	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := closed.Addr().(*net.TCPAddr).Port
	closed.Close()

	silent, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { silent.Close() })
	go func() {
		for {
			c, err := silent.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { c.Close() })
		}
	}()

	for _, tt := range []struct {
		port   int
		status string
		code   string
	}{
		{closedPort, "failed", "connection_refused"},
		{silent.Addr().(*net.TCPAddr).Port, "queued", "timeout"}, // transient: back to the queue
	} {
		q, err := st.Enqueue(context.Background(), "example.com", int32(tt.port))
		if err != nil {
			t.Fatal(err)
		}
		if ok, err := w.ProcessOne(context.Background()); !ok || err != nil {
			t.Fatalf("ProcessOne = %v, %v", ok, err)
		}
		if got := mustGet(t, st, q.ID); got.Status != tt.status || *got.Error != tt.code {
			t.Errorf("port %d: status %s error %v, want %s %s", tt.port, got.Status, *got.Error, tt.status, tt.code)
		}
	}
}

func TestLeaseMustOutlastScanTimeout(t *testing.T) {
	st := newTestStore(t)
	real := scanner.New(scanner.Options{}) // 60 s scan timeout
	for _, tt := range []struct {
		lease, margin time.Duration
		ok            bool
	}{
		{0, 0, true}, // defaults: 90 s > 60 s + 15 s
		{60 * time.Second, 0, false},
		{75 * time.Second, 0, false}, // must exceed, not equal
		{76 * time.Second, 0, true},
		{70 * time.Second, 5 * time.Second, true},
	} {
		_, err := New(st, real, Config{Lease: tt.lease, LeaseMargin: tt.margin})
		if (err == nil) != tt.ok {
			t.Errorf("lease %v margin %v: err = %v, want ok=%v", tt.lease, tt.margin, err, tt.ok)
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
	a := mustNew(t, st, blocking, Config{Lease: 500 * time.Millisecond, Logger: quiet})
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

	b := mustNew(t, st, fastScanner("X25519MLKEM768"), Config{Lease: time.Minute, Logger: quiet})
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
	a := mustNew(t, st, stalled, Config{Lease: 300 * time.Millisecond, Logger: quiet})
	doneA := make(chan error, 1)
	go func() { _, err := a.ProcessOne(context.Background()); doneA <- err }()
	<-started

	time.Sleep(400 * time.Millisecond)
	b := mustNew(t, st, fastScanner("X25519MLKEM768"), Config{Logger: quiet})
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

func runInBackground(t *testing.T, w *Worker) {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	t.Cleanup(func() {
		stop()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
}

// A slot's beat advances on every iteration, idle ones included, and not while
// an iteration (a scan) is still running.
func TestSlotBeatsFromTheLoop(t *testing.T) {
	st := newTestStore(t)
	started, release := make(chan struct{}, 1), make(chan struct{})
	sc := scanFunc(func(_ context.Context, host string, port int) (*scanner.Result, error) {
		started <- struct{}{}
		<-release
		return canned(host, port, "X25519MLKEM768"), nil
	})
	w := mustNew(t, st, sc, Config{Concurrency: 1, PollInterval: 20 * time.Millisecond, ShutdownTimeout: time.Second, Logger: quiet})
	runInBackground(t, w)

	time.Sleep(200 * time.Millisecond)
	if age := time.Since(w.SlotBeats()[0]); age > 100*time.Millisecond {
		t.Fatalf("idle slot last beat %v ago, want a beat every poll (20ms)", age)
	}

	enqueue(t, st, "example.com")
	<-started
	before := w.SlotBeats()[0]
	time.Sleep(300 * time.Millisecond)
	if during := w.SlotBeats()[0]; !during.Equal(before) {
		t.Errorf("slot beat during a scan (%v → %v), want no beat until it ends", before, during)
	}

	close(release)
	time.Sleep(150 * time.Millisecond)
	if age := time.Since(w.SlotBeats()[0]); age > 100*time.Millisecond {
		t.Errorf("no fresh beat after the scan finished (last %v ago)", age)
	}
}

// With two slots, a stuck slot stops beating while the other keeps going: the
// healthcheck writer (cmd/worker) sees exactly which one.
func TestSlotBeatsShowTheStuckSlot(t *testing.T) {
	st := newTestStore(t)
	started := make(chan struct{}, 1)
	stuck := scanFunc(func(ctx context.Context, _ string, _ int) (*scanner.Result, error) {
		started <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	})
	w := mustNew(t, st, stuck, Config{Concurrency: 2, PollInterval: 20 * time.Millisecond, ShutdownTimeout: 100 * time.Millisecond, Logger: quiet})
	runInBackground(t, w)

	time.Sleep(100 * time.Millisecond)
	enqueue(t, st, "example.com")
	<-started
	stuckAt := time.Now()
	time.Sleep(300 * time.Millisecond)

	var fresh, silent int
	for _, b := range w.SlotBeats() {
		if b.After(stuckAt) {
			fresh++
		} else {
			silent++
		}
	}
	if fresh != 1 || silent != 1 {
		t.Errorf("after one slot got stuck: %d slots beating, %d silent; want 1 and 1 (%v)", fresh, silent, w.SlotBeats())
	}
}

// Iterations that fail (database down) never beat.
func TestNoSlotBeatsWhenTheQueueFails(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	st := store.NewStore(pool)
	pool.Close() // every query fails from now on
	w := mustNew(t, st, fastScanner("X25519MLKEM768"), Config{Concurrency: 1, PollInterval: 20 * time.Millisecond, Logger: quiet})
	runInBackground(t, w)
	time.Sleep(50 * time.Millisecond)
	first := w.SlotBeats()[0]
	time.Sleep(200 * time.Millisecond)
	if b := w.SlotBeats()[0]; !b.Equal(first) {
		t.Errorf("slot beat with a failing queue (%v → %v)", first, b)
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
	w := mustNew(t, st, slow, Config{Concurrency: 2, PollInterval: 20 * time.Millisecond, Logger: quiet})
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
	w := mustNew(t, st, blocking, Config{Concurrency: 1, ShutdownTimeout: 200 * time.Millisecond, Logger: quiet})
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
