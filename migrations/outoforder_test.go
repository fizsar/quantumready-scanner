package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
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

// The path production will follow: 00015 (M1) and 00017 (demo requests)
// arrive from showcase on a database at 00003, then main's migrations (here 4
// and 14 of phase 3, 16 and 18) arrive, some of them below 17. Only the 4–16
// gap is filled out of order; any other missing migration still fails.
func TestOutOfOrderOnlyForPhase3(t *testing.T) {
	testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		showcase := fstest.MapFS{"00001_a.sql": sqlFile("t1"), "00002_b.sql": sqlFile("t2"),
			"00015_m1.sql": sqlFile("t15"), "00017_demo.sql": sqlFile("t17")}
		if _, err := up(ctx, db, showcase); err != nil {
			t.Fatalf("showcase: %v", err)
		}
		main := fstest.MapFS{"00004_p3.sql": sqlFile("t4"), "00014_p3.sql": sqlFile("t14"),
			"00016_m1_grants.sql": sqlFile("t16"), "00018_demo_grants.sql": sqlFile("t18")}
		for k, v := range showcase {
			main[k] = v
		}
		res, err := up(ctx, db, main)
		if err != nil {
			t.Fatalf("main after showcase: %v", err)
		}
		var got []int64
		for _, r := range res {
			got = append(got, r.Source.Version)
		}
		if !slices.Equal(got, []int64{4, 14, 16, 18}) {
			t.Errorf("main after showcase applied %v, want [4 14 16 18]", got)
		}
		// A gap outside 4–16 (here 3) is refused, and nothing is applied.
		main["00003_x.sql"] = sqlFile("t3")
		if _, err := up(ctx, db, main); err == nil || !strings.Contains(err.Error(), "[3]") {
			t.Errorf("missing 00003: %v, want a refusal naming it", err)
		}
		if tableExists(ctx, db, "t3") {
			t.Error("00003 was applied out of order")
		}
		return nil
	})
}

// showcaseFiles is what the showcase branch ships: 00001–00003, 00015 and 00017.
func showcaseFiles(t *testing.T) fstest.MapFS {
	t.Helper()
	out := fstest.MapFS{}
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if v, _ := strconv.Atoi(n[:5]); v <= 3 || v == 15 || v == 17 {
			data, err := fs.ReadFile(files, n)
			if err != nil {
				t.Fatal(err)
			}
			out[n] = &fstest.MapFile{Data: data}
		}
	}
	if len(out) != 5 {
		t.Fatalf("showcase files: %d, want 5", len(out))
	}
	return out
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

// The real path of production: a database at 00003 gets 00015 (M1) and
// 00017 (demo requests) from showcase, then main's migrations: 00004–00014
// and 00016 out of order, and 00018. The showcase tables must end up with
// exactly the privileges of a database migrated in order; the API (qr_app)
// must be able to count, store demo requests and purge both, without reading
// a request back; the other roles must not touch them.
func TestRealPathProductionToMain(t *testing.T) {
	var applied, again []int64
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		p, err := goose.NewProvider(goose.DialectPostgres, db, files)
		if err != nil {
			return err
		}
		if _, err := p.UpTo(ctx, 3); err != nil {
			return err
		}
		res, err := up(ctx, db, showcaseFiles(t))
		if err != nil || !slices.Equal(versions(res), []int64{15, 17}) {
			t.Fatalf("showcase on production: %v %v, want 00015 and 00017", versions(res), err)
		}
		if res, err = Up(ctx, db); err != nil {
			t.Fatalf("main after M1: %v", err)
		}
		applied = versions(res)
		if res, err = Up(ctx, db); err != nil {
			t.Fatalf("main again: %v", err)
		}
		again = versions(res)
		return nil
	})
	if want := []int64{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 16, 18}; !slices.Equal(applied, want) {
		t.Errorf("main after M1 applied %v, want %v", applied, want)
	}
	if len(again) != 0 {
		t.Errorf("second Up applied %v, want nothing", again)
	}

	grants, rls := m1Privileges(t, pool)
	if !slices.Equal(grants, wantM1Grants) {
		t.Errorf("M1 privileges after the real path:\n %v\nwant\n %v", grants, wantM1Grants)
	}
	if rls != wantM1RLS {
		t.Errorf("M1 RLS after the real path: %s, want %s", rls, wantM1RLS)
	}
	// The same as a database migrated in order.
	fresh := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := Up(ctx, db)
		return err
	})
	if g, r := m1Privileges(t, fresh); !slices.Equal(g, grants) || r != rls {
		t.Errorf("in-order database differs: %v %s", g, r)
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
