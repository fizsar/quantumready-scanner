package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/fizsar/quantumready-saas/internal/store"
)

// Apertura (decision of 01/11/2026): a public scan is full for the operator's
// own domains (SCAN_FULL_DOMAINS) and basic for any other; reuse never hands
// a basic scan to a full request or the other way round.
func TestFullScanDomains(t *testing.T) {
	own, err := ParseAllowlist("fizsar-own.org")
	if err != nil {
		t.Fatal(err)
	}
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100)}, openPublic(PublicLimits{}), func(c *Config) { c.FullScanHosts = own })
	anon := a.client("198.51.100.9")
	scan := func(host string, code int) scanView {
		t.Helper()
		w := anon.do(t, "POST", "/api/scans", js(map[string]string{"host": host}))
		expect(t, w, code, "")
		var v scanView
		_ = json.Unmarshal(w.Body.Bytes(), &v)
		return v
	}

	// A public basic scan of the own domain in flight (from before it was on
	// the list): a full request must not get it.
	basic, err := store.NewStore(a.owner).Enqueue(context.Background(), "fizsar-own.org", 443)
	if err != nil {
		t.Fatal(err)
	}
	full := scan("fizsar-own.org", http.StatusAccepted)
	if full.Intensity != "full" || full.ID == basic.ID {
		t.Errorf("own domain: %s intensity %q, want a new full scan (not the basic %s)", full.ID, full.Intensity, basic.ID)
	}
	if again := scan("fizsar-own.org", http.StatusOK); again.ID != full.ID || again.Intensity != "full" {
		t.Errorf("own domain again: %s %q, want the same full scan reused", again.ID, again.Intensity)
	}
	// Exact match, as SCAN_ALLOWLIST: a subdomain of an own domain is any domain.
	for _, host := range []string{"other-site.org", "www.fizsar-own.org"} {
		if v := scan(host, http.StatusAccepted); v.Intensity != "basic" {
			t.Errorf("%s: intensity %q, want basic", host, v.Intensity)
		}
	}
}

// Without the list (unset SCAN_FULL_DOMAINS), every public scan is basic.
func TestNoFullScanDomains(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100)}, openPublic(PublicLimits{}))
	w := a.client("198.51.100.10").do(t, "POST", "/api/scans", js(map[string]string{"host": "fizsar-own.org"}))
	expect(t, w, http.StatusAccepted, "")
	var v scanView
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if v.Intensity != "basic" {
		t.Errorf("intensity %q, want basic", v.Intensity)
	}
}
