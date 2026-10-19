package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

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
