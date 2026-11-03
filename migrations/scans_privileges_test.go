package migrations

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fizsar/quantumready-saas/internal/store"
	"github.com/fizsar/quantumready-saas/internal/testdb"
)

// scansPrivileges lists every privilege on scans held by anyone but its
// owner, table-wide ("scans role PRIVILEGE") and per column
// ("scans.column role PRIVILEGE").
func scansPrivileges(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT c.relname || ' ' || CASE WHEN a.grantee = 0 THEN 'PUBLIC' ELSE a.grantee::regrole::text END || ' ' || a.privilege_type
		FROM pg_class c, aclexplode(coalesce(c.relacl, acldefault('r', c.relowner))) a
		WHERE c.oid = to_regclass('scans') AND a.grantee <> c.relowner
		UNION ALL
		SELECT c.relname || '.' || at.attname || ' ' || CASE WHEN a.grantee = 0 THEN 'PUBLIC' ELSE a.grantee::regrole::text END || ' ' || a.privilege_type
		FROM pg_class c JOIN pg_attribute at ON at.attrelid = c.oid, aclexplode(at.attacl) a
		WHERE c.oid = to_regclass('scans') AND at.attacl IS NOT NULL AND a.grantee <> c.relowner
		ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// wantScansGrants (00021, security review of 01/11/2026): the API enqueues
// (target, owner, intensity, who asked) and reads; status, created_at, id and
// expires_at come from the database. The worker reads and updates only the
// columns of its work: never the target, the owner, the intensity or the expiry.
var wantScansGrants = []string{
	"scans qr_app SELECT",
	"scans qr_worker SELECT",
	"scans.attempts qr_worker UPDATE",
	"scans.error qr_worker UPDATE",
	"scans.finished_at qr_worker UPDATE",
	"scans.grade qr_worker UPDATE",
	"scans.host qr_app INSERT",
	"scans.intensity qr_app INSERT",
	"scans.lease_token qr_worker UPDATE",
	"scans.lease_until qr_worker UPDATE",
	"scans.not_before qr_worker UPDATE",
	"scans.organization_id qr_app INSERT",
	"scans.port qr_app INSERT",
	"scans.pqc qr_worker UPDATE",
	"scans.requested_by qr_app INSERT",
	"scans.result qr_worker UPDATE",
	"scans.retries qr_worker UPDATE",
	"scans.scanner_schema qr_worker UPDATE",
	"scans.score qr_worker UPDATE",
	"scans.scoring qr_worker UPDATE",
	"scans.scoring_version qr_worker UPDATE",
	"scans.started_at qr_worker UPDATE",
	"scans.status qr_worker UPDATE",
}

func denied(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "42501"
}

// What qr_app and qr_worker can and cannot write in scans.
func TestScansColumnPrivileges(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error { _, err := Up(ctx, db); return err })
	ctx := context.Background()
	app := testdb.AsRole(t, pool, "qr_app")
	worker := testdb.AsRole(t, pool, "qr_worker")

	// qr_app enqueues a public scan as the API does; the database sets the rest.
	var sc store.Scan
	err := pgx.BeginFunc(ctx, app, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.org_id', '', true), set_config('app.user_id', '', true)"); err != nil {
			return err
		}
		var err error
		sc, err = store.New(tx).EnqueueScan(ctx, store.EnqueueScanParams{Host: "a.example", Port: 443, Intensity: store.IntensityBasic})
		return err
	})
	if err != nil {
		t.Fatalf("qr_app enqueues: %v", err)
	}
	if sc.Status != "queued" || sc.ExpiresAt == nil || !sc.ExpiresAt.Equal(sc.CreatedAt.Add(store.PublicScanTTL)) {
		t.Errorf("enqueued scan: status %s, created %v, expires %v; want queued and created + %v", sc.Status, sc.CreatedAt, sc.ExpiresAt, store.PublicScanTTL)
	}

	// An organization's scan gets the organization's retention.
	org := uuid.New()
	if _, err := pool.Exec(ctx, "INSERT INTO organizations (id, name) VALUES ($1, 'Org')", org); err != nil {
		t.Fatal(err)
	}
	var orgExp, orgCreated time.Time
	if err := pool.QueryRow(ctx, "INSERT INTO scans (host, port, organization_id, intensity) VALUES ('o.example', 443, $1, 'full') RETURNING created_at, expires_at", org).Scan(&orgCreated, &orgExp); err != nil {
		t.Fatal(err)
	}
	if !orgExp.Equal(orgCreated.Add(store.OrgScanTTL)) {
		t.Errorf("organization scan expires %v, want created + %v", orgExp, store.OrgScanTTL)
	}

	// Everything else qr_app might try, as a compromised API would.
	for name, q := range map[string]string{
		"status done": "INSERT INTO scans (host, port, status, result, scoring_version, grade) VALUES ('b.example', 443, 'done', '{}', 'x', 'A')",
		"result":      "INSERT INTO scans (host, port, result) VALUES ('b.example', 443, '{}')",
		"expiry":      "INSERT INTO scans (host, port, expires_at) VALUES ('b.example', 443, now() + interval '10 years')",
		"created_at":  "INSERT INTO scans (host, port, created_at) VALUES ('b.example', 443, now() + interval '10 years')",
		"id":          "INSERT INTO scans (id, host, port) VALUES (gen_random_uuid(), 'b.example', 443)",
		"update":      "UPDATE scans SET status = 'failed'",
		"delete":      "DELETE FROM scans",
	} {
		err := pgx.BeginFunc(ctx, app, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "SELECT set_config('app.org_id', '', true)")
			if err == nil {
				_, err = tx.Exec(ctx, q)
			}
			return err
		})
		if !denied(err) {
			t.Errorf("qr_app %s: %v, want permission denied", name, err)
		}
	}

	// The worker cannot move a queued scan to another target, owner,
	// intensity or expiry, nor touch who asked or how many attempts it gets.
	for _, col := range []string{"host = 'evil.example'", "port = 22", "organization_id = NULL", "intensity = 'full'",
		"expires_at = now() + interval '10 years'", "created_at = now()", "requested_by = NULL", "max_attempts = 100", "id = gen_random_uuid()"} {
		if _, err := worker.Exec(ctx, "UPDATE scans SET "+col+" WHERE id = $1", sc.ID); !denied(err) {
			t.Errorf("qr_worker SET %s: %v, want permission denied", col, err)
		}
	}
	// Its own work still goes through (claim and fail; complete is covered by
	// the worker tests, which run as qr_worker).
	wq, code := store.New(worker), "scan_error"
	lease := uuid.New()
	claimed, err := wq.ClaimScan(ctx, store.ClaimScanParams{LeaseToken: &lease, LeaseSeconds: 90})
	if err != nil {
		t.Fatalf("qr_worker claims: %v", err)
	}
	if n, err := wq.FailScan(ctx, store.FailScanParams{ID: claimed.ID, LeaseToken: &lease, Error: &code}); err != nil || n != 1 {
		t.Errorf("qr_worker fails its scan: %d %v", n, err)
	}
}
