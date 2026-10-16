package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/testdb"
	"github.com/fizsar/quantumready-saas/migrations"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	return NewStore(pool)
}

func ptr[T any](v T) *T { return &v }

func completeParams(sc Scan) CompleteScanParams {
	return CompleteScanParams{
		ID: sc.ID, LeaseToken: sc.LeaseToken,
		Result: []byte(`{"host":"example.com"}`), ScannerSchema: ptr(int32(1)),
		Scoring: []byte(`{"score":95}`), ScoringVersion: ptr("2026.1"),
		Score: ptr(int32(95)), Grade: ptr("A"), Pqc: ptr(true),
	}
}

func findingsFor(sc Scan) []InsertFindingsParams {
	return []InsertFindingsParams{
		{ScanID: sc.ID, Position: 0, Code: "KEX_SINGLE_HYBRID", Axis: "quantum", Severity: "low", Category: "key_exchange", Params: []byte(`{"group":"X25519MLKEM768"}`)},
		{ScanID: sc.ID, Position: 1, Code: "FS_ALL", Axis: "current", Severity: "pass", Category: "cipher_suites", Params: []byte(`{}`)},
	}
}

func TestEnqueueClaimComplete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	queued, err := s.Enqueue(ctx, "example.com", 443)
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != "queued" || queued.Attempts != 0 || queued.LeaseToken != nil || queued.OrganizationID != nil {
		t.Fatalf("enqueued = %+v", queued)
	}

	claimed, err := s.Claim(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != queued.ID || claimed.Status != "running" || claimed.Attempts != 1 || claimed.LeaseToken == nil ||
		claimed.LeaseUntil == nil || claimed.StartedAt == nil {
		t.Fatalf("claimed = %+v", claimed)
	}
	if d := time.Until(*claimed.LeaseUntil); d < 50*time.Second || d > 70*time.Second {
		t.Errorf("lease ends in %v, want about 1m", d)
	}
	if _, err := s.Claim(ctx, time.Minute); !errors.Is(err, ErrNoScan) {
		t.Fatalf("second claim = %v, want ErrNoScan (lease still valid)", err)
	}

	if err := s.Complete(ctx, completeParams(claimed), findingsFor(claimed)); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetScan(ctx, claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != "done" || done.LeaseToken != nil || done.LeaseUntil != nil || done.FinishedAt == nil ||
		*done.ScoringVersion != "2026.1" || *done.Grade != "A" || *done.Score != 95 || !*done.Pqc {
		t.Fatalf("done = %+v", done)
	}
	fs, err := s.ListFindings(ctx, claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 2 || fs[0].Code != "KEX_SINGLE_HYBRID" || fs[1].Code != "FS_ALL" {
		t.Fatalf("findings = %+v", fs)
	}
}

func TestClaimEmptyAndFIFO(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Claim(ctx, time.Minute); !errors.Is(err, ErrNoScan) {
		t.Fatalf("empty queue: %v", err)
	}
	var ids []uuid.UUID
	for i := range 3 {
		sc, err := s.Enqueue(ctx, fmt.Sprintf("host%d.example", i), 443)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, sc.ID)
	}
	for i, want := range ids {
		sc, err := s.Claim(ctx, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if sc.ID != want {
			t.Errorf("claim %d = %s, want %s (oldest first)", i, sc.Host, want)
		}
	}
}

// Concurrent workers never claim the same scan (FOR UPDATE SKIP LOCKED).
func TestConcurrentClaimsAreExclusive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const jobs, workers = 30, 8
	for i := range jobs {
		if _, err := s.Enqueue(ctx, fmt.Sprintf("h%d.example", i), 443); err != nil {
			t.Fatal(err)
		}
	}
	var (
		mu   sync.Mutex
		seen = map[uuid.UUID]int{}
		wg   sync.WaitGroup
	)
	for range workers {
		wg.Go(func() {
			for {
				sc, err := s.Claim(ctx, time.Minute)
				if errors.Is(err, ErrNoScan) {
					return
				}
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				seen[sc.ID]++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if len(seen) != jobs {
		t.Errorf("claimed %d distinct scans, want %d", len(seen), jobs)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("scan %s claimed %d times", id, n)
		}
	}
}

// A worker dies (or stalls) holding a lease: once it expires another worker
// claims the scan, and the first one can no longer complete or fail it.
func TestExpiredLeaseIsReclaimed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Enqueue(ctx, "example.com", 443); err != nil {
		t.Fatal(err)
	}

	a, err := s.Claim(ctx, 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)

	b, err := s.Claim(ctx, time.Minute)
	if err != nil {
		t.Fatalf("reclaim after expiry: %v", err)
	}
	if b.ID != a.ID || b.Attempts != 2 || *b.LeaseToken == *a.LeaseToken {
		t.Fatalf("reclaimed = %+v (first claim %+v)", b, a)
	}

	if err := s.Complete(ctx, completeParams(a), findingsFor(a)); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("late Complete by the first worker = %v, want ErrLeaseLost", err)
	}
	if err := s.Fail(ctx, a.ID, *a.LeaseToken, "unreachable"); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("late Fail by the first worker = %v, want ErrLeaseLost", err)
	}
	if fs, _ := s.ListFindings(ctx, a.ID); len(fs) != 0 {
		t.Errorf("the rejected completion left %d findings", len(fs))
	}

	if err := s.Complete(ctx, completeParams(b), findingsFor(b)); err != nil {
		t.Fatalf("Complete by the new holder: %v", err)
	}
}

func TestAttemptsExhausted(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sc, err := s.Enqueue(ctx, "example.com", 443)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 { // max_attempts defaults to 3
		c, err := s.Claim(ctx, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("claim %d: %v", i+1, err)
		}
		if c.Attempts != int32(i+1) {
			t.Fatalf("attempts = %d, want %d", c.Attempts, i+1)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if _, err := s.Claim(ctx, time.Minute); !errors.Is(err, ErrNoScan) {
		t.Fatalf("claim after 3 attempts = %v, want ErrNoScan", err)
	}
	ids, err := s.FailExpiredScans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != sc.ID {
		t.Fatalf("FailExpiredScans = %v", ids)
	}
	got, _ := s.GetScan(ctx, sc.ID)
	if got.Status != "failed" || got.Error == nil || *got.Error != "lease_expired" || got.LeaseToken != nil {
		t.Fatalf("after exhaustion = %+v", got)
	}
}

func TestFail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Enqueue(ctx, "example.com", 443); err != nil {
		t.Fatal(err)
	}
	c, _ := s.Claim(ctx, time.Minute)
	if err := s.Fail(ctx, c.ID, uuid.New(), "unreachable"); !errors.Is(err, ErrLeaseLost) {
		t.Errorf("Fail with a wrong token = %v, want ErrLeaseLost", err)
	}
	if err := s.Fail(ctx, c.ID, *c.LeaseToken, "unreachable"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetScan(ctx, c.ID)
	if got.Status != "failed" || *got.Error != "unreachable" || got.FinishedAt == nil {
		t.Fatalf("failed scan = %+v", got)
	}
}

// publicBasic is EnqueueOrReuse for a public basic scan (the phase 2 behaviour).
// The global cap counts every scan started in the last minute, by any worker.
func TestClaimCapped(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, h := range []string{"a.example", "b.example", "c.example"} {
		if _, _, err := publicBasic(ctx, s, h, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 2 {
		if _, err := s.ClaimCapped(ctx, time.Minute, 2); err != nil {
			t.Fatalf("claim %d: %v", i, err)
		}
	}
	if _, err := s.ClaimCapped(ctx, time.Minute, 2); !errors.Is(err, ErrNoScan) {
		t.Errorf("third claim within the minute: %v, want ErrNoScan", err)
	}
	if _, err := s.pool.Exec(ctx, "UPDATE scans SET started_at = now() - interval '61 seconds' WHERE started_at IS NOT NULL"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimCapped(ctx, time.Minute, 2); err != nil {
		t.Errorf("after a minute: %v", err)
	}
}

func publicBasic(ctx context.Context, s *Store, host string, cooldown time.Duration) (Scan, bool, error) {
	return s.EnqueueOrReuse(ctx, Public, ScanRequest{Host: host, Port: 443, Intensity: IntensityBasic, Cooldown: cooldown})
}

func TestEnqueueOrReuse(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	first, reused, err := publicBasic(ctx, s, "example.com", time.Minute)
	if err != nil || reused {
		t.Fatalf("first = %v reused=%v", err, reused)
	}
	again, reused, err := publicBasic(ctx, s, "example.com", time.Minute)
	if err != nil || !reused || again.ID != first.ID {
		t.Fatalf("within cooldown: %v reused=%v id=%s, want the first scan", err, reused, again.ID)
	}
	if _, reused, _ := publicBasic(ctx, s, "other.example", time.Minute); reused {
		t.Error("another target was reused")
	}

	// A failed scan is not reused.
	c, _ := s.Claim(ctx, time.Minute)
	if err := s.Fail(ctx, c.ID, *c.LeaseToken, "unreachable"); err != nil {
		t.Fatal(err)
	}
	if fresh, reused, _ := publicBasic(ctx, s, "example.com", time.Minute); reused || fresh.ID == first.ID {
		t.Errorf("a failed scan was reused")
	}

	// Outside the cooldown a new scan is created.
	time.Sleep(300 * time.Millisecond)
	if _, reused, _ := publicBasic(ctx, s, "other.example", 200*time.Millisecond); reused {
		t.Error("reused after the cooldown")
	}
}

// Concurrent requests for the same target create a single scan.
func TestEnqueueOrReuseConcurrent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	var (
		mu  sync.Mutex
		ids = map[uuid.UUID]bool{}
		wg  sync.WaitGroup
	)
	for range 10 {
		wg.Go(func() {
			sc, _, err := publicBasic(ctx, s, "example.com", time.Minute)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			ids[sc.ID] = true
			mu.Unlock()
		})
	}
	wg.Wait()
	if len(ids) != 1 {
		t.Errorf("%d scans created, want 1", len(ids))
	}
}

func TestSchemaConstraints(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Enqueue(ctx, "example.com", 70000); err == nil {
		t.Error("port 70000 accepted")
	}
	if _, err := s.Enqueue(ctx, "", 443); err == nil {
		t.Error("empty host accepted")
	}
	// A done scan must carry its result and scoring version.
	if _, err := s.Enqueue(ctx, "example.com", 443); err != nil {
		t.Fatal(err)
	}
	c, _ := s.Claim(ctx, time.Minute)
	p := completeParams(c)
	p.ScoringVersion = nil
	if err := s.Complete(ctx, p, findingsFor(c)); err == nil {
		t.Error("done scan without scoring_version accepted")
	}
}
