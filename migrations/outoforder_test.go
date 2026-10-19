package migrations

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/pressly/goose/v3"

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

// The path production will follow: 00015 (M1) arrives from showcase on a
// database at 00003, then phase 3 (here 4 and 14) arrives below it. Only that
// gap is filled out of order; any other missing migration still fails.
func TestOutOfOrderOnlyForPhase3(t *testing.T) {
	testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		showcase := fstest.MapFS{"00001_a.sql": sqlFile("t1"), "00002_b.sql": sqlFile("t2"), "00015_m1.sql": sqlFile("t15")}
		if _, err := up(ctx, db, showcase); err != nil {
			t.Fatalf("showcase: %v", err)
		}
		main := fstest.MapFS{"00004_p3.sql": sqlFile("t4"), "00014_p3.sql": sqlFile("t14")}
		for k, v := range showcase {
			main[k] = v
		}
		res, err := up(ctx, db, main)
		if err != nil {
			t.Fatalf("phase 3 after M1: %v", err)
		}
		if len(res) != 2 || !tableExists(ctx, db, "t4") || !tableExists(ctx, db, "t14") {
			t.Errorf("phase 3 after M1: applied %d, tables t4 %v t14 %v", len(res), tableExists(ctx, db, "t4"), tableExists(ctx, db, "t14"))
		}
		// A gap outside 4–14 (here 3) is refused, and nothing is applied.
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

// The real files: production is at 00003; Up applies 00015 and nothing else
// is missing.
func TestM1FromProduction(t *testing.T) {
	testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		p, err := goose.NewProvider(goose.DialectPostgres, db, files)
		if err != nil {
			return err
		}
		if _, err := p.UpTo(ctx, 3); err != nil {
			return err
		}
		res, err := Up(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 || res[0].Source.Version != 15 {
			t.Errorf("applied %v, want only 00015", res)
		}
		for _, table := range []string{"usage_daily", "blocked_targets_daily"} {
			var rls bool
			if err := db.QueryRowContext(ctx, `SELECT relrowsecurity FROM pg_class WHERE oid = to_regclass($1)`, table).Scan(&rls); err != nil {
				t.Fatalf("%s: %v", table, err)
			}
			if rls {
				t.Errorf("%s: RLS on, want off (aggregate counters, no tenant)", table)
			}
		}
		return nil
	})
}
