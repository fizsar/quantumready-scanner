package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fizsar/quantumready-saas/internal/store"
)

func addDomain(t *testing.T, c *client, name string) domainView {
	t.Helper()
	w := c.do(t, "POST", "/api/org/domains", js(map[string]string{"name": name}))
	expect(t, w, http.StatusCreated, "")
	var d domainView
	_ = json.Unmarshal(w.Body.Bytes(), &d)
	return d
}

func TestDomains(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100), DomainChecks: NewWindowLimiter(2, time.Hour)})
	ana, me := a.signedIn(t, "ana@example.com", "203.0.113.1")
	carl, _ := a.signedIn(t, "carl@example.com", "203.0.113.3")
	a.join(t, "carl@example.com", *me.ActiveOrgID, "viewer")

	d := addDomain(t, ana, " FizSar.com. ")
	if d.Name != "fizsar.com" || d.Status != "pending" || d.Verified || d.Record.Type != "TXT" ||
		d.Record.Name != "_quantumready.fizsar.com" || !regexp.MustCompile(`^qr-verify=[a-z2-7]{26}$`).MatchString(d.Record.Value) {
		t.Fatalf("new domain: %+v", d)
	}
	expect(t, ana.do(t, "POST", "/api/org/domains", `{"name":"fizsar.com"}`), http.StatusConflict, "domain_exists")
	for in, code := range map[string]string{
		"co.uk": "domain_public_suffix", "github.io": "domain_public_suffix", "com": "invalid_domain",
		"192.0.2.10": "domain_ip", "[2001:db8::1]": "domain_ip", "printer.local": "domain_reserved",
		"x.in-addr.arpa": "domain_reserved", "foo.notatld": "domain_unknown_tld", "fizsar.com:8443": "invalid_domain",
	} {
		expect(t, ana.do(t, "POST", "/api/org/domains", js(map[string]string{"name": in})), http.StatusBadRequest, code)
	}
	sub := addDomain(t, ana, "quantumready.fizsar.com") // subdomains can be verified on their own

	// Viewers see, cannot change.
	w := carl.do(t, "GET", "/api/org/domains", "")
	expect(t, w, http.StatusOK, "")
	if !strings.Contains(w.Body.String(), "_quantumready.fizsar.com") {
		t.Errorf("viewer's list: %s", w.Body)
	}
	expect(t, carl.do(t, "POST", "/api/org/domains", `{"name":"example.org"}`), http.StatusForbidden, "forbidden")
	expect(t, carl.do(t, "POST", "/api/org/domains/"+d.ID.String()+"/verify", ""), http.StatusForbidden, "forbidden")
	expect(t, carl.do(t, "DELETE", "/api/org/domains/"+d.ID.String(), ""), http.StatusForbidden, "forbidden")

	// Verify: queued for the worker, limited per domain.
	w = ana.do(t, "POST", "/api/org/domains/"+d.ID.String()+"/verify", "")
	expect(t, w, http.StatusAccepted, "")
	if !strings.Contains(w.Body.String(), `"check_pending":true`) {
		t.Errorf("verify: %s", w.Body)
	}
	expect(t, ana.do(t, "POST", "/api/org/domains/"+d.ID.String()+"/verify", ""), http.StatusAccepted, "")
	expect(t, ana.do(t, "POST", "/api/org/domains/"+d.ID.String()+"/verify", ""), http.StatusTooManyRequests, "too_many_attempts")

	// Delete.
	if w := ana.do(t, "DELETE", "/api/org/domains/"+sub.ID.String(), ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	expect(t, ana.do(t, "GET", "/api/org/domains/"+sub.ID.String(), ""), http.StatusNotFound, "not_found")
	expect(t, ana.do(t, "DELETE", "/api/org/domains/nope", ""), http.StatusNotFound, "not_found")

	// At most 20 per organization.
	for i := range maxDomainsPerOrg - 1 {
		addDomain(t, ana, fmt.Sprintf("site%d.example.org", i))
	}
	expect(t, ana.do(t, "POST", "/api/org/domains", `{"name":"one-too-many.org"}`), http.StatusConflict, "limit_reached")

	var actions string
	_ = a.owner.QueryRow(context.Background(), "SELECT string_agg(action, ',' ORDER BY id) FROM audit_log WHERE action LIKE 'domain.%' AND target LIKE '%fizsar.com'").Scan(&actions)
	if actions != "domain.added,domain.added,domain.check_requested,domain.check_requested,domain.deleted" {
		t.Errorf("audit: %s", actions)
	}
}

func TestDomainGracePeriod(t *testing.T) {
	now := time.Now()
	ago := func(d time.Duration) *time.Time { x := now.Add(-d); return &x }
	for _, tc := range []struct {
		d    store.Domain
		want bool
	}{
		{store.Domain{Status: store.DomainPending}, false},
		{store.Domain{Status: store.DomainVerified}, true},
		{store.Domain{Status: store.DomainLost, LostAt: ago(13 * 24 * time.Hour)}, true},
		{store.Domain{Status: store.DomainLost, LostAt: ago(15 * 24 * time.Hour)}, false},
		{store.Domain{Status: store.DomainLost}, false},
	} {
		if got := tc.d.Verified(now); got != tc.want {
			t.Errorf("%s lost %v: verified=%v", tc.d.Status, tc.d.LostAt, got)
		}
	}
}
