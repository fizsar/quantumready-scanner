package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/testdb"
	"github.com/fizsar/quantumready-saas/migrations"
)

// queued inserts a queued scan created ago, public (org nil) or of org.
func queued(t *testing.T, s *Store, host string, org *uuid.UUID, ago time.Duration) uuid.UUID {
	t.Helper()
	intensity := IntensityBasic
	if org != nil {
		intensity = IntensityFull
	}
	var id uuid.UUID
	if err := s.pool.QueryRow(context.Background(), `INSERT INTO scans (host, port, organization_id, intensity, created_at)
		VALUES ($1, 443, $2, $3, now() - make_interval(secs => $4)) RETURNING id`, host, org, intensity, ago.Seconds()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func queueStore(t *testing.T) (*Store, *Store, uuid.UUID) {
	t.Helper()
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	org, _ := orgWithOwners(t, pool, 1)
	return NewStore(pool), NewStore(testdb.AsRole(t, pool, RoleWorker)), org
}

// Organizations' scans go first; within each group, the oldest.
func TestQueuePriority(t *testing.T) {
	owner, worker, org := queueStore(t)
	ctx := context.Background()
	pubOld := queued(t, owner, "public-old.example", nil, 10*time.Minute)
	pubNew := queued(t, owner, "public-new.example", nil, time.Minute)
	orgNew := queued(t, owner, "org-new.example", &org, 30*time.Second)
	orgOld := queued(t, owner, "org-old.example", &org, 5*time.Minute)
	var got []uuid.UUID
	for range 4 {
		sc, err := worker.Claim(ctx, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, sc.ID)
	}
	want := []uuid.UUID{orgOld, orgNew, pubOld, pubNew}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("claim order %v, want %v (org old, org new, public old, public new)", got, want)
		}
	}
}

// Only public scans expire in the queue, and only after the timeout.
func TestExpireQueuedPublicScans(t *testing.T) {
	owner, worker, org := queueStore(t)
	ctx := context.Background()
	pubOld := queued(t, owner, "public-old.example", nil, 16*time.Minute)
	pubNew := queued(t, owner, "public-new.example", nil, 10*time.Minute)
	orgOld := queued(t, owner, "org-old.example", &org, 2*time.Hour)
	ids, err := worker.ExpireQueuedPublicScans(ctx, (15 * time.Minute).Seconds())
	if err != nil || len(ids) != 1 || ids[0] != pubOld {
		t.Fatalf("expired %v (%v), want only the old public scan", ids, err)
	}
	state := func(id uuid.UUID) string {
		var s string
		var e *string
		_ = owner.pool.QueryRow(ctx, "SELECT status, error FROM scans WHERE id = $1", id).Scan(&s, &e)
		if e != nil {
			s += ":" + *e
		}
		return s
	}
	if state(pubOld) != "failed:queue_timeout" || state(pubNew) != "queued" || state(orgOld) != "queued" {
		t.Errorf("states: %s %s %s", state(pubOld), state(pubNew), state(orgOld))
	}
}

// The public queue length and positions ignore organizations' scans.
func TestPublicQueueCounts(t *testing.T) {
	owner, _, org := queueStore(t)
	ctx := context.Background()
	queued(t, owner, "org1.example", &org, 20*time.Minute)
	p1 := queued(t, owner, "p1.example", nil, 3*time.Minute)
	queued(t, owner, "org2.example", &org, 2*time.Minute)
	queued(t, owner, "p2.example", nil, time.Minute)
	app := NewStore(testdb.AsRole(t, owner.pool, RoleApp))
	if n, err := app.PublicQueueLength(ctx); err != nil || n != 2 {
		t.Errorf("public queue length %d (%v), want 2", n, err)
	}
	var created time.Time
	_ = owner.pool.QueryRow(ctx, "SELECT created_at FROM scans WHERE id = $1", p1).Scan(&created)
	if n, err := app.PublicQueuePosition(ctx, created); err != nil || n != 1 {
		t.Errorf("position of the oldest public scan %d (%v), want 1", n, err)
	}
}
