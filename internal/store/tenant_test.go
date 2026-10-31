package store

import (
	"context"
	"errors"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// world is two organizations with their users, domains and scans, plus a
// public scan, created as the owner role (no RLS).
type world struct {
	s                    *Store
	orgA, orgB           Organization
	userA, userB         User // userA: owner of A and viewer of B; userB: owner of B
	domainA, domainB     Domain
	public, scanA, scanB Scan
}

func newWorld(t *testing.T) *world {
	t.Helper()
	ctx := context.Background()
	w := &world{s: newTestStore(t)}
	q := w.s.Queries
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var err error
	w.userA, err = q.CreateUser(ctx, CreateUserParams{Email: "a@example.com", PasswordHash: "x", Lang: "es"})
	must(err)
	w.userB, err = q.CreateUser(ctx, CreateUserParams{Email: "b@example.com", PasswordHash: "x", Lang: "en"})
	must(err)
	w.orgA, err = q.CreateOrganization(ctx, CreateOrganizationParams{ID: uuid.New(), Name: "A"})
	must(err)
	w.orgB, err = q.CreateOrganization(ctx, CreateOrganizationParams{ID: uuid.New(), Name: "B"})
	must(err)
	must(q.AddMembership(ctx, AddMembershipParams{OrganizationID: w.orgA.ID, UserID: w.userA.ID, Role: "owner"}))
	must(q.AddMembership(ctx, AddMembershipParams{OrganizationID: w.orgB.ID, UserID: w.userB.ID, Role: "owner"}))
	must(q.AddMembership(ctx, AddMembershipParams{OrganizationID: w.orgB.ID, UserID: w.userA.ID, Role: "viewer"}))
	w.domainA, err = q.CreateDomain(ctx, CreateDomainParams{OrganizationID: w.orgA.ID, Name: "a.example", Token: "ta"})
	must(err)
	w.domainB, err = q.CreateDomain(ctx, CreateDomainParams{OrganizationID: w.orgB.ID, Name: "b.example", Token: "tb"})
	must(err)
	scan := func(org *uuid.UUID, intensity string) Scan {
		sc, err := q.EnqueueScan(ctx, EnqueueScanParams{Host: "x.example", Port: 443, OrganizationID: org, Intensity: intensity})
		must(err)
		err = q.InsertFindings(ctx, findingsFor(sc))
		must(err)
		return sc
	}
	w.public = scan(nil, IntensityBasic)
	w.scanA = scan(&w.orgA.ID, IntensityFull)
	w.scanB = scan(&w.orgB.ID, IntensityFull)
	return w
}

func (w *world) tenant(org Organization, user User) Tenant {
	return Tenant{OrgID: &org.ID, UserID: &user.ID}
}

// ids runs a raw query (no organization filter at all) in the tenant's
// transaction and returns the first column: what row-level security lets
// through when the application forgets to filter.
func (w *world) ids(t *testing.T, tn Tenant, query string) []string {
	t.Helper()
	var out []string
	err := w.s.Tx(context.Background(), tn, func(q *Queries) error {
		rows, err := q.db.Query(context.Background(), query)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	sort.Strings(out)
	return out
}

func sorted(s ...string) []string {
	sort.Strings(s)
	return s
}

// Every table with an organization_id column, and findings (which belong to
// a scan), must have row-level security enabled and forced, with policies.
// A new table that forgets it fails here.
func TestRLSCoversEveryOrganizationTable(t *testing.T) {
	s := newTestStore(t)
	rows, err := s.pool.Query(context.Background(), `
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity,
		       (SELECT count(*) FROM pg_policy p WHERE p.polrelid = c.oid)
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relkind = 'r'
		  AND (c.relname IN ('organizations', 'findings') OR EXISTS (
		      SELECT 1 FROM pg_attribute a WHERE a.attrelid = c.oid AND a.attname = 'organization_id' AND NOT a.attisdropped))`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		var enabled, forced bool
		var policies int
		if err := rows.Scan(&name, &enabled, &forced, &policies); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
		if !enabled || !forced || policies == 0 {
			t.Errorf("%s: RLS enabled=%v forced=%v policies=%d", name, enabled, forced, policies)
		}
	}
	want := sorted("audit_log", "domains", "findings", "invitations", "memberships", "organizations", "scans")
	if sort.Strings(tables); !slices.Equal(tables, want) {
		t.Errorf("organization tables = %v, want %v", tables, want)
	}
}

func TestTenantIsolation(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	a := w.tenant(w.orgA, w.userA)
	b := w.tenant(w.orgB, w.userB)

	t.Run("application queries stay in the tenant", func(t *testing.T) {
		err := w.s.Tx(ctx, a, func(q *Queries) error {
			ds, err := q.ListDomains(ctx, w.orgA.ID)
			if err != nil || len(ds) != 1 || ds[0].ID != w.domainA.ID {
				t.Errorf("ListDomains(A) = %v, %v", ds, err)
			}
			// Asking for B's organization from A's context: RLS returns nothing.
			if ds, _ := q.ListDomains(ctx, w.orgB.ID); len(ds) != 0 {
				t.Errorf("ListDomains(B) from A = %v", ds)
			}
			if _, err := q.GetDomain(ctx, GetDomainParams{OrganizationID: w.orgA.ID, ID: w.domainB.ID}); !errors.Is(err, pgx.ErrNoRows) {
				t.Errorf("GetDomain(B's domain) from A: %v", err)
			}
			scans, err := q.ListOrganizationScans(ctx, ListOrganizationScansParams{OrganizationID: &w.orgB.ID, MaxRows: 10})
			if err != nil || len(scans) != 0 {
				t.Errorf("ListOrganizationScans(B) from A = %v, %v", scans, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.s.ScanFor(ctx, a, w.scanB.ID); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("ScanFor(A, B's scan) = %v, want ErrNoRows", err)
		}
		if _, err := w.s.ScanFor(ctx, a, w.public.ID); err != nil {
			t.Errorf("ScanFor(A, public scan) = %v", err)
		}
		if fs, err := w.s.FindingsFor(ctx, a, w.scanB.ID); err != nil || len(fs) != 0 {
			t.Errorf("FindingsFor(A, B's scan) = %d findings, %v", len(fs), err)
		}
		// The explicit filter alone also protects (first layer): asking for
		// scan B with organization A in the WHERE clause, as the owner role.
		if _, err := w.s.GetScanForTenant(ctx, GetScanForTenantParams{ID: w.scanB.ID, OrganizationID: &w.orgA.ID}); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("GetScanForTenant(B's scan, org A) without RLS = %v", err)
		}
	})

	t.Run("a query without organization filter sees only the tenant (RLS)", func(t *testing.T) {
		if got, want := w.ids(t, a, "SELECT id::text FROM domains"), sorted(w.domainA.ID.String()); !slices.Equal(got, want) {
			t.Errorf("domains seen by A = %v, want %v", got, want)
		}
		if got, want := w.ids(t, a, "SELECT id::text FROM scans"), sorted(w.public.ID.String(), w.scanA.ID.String()); !slices.Equal(got, want) {
			t.Errorf("scans seen by A = %v, want public + A's", got)
		}
		if got := w.ids(t, a, "SELECT DISTINCT scan_id::text FROM findings"); slices.Contains(got, w.scanB.ID.String()) {
			t.Errorf("A sees findings of B's scan: %v", got)
		}
		// userA belongs to A and B: sees both organizations and its own
		// memberships, but only A's member list.
		if got := w.ids(t, a, "SELECT id::text FROM organizations"); len(got) != 2 {
			t.Errorf("organizations seen by userA = %v, want A and B", got)
		}
		if got := w.ids(t, a, "SELECT user_id::text || '@' || organization_id::text FROM memberships"); len(got) != 2 ||
			slices.Contains(got, w.userB.ID.String()+"@"+w.orgB.ID.String()) {
			t.Errorf("memberships seen by userA in A = %v", got)
		}
		if got := w.ids(t, b, "SELECT id::text FROM organizations"); !slices.Equal(got, []string{w.orgB.ID.String()}) {
			t.Errorf("organizations seen by userB = %v, want only B", got)
		}
	})

	t.Run("writes outside the tenant are refused or touch nothing", func(t *testing.T) {
		exec := func(sql string, args ...any) (int64, error) {
			var n int64
			err := w.s.Tx(ctx, a, func(q *Queries) error {
				tag, err := q.db.Exec(ctx, sql, args...)
				n = tag.RowsAffected()
				return err
			})
			return n, err
		}
		if _, err := exec("INSERT INTO domains (organization_id, name, token) VALUES ($1, 'evil.example', 't')", w.orgB.ID); err == nil {
			t.Error("A inserted a domain into B")
		}
		if _, err := exec("INSERT INTO scans (host, organization_id, intensity) VALUES ('x.example', $1, 'full')", w.orgB.ID); err == nil {
			t.Error("A inserted a scan into B")
		}
		if n, err := exec("UPDATE domains SET name = 'evil.example' WHERE id = $1", w.domainB.ID); err != nil || n != 0 {
			t.Errorf("A updated B's domain: %d rows, %v", n, err)
		}
		if n, err := exec("DELETE FROM domains WHERE id = $1", w.domainB.ID); err != nil || n != 0 {
			t.Errorf("A deleted B's domain: %d rows, %v", n, err)
		}
		// userA is a viewer of B and sees that membership, but cannot make
		// itself owner of B while working in A.
		if n, err := exec("UPDATE memberships SET role = 'owner' WHERE organization_id = $1 AND user_id = $2", w.orgB.ID, w.userA.ID); err != nil || n != 0 {
			t.Errorf("userA promoted itself in B from A: %d rows, %v", n, err)
		}
		if n, err := exec("UPDATE organizations SET name = 'pwned' WHERE id = $1", w.orgB.ID); err != nil || n != 0 {
			t.Errorf("A renamed B: %d rows, %v", n, err)
		}
		// The API role cannot change or delete scans at all (only the worker).
		if _, err := exec("UPDATE scans SET grade = 'A' WHERE id = $1", w.scanA.ID); err == nil {
			t.Error("qr_app may update scans")
		}
	})

	t.Run("the public tenant sees only public scans", func(t *testing.T) {
		if got := w.ids(t, Public, "SELECT id::text FROM scans"); !slices.Equal(got, []string{w.public.ID.String()}) {
			t.Errorf("public sees scans %v", got)
		}
		for _, table := range []string{"domains", "organizations", "memberships", "invitations", "audit_log"} {
			if got := w.ids(t, Public, "SELECT 'x' FROM "+table); len(got) != 0 {
				t.Errorf("public sees %d rows of %s", len(got), table)
			}
		}
	})

	t.Run("the tenant does not leak to the next use of a pooled connection", func(t *testing.T) {
		for range 5 { // several rounds so a reused connection is likely
			_ = w.ids(t, a, "SELECT id::text FROM domains")
			var role, org string
			if err := w.s.pool.QueryRow(ctx, "SELECT current_user, coalesce(current_setting('app.org_id', true), '')").Scan(&role, &org); err != nil {
				t.Fatal(err)
			}
			if role == "qr_app" || org != "" {
				t.Fatalf("after a tenant transaction: role %q, app.org_id %q", role, org)
			}
		}
	})
}

// A basic scan never answers a full request, and a tenant never gets
// another tenant's scan through the reuse cooldown.
func TestReuseNeverCrossesIntensityOrTenant(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	a := w.tenant(w.orgA, w.userA)
	b := w.tenant(w.orgB, w.userB)
	req := func(intensity string) ScanRequest {
		return ScanRequest{Host: "target.example", Port: 443, Intensity: intensity, Cooldown: time.Hour}
	}
	enqueue := func(tn Tenant, intensity string) (Scan, bool) {
		t.Helper()
		sc, reused, err := w.s.EnqueueOrReuse(ctx, tn, req(intensity))
		if err != nil {
			t.Fatal(err)
		}
		return sc, reused
	}
	pub, _ := enqueue(Public, IntensityBasic)
	if pub.OrganizationID != nil || pub.ExpiresAt == nil || time.Until(*pub.ExpiresAt) < 29*24*time.Hour {
		t.Errorf("public scan: org %v, expires %v", pub.OrganizationID, pub.ExpiresAt)
	}
	fullA, reused := enqueue(a, IntensityFull)
	if reused || fullA.ID == pub.ID || fullA.Intensity != IntensityFull || *fullA.RequestedBy != w.userA.ID ||
		fullA.ExpiresAt == nil || time.Until(*fullA.ExpiresAt) < 394*24*time.Hour { // an organization's history: 13 months
		t.Errorf("full for A reused the public basic scan or lost its owner: %+v", fullA)
	}
	if again, reused := enqueue(a, IntensityFull); !reused || again.ID != fullA.ID {
		t.Errorf("A's second full request: reused=%v id=%s, want A's scan", reused, again.ID)
	}
	if fullB, reused := enqueue(b, IntensityFull); reused || fullB.ID == fullA.ID {
		t.Error("B got A's full scan")
	}
	if basicA, reused := enqueue(a, IntensityBasic); reused || basicA.ID == pub.ID {
		t.Error("A's basic request got the public scan")
	}
	if again, reused := enqueue(Public, IntensityBasic); !reused || again.ID != pub.ID {
		t.Errorf("second public request: reused=%v, want the public scan", reused)
	}
	if _, _, err := w.s.EnqueueOrReuse(ctx, Public, req(IntensityFull)); !errors.Is(err, ErrFullNeedsOrg) {
		t.Errorf("public full request: %v, want ErrFullNeedsOrg", err)
	}
	// One of the operator's own domains (SCAN_FULL_DOMAINS): public and full,
	// never the public basic scan, and reused as full only.
	listed := req(IntensityFull)
	listed.OperatorListed = true
	pubFull, reused, err := w.s.EnqueueOrReuse(ctx, Public, listed)
	if err != nil || reused || pubFull.ID == pub.ID || pubFull.Intensity != IntensityFull || pubFull.OrganizationID != nil ||
		pubFull.ExpiresAt == nil || time.Until(*pubFull.ExpiresAt) > 31*24*time.Hour {
		t.Errorf("listed public full: %+v reused=%v err=%v, want a new public full scan expiring in 30 days", pubFull, reused, err)
	}
	if again, reused, _ := w.s.EnqueueOrReuse(ctx, Public, listed); !reused || again.ID != pubFull.ID {
		t.Errorf("listed public full again: reused=%v, want the same", reused)
	}
	for intensity, want := range map[string]Scan{IntensityBasic: pub, IntensityFull: pubFull} {
		got, err := w.s.RecentPublicScan(ctx, "target.example", 443, intensity, time.Hour, time.Hour, "")
		if err != nil || got.ID != want.ID {
			t.Errorf("RecentPublicScan(%s) = %s, %v; want %s", intensity, got.ID, err, want.ID)
		}
	}
	if _, _, err := w.s.EnqueueOrReuse(ctx, a, req("lab")); err == nil {
		t.Error("unknown intensity accepted")
	}
}
