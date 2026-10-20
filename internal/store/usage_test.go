package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// The domains of blocked scan requests are deleted after keep_days.
func TestPurgeBlockedTargets(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, ago := range []int{40, 31, 30, 1, 0} {
		d := now.AddDate(0, 0, -ago)
		day := pgtype.Date{Time: time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
		if err := st.CountBlockedTarget(ctx, CountBlockedTargetParams{Day: day, Domain: "example.org", MaxDomains: 10}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := st.PurgeBlockedTargets(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("purged %d rows, want 2 (40 and 31 days old)", n)
	}
	var left int
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM blocked_targets_daily`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 3 {
		t.Errorf("%d rows left, want 3 (30, 1 and 0 days old)", left)
	}
}

// Counters add up per day, event and dim; past max_dims different dims a
// day, new ones go to "other", so junk cannot grow the table without bound.
func TestCountUsageCapsDims(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	day := pgtype.Date{Time: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), Valid: true}
	count := func(dim string) {
		t.Helper()
		if err := st.CountUsage(ctx, CountUsageParams{Day: day, Event: "visit_source", Dim: dim, MaxDims: 3}); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{"a.com", "b.com", "a.com", "c.com", "d.com", "e.com", "b.com"} {
		count(d)
	}
	rows, err := st.UsageSince(ctx, day)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, r := range rows {
		got[r.Dim] = r.Count
	}
	want := map[string]int64{"a.com": 2, "b.com": 2, "c.com": 1, "other": 2}
	if len(got) != len(want) {
		t.Fatalf("rows %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %d, want %d (all: %v)", k, got[k], v, got)
		}
	}
	// An event outside the closed list is refused by the table itself.
	if err := st.CountUsage(ctx, CountUsageParams{Day: day, Event: "signup", Dim: "", MaxDims: 3}); err == nil {
		t.Error("an unknown event was stored")
	}
}
