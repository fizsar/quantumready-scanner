package migrations

import (
	"context"
	"database/sql"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/fizsar/quantumready-saas/internal/testdb"
)

var fileName = regexp.MustCompile(`^\d{5}_[a-z0-9_]+\.sql$`)

func TestMigrationFiles(t *testing.T) {
	names, err := fs.Glob(FS(), "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no migrations embedded")
	}
	for _, name := range names {
		if !fileName.MatchString(name) {
			t.Errorf("%s: want NNNNN_snake_case.sql", name)
		}
		body, err := fs.ReadFile(FS(), name)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range []string{"-- +goose Up", "-- +goose Down"} {
			if !strings.Contains(string(body), marker) {
				t.Errorf("%s: missing %q", name, marker)
			}
		}
	}
}

// TestUpPostgres runs against a real PostgreSQL (see testdb.DSN).
func TestUpPostgres(t *testing.T) {
	db, err := sql.Open("pgx", testdb.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	if _, err := Up(ctx, db); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	again, err := Up(ctx, db)
	if err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("second Up applied %d migrations, want 0", len(again))
	}
}
