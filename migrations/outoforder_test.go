package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/fizsar/quantumready-saas/internal/store"
	"github.com/fizsar/quantumready-saas/internal/testdb"
)

func sqlFile(table string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("-- +goose Up\nCREATE TABLE " + table + " (id int);\n-- +goose Down\nDROP TABLE " + table + ";\n")}
}

func tableExists(ctx context.Context, db *sql.DB, name string) bool {
	var ok bool
	_ = db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&ok)
	return ok
}

// Migrations apply in order only: a pending one below the newest applied
// one is refused, and nothing is applied. (The window for 00004–00016 out of
// order closed with the rollback window to showcase, on 16/11/2026.)
func TestNoOutOfOrder(t *testing.T) {
	testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		first := fstest.MapFS{"00001_a.sql": sqlFile("t1"), "00002_b.sql": sqlFile("t2"), "00015_c.sql": sqlFile("t15")}
		if _, err := up(ctx, db, first); err != nil {
			t.Fatalf("first: %v", err)
		}
		later := fstest.MapFS{"00004_d.sql": sqlFile("t4"), "00016_e.sql": sqlFile("t16")}
		for k, v := range first {
			later[k] = v
		}
		if _, err := up(ctx, db, later); err == nil || !strings.Contains(err.Error(), "[4]") {
			t.Errorf("00004 below 00015: %v, want a refusal naming it", err)
		}
		if tableExists(ctx, db, "t4") || tableExists(ctx, db, "t16") {
			t.Error("a migration was applied after the refusal")
		}
		// In order, nothing missing below: applied.
		delete(later, "00004_d.sql")
		res, err := up(ctx, db, later)
		if err != nil || !slices.Equal(versions(res), []int64{16}) {
			t.Errorf("00016 after 00015: %v %v", versions(res), err)
		}
		return nil
	})
}

func versions(res []*goose.MigrationResult) []int64 {
	var v []int64
	for _, r := range res {
		v = append(v, r.Source.Version)
	}
	return v
}

