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

// outOfOrder is the one gap goose may fill below the newest applied
// migration: main's migrations (00004–00014 of phase 3 and 00016, the M1
// grants), when they reach a production database that already has the ones
// deployed from the showcase branch before phase 3 (00015 M1, 00017 demo
// requests). Any other missing migration is still an error. Remove after 3.7.
var outOfOrder = struct{ from, to int64 }{4, 16}

// Up applies all pending migrations and returns the ones it ran. It holds a
// PostgreSQL advisory lock, so concurrent runs are safe, and it is a no-op
// when the schema is already current.
func Up(ctx context.Context, db *sql.DB) ([]*goose.MigrationResult, error) {
	return up(ctx, db, files)
}

func up(ctx context.Context, db *sql.DB, fsys fs.FS) ([]*goose.MigrationResult, error) {
	p, err := provider(db, fsys, false)
	if err != nil {
		return nil, err
	}
	missing, err := missingBelowNewest(ctx, p)
	if err != nil {
		return nil, err
	}
	if len(missing) == 0 {
		return p.Up(ctx)
	}
	for _, v := range missing {
		if v < outOfOrder.from || v > outOfOrder.to {
			return nil, fmt.Errorf("migrations: %v are missing below the newest applied one; only %d–%d may be applied out of order",
				missing, outOfOrder.from, outOfOrder.to)
		}
	}
	p, err = provider(db, fsys, true)
	if err != nil {
		return nil, err
	}
	return p.Up(ctx)
}

func provider(db *sql.DB, fsys fs.FS, allowOutOfOrder bool) (*goose.Provider, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithSessionLocker(locker), goose.WithAllowOutofOrder(allowOutOfOrder))
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
