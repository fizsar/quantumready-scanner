package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// publicViewKeys are the only top-level fields of a scan as the API serves it
// (scanView). The web's "Download JSON" saves exactly that response.
var publicViewKeys = []string{"id", "host", "port", "intensity", "status", "retries", "created_at", "started_at", "finished_at",
	"error", "reused", "age_seconds", "queue_position", "queue_eta_seconds", "lang", "report"}

// An organization's scan, downloaded from its page ("Download JSON" saves the
// GET /api/org/scans/{id} response as is), carries the same fields as a
// public one: no organization, user, domain or lease identifiers, and no
// member's address. The public download keeps its shape too.
func TestOrgScanDownloadHasNoInternalData(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100)})
	ana, me := a.signedIn(t, "ana@example.com", "203.0.113.1")
	orgA := *me.ActiveOrgID
	d := addDomain(t, ana, "fizsar.com")
	a.verify(t, d.ID, "verified", 0)

	sc, fs := labScan(t, "kyber", "lab.fizsar.com")
	ctx := context.Background()
	if _, err := a.owner.Exec(ctx, `INSERT INTO scans (id, host, port, status, created_at, started_at, finished_at, result, scoring,
			scoring_version, grade, score, organization_id, intensity)
		VALUES ($1, $2, 443, 'done', $3, $3, $4, $5, $6, $6::jsonb->>'scoring_version', $6::jsonb->>'grade', ($6::jsonb->>'score')::int, $7, 'full')`,
		sc.ID, sc.Host, sc.CreatedAt, sc.FinishedAt, sc.Result, sc.Scoring, orgA); err != nil {
		t.Fatal(err)
	}
	for _, f := range fs {
		if _, err := a.owner.Exec(ctx, `INSERT INTO findings (scan_id, position, code, axis, severity, category, params) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			sc.ID, f.Position, f.Code, f.Axis, f.Severity, f.Category, f.Params); err != nil {
			t.Fatal(err)
		}
	}

	check := func(name string, body []byte) {
		t.Helper()
		var top map[string]json.RawMessage
		if err := json.Unmarshal(body, &top); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for k := range top {
			if !slices.Contains(publicViewKeys, k) {
				t.Errorf("%s: field %q is not part of the public scan view", name, k)
			}
		}
		raw := string(body)
		for _, leak := range []string{orgA.String(), me.User.ID.String(), d.ID.String(), "ana@example.com",
			"organization", "user_id", "requested_by", "lease", "domain_id", "attempts"} {
			if strings.Contains(raw, leak) {
				t.Errorf("%s: the downloaded JSON contains %q", name, leak)
			}
		}
	}

	w := ana.do(t, "GET", "/api/org/scans/"+sc.ID.String(), "")
	expect(t, w, http.StatusOK, "")
	check("organization scan", w.Body.Bytes())

	// The public download: a public scan through the public endpoint.
	pub, _ := labScan(t, "export", "fizsar.com")
	if _, err := a.owner.Exec(ctx, `INSERT INTO scans (id, host, port, status, created_at, started_at, finished_at, result, scoring,
			scoring_version, grade, score)
		VALUES ($1, $2, 443, 'done', $3, $3, $4, $5, $6, $6::jsonb->>'scoring_version', $6::jsonb->>'grade', ($6::jsonb->>'score')::int)`,
		pub.ID, pub.Host, pub.CreatedAt, pub.FinishedAt, pub.Result, pub.Scoring); err != nil {
		t.Fatal(err)
	}
	w = a.client("203.0.113.9").do(t, "GET", "/api/scans/"+pub.ID.String(), "")
	expect(t, w, http.StatusOK, "")
	check("public scan", w.Body.Bytes())
}

// verify marks a domain verified (or lost since lostDaysAgo) as the worker would.
func (a *authAPI) verify(t *testing.T, id uuid.UUID, status string, lostDaysAgo int) {
	t.Helper()
	if _, err := a.owner.Exec(context.Background(), `UPDATE domains SET status = $2, verified_at = now() - interval '30 days',
		lost_at = CASE WHEN $2 = 'lost' THEN now() - make_interval(days => $3) END WHERE id = $1`, id, status, lostDaysAgo); err != nil {
		t.Fatal(err)
	}
}

func TestOrganizationFullScans(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100), OrgScans: NewWindowLimiter(3, time.Hour)})
	ana, me := a.signedIn(t, "ana@example.com", "203.0.113.1")
	orgA := *me.ActiveOrgID
	carl, _ := a.signedIn(t, "carl@example.com", "203.0.113.3")
	a.join(t, "carl@example.com", orgA, "viewer")
	scan := func(c *client, host string) *httptestRecorder {
		return c.do(t, "POST", "/api/org/scans", js(map[string]string{"host": host}))
	}

	d := addDomain(t, ana, "fizsar.com")
	expect(t, scan(ana, "fizsar.com"), http.StatusForbidden, "domain_not_verified") // still pending
	a.verify(t, d.ID, "verified", 0)

	w := scan(ana, "https://QuantumReady.fizsar.com/") // a subdomain of a verified domain
	expect(t, w, http.StatusAccepted, "")
	var v scanView
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if v.Intensity != "full" || v.Host != "quantumready.fizsar.com" || w.Header().Get("Location") != "/api/org/scans/"+v.ID.String() {
		t.Fatalf("full scan: %s", w.Body)
	}
	expect(t, scan(ana, "quantumready.fizsar.com"), http.StatusOK, "") // reused within the cooldown
	expect(t, scan(ana, "example.org"), http.StatusForbidden, "domain_not_verified")
	expect(t, scan(ana, "fizsar.com:8443"), http.StatusBadRequest, "port_not_allowed")
	expect(t, scan(carl, "fizsar.com"), http.StatusForbidden, "forbidden") // viewers read only

	// History: members see it; the public endpoint never shows an organization's scan.
	w = carl.do(t, "GET", "/api/org/scans", "")
	var list struct{ Scans []scanSummary }
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Scans) != 1 || list.Scans[0].ID != v.ID || list.Scans[0].Intensity != "full" {
		t.Errorf("history: %s", w.Body)
	}
	expect(t, carl.do(t, "GET", "/api/org/scans/"+v.ID.String(), ""), http.StatusOK, "")
	expect(t, a.client("203.0.113.9").do(t, "GET", "/api/scans/"+v.ID.String(), ""), http.StatusNotFound, "not_found")
	var public uuid.UUID
	if err := a.owner.QueryRow(context.Background(), "INSERT INTO scans (host, port) VALUES ('public.example', 443) RETURNING id").Scan(&public); err != nil {
		t.Fatal(err)
	}
	expect(t, carl.do(t, "GET", "/api/org/scans/"+public.String(), ""), http.StatusNotFound, "not_found") // not part of the history

	// Lost: allowed during the 14-day grace, not after.
	a.verify(t, d.ID, "lost", 13)
	expect(t, scan(ana, "fizsar.com"), http.StatusAccepted, "")
	a.verify(t, d.ID, "lost", 15)
	expect(t, scan(ana, "www.fizsar.com"), http.StatusForbidden, "domain_not_verified")

	// Per-organization limit (reused scans did not count).
	a.verify(t, d.ID, "verified", 0)
	expect(t, scan(ana, "a.fizsar.com"), http.StatusAccepted, "")
	w = scan(ana, "b.fizsar.com")
	expect(t, w, http.StatusTooManyRequests, "too_many_attempts")

	// A finished organization scan shows its findings (read under the
	// organization's tenant: with the public one RLS would hide them).
	sc, fs := labScan(t, "kyber", "lab.fizsar.com")
	ctx := context.Background()
	if _, err := a.owner.Exec(ctx, `INSERT INTO scans (id, host, port, status, created_at, started_at, finished_at, result, scoring,
			scoring_version, grade, score, organization_id, intensity)
		VALUES ($1, $2, 443, 'done', $3, $3, $4, $5, $6, $6::jsonb->>'scoring_version', $6::jsonb->>'grade', ($6::jsonb->>'score')::int, $7, 'full')`,
		sc.ID, sc.Host, sc.CreatedAt, sc.FinishedAt, sc.Result, sc.Scoring, orgA); err != nil {
		t.Fatal(err)
	}
	for _, f := range fs {
		if _, err := a.owner.Exec(ctx, `INSERT INTO findings (scan_id, position, code, axis, severity, category, params) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			sc.ID, f.Position, f.Code, f.Axis, f.Severity, f.Category, f.Params); err != nil {
			t.Fatal(err)
		}
	}
	w = carl.do(t, "GET", "/api/org/scans/"+sc.ID.String(), "")
	var done scanView
	_ = json.Unmarshal(w.Body.Bytes(), &done)
	if done.Report == nil || len(done.Report.Findings) != len(fs) || len(fs) == 0 {
		t.Errorf("finished organization scan: %d findings shown, %d stored", func() int {
			if done.Report == nil {
				return -1
			}
			return len(done.Report.Findings)
		}(), len(fs))
	}
	_ = json.Unmarshal(carl.do(t, "GET", "/api/org/scans", "").Body.Bytes(), &list)
	i := slices.IndexFunc(list.Scans, func(s scanSummary) bool { return s.ID == sc.ID })
	if i < 0 || list.Scans[i].Grade == "" || list.Scans[i].Score == nil {
		t.Errorf("history summary of a finished scan: %+v", list.Scans)
	}
}
