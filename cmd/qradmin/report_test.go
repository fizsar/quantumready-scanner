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

func TestPublicReportUsage(t *testing.T) {
	env := func(string) string { return "" }
	for _, args := range [][]string{{"public-report", "extra"}, {"public-report", "-days", "0"}, {"public-report", "-days", "32"}, {"public-report", "-min", "1"}, {"public-report"}} {
		if err := run(context.Background(), args, env, &bytes.Buffer{}); err == nil {
			t.Errorf("run(%q) accepted", args)
		}
	}
}

func TestMaxQueueByDay(t *testing.T) {
	day := func(d int, hm string) time.Time {
		t0, _ := time.Parse("2006-01-02 15:04", "2026-11-0"+string(rune('0'+d))+" "+hm)
		return t0
	}
	span := func(a, b time.Time) store.PublicQueueSpansRow {
		return store.PublicQueueSpansRow{CreatedAt: a, LeftQueue: b}
	}
	since, now := day(2, "00:00"), day(4, "12:00")
	got := maxQueueByDay([]store.PublicQueueSpansRow{
		span(day(1, "23:00"), day(2, "00:30")), // waits across midnight into the first day
		span(day(2, "10:00"), day(2, "10:05")),
		span(day(2, "10:01"), day(2, "10:06")),
		span(day(2, "10:02"), day(2, "10:03")), // three at once at 10:02
		span(day(2, "10:05"), day(2, "10:07")), // enters as one leaves: still three at most
		span(day(3, "10:00"), day(3, "10:05")),
		span(day(3, "10:05"), day(3, "10:10")), // takes over as the other leaves: one at a time
		span(day(3, "23:50"), day(4, "00:10")), // across midnight: counts on day 4 too
		span(day(4, "08:00"), day(4, "08:00")), // never waited
	}, since, now)
	want := map[string]int64{"2026-11-02": 3, "2026-11-03": 1, "2026-11-04": 1}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%s: %d, want %d (all: %v)", k, got[k], n, got)
		}
	}
	if _, ok := got["2026-11-01"]; ok {
		t.Errorf("a day before since is reported: %v", got)
	}
}

func TestPublicReport(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO organizations (id, name) VALUES ('00000000-0000-4000-8000-0000000000aa', 'Acme');
		-- Public scans: yesterday and today (UTC).
		INSERT INTO scans (host, status, created_at, started_at, finished_at, error, intensity, result, scoring_version, grade) VALUES
		  ('www.often.org',  'done',   now() - interval '1 day',  now() - interval '1 day',  now() - interval '1 day', NULL, 'basic', '{}', '2026.3', 'A'),
		  ('often.org',      'done',   now() - interval '1 day',  now() - interval '1 day',  now() - interval '1 day', NULL, 'basic', '{}', '2026.3', 'B'),
		  ('shop.often.org', 'failed', now(),                     now(),                     now(),                   'dns_not_found', 'basic', NULL, NULL, NULL),
		  ('fizsar.com',     'done',   now(),                     now(),                     now(),                   NULL, 'full', '{}', '2026.3', 'A'),
		  ('twice.org',      'failed', now(),                     NULL,                      now(),                   'queue_timeout', 'basic', NULL, NULL, NULL),
		  ('twice.org',      'queued', now(),                     NULL,                      NULL,                    NULL, 'basic', NULL, NULL, NULL),
		  -- Before the period: not counted.
		  ('old.org',        'done',   now() - interval '20 days', now() - interval '20 days', now() - interval '20 days', NULL, 'basic', '{}', '2026.3', 'A');
		-- An organization's scan: never in the public report.
		INSERT INTO scans (organization_id, host, status, created_at, started_at, finished_at, intensity, result, scoring_version, grade)
		VALUES ('00000000-0000-4000-8000-0000000000aa', 'private.org', 'done', now(), now(), now(), 'full', '{}', '2026.3', 'A');
		INSERT INTO scan_exclusions (name, reason, created_at) VALUES ('excluded.org', 'owner asked', now()), ('older.org', NULL, now() - interval '30 days')`); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	now := time.Now().UTC()
	since := now.Truncate(24*time.Hour).AddDate(0, 0, -6)
	if err := publicReport(ctx, store.NewStore(pool).Queries, since, now, 3, &out); err != nil {
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
	today, yesterday := now.Format(time.DateOnly), now.AddDate(0, 0, -1).Format(time.DateOnly)
	// day scans done failed full domains queue_timeouts max_queue exclusions_added
	if got := fields(yesterday); got != yesterday+" 2 2 0 0 1 0 0 0" {
		t.Errorf("yesterday: %q", got)
	}
	if got := fields(today); !strings.HasPrefix(got, today+" 4 1 2 1 3 1 ") || !strings.HasSuffix(got, " 1") {
		t.Errorf("today: %q", got)
	}
	if got := fields("total"); !strings.HasPrefix(got, "total 6 3 2 1 3 1 ") {
		t.Errorf("total: %q", got)
	}
	// The most scanned: often.org (3, by registrable domain); twice.org (2) is under the threshold.
	if !strings.Contains(s, "often.org  3") && fields("often.org") != "often.org 3" {
		t.Errorf("often.org not listed with 3")
	}
	for _, never := range []string{"twice.org", "private.org", "old.org", "excluded.org"} {
		if strings.Contains(s, never) {
			t.Errorf("%s in the report", never)
		}
	}
	if !strings.Contains(s, "Exclusions in force: 2") || !strings.Contains(s, "Not recorded: refusals by limit") {
		t.Errorf("exclusions or the not-recorded note missing")
	}
	if strings.Contains(s, "IP") {
		t.Errorf("the report mentions IPs")
	}
}
