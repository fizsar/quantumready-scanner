package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// Schema creates a fresh schema, applies migrate to it and returns a pool
// whose connections use it (search_path). Test packages can then run in
// parallel against the same database without seeing each other's rows. The
// schema is dropped at cleanup. Skips or fails like DSN without a database.
func Schema(t testing.TB, migrate func(context.Context, *sql.DB) error) *pgxpool.Pool {
	t.Helper()
	dsn := DSN(t)
	ctx := context.Background()

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "test_" + hex.EncodeToString(b)
	ident := pgx.Identifier{name}.Sanitize()

	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer admin.Close(ctx)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+ident); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), dsn)
		if err != nil {
			t.Errorf("drop schema: %v", err)
			return
		}
		defer c.Close(context.Background())
		if _, err := c.Exec(context.Background(), "DROP SCHEMA "+ident+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = name

	db := stdlib.OpenDB(*cfg.ConnConfig.Copy())
	defer db.Close()
	if err := migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// AsRole returns a second pool on the same schema whose connections act as
// role (SET ROLE), e.g. "qr_worker": tests then run code with exactly the
// privileges it has in production.
func AsRole(t testing.TB, pool *pgxpool.Pool, role string) *pgxpool.Pool {
	t.Helper()
	cfg := pool.Config()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE "+pgx.Identifier{role}.Sanitize())
		return err
	}
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}
