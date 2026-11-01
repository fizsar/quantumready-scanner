package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// A login that is not itself a superuser but is a member of one can SET
// ROLE to it at any time: the check must refuse it too.
func TestCheckUnprivilegedMemberOfSuperuser(t *testing.T) {
	pool := newTestStore(t).Pool()
	ctx := context.Background()
	var owner string
	if err := pool.QueryRow(ctx, "SELECT current_user").Scan(&owner); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	member := "qr_test_member_" + hex.EncodeToString(b)
	if _, err := pool.Exec(ctx, "CREATE ROLE "+member+" LOGIN NOSUPERUSER NOBYPASSRLS NOINHERIT PASSWORD '"+testRolePassword+"'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DROP ROLE "+member) })
	for _, g := range []string{pgx.Identifier{owner}.Sanitize(), RoleApp} {
		if _, err := pool.Exec(ctx, "GRANT "+g+" TO "+member); err != nil {
			t.Fatal(err)
		}
	}
	c, err := pgx.ConnectConfig(ctx, connAs(t, member, testRolePassword))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	if _, err := c.Exec(ctx, "SET ROLE qr_app"); err != nil {
		t.Fatal(err)
	}
	err = CheckUnprivileged(ctx, c)
	// What the member can do after passing the check:
	var super string
	_, _ = c.Exec(ctx, "SET ROLE "+pgx.Identifier{owner}.Sanitize())
	_ = c.QueryRow(ctx, "SHOW is_superuser").Scan(&super)
	if !errors.Is(err, ErrPrivilegedRole) {
		t.Errorf("member of the superuser %s passed the check (err %v); after SET ROLE, is_superuser=%s", owner, err, super)
	}
}
