package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/fizsar/quantumready-saas/internal/testdb"
)

// The test database's login is the owner, a superuser (as production's until
// Apertura). These tests give qr_app and qr_worker LOGIN with a fixed test
// password; roles are global to the cluster, so the password is the same in
// every run and the roles go back to NOLOGIN afterwards.
const testRolePassword = "0123456789abcdef0123456789abcdef"

func connAs(t *testing.T, user, password string) *pgx.ConnConfig {
	t.Helper()
	cfg, err := pgx.ParseConfig(testdb.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.User, cfg.Password = user, password
	return cfg
}

func TestSyncLoginRoles(t *testing.T) {
	pool := newTestStore(t).Pool() // migrations applied: the roles exist
	db := stdlib.OpenDB(*pool.Config().ConnConfig.Copy())
	t.Cleanup(func() { _ = db.Close() }) // registered first: runs last
	ctx := context.Background()
	nologin := func() {
		for _, r := range LoginRoles {
			if _, err := db.ExecContext(context.Background(), "ALTER ROLE "+r.Role+" WITH NOLOGIN PASSWORD NULL"); err != nil {
				t.Errorf("reset %s: %v", r.Role, err)
			}
		}
	}
	nologin() // as the migrations leave them: the sync must give them LOGIN
	t.Cleanup(nologin)

	for _, bad := range []string{"short", strings.Repeat("ab", 15), strings.Repeat("AB", 16), "0123456789abcdef0123456789abcdeg", "x' SUPERUSER '"} {
		env := map[string]string{"QR_APP_DB_PASSWORD": bad}
		if _, err := SyncLoginRoles(ctx, db, func(k string) string { return env[k] }); err == nil {
			t.Errorf("password %q accepted", bad)
		}
	}
	// Unset: left alone.
	if roles, err := SyncLoginRoles(ctx, db, func(string) string { return "" }); err != nil || len(roles) != 0 {
		t.Errorf("no passwords: %v %v", roles, err)
	}
	env := map[string]string{"QR_APP_DB_PASSWORD": testRolePassword, "QR_WORKER_DB_PASSWORD": testRolePassword}
	roles, err := SyncLoginRoles(ctx, db, func(k string) string { return env[k] })
	if err != nil || strings.Join(roles, ",") != "qr_app,qr_worker" {
		t.Fatalf("sync: %v %v", roles, err)
	}
	for _, role := range []string{RoleApp, RoleWorker} {
		c, err := pgx.ConnectConfig(ctx, connAs(t, role, testRolePassword))
		if err != nil {
			t.Fatalf("log in as %s: %v", role, err)
		}
		if err := CheckUnprivileged(ctx, c); err != nil {
			t.Errorf("%s: %v", role, err)
		}
		_ = c.Close(ctx)
		if c, err := pgx.ConnectConfig(ctx, connAs(t, role, "wrong")); err == nil {
			_ = c.Close(ctx)
			t.Errorf("%s logged in with a wrong password", role)
		}
	}
	// qr_mailer is not a login user (not deployed): it stays NOLOGIN.
	if c, err := pgx.ConnectConfig(ctx, connAs(t, RoleMailer, testRolePassword)); err == nil {
		_ = c.Close(ctx)
		t.Error("qr_mailer can log in")
	}
}

// A superuser login fails the check even after SET ROLE (it could RESET
// ROLE), and so does a role with BYPASSRLS; the pool refuses such
// connections and the service does not start.
func TestCheckUnprivileged(t *testing.T) {
	pool := newTestStore(t).Pool()
	ctx := context.Background()

	owner, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	if err := CheckUnprivileged(ctx, owner); !errors.Is(err, ErrPrivilegedRole) {
		t.Errorf("superuser login: %v, want ErrPrivilegedRole", err)
	}
	if _, err := owner.Exec(ctx, "SET ROLE qr_app"); err != nil {
		t.Fatal(err)
	}
	if err := CheckUnprivileged(ctx, owner); !errors.Is(err, ErrPrivilegedRole) {
		t.Errorf("superuser login acting as qr_app: %v, want ErrPrivilegedRole", err)
	}
	if _, err := owner.Exec(ctx, "RESET ROLE"); err != nil {
		t.Fatal(err)
	}

	// A login role with BYPASSRLS (and nothing else).
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	bypass := "qr_test_bypass_" + hex.EncodeToString(b)
	lastBypassRole = bypass
	if _, err := owner.Exec(ctx, "CREATE ROLE "+bypass+" LOGIN BYPASSRLS PASSWORD '"+testRolePassword+"'"); err != nil {
		t.Fatal(err)
	}
	// On the pool, not on owner: the defer above closes owner before any
	// cleanup runs, and the role (global to the cluster) stayed behind.
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DROP ROLE "+bypass); err != nil {
			t.Errorf("drop %s: %v", bypass, err)
		}
	})
	c, err := pgx.ConnectConfig(ctx, connAs(t, bypass, testRolePassword))
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckUnprivileged(ctx, c); !errors.Is(err, ErrPrivilegedRole) {
		t.Errorf("BYPASSRLS login: %v, want ErrPrivilegedRole", err)
	}
	_ = c.Close(ctx)

	// The pool of a service logging in as the owner: no connection, and the
	// start check returns the error.
	cfg := pool.Config().Copy()
	if err := UseRole(cfg, RoleApp); err != nil {
		t.Fatal(err)
	}
	RequireUnprivileged(cfg)
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err := p.Exec(ctx, "SELECT 1"); !errors.Is(err, ErrPrivilegedRole) {
		t.Errorf("query on a superuser pool: %v, want ErrPrivilegedRole", err)
	}
	if reachable, err := CheckPoolAtStart(ctx, p, 5*time.Second); !reachable || !errors.Is(err, ErrPrivilegedRole) {
		t.Errorf("start check: reachable=%v err=%v, want ErrPrivilegedRole", reachable, err)
	}
}

// A database that does not answer does not stop the start: reachable=false
// within the wait, and every later connection is still checked.
func TestCheckPoolAtStartUnreachable(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	RequireUnprivileged(cfg)
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	start := time.Now()
	reachable, err := CheckPoolAtStart(context.Background(), p, 2*time.Second)
	if reachable || err != nil || time.Since(start) > 5*time.Second {
		t.Errorf("unreachable: reachable=%v err=%v after %v", reachable, err, time.Since(start))
	}
}

// lastBypassRole is the BYPASSRLS login role TestCheckUnprivileged created last.
var lastBypassRole string

// TestCheckUnprivileged creates a BYPASSRLS login role (global to the
// cluster): it must be gone once the test has run. Only its own role is
// looked for: another checkout testing on the same cluster may have older
// code that leaves its own behind.
func TestNoLeftoverBypassRoles(t *testing.T) {
	lastBypassRole = ""
	t.Run("check", TestCheckUnprivileged)
	if lastBypassRole == "" {
		t.Fatal("TestCheckUnprivileged created no role")
	}
	var left bool
	if err := newTestStore(t).Pool().QueryRow(context.Background(), "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)", lastBypassRole).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left {
		t.Errorf("BYPASSRLS login role %s left in the cluster", lastBypassRole)
	}
}
