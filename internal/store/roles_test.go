package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fizsar/quantumready-saas/internal/testdb"
)

// asRole runs sql as role in a fresh transaction (rolled back) and returns
// the error, if any.
func asRole(t *testing.T, s *Store, role, sql string, args ...any) error {
	t.Helper()
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+role); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, sql, args...)
	return err
}

func permissionDenied(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "42501"
}

// The worker sees the scan queue of every organization and stores
// findings, and nothing else: no accounts, sessions, tokens, organizations,
// domains, audit log or emails. The mailer only touches the email outbox.
func TestBackgroundRolesLeastPrivilege(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	var bypass bool
	if err := w.s.pool.QueryRow(ctx, "SELECT rolbypassrls FROM pg_roles WHERE rolname = 'qr_worker'").Scan(&bypass); err != nil || bypass {
		t.Errorf("qr_worker BYPASSRLS = %v (%v), want false", bypass, err)
	}

	forbidden := []string{"users", "sessions", "auth_tokens", "signup_codes", "organizations", "memberships",
		"invitations", "domains", "audit_log", "email_outbox", "deletion_tombstones"}
	for _, table := range forbidden {
		if err := asRole(t, w.s, RoleWorker, "SELECT 1 FROM "+table+" LIMIT 1"); !permissionDenied(err) {
			t.Errorf("qr_worker reads %s: %v, want permission denied", table, err)
		}
	}
	for _, table := range append(forbidden, "scans", "findings") {
		if table == "email_outbox" {
			continue
		}
		if err := asRole(t, w.s, RoleMailer, "SELECT 1 FROM "+table+" LIMIT 1"); !permissionDenied(err) {
			t.Errorf("qr_mailer reads %s: %v, want permission denied", table, err)
		}
	}

	// What the worker needs works, for every organization (no tenant set).
	ws := NewStore(testdb.AsRole(t, w.s.pool, RoleWorker))
	var n int
	if err := ws.pool.QueryRow(ctx, "SELECT count(*) FROM scans").Scan(&n); err != nil || n != 3 {
		t.Errorf("qr_worker sees %d scans (%v), want all 3 (public, A, B)", n, err)
	}
	c, err := ws.Claim(ctx, 60e9)
	if err != nil {
		t.Fatalf("qr_worker cannot claim: %v", err)
	}
	if err := ws.Complete(ctx, completeParams(c), []InsertFindingsParams{{ScanID: c.ID, Position: 9, Code: "FS_ALL", Axis: "current", Severity: "pass", Category: "cipher_suites", Params: []byte(`{}`)}}); err != nil {
		t.Errorf("qr_worker cannot complete a scan: %v", err)
	}
	// ...but cannot create, delete or read findings back.
	for _, sql := range []string{
		"INSERT INTO scans (host) VALUES ('x.example')",
		"DELETE FROM scans",
		"SELECT 1 FROM findings",
		"DELETE FROM findings",
	} {
		if err := asRole(t, w.s, RoleWorker, sql); !permissionDenied(err) {
			t.Errorf("qr_worker: %s: %v, want permission denied", sql, err)
		}
	}
}

func TestUseRoleRejectsUnknownRoles(t *testing.T) {
	for _, role := range []string{"postgres", "qr_app; DROP TABLE scans", ""} {
		if err := UseRole(nil, role); err == nil {
			t.Errorf("UseRole(%q) accepted", role)
		}
	}
}