// m1Privileges lists every privilege on the tables that come from showcase
// (M1 and demo_requests) held by anyone but their owner, table-wide ("table
// role PRIVILEGE") and per column ("table.column role PRIVILEGE"), and their
// RLS state.
func m1Privileges(t *testing.T, pool *pgxpool.Pool) (grants []string, rls string) {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, `
		SELECT c.relname || ' ' || CASE WHEN a.grantee = 0 THEN 'PUBLIC' ELSE a.grantee::regrole::text END || ' ' || a.privilege_type
		FROM pg_class c, aclexplode(coalesce(c.relacl, acldefault('r', c.relowner))) a
		WHERE c.oid IN (to_regclass('usage_daily'), to_regclass('blocked_targets_daily'), to_regclass('demo_requests')) AND a.grantee <> c.relowner
		UNION ALL
		SELECT c.relname || '.' || at.attname || ' ' || CASE WHEN a.grantee = 0 THEN 'PUBLIC' ELSE a.grantee::regrole::text END || ' ' || a.privilege_type
		FROM pg_class c JOIN pg_attribute at ON at.attrelid = c.oid, aclexplode(at.attacl) a
		WHERE c.oid IN (to_regclass('usage_daily'), to_regclass('blocked_targets_daily'), to_regclass('demo_requests')) AND at.attacl IS NOT NULL AND a.grantee <> c.relowner
		ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	if grants, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(ctx, `
		SELECT string_agg(c.relname || ' rls=' || c.relrowsecurity || ' force=' || c.relforcerowsecurity
		       || ' policies=' || (SELECT count(*) FROM pg_policy p WHERE p.polrelid = c.oid), ', ' ORDER BY c.relname)
		FROM pg_class c WHERE c.oid IN (to_regclass('usage_daily'), to_regclass('blocked_targets_daily'), to_regclass('demo_requests'))`).Scan(&rls)
	if err != nil {
		t.Fatal(err)
	}
	return grants, rls
}

var (
	wantM1Grants = []string{
		"blocked_targets_daily qr_app DELETE",
		"blocked_targets_daily qr_app INSERT",
		"blocked_targets_daily qr_app SELECT",
		"blocked_targets_daily qr_app UPDATE",
		"demo_requests qr_app DELETE",
		"demo_requests qr_app INSERT",
		"demo_requests.created_at qr_app SELECT",
		"demo_requests.id qr_app SELECT",
		"usage_daily qr_app INSERT",
		"usage_daily qr_app SELECT",
		"usage_daily qr_app UPDATE",
	}
	wantM1RLS = "blocked_targets_daily rls=false force=false policies=0, demo_requests rls=false force=false policies=0, usage_daily rls=false force=false policies=0"
)

// The tables M1 and the demo form brought (usage_daily, blocked_targets_daily,
// demo_requests): exactly these privileges and no RLS; the API (qr_app) can
// count, store demo requests and purge both, without reading a request back;
// the other roles cannot touch them. (Until 16/11/2026 this ran on the path
// production really took, through the showcase branch; production now has
// every migration, in order from here on.)
func TestM1Privileges(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := Up(ctx, db)
		return err
	})
	grants, rls := m1Privileges(t, pool)
	if !slices.Equal(grants, wantM1Grants) {
		t.Errorf("M1 privileges:\n %v\nwant\n %v", grants, wantM1Grants)
	}
	if rls != wantM1RLS {
		t.Errorf("M1 RLS: %s, want %s", rls, wantM1RLS)
	}

	ctx := context.Background()
	day := pgtype.Date{Time: time.Now().UTC().Truncate(24 * time.Hour), Valid: true}
	demo := store.InsertDemoRequestParams{Name: "Ana", Email: "ana@example.org", Company: "Acme", Domain: "example.org", Lang: "es", PrivacyVersion: "2026-10-20"}
	old := pgtype.Date{Time: day.Time.AddDate(0, 0, -40), Valid: true}
	work := func(q *store.Queries) error {
		if err := q.CountUsage(ctx, store.CountUsageParams{Day: day, Event: "page_view", Dim: "/", MaxDims: 200}); err != nil {
			return err
		}
		if err := q.CountBlockedTarget(ctx, store.CountBlockedTargetParams{Day: old, Domain: "example.org", MaxDomains: 1000}); err != nil {
			return err
		}
		n, err := q.PurgeBlockedTargets(ctx, 30)
		if err == nil && n != 1 {
			return fmt.Errorf("purged %d rows, want 1", n)
		}
		if err != nil {
			return err
		}
		if err := q.CountUsage(ctx, store.CountUsageParams{Day: day, Event: "demo_requested", Dim: "home", MaxDims: 200}); err != nil {
			return fmt.Errorf("the demo_requested event (00017): %w", err)
		}
		if _, err := q.InsertDemoRequest(ctx, demo); err != nil {
			return fmt.Errorf("store a demo request: %w", err)
		}
		if _, err := q.PurgeDemoRequests(ctx, 12); err != nil {
			return fmt.Errorf("purge demo requests: %w", err)
		}
		return nil
	}
	app := store.NewStore(testdb.AsRole(t, pool, "qr_app")).Queries
	if err := work(app); err != nil {
		t.Errorf("qr_app counting, storing and purging: %v", err)
	}
	denied := func(err error) bool {
		var pg *pgconn.PgError
		return errors.As(err, &pg) && pg.Code == "42501"
	}
	// qr_app stores requests but cannot read them back (names, emails, messages).
	if _, err := app.ListDemoRequests(ctx, 10); !denied(err) {
		t.Errorf("qr_app lists demo requests: %v, want permission denied", err)
	}
	for _, role := range []string{"qr_worker", "qr_mailer"} {
		q := store.NewStore(testdb.AsRole(t, pool, role)).Queries
		for name, f := range map[string]func() error{
			"count": func() error {
				return q.CountUsage(ctx, store.CountUsageParams{Day: day, Event: "page_view", Dim: "/", MaxDims: 200})
			},
			"purge":       func() error { _, err := q.PurgeBlockedTargets(ctx, 30); return err },
			"read":        func() error { _, err := q.UsageSince(ctx, day); return err },
			"demo":        func() error { _, err := q.InsertDemoRequest(ctx, demo); return err },
			"purge demos": func() error { _, err := q.PurgeDemoRequests(ctx, 12); return err },
		} {
			var pg *pgconn.PgError
			if err := f(); !errors.As(err, &pg) || pg.Code != "42501" {
				t.Errorf("%s %s: %v, want permission denied", role, name, err)
			}
		}
	}
}
