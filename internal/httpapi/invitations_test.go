package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type httptestRecorder = httptest.ResponseRecorder

func tok(t *testing.T, a *authAPI, email string) string {
	t.Helper()
	return a.lastToken(t, email, "invitation")
}

func TestInvitations(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100)})
	ana, me := a.signedIn(t, "ana@example.com", "203.0.113.1")
	orgA := *me.ActiveOrgID
	bea, meB := a.signedIn(t, "bea@example.com", "203.0.113.2")
	carl, meC := a.signedIn(t, "carl@example.com", "203.0.113.3")
	anon := a.client("203.0.113.9")
	inv := func(c *client, email, role string) *httptestRecorder {
		return c.do(t, "POST", "/api/org/invitations", js(map[string]string{"email": email, "role": role}))
	}
	preview := func(token string) *httptestRecorder {
		return anon.do(t, "POST", "/api/invitations/preview", js(map[string]string{"token": token}))
	}
	accept := func(c *client, token string) *httptestRecorder {
		return c.do(t, "POST", "/api/invitations/accept", js(map[string]string{"token": token}))
	}

	// Create; a second invitation to the same address replaces the first.
	expect(t, inv(ana, "Bea@Example.com", "admin"), http.StatusCreated, "")
	first := tok(t, a, "bea@example.com")
	expect(t, inv(ana, "bea@example.com", "admin"), http.StatusCreated, "")
	token := tok(t, a, "bea@example.com")
	expect(t, preview(first), http.StatusBadRequest, "invalid_token")
	if e := a.outbox(t); !strings.Contains(e[len(e)-1].body, "ana@example.com te invita") || !strings.Contains(e[len(e)-1].body, "7 días") {
		t.Errorf("invitation email: %s", e[len(e)-1].body)
	}
	expect(t, inv(ana, "x@example.com", "owner"), http.StatusBadRequest, "invalid_role")
	expect(t, inv(ana, "ana@example.com", "viewer"), http.StatusConflict, "already_member")
	expect(t, inv(ana, "nope", "viewer"), http.StatusBadRequest, "invalid_email")

	w := ana.do(t, "GET", "/api/org/invitations", "")
	var list struct{ Invitations []invitationView }
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Invitations) != 1 || list.Invitations[0].Email != "bea@example.com" || *list.Invitations[0].InviterEmail != "ana@example.com" {
		t.Fatalf("pending: %s", w.Body)
	}

	// Preview without an account; only the invited address can accept.
	w = preview(token)
	expect(t, w, http.StatusOK, "")
	var p invitationPreview
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if p.Organization == "" || p.Role != "admin" || p.Email != "bea@example.com" {
		t.Errorf("preview: %s", w.Body)
	}
	expect(t, accept(carl, token), http.StatusForbidden, "invitation_other_email")
	expect(t, accept(anon, token), http.StatusUnauthorized, "unauthenticated")
	w = accept(bea, token) // not spent by carl's try
	expect(t, w, http.StatusOK, "")
	if o := bea.org(t); o.ID != orgA || o.Role != "admin" {
		t.Errorf("after accepting: %+v", o)
	}
	expect(t, accept(bea, token), http.StatusBadRequest, "invalid_token")
	if m := ana.members(t); m["bea@example.com"].Role != "admin" {
		t.Errorf("members: %v", m)
	}

	// Admins invite admins and viewers; viewers nobody.
	expect(t, inv(bea, "dan@example.com", "viewer"), http.StatusCreated, "")
	a.join(t, "carl@example.com", orgA, "viewer")
	expect(t, inv(carl, "zoe@example.com", "viewer"), http.StatusForbidden, "forbidden")
	expect(t, carl.do(t, "GET", "/api/org/invitations", ""), http.StatusForbidden, "forbidden")

	// Revoke.
	_ = json.Unmarshal(ana.do(t, "GET", "/api/org/invitations", "").Body.Bytes(), &list)
	danTok := tok(t, a, "dan@example.com")
	id := list.Invitations[0].ID.String()
	expect(t, carl.do(t, "DELETE", "/api/org/invitations/"+id, ""), http.StatusForbidden, "forbidden")
	if w := ana.do(t, "DELETE", "/api/org/invitations/"+id, ""); w.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", w.Code, w.Body)
	}
	expect(t, ana.do(t, "DELETE", "/api/org/invitations/"+id, ""), http.StatusNotFound, "not_found")
	expect(t, preview(danTok), http.StatusBadRequest, "invalid_token")

	// Expired.
	expect(t, inv(ana, "fran@example.com", "viewer"), http.StatusCreated, "")
	franTok := tok(t, a, "fran@example.com")
	if _, err := a.owner.Exec(context.Background(), "UPDATE invitations SET expires_at = now() - interval '1 second' WHERE email = 'fran@example.com'"); err != nil {
		t.Fatal(err)
	}
	expect(t, preview(franTok), http.StatusBadRequest, "invalid_token")

	// Signing up with an invitation: no code, no own organization, verified.
	expect(t, inv(ana, "eva@example.com", "viewer"), http.StatusCreated, "")
	evaTok := tok(t, a, "eva@example.com")
	reg := func(email, token string) *httptestRecorder {
		return anon.do(t, "POST", "/api/auth/register", js(map[string]string{"email": email, "password": goodPass, "invitation_token": token}))
	}
	expect(t, reg("other@example.com", evaTok), http.StatusForbidden, "invitation_other_email")
	if w := reg("eva@example.com", evaTok); w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), "registered") {
		t.Fatalf("register with invitation: %d %s", w.Code, w.Body)
	}
	expect(t, reg("eva@example.com", evaTok), http.StatusBadRequest, "invalid_token")
	eva := a.client("203.0.113.5")
	w = eva.login(t, "eva@example.com", goodPass)
	expect(t, w, http.StatusOK, "")
	var meE meView
	_ = json.Unmarshal(w.Body.Bytes(), &meE)
	if len(meE.Organizations) != 1 || meE.Organizations[0].ID != orgA || meE.Organizations[0].Role != "viewer" {
		t.Errorf("invited user's organizations: %+v", meE.Organizations)
	}
	// An address that already has an account answers like a new signup (no
	// enumeration): its owner gets an email, the invitation is not spent.
	a.verifiedUser(t, "eli@example.com")
	expect(t, inv(ana, "eli@example.com", "viewer"), http.StatusCreated, "")
	eliTok := tok(t, a, "eli@example.com")
	expect(t, reg("eli@example.com", eliTok), http.StatusAccepted, "")
	if a.count(t, "eli@example.com", "account_exists") != 1 {
		t.Error("no account_exists email")
	}
	expect(t, preview(eliTok), http.StatusOK, "")

	// Switching and creating organizations.
	beaOwn := *meB.ActiveOrgID
	sw := func(c *client, org string) *httptestRecorder {
		return c.do(t, "POST", "/api/organizations/"+org+"/switch", "")
	}
	expect(t, sw(bea, beaOwn.String()), http.StatusOK, "")
	if o := bea.org(t); o.ID != beaOwn || o.Role != "owner" {
		t.Errorf("switched: %+v", o)
	}
	expect(t, sw(bea, meC.ActiveOrgID.String()), http.StatusNotFound, "not_found") // carl's, not hers
	expect(t, sw(bea, "nope"), http.StatusNotFound, "not_found")
	expect(t, sw(bea, orgA.String()), http.StatusOK, "")
	w = bea.do(t, "POST", "/api/organizations", `{"name":"Bea Consulting"}`)
	expect(t, w, http.StatusCreated, "")
	if o := bea.org(t); o.Name != "Bea Consulting" || o.Role != "owner" {
		t.Errorf("new organization: %+v", o)
	}
	expect(t, bea.do(t, "POST", "/api/organizations", `{"name":""}`), http.StatusBadRequest, "invalid_organization")

	var actions []string
	rows, _ := a.owner.Query(context.Background(), "SELECT action FROM audit_log WHERE action LIKE 'invitation.%' OR action = 'org.created' ORDER BY id")
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		actions = append(actions, s)
	}
	if got := strings.Join(actions, ","); got != "invitation.created,invitation.created,invitation.accepted,invitation.created,invitation.revoked,invitation.created,invitation.created,invitation.created,org.created" {
		t.Errorf("audit: %s", got)
	}
}
