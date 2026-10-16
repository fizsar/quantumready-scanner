package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fizsar/quantumready-saas/internal/testdb"
	"github.com/fizsar/quantumready-saas/migrations"
)

func orgWithOwners(t *testing.T, pool *pgxpool.Pool, n int) (org uuid.UUID, owners []uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	org = uuid.New()
	if _, err := pool.Exec(ctx, "INSERT INTO organizations (id, name) VALUES ($1, 'Acme')", org); err != nil {
		t.Fatal(err)
	}
	for range n {
		u := uuid.New()
		if _, err := pool.Exec(ctx, "INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'x')", u, u.String()[:8]+"@example.com"); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO memberships (organization_id, user_id, role) VALUES ($1, $2, 'owner')", org, u); err != nil {
			t.Fatal(err)
		}
		owners = append(owners, u)
	}
	return org, owners
}

func make32(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

// invitation_by_token runs with the owner's rights, so it must stay narrow.
func TestInvitationLookupIsNarrow(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	ctx := context.Background()
	var schema string
	_ = pool.QueryRow(ctx, "SELECT current_schema()").Scan(&schema)

	// One function by that name, taking only a bytea; SECURITY DEFINER with
	// an explicit search path ending in pg_temp.
	var n int
	var args, config string
	var definer bool
	err := pool.QueryRow(ctx, `SELECT count(*) OVER (), pg_get_function_identity_arguments(p.oid), array_to_string(p.proconfig, ';'), p.prosecdef
		FROM pg_proc p WHERE p.proname = 'invitation_by_token' AND p.pronamespace = current_schema()::regnamespace`).Scan(&n, &args, &config, &definer)
	if err != nil {
		t.Fatal(err)
	}
	want := "search_path=pg_catalog, " + schema + ", pg_temp"
	if n != 1 || args != "hash bytea" || !definer || config != want {
		t.Errorf("function: %d overloads, args %q, definer %v, config %q (want %q)", n, args, definer, config, want)
	}
	// Only the columns the preview and the acceptance use.
	var cols string
	_ = pool.QueryRow(ctx, `SELECT string_agg(a, ',' ORDER BY o) FROM pg_proc p,
		unnest(p.proargnames, p.proargmodes) WITH ORDINALITY AS x(a, m, o)
		WHERE p.proname = 'invitation_by_token' AND p.pronamespace = current_schema()::regnamespace AND m = 't'`).Scan(&cols)
	if cols != "id,organization_id,organization_name,email,role,inviter_email,expires_at" {
		t.Errorf("returned columns: %s", cols)
	}
	// EXECUTE: qr_app only (not PUBLIC, not the background roles).
	for role, want := range map[string]bool{"qr_app": true, "qr_worker": false, "qr_mailer": false, "public": false} {
		var ok bool
		q := "SELECT has_function_privilege($1, 'invitation_by_token(bytea)', 'EXECUTE')"
		if role == "public" {
			q = "SELECT coalesce(bool_or(grantee = 0), false) FROM pg_proc p, aclexplode(p.proacl) WHERE p.proname = 'invitation_by_token' AND privilege_type = 'EXECUTE' AND $1 <> ''"
		}
		if err := pool.QueryRow(ctx, q, role).Scan(&ok); err != nil || ok != want {
			t.Errorf("EXECUTE for %s = %v (%v), want %v", role, ok, err, want)
		}
	}

	// Lookup only by a valid 32-byte hash of a pending, unexpired invitation.
	org, owners := orgWithOwners(t, pool, 1)
	tok := make32("token")
	if _, err := pool.Exec(ctx, `INSERT INTO invitations (organization_id, email, role, token_hash, invited_by, expires_at)
		VALUES ($1, 'x@example.com', 'viewer', $2, $3, now() + interval '1 day')`, org, tok, owners[0]); err != nil {
		t.Fatal(err)
	}
	app := testdb.AsRole(t, pool, RoleApp)
	count := func(arg any) int {
		var c int
		if err := app.QueryRow(ctx, "SELECT count(*) FROM invitation_by_token($1)", arg).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	if count(tok) != 1 {
		t.Error("valid token not found")
	}
	for name, arg := range map[string][]byte{"empty": {}, "prefix": tok[:16], "wrong": make32("other"), "longer": append(append([]byte{}, tok...), 0)} {
		if count(arg) != 0 {
			t.Errorf("%s hash found an invitation", name)
		}
	}
	for _, q := range []string{"UPDATE invitations SET accepted_at = now()", "UPDATE invitations SET accepted_at = NULL, revoked_at = now()",
		"UPDATE invitations SET revoked_at = NULL, expires_at = now() - interval '1 second'"} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
		if count(tok) != 0 {
			t.Errorf("after %q the invitation is still returned", q)
		}
	}
	// The worker cannot call it.
	var pgErr *pgconn.PgError
	if _, err := testdb.AsRole(t, pool, RoleWorker).Exec(ctx, "SELECT * FROM invitation_by_token($1)", tok); !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Errorf("qr_worker call: %v", err)
	}
}

