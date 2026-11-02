package store

import (
	"context"
	"strings"
	"testing"

	"github.com/fizsar/quantumready-saas/internal/testdb"
)

// Migration 00022: refused public scan requests, per day and kind. qr_app can
// only add through add_scan_refusals; the table holds a day, a kind from the
// closed list and a number, nothing about who asked.
func TestScanRefusals(t *testing.T) {
	owner := newTestStore(t).Pool()
	ctx := context.Background()
	app := NewStore(testdb.AsRole(t, owner, RoleApp))

	for _, add := range []struct {
		kind string
		n    int64
	}{{"client_limit", 2}, {"client_limit", 3}, {"proof_failed", 1}, {"excluded", 4}} {
		if err := app.AddScanRefusals(ctx, add.kind, add.n); err != nil {
			t.Fatalf("qr_app adds %d %s: %v", add.n, add.kind, err)
		}
	}
	got := map[string]int64{}
	rows, err := owner.Query(ctx, "SELECT kind, count FROM scan_refusals_daily WHERE day = (now() AT TIME ZONE 'UTC')::date")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k string
		var n int64
		if err := rows.Scan(&k, &n); err != nil {
			t.Fatal(err)
		}
		got[k] = n
	}
	if len(got) != 3 || got["client_limit"] != 5 || got["proof_failed"] != 1 || got["excluded"] != 4 {
		t.Errorf("today's counts: %v", got)
	}

	// Only the closed list, only positive counts.
	for _, bad := range []struct {
		kind string
		n    int64
	}{{"203.0.113.7", 1}, {"example.org", 1}, {"busy", 1}, {"client_limit", 0}, {"client_limit", -3}} {
		if err := app.AddScanRefusals(ctx, bad.kind, bad.n); err == nil {
			t.Errorf("accepted %d %q", bad.n, bad.kind)
		}
	}

	// qr_app cannot read, write, change or delete the rows itself; the
	// worker and the mailer cannot even add.
	appPool := testdb.AsRole(t, owner, RoleApp)
	for _, sql := range []string{
		"SELECT count(*) FROM scan_refusals_daily",
		"INSERT INTO scan_refusals_daily (day, kind, count) VALUES (current_date, 'excluded', 1)",
		"UPDATE scan_refusals_daily SET count = 1",
		"DELETE FROM scan_refusals_daily",
	} {
		if _, err := appPool.Exec(ctx, sql); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("qr_app ran %q: %v", sql, err)
		}
	}
	for _, role := range []string{RoleWorker, RoleMailer} {
		if err := NewStore(testdb.AsRole(t, owner, role)).AddScanRefusals(ctx, "excluded", 1); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%s added a refusal: %v", role, err)
		}
	}

	// The Go list and the migration's agree: every kind the API may write is accepted.
	for _, kind := range RefusalKinds {
		if err := app.AddScanRefusals(ctx, kind, 1); err != nil {
			t.Errorf("%s refused by the database: %v", kind, err)
		}
	}

	// Exactly these columns: a column for an address, a domain or a client
	// would be the start of identifying who asked.
	var cols string
	if err := owner.QueryRow(ctx, `SELECT string_agg(column_name || ' ' || data_type, ', ' ORDER BY ordinal_position)
		FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'scan_refusals_daily'`).Scan(&cols); err != nil {
		t.Fatal(err)
	}
	if cols != "day date, kind text, count bigint" {
		t.Errorf("columns: %s", cols)
	}
}
