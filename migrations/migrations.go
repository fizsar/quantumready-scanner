// Package migrations embeds the SQL schema migrations and applies them with goose.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed *.sql
var files embed.FS

// FS returns the embedded migration files.
func FS() fs.FS { return files }

// Up applies all pending migrations, in order, and returns the ones it ran.
// It holds a PostgreSQL advisory lock, so concurrent runs are safe, and it is
// a no-op when the schema is already current. A pending migration below the
// newest applied one is an error: migrations apply in order only. (Until the
// rollback window to the showcase branch closed on 16/11/2026, 00004–00016
// could be applied out of order: production had 00015 and 00017 from
// showcase before the phase 3 migrations.)
func Up(ctx context.Context, db *sql.DB) ([]*goose.MigrationResult, error) {
	return up(ctx, db, files)
}

func up(ctx context.Context, db *sql.DB, fsys fs.FS) ([]*goose.MigrationResult, error) {
	p, err := provider(db, fsys)
	if err != nil {
		return nil, err
	}
	missing, err := missingBelowNewest(ctx, p)
	if err != nil {
		return nil, err
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("migrations: %v are missing below the newest applied one; migrations apply in order only", missing)
	}
	return p.Up(ctx)
}

func provider(db *sql.DB, fsys fs.FS) (*goose.Provider, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithSessionLocker(locker))
}

// missingBelowNewest lists the pending migrations older than the newest
// applied one: the ones goose refuses without out-of-order.
func missingBelowNewest(ctx context.Context, p *goose.Provider) ([]int64, error) {
	status, err := p.Status(ctx)
	if err != nil {
		return nil, err
	}
	var newest int64
	for _, s := range status {
		if s.State == goose.StateApplied {
			newest = max(newest, s.Source.Version)
		}
	}
	var missing []int64
	for _, s := range status {
		if s.State == goose.StatePending && s.Source.Version < newest {
			missing = append(missing, s.Source.Version)
		}
	}
	return missing, nil
}
