package migrations

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/fizsar/quantumready-saas/internal/testdb"
)

// Scans stored before phase 3 (production has a dozen) become public, get
// their intensity from the stored result, and public basic ones expire.
func TestAccountsMigrationKeepsExistingScans(t *testing.T) {
	var p *goose.Provider
	pool := testdb.Schema(t, func(ctx context.Context, d *sql.DB) error {
		var err error
		if p, err = goose.NewProvider(goose.DialectPostgres, d, files); err != nil {
			return err
		}
		if _, err := p.UpTo(ctx, 3); err != nil {
			return err
		}
		for _, q := range []string{
			// Phase 2, before milestone 2.7.3: no intensity in the result.
			`INSERT INTO scans (host, status, result, scoring_version, grade) VALUES ('old.example', 'done', '{"host":"old.example"}', '2026.1', 'A')`,
			// Milestone 2.7.5: full.
			`INSERT INTO scans (host, status, result, scoring_version, grade) VALUES ('full.example', 'done', '{"intensity":"full"}', '2026.2', 'A')`,
			`INSERT INTO scans (host) VALUES ('queued.example')`,
		} {
			if _, err := d.ExecContext(ctx, q); err != nil {
				return err
			}
		}
		_, err = p.Up(ctx)
		return err
	})
	rows, err := pool.Query(context.Background(), `SELECT host, intensity, organization_id IS NULL, expires_at FROM scans ORDER BY host`)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		host, intensity string
		public          bool
		expires         *time.Time
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.host, &r.intensity, &r.public, &r.expires); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	want := map[string]struct {
		intensity string
		expires   bool
	}{"full.example": {"full", false}, "old.example": {"basic", true}, "queued.example": {"basic", true}}
	if len(got) != 3 {
		t.Fatalf("rows = %v", got)
	}
	for _, r := range got {
		w := want[r.host]
		if r.intensity != w.intensity || !r.public || (r.expires != nil) != w.expires {
			t.Errorf("%s: intensity %s, public %v, expires %v; want %s, public, expires %v", r.host, r.intensity, r.public, r.expires, w.intensity, w.expires)
		}
		if r.expires != nil && time.Until(*r.expires) < 29*24*time.Hour {
			t.Errorf("%s expires %v, want in 30 days", r.host, r.expires)
		}
	}

	// The migration can be rolled back (keeping the scans) and applied again.
	ctx := context.Background()
	db := stdlib.OpenDBFromPool(pool) // same schema (search_path); the setup *sql.DB is closed
	t.Cleanup(func() { db.Close() })
	p, err = goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DownTo(ctx, 3); err != nil {
		t.Fatalf("down to 3: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM scans").Scan(&n); err != nil || n != 3 {
		t.Fatalf("after down: %d scans, %v", n, err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("up again: %v", err)
	}
}
