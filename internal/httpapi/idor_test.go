package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// tenantWorld is one organization seen from the API: its members (by
// role), a pending invitation and an organization scan.
type tenantWorld struct {
	org        uuid.UUID
	name       string
	clients    map[string]*client // owner, admin, viewer
	userIDs    map[string]uuid.UUID
	emails     []string
	invitation uuid.UUID
	inviteTok  string
	inviteTo   string
	scan       uuid.UUID
	domain     uuid.UUID
	tag        string
}

func buildTenant(t *testing.T, a *authAPI, tag string, ipBase int) *tenantWorld {
	t.Helper()
	w := &tenantWorld{tag: tag, name: "Org " + tag, clients: map[string]*client{}, userIDs: map[string]uuid.UUID{}}
	owner, me := a.signedIn(t, "owner-"+tag+"@example.com", fmt.Sprintf("203.0.113.%d", ipBase))
	w.org = *me.ActiveOrgID
	expect(t, owner.do(t, "PATCH", "/api/org", js(map[string]string{"name": w.name})), http.StatusNoContent, "")
	w.clients["owner"] = owner
	for i, role := range []string{"admin", "viewer"} {
		email := role + "-" + tag + "@example.com"
		c, _ := a.signedIn(t, email, fmt.Sprintf("203.0.113.%d", ipBase+1+i))
		a.join(t, email, w.org, role)
		w.clients[role] = c
	}
	for email, m := range owner.members(t) {
		w.userIDs[email] = m.UserID
		w.emails = append(w.emails, email)
	}
	w.inviteTo = "pending-" + tag + "@example.com"
	r := owner.do(t, "POST", "/api/org/invitations", js(map[string]string{"email": w.inviteTo, "role": "viewer"}))
	expect(t, r, http.StatusCreated, "")
	var iv invitationView
	_ = json.Unmarshal(r.Body.Bytes(), &iv)
	w.invitation = iv.ID
	w.inviteTok = a.lastToken(t, w.inviteTo, "invitation")
	w.domain = addDomain(t, owner, "secret-"+tag+"-domain.com").ID
	a.verify(t, w.domain, "verified", 0)
	if err := a.owner.QueryRow(context.Background(), `INSERT INTO scans (host, port, organization_id, intensity, status)
		VALUES ('secret-`+tag+`.example', 443, $1, 'full', 'queued') RETURNING id`, w.org).Scan(&w.scan); err != nil {
		t.Fatal(err)
	}
	return w
}

// secrets are strings of w that must never appear in another tenant's answers.
func (w *tenantWorld) secrets() []string {
	s := []string{w.org.String(), w.name, w.invitation.String(), w.inviteTo, w.scan.String(), w.domain.String(), "secret-" + w.tag}
	for _, id := range w.userIDs {
		s = append(s, id.String())
	}
	return append(s, w.emails...)
}

