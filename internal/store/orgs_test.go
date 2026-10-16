package store

import (
	"context"
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

	// Deleting the organization still removes its members.
	if _, err := pool.Exec(ctx, "DELETE FROM organizations WHERE id = $1", org2); err != nil {
		t.Errorf("delete organization: %v", err)
	}
}
