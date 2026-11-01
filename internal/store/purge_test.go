package store

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/testdb"
)

// Apertura step 3: public scans are kept 30 days. The worker deletes the
// finished public ones past their expiry, with their findings; nothing in the
// queue and nothing of an organization. Only qr_worker may run it.
func TestPurgeExpiredPublicScans(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	owner := w.s.Pool()
	type row struct {
		org             *uuid.UUID
		status, expires string // expires: an SQL expression, or "" for NULL
		created         string
		gone            bool
	}
	rows := map[string]row{
		"public done, expired":                 {nil, "done", "now() - interval '1 hour'", "now() - interval '31 days'", true},
		"public failed, expired":               {nil, "failed", "now() - interval '1 hour'", "now() - interval '31 days'", true},
		"public done, no expiry, 40 days old":  {nil, "done", "", "now() - interval '40 days'", true},
		"public done, no expiry, 10 days old":  {nil, "done", "", "now() - interval '10 days'", false},
		"public done, not expired":             {nil, "done", "now() + interval '1 day'", "now() - interval '29 days'", false},
		"public queued, expired":               {nil, "queued", "now() - interval '1 hour'", "now() - interval '31 days'", false},
		"public running, expired":              {nil, "running", "now() - interval '1 hour'", "now() - interval '31 days'", false},
		"organization done, past its 30 days":  {&w.orgA.ID, "done", "now() + interval '1 year'", "now() - interval '40 days'", false},
		"organization done, expiry passed too": {&w.orgA.ID, "done", "now() - interval '1 hour'", "now() - interval '14 months'", false},
	}
	ids := map[string]uuid.UUID{}
	for name, r := range rows {
		sc, err := w.s.EnqueueScan(ctx, EnqueueScanParams{Host: "purge.example", Port: 443, OrganizationID: r.org, Intensity: IntensityBasic})
		if err != nil {
			t.Fatal(err)
		}
		if err := w.s.InsertFindings(ctx, findingsFor(sc)); err != nil {
			t.Fatal(err)
		}
		expires, lease := "NULL", leaseFor(r.status)
		if r.expires != "" {
			expires = r.expires
		}
		if _, err := owner.Exec(ctx, "UPDATE scans SET status = $2, created_at = "+r.created+", expires_at = "+expires+", "+lease+" WHERE id = $1", sc.ID, r.status); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		ids[name] = sc.ID
	}

	if _, err := NewStore(testdb.AsRole(t, owner, RoleApp)).PurgeExpiredPublicScans(ctx); err == nil {
		t.Error("qr_app ran the purge: only qr_worker may")
	}
	worker := NewStore(testdb.AsRole(t, owner, RoleWorker))
	n, err := worker.PurgeExpiredPublicScans(ctx)
	if err != nil || n != 3 {
		t.Fatalf("purge as qr_worker: %d rows, %v; want 3", n, err)
	}
	for name, r := range rows {
		var scans, findings int
		if err := owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM scans WHERE id = $1), (SELECT count(*) FROM findings WHERE scan_id = $1)", ids[name]).Scan(&scans, &findings); err != nil {
			t.Fatal(err)
		}
		if gone := scans == 0; gone != r.gone || (gone && findings != 0) {
			t.Errorf("%s: deleted=%v (findings left %d), want deleted=%v", name, gone, findings, r.gone)
		}
	}
	if n, _ := worker.PurgeExpiredPublicScans(ctx); n != 0 {
		t.Errorf("second purge: %d rows, want 0", n)
	}
}

// leaseFor sets what a scan in that status must have: a lease when running
// (scans_lease_consistent), a result and a grade when done
// (scans_done_has_result).
func leaseFor(status string) string {
	switch status {
	case "running":
		return "lease_token = gen_random_uuid(), lease_until = now() + interval '1 minute'"
	case "done":
		return "lease_token = NULL, lease_until = NULL, result = '{}', scoring_version = '2026.3', grade = 'A'"
	}
	return "lease_token = NULL, lease_until = NULL"
}

// ADR-0007 (migration 00020): a finished scan is out of the worker's reach;
// a queued or running one is still its to update.
func TestFinishedScansFrozenForWorker(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	owner := w.s.Pool()
	worker := testdb.AsRole(t, owner, RoleWorker)
	for status, editable := range map[string]bool{"queued": true, "running": true, "done": false, "failed": false} {
		sc, err := w.s.EnqueueScan(ctx, EnqueueScanParams{Host: "frozen.example", Port: 443, Intensity: IntensityBasic})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := owner.Exec(ctx, "UPDATE scans SET status = $2, "+leaseFor(status)+" WHERE id = $1", sc.ID, status); err != nil {
			t.Fatal(err)
		}
		tag, err := worker.Exec(ctx, "UPDATE scans SET grade = 'F' WHERE id = $1", sc.ID)
		if err != nil {
			t.Fatalf("%s: %v", status, err)
		}
		var grade string
		_ = owner.QueryRow(ctx, "SELECT grade FROM scans WHERE id = $1", sc.ID).Scan(&grade)
		if changed := tag.RowsAffected() == 1 && grade == "F"; changed != editable {
			t.Errorf("%s scan: worker changed it = %v, want %v", status, changed, editable)
		}
	}
}