// snapshot is the state of every organization table, to prove a refused
// request changed nothing.
func (a *authAPI) snapshot(t *testing.T) string {
	t.Helper()
	var s string
	err := a.owner.QueryRow(context.Background(), `SELECT concat_ws('|',
		(SELECT string_agg(id || ':' || name, ',' ORDER BY id) FROM organizations),
		(SELECT string_agg(organization_id || ':' || user_id || ':' || role, ',' ORDER BY organization_id, user_id) FROM memberships),
		(SELECT string_agg(id || ':' || coalesce(accepted_at::text, '') || ':' || coalesce(revoked_at::text, ''), ',' ORDER BY id) FROM invitations),
		(SELECT string_agg(id || ':' || coalesce(organization_id::text, ''), ',' ORDER BY id) FROM scans),
		(SELECT string_agg(id || ':' || organization_id || ':' || name || ':' || status || ':' || block_public || ':' || coalesce(check_requested_at::text, ''), ',' ORDER BY id) FROM domains))`).Scan(&s)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestIDOR: every endpoint that takes an ID, called from one organization
// with the other's IDs, answers 404 (or 403 where the role check comes
// first), changes nothing, and nothing of the other organization ever shows
// up in an answer.
func TestIDOR(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(1000, 1000), LoginIP: NewLimiter(1000, 1000)})
	A := buildTenant(t, a, "a", 10)
	B := buildTenant(t, a, "b", 20)

	cross := func(from, to *tenantWorld) {
		before := a.snapshot(t)
		for role, c := range from.clients {
			var answers []string
			check := func(what string, w *httptestRecorder, allowed ...int) {
				t.Helper()
				answers = append(answers, w.Body.String())
				for _, s := range allowed {
					if w.Code == s {
						return
					}
				}
				t.Errorf("%s %s → %s: %d %s", role, what, to.name, w.Code, w.Body)
			}
			for email, id := range to.userIDs {
				check("PATCH member "+email, c.do(t, "PATCH", "/api/org/members/"+id.String(), `{"role":"owner"}`), 404, 403)
				check("DELETE member "+email, c.do(t, "DELETE", "/api/org/members/"+id.String(), ""), 404, 403)
			}
			check("DELETE invitation", c.do(t, "DELETE", "/api/org/invitations/"+to.invitation.String(), ""), 404, 403)
			check("switch", c.do(t, "POST", "/api/organizations/"+to.org.String()+"/switch", ""), 404)
			dom := "/api/org/domains/" + to.domain.String()
			check("GET domain", c.do(t, "GET", dom, ""), 404)
			check("DELETE domain", c.do(t, "DELETE", dom, ""), 404, 403)
			check("verify domain", c.do(t, "POST", dom+"/verify", ""), 404, 403)
			check("opt-out of its domain", c.do(t, "PATCH", dom, `{"block_public":true}`), 404, 403)
			check("GET scan", c.do(t, "GET", "/api/scans/"+to.scan.String(), ""), 404)
			check("GET org scan", c.do(t, "GET", "/api/org/scans/"+to.scan.String(), ""), 404)
			// The other organization's verified domain does not let anyone here scan it.
			check("full scan of its domain", c.do(t, "POST", "/api/org/scans", js(map[string]string{"host": "secret-" + to.tag + "-domain.com"})), 403)
			check("accept invitation", c.do(t, "POST", "/api/invitations/accept", js(map[string]string{"token": to.inviteTok})), 403)
			// The organization comes from the session, never from the request.
			tamper := func(r *http.Request) {
				r.Header.Set("X-Org-ID", to.org.String())
				q := r.URL.Query()
				q.Set("organization_id", to.org.String())
				r.URL.RawQuery = q.Encode()
			}
			for _, path := range []string{"/api/org", "/api/org/members", "/api/org/invitations", "/api/org/domains", "/api/org/scans", "/api/auth/me"} {
				check("GET "+path, c.do(t, "GET", path, "", tamper), 200, 403)
			}
			check("PATCH /api/org (tampered)", c.do(t, "PATCH", "/api/org", `{"name":"`+from.name+`"}`, tamper), 204, 403)
			for _, ans := range answers {
				for _, s := range to.secrets() {
					if strings.Contains(ans, s) {
						t.Errorf("%s of %s saw %q of %s in %s", role, from.name, s, to.name, ans)
					}
				}
			}
		}
		if after := a.snapshot(t); after != before {
			t.Errorf("cross requests from %s changed the database:\nbefore %s\nafter  %s", from.name, before, after)
		}
	}
	cross(A, B)
	cross(B, A)
}

