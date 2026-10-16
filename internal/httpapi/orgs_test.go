package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/org"
)

// signedIn registers, verifies and signs in email from its own address.
func (a *authAPI) signedIn(t *testing.T, email, ip string) (*client, meView) {
	t.Helper()
	a.verifiedUser(t, email)
	c := a.client(ip)
	w := c.login(t, email, goodPass)
	expect(t, w, http.StatusOK, "")
	var me meView
	_ = json.Unmarshal(w.Body.Bytes(), &me)
	return c, me
}

// join adds email to org with role (as the database owner: invitations are
// tested on their own) and makes org the active one of their sessions.
func (a *authAPI) join(t *testing.T, email string, org uuid.UUID, role string) {
	t.Helper()
	ctx := context.Background()
	if _, err := a.owner.Exec(ctx, `INSERT INTO memberships (organization_id, user_id, role)
		SELECT $1, id, $3 FROM users WHERE email = $2`, org, email, role); err != nil {
		t.Fatal(err)
	}
	if _, err := a.owner.Exec(ctx, "UPDATE sessions SET active_org_id = $1 WHERE user_id = (SELECT id FROM users WHERE email = $2)", org, email); err != nil {
		t.Fatal(err)
	}
}

func (c *client) org(t *testing.T) orgDetail {
	t.Helper()
	w := c.do(t, "GET", "/api/org", "")
	expect(t, w, http.StatusOK, "")
	var o orgDetail
	_ = json.Unmarshal(w.Body.Bytes(), &o)
	return o
}

func (c *client) members(t *testing.T) map[string]memberView {
	t.Helper()
	w := c.do(t, "GET", "/api/org/members", "")
	expect(t, w, http.StatusOK, "")
	var b struct{ Members []memberView }
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	out := map[string]memberView{}
	for _, m := range b.Members {
		out[m.Email] = m
	}
	return out
}

