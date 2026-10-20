package migrations

import (
	"context"
	"database/sql"
	"slices"
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

// The real files from production's state: at 00003 Up applies 00015 and
// 00017; at 00015 (production since M1) only 00017. Nothing else is missing.
func TestShowcaseFromProduction(t *testing.T) {
	for _, tc := range []struct {
		at   int64
		want []int64
	}{{3, []int64{15, 17}}, {15, []int64{17}}} {
		testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
			p, err := goose.NewProvider(goose.DialectPostgres, db, files)
			if err != nil {
				return err
			}
			if _, err := p.UpTo(ctx, tc.at); err != nil {
				return err
			}
			res, err := Up(ctx, db)
			if err != nil {
				t.Fatalf("from %d: %v", tc.at, err)
			}
			var got []int64
			for _, r := range res {
				got = append(got, r.Source.Version)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("from %d: applied %v, want %v", tc.at, got, tc.want)
			}
			for _, table := range []string{"usage_daily", "blocked_targets_daily", "demo_requests"} {
				var rls bool
				if err := db.QueryRowContext(ctx, `SELECT relrowsecurity FROM pg_class WHERE oid = to_regclass($1)`, table).Scan(&rls); err != nil {
					t.Fatalf("%s: %v", table, err)
				}
				if rls {
					t.Errorf("%s: RLS on, want off (no tenant)", table)
				}
			}
			// The M1 event list now includes demo_requested, and still nothing else.
			for ev, ok := range map[string]bool{"demo_requested": true, "lang_switch": true, "form_field": false} {
				_, err := db.ExecContext(ctx, `INSERT INTO usage_daily (day, event, dim, count) VALUES (current_date, $1, '', 1)`, ev)
				if (err == nil) != ok {
					t.Errorf("event %q accepted=%v, want %v", ev, err == nil, ok)
				}
			}
			return nil
		})
	}
}