func TestLastOwner(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	ctx := context.Background()
	st := NewStore(pool)
	demote := func(string, string) (string, bool, error) { return "viewer", false, nil }
	remove := func(string, string) (string, bool, error) { return "", true, nil }

	org, owners := orgWithOwners(t, pool, 1)
	a := owners[0]
	tn := Tenant{OrgID: &org, UserID: &a}
	for name, d := range map[string]MemberDecision{"demote": demote, "leave": remove} {
		if err := st.ChangeMember(ctx, tn, a, d); !errors.Is(err, ErrLastOwner) {
			t.Errorf("%s the only owner: %v", name, err)
		}
	}
	// Not a member: ErrNotFound for the target, ErrNotMember for the actor.
	stranger := uuid.New()
	if err := st.ChangeMember(ctx, tn, stranger, remove); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown target: %v", err)
	}
	if err := st.ChangeMember(ctx, Tenant{OrgID: &org, UserID: &stranger}, a, remove); !errors.Is(err, ErrNotMember) {
		t.Errorf("unknown actor: %v", err)
	}

	// Two owners demoting each other at once: one wins, the other fails.
	org2, two := orgWithOwners(t, pool, 2)
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx1.Rollback(ctx) }()
	if _, err := tx1.Exec(ctx, "UPDATE memberships SET role = 'viewer' WHERE organization_id = $1 AND user_id = $2", org2, two[0]); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := pool.Exec(ctx, "UPDATE memberships SET role = 'viewer' WHERE organization_id = $1 AND user_id = $2", org2, two[1])
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("second demotion did not wait for the first: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var pg *pgconn.PgError
	if err := <-done; !errors.As(err, &pg) || pg.ConstraintName != "memberships_last_owner" {
		t.Errorf("second demotion: %v", err)
	}
	var owners2 int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM memberships WHERE organization_id = $1 AND role = 'owner'", org2).Scan(&owners2)
	if owners2 != 1 {
		t.Errorf("%d owners left", owners2)
	}

	// Members plus pending invitations stay within MaxMembers.
	invite := func(email string) error {
		_, err := st.Invite(ctx, tn, NewInvitation{Email: email, Role: "viewer", TokenHash: make32(email), TTL: time.Hour,
			Mail: Email{To: email, Template: "invitation", Lang: "es", Subject: "s", Body: "b"}, MaxMembers: 2})
		return err
	}
	if err := invite("x@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := invite("y@example.com"); !errors.Is(err, ErrLimit) {
		t.Errorf("third member (one + one pending): %v", err)
	}

	// Deleting the organization still removes its members.
	if _, err := pool.Exec(ctx, "DELETE FROM organizations WHERE id = $1", org2); err != nil {
		t.Errorf("delete organization: %v", err)
	}
}
