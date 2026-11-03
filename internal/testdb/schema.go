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

// rolesLock is a PostgreSQL advisory lock shared by every test package on
// the database. Roles belong to the whole cluster, not to a test's schema, and
// PostgreSQL refuses two concurrent changes of the same role ("tuple
// concurrently updated"): migration 00005 runs ALTER ROLE qr_worker each time a
// schema is migrated, and some tests change the service roles themselves. So
// migrations hold it shared (they still run in parallel with each other), and
// LockRoles exclusive.
const rolesLock int64 = 0x51525f524f4c4553 // "QR_ROLES"

// LockRoles holds rolesLock exclusively until the test ends: for tests that
// change the cluster's roles (ALTER ROLE, CREATE ROLE…). Call it after the
// test's schema is migrated, since Schema waits for the shared lock.
func LockRoles(t testing.TB) {
	t.Helper()
	ctx := context.Background()
	c, err := pgx.Connect(ctx, DSN(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := c.Exec(ctx, "SELECT pg_advisory_lock($1)", rolesLock); err != nil {
		_ = c.Close(ctx)
		t.Fatalf("lock roles: %v", err)
	}
	// Closing the session releases the lock.
	t.Cleanup(func() { _ = c.Close(context.Background()) })
}

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
	// Shared: migrations run in parallel with each other, never while a test
	// holds LockRoles.
	if _, err := admin.Exec(ctx, "SELECT pg_advisory_lock_shared($1)", rolesLock); err != nil {
		t.Fatalf("lock roles (shared): %v", err)
	}
	err = migrate(ctx, db)
	if _, uerr := admin.Exec(ctx, "SELECT pg_advisory_unlock_shared($1)", rolesLock); uerr != nil {
		t.Fatalf("unlock roles: %v", uerr)
	}
	if err != nil {
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
