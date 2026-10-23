package main

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/fizsar/quantumready-saas/internal/store"
	"github.com/fizsar/quantumready-saas/internal/testdb"
	"github.com/fizsar/quantumready-saas/migrations"
)

func TestStatsUsage(t *testing.T) {
	env := func(string) string { return "" }
	for _, args := range [][]string{{"stats", "extra"}, {"stats", "-days", "0"}, {"stats", "-days", "367"}, {"stats"}} {
		if err := run(context.Background(), args, env, &bytes.Buffer{}); err == nil {
			t.Errorf("run(%q) accepted", args)
		}
	}
}

func TestStats(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO usage_daily (day, event, dim, count) VALUES
		  (current_date - 1, 'page_view', '/', 7),
		  (current_date - 1, 'visit_source', 'linkedin.com', 4),
		  (current_date, 'page_view', '/', 3),
		  (current_date, 'page_view', '/examples', 2),
		  (current_date, 'scan_blocked_403', '', 6),
		  (current_date - 40, 'page_view', '/', 1000);
		INSERT INTO blocked_targets_daily (day, domain, count) VALUES
		  (current_date - 1, 'often.org', 2), (current_date, 'often.org', 1),
		  (current_date, 'twice.org', 2),
		  (current_date, 'once.org', 1),
		  (current_date, 'popular.org', 9)`); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	since := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -29)
	if err := stats(ctx, store.NewStore(pool).Queries, since, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	t.Log("\n" + s)
	fields := func(prefix string) string {
		for _, l := range strings.Split(s, "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), prefix) {
				return strings.Join(strings.Fields(l), " ")
			}
		}
		return ""
	}
	// day × event columns: page_view visit_source scan_started scan_blocked_403 ...
	if got := fields("total"); got != "total 12 4 0 6 0 0 0 0" {
		t.Errorf("total line: %q (the 40-day-old row must not count)", got)
	}
	if got := fields("/examples"); got != "/examples 2" {
		t.Errorf("page_view by route: %q", got)
	}
	if got := fields("linkedin.com"); got != "linkedin.com 4" {
		t.Errorf("visit sources: %q", got)
	}
	// The threshold of 3 lives in the query: 3 over two days is listed, 2 is not.
	if !strings.Contains(s, "popular.org") || !strings.Contains(s, "often.org") || fields("often.org") != "often.org 3" {
		t.Errorf("domains asked 3+ times missing:\n%s", s)
	}
	if strings.Contains(s, "twice.org") || strings.Contains(s, "once.org") {
		t.Errorf("domains asked fewer than 3 times listed:\n%s", s)
	}
	if strings.Contains(s, "scan_blocked_403 (top") {
		t.Error("scan_blocked_403 has no values to list")
	}
	if strings.Index(s, "popular.org") > strings.Index(s, "often.org") {
		t.Error("refused targets not sorted by count")
	}
}

func TestStatsEmpty(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	var out bytes.Buffer
	if err := stats(context.Background(), store.NewStore(pool).Queries, time.Now().UTC(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "(none)") {
		t.Errorf("empty stats: %q", out.String())
	}
}
