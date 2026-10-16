package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fizsar/quantumready-saas/internal/testdb"
	"github.com/fizsar/quantumready-saas/migrations"
)

func TestSuffixes(t *testing.T) {
	if got := Suffixes("a.b.example.com"); !slices.Equal(got, []string{"a.b.example.com", "b.example.com", "example.com"}) {
		t.Errorf("Suffixes = %v", got)
	}
	if got := Suffixes("com"); got != nil {
		t.Errorf("Suffixes(com) = %v", got)
	}
}

// public_scan_blocked runs with the owner's rights for anonymous requests:
// a yes or no, nothing more, hardened like invitation_by_token.
func TestPublicScanBlockedIsNarrow(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	ctx := context.Background()
	var schema, config, result string
	var definer bool
	_ = pool.QueryRow(ctx, "SELECT current_schema()").Scan(&schema)
	if err := pool.QueryRow(ctx, `SELECT array_to_string(proconfig, ';'), prosecdef, pg_get_function_result(oid)
		FROM pg_proc WHERE proname = 'public_scan_blocked' AND pronamespace = current_schema()::regnamespace`).Scan(&config, &definer, &result); err != nil {
		t.Fatal(err)
	}
	if config != "search_path=pg_catalog, "+schema+", pg_temp" || !definer || result != "boolean" {
		t.Errorf("function: config %q, definer %v, result %q", config, definer, result)
	}
	for role, want := range map[string]bool{"qr_app": true, "qr_worker": false, "qr_mailer": false} {
		var ok bool
		_ = pool.QueryRow(ctx, "SELECT has_function_privilege($1, 'public_scan_blocked(text[])', 'EXECUTE')", role).Scan(&ok)
		if ok != want {
			t.Errorf("EXECUTE for %s = %v", role, ok)
		}
	}
	var public bool
	_ = pool.QueryRow(ctx, `SELECT coalesce(bool_or(grantee = 0), false) FROM pg_proc p, aclexplode(p.proacl)
		WHERE p.proname = 'public_scan_blocked'`).Scan(&public)
	if public {
		t.Error("PUBLIC can execute public_scan_blocked")
	}
	// The API's role cannot read, change or delete the exclusion list (it
	// only adds to it, when a blocked domain is removed).
	for _, q := range []string{"SELECT * FROM scan_exclusions", "DELETE FROM scan_exclusions", "UPDATE scan_exclusions SET reason = 'x'"} {
		var pg *pgconn.PgError
		if _, err := testdb.AsRole(t, pool, RoleApp).Exec(ctx, q); !errors.As(err, &pg) || pg.Code != "42501" {
			t.Errorf("qr_app: %s: %v", q, err)
		}
	}

	org, _ := orgWithOwners(t, pool, 1)
	for _, q := range []string{
		"INSERT INTO scan_exclusions (name) VALUES ('excluded.example')",
		"INSERT INTO domains (organization_id, name, token, status, block_public) VALUES ('" + org.String() + "', 'optout.example', 't', 'verified', true)",
		"INSERT INTO domains (organization_id, name, token, status, block_public) VALUES ('" + org.String() + "', 'pending.example', 't', 'pending', true)",
		"INSERT INTO domains (organization_id, name, token, status, block_public) VALUES ('" + org.String() + "', 'verified.example', 't', 'verified', false)",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	app := NewStore(testdb.AsRole(t, pool, RoleApp))
	for host, want := range map[string]bool{
		"excluded.example": true, "www.excluded.example": true, "notexcluded.example": false,
		"optout.example": true, "a.b.optout.example": true, // the owner's block covers subdomains too
		"pending.example":  true,  // a block stays in force whatever the status (the API only sets it while verified)
		"verified.example": false, // verified but not opted out: like any domain
	} {
		if got, err := app.PublicScanBlocked(ctx, host); err != nil || got != want {
			t.Errorf("PublicScanBlocked(%s) = %v, %v; want %v", host, got, err, want)
		}
	}
	var blocked bool
	for _, names := range [][]string{{}, make([]string, 128)} {
		if err := app.pool.QueryRow(ctx, "SELECT public_scan_blocked($1)", names).Scan(&blocked); err != nil || blocked {
			t.Errorf("%d names: %v %v", len(names), blocked, err)
		}
	}
}