func TestOrganizationLimits(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(1000, 1000)})
	ana, _ := a.signedIn(t, "ana@example.com", "203.0.113.1")
	for i := range maxPendingInvitations {
		expect(t, ana.do(t, "POST", "/api/org/invitations", js(map[string]string{"email": fmt.Sprintf("p%d@example.com", i), "role": "viewer"})), http.StatusCreated, "")
	}
	expect(t, ana.do(t, "POST", "/api/org/invitations", `{"email":"one-more@example.com","role":"viewer"}`), http.StatusConflict, "limit_reached")
	// Re-inviting an address replaces its invitation: no new slot needed.
	expect(t, ana.do(t, "POST", "/api/org/invitations", `{"email":"p0@example.com","role":"admin"}`), http.StatusCreated, "")

	b := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(1000, 1000), OrgInvitations: NewWindowLimiter(2, time.Hour)})
	bob, _ := b.signedIn(t, "bob@example.com", "203.0.113.2")
	for i := range 2 {
		expect(t, bob.do(t, "POST", "/api/org/invitations", js(map[string]string{"email": fmt.Sprintf("q%d@example.com", i), "role": "viewer"})), http.StatusCreated, "")
	}
	w := bob.do(t, "POST", "/api/org/invitations", `{"email":"q9@example.com","role":"viewer"}`)
	expect(t, w, http.StatusTooManyRequests, "too_many_attempts")
	if w.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After")
	}

	for i := range maxOrgsPerUser - 1 { // the first one came with the account
		expect(t, bob.do(t, "POST", "/api/organizations", js(map[string]string{"name": fmt.Sprintf("Org %d", i)})), http.StatusCreated, "")
	}
	expect(t, bob.do(t, "POST", "/api/organizations", `{"name":"Too many"}`), http.StatusConflict, "limit_reached")
}

// A role in one organization never carries over to another.
func TestNoRoleEscalationAcrossOrganizations(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(1000, 1000), LoginIP: NewLimiter(1000, 1000)})
	A := buildTenant(t, a, "a", 10)
	B := buildTenant(t, a, "b", 20)
	ana := A.clients["owner"] // owner of A, and now viewer of B
	anaID := A.userIDs["owner-a@example.com"]
	a.join(t, "owner-a@example.com", B.org, "viewer") // also makes B her active organization
	before := a.snapshot(t)

	expect(t, ana.do(t, "PATCH", "/api/org/members/"+anaID.String(), `{"role":"owner"}`), http.StatusForbidden, "forbidden")
	expect(t, ana.do(t, "DELETE", "/api/org/members/"+B.userIDs["owner-b@example.com"].String(), ""), http.StatusForbidden, "forbidden")
	expect(t, ana.do(t, "PATCH", "/api/org", `{"name":"Mine now"}`), http.StatusForbidden, "forbidden")
	expect(t, ana.do(t, "POST", "/api/org/invitations", `{"email":"friend@example.com","role":"admin"}`), http.StatusForbidden, "forbidden")
	expect(t, ana.do(t, "DELETE", "/api/org/invitations/"+B.invitation.String(), ""), http.StatusForbidden, "forbidden")
	if o := ana.org(t); o.ID != B.org || o.Role != "viewer" {
		t.Errorf("in B: %+v", o)
	}
	if after := a.snapshot(t); after != before {
		t.Errorf("a viewer of B changed something:\n%s\n%s", before, after)
	}

	// Back in A she is owner again, and what she does stays in A.
	expect(t, ana.do(t, "POST", "/api/organizations/"+A.org.String()+"/switch", ""), http.StatusOK, "")
	expect(t, ana.do(t, "PATCH", "/api/org/members/"+B.userIDs["admin-b@example.com"].String(), `{"role":"viewer"}`), http.StatusNotFound, "not_found")
	if w := ana.do(t, "PATCH", "/api/org/members/"+A.userIDs["viewer-a@example.com"].String(), `{"role":"admin"}`); w.Code != http.StatusNoContent {
		t.Fatalf("own organization: %d %s", w.Code, w.Body)
	}
	var role string
	_ = a.owner.QueryRow(context.Background(), "SELECT role FROM memberships WHERE organization_id = $1 AND user_id = $2",
		B.org, anaID).Scan(&role)
	if role != "viewer" {
		t.Errorf("her role in B became %q", role)
	}
}