func TestOrganizationRoles(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100)})
	ana, me := a.signedIn(t, "ana@example.com", "203.0.113.1")
	orgA := *me.ActiveOrgID
	bea, _ := a.signedIn(t, "bea@example.com", "203.0.113.2")
	carl, _ := a.signedIn(t, "carl@example.com", "203.0.113.3")
	dan, _ := a.signedIn(t, "dan@example.com", "203.0.113.4")
	a.join(t, "bea@example.com", orgA, "admin")
	a.join(t, "carl@example.com", orgA, "viewer")
	a.join(t, "dan@example.com", orgA, "viewer")
	ids := map[string]string{}
	for email, m := range ana.members(t) {
		ids[email] = m.UserID.String()
	}
	if len(ids) != 4 {
		t.Fatalf("members: %v", ids)
	}
	patch := func(c *client, who, r string) int {
		return c.do(t, "PATCH", "/api/org/members/"+ids[who], js(map[string]string{"role": r})).Code
	}
	del := func(c *client, who string) int { return c.do(t, "DELETE", "/api/org/members/"+ids[who], "").Code }

	// What each role sees.
	if o := carl.org(t); o.ID != orgA || o.Role != "viewer" || !slices.Equal(o.Permissions, []org.Action{org.View}) {
		t.Errorf("viewer: %+v", o)
	}
	if o := ana.org(t); o.Role != "owner" || len(o.Permissions) != 7 {
		t.Errorf("owner: %+v", o)
	}
	if m := carl.members(t); len(m) != 4 || !m["carl@example.com"].You || m["ana@example.com"].You || m["bea@example.com"].Role != "admin" {
		t.Errorf("viewer's member list: %+v", m)
	}

	// Viewer: read only.
	expect(t, carl.do(t, "PATCH", "/api/org", `{"name":"Hacked"}`), http.StatusForbidden, "forbidden")
	if patch(carl, "carl@example.com", "owner") != http.StatusForbidden || del(carl, "dan@example.com") != http.StatusForbidden {
		t.Error("a viewer changed roles or removed a member")
	}
	// Admin: renames, removes viewers only, cannot change roles.
	if w := bea.do(t, "PATCH", "/api/org", `{"name":"Acme SL"}`); w.Code != http.StatusNoContent {
		t.Fatalf("admin rename: %d %s", w.Code, w.Body)
	}
	if o := carl.org(t); o.Name != "Acme SL" {
		t.Errorf("name = %q", o.Name)
	}
	expect(t, bea.do(t, "PATCH", "/api/org", `{"name":"  "}`), http.StatusBadRequest, "invalid_organization")
	if patch(bea, "carl@example.com", "admin") != http.StatusForbidden || patch(bea, "bea@example.com", "owner") != http.StatusForbidden {
		t.Error("an admin changed a role")
	}
	if del(bea, "ana@example.com") != http.StatusForbidden {
		t.Error("an admin removed an owner")
	}
	if del(bea, "dan@example.com") != http.StatusNoContent {
		t.Error("an admin could not remove a viewer")
	}
	expect(t, dan.do(t, "GET", "/api/org/members", ""), http.StatusOK, "") // back to his own organization
	if o := dan.org(t); o.ID == orgA || o.Role != "owner" {
		t.Errorf("removed member still in the organization: %+v", o)
	}

	// Owner: any role; the last owner stays.
	expect(t, ana.do(t, "PATCH", "/api/org/members/"+ids["carl@example.com"], `{"role":"root"}`), http.StatusBadRequest, "invalid_role")
	expect(t, ana.do(t, "PATCH", "/api/org/members/"+ids["ana@example.com"], `{"role":"admin"}`), http.StatusConflict, "last_owner")
	expect(t, ana.do(t, "POST", "/api/org/leave", ""), http.StatusConflict, "last_owner")
	if patch(ana, "bea@example.com", "owner") != http.StatusNoContent || patch(ana, "ana@example.com", "admin") != http.StatusNoContent {
		t.Fatal("handing over ownership failed")
	}
	if patch(ana, "carl@example.com", "admin") != http.StatusForbidden || del(ana, "bea@example.com") != http.StatusForbidden {
		t.Error("an ex-owner (now admin) kept owner powers")
	}
	expect(t, bea.do(t, "POST", "/api/org/leave", ""), http.StatusConflict, "last_owner")

	// Not a member, not an ID.
	expect(t, bea.do(t, "DELETE", "/api/org/members/"+uuid.NewString(), ""), http.StatusNotFound, "not_found")
	expect(t, bea.do(t, "DELETE", "/api/org/members/nope", ""), http.StatusNotFound, "not_found")
	expect(t, bea.do(t, "DELETE", "/api/org/members/"+ids["dan@example.com"], ""), http.StatusNotFound, "not_found")

	// Leaving.
	if w := carl.do(t, "POST", "/api/org/leave", ""); w.Code != http.StatusNoContent {
		t.Fatalf("leave: %d %s", w.Code, w.Body)
	}
	if o := carl.org(t); o.ID == orgA {
		t.Error("still in the organization after leaving")
	}
	if m := bea.members(t); len(m) != 2 {
		t.Errorf("members after leaving: %v", m)
	}

	// CSRF and session apply.
	csrf := bea.csrf
	bea.csrf = ""
	expect(t, bea.do(t, "PATCH", "/api/org", `{"name":"x"}`), http.StatusForbidden, "csrf_failed")
	bea.csrf = csrf
	expect(t, a.client("203.0.113.9").do(t, "GET", "/api/org", ""), http.StatusUnauthorized, "unauthenticated")

	rows, err := a.owner.Query(context.Background(), "SELECT action FROM audit_log WHERE organization_id = $1 ORDER BY id", orgA)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		got = append(got, s)
	}
	want := "account.register,org.renamed,member.removed,member.role_changed,member.role_changed,member.left"
	if strings.Join(got, ",") != want {
		t.Errorf("audit = %v\nwant    %s", got, want)
	}
}

func TestNoOrganization(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100)})
	c, me := a.signedIn(t, "ana@example.com", "203.0.113.1")
	if _, err := a.owner.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", *me.ActiveOrgID); err != nil {
		t.Fatal(err)
	}
	expect(t, c.do(t, "GET", "/api/org", ""), http.StatusForbidden, "no_organization")
	expect(t, c.do(t, "GET", "/api/auth/me", ""), http.StatusOK, "") // the account itself still works
}
