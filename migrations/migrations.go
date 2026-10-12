// Package migrations embeds the SQL schema migrations and applies them with goose.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed *.sql
var files embed.FS

// FS returns the embedded migration files.
func FS() fs.FS { return files }

// Up applies all pending migrations and returns the ones it ran. It holds a
// PostgreSQL advisory lock, so concurrent runs are safe, and it is a no-op
// when the schema is already current.
func Up(ctx context.Context, db *sql.DB) ([]*goose.MigrationResult, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, files, goose.WithSessionLocker(locker))
	if err != nil {
		return nil, err
	}
	return p.Up(ctx)
}
