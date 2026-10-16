package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/altcha"
)

// doneScan stores a finished scan of host (a real lab result) that ended
// ago, public or of org, with the given intensity.
func (a *authAPI) doneScan(t *testing.T, host string, org *uuid.UUID, intensity string, ago time.Duration) uuid.UUID {
	t.Helper()
	sc, _ := labScan(t, "kyber", host)
	id := uuid.New()
	finished := time.Now().Add(-ago)
	if _, err := a.owner.Exec(context.Background(), `INSERT INTO scans (id, host, port, status, created_at, started_at, finished_at,
			result, scoring, scoring_version, grade, score, organization_id, intensity)
		VALUES ($1, $2, 443, 'done', $3, $3, $4, $5, $6, $6::jsonb->>'scoring_version', $6::jsonb->>'grade', ($6::jsonb->>'score')::int, $7, $8)`,
		id, host, finished.Add(-30*time.Second), finished, sc.Result, sc.Scoring, org, intensity); err != nil {
		t.Fatal(err)
	}
	return id
}

func viewOf(t *testing.T, w *httptestRecorder) scanView {
	t.Helper()
	var v scanView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	return v
}

func TestPublicReuseBeforeLimits(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{}, openPublic(PublicLimits{PerDomain: NewWindowLimiter(1, time.Hour)}),
		func(c *Config) { c.Limiter = NewLimiter(1, 1) })
	pub := func(ip, host string) *httptestRecorder {
		return a.client(ip).do(t, "POST", "/api/scans", js(map[string]string{"host": host}))
	}

	// A finished public result 30 minutes old is reused even with the IP's
	// and the domain's quotas used up, and says how old it is.
	expect(t, pub("198.51.100.1", "new.example.org"), http.StatusAccepted, "") // spends the IP's slot and example.org's
	old := a.doneScan(t, "www.example.org", nil, "basic", 30*time.Minute)
	w := pub("198.51.100.1", "www.example.org")
	expect(t, w, http.StatusOK, "")
	v := viewOf(t, w)
	if v.ID != old || !v.Reused || v.AgeSeconds == nil || *v.AgeSeconds < 1790 || *v.AgeSeconds > 1900 || v.Report == nil {
		t.Errorf("reused result: %s", w.Body)
	}
	expect(t, pub("198.51.100.2", "www.example.org"), http.StatusOK, "")                                  // another client, domain quota used up
	expect(t, pub("198.51.100.2", "shop.example.org"), http.StatusTooManyRequests, "domain_rate_limited") // nothing to reuse
	// The in-flight scan of a minute ago (2-minute cooldown) too, with the IP's quota used up.
	w = pub("198.51.100.1", "new.example.org")
	expect(t, w, http.StatusOK, "")
	if v := viewOf(t, w); v.Status != "queued" || !v.Reused {
		t.Errorf("in-flight reuse: %s", w.Body)
	}

	// Older than an hour: no reuse, and the quota applies.
	a.doneScan(t, "old.example.net", nil, "basic", 61*time.Minute)
	expect(t, pub("198.51.100.3", "x.example.net"), http.StatusAccepted, "") // spends example.net's slot
	expect(t, pub("198.51.100.4", "old.example.net"), http.StatusTooManyRequests, "domain_rate_limited")

	// Never an organization's scan, even basic and recent.
	ana, me := a.signedIn(t, "ana@example.com", "203.0.113.1")
	_ = ana
	orgScan := a.doneScan(t, "orgonly.example.com", me.ActiveOrgID, "basic", time.Minute)
	w = pub("198.51.100.5", "orgonly.example.com")
	expect(t, w, http.StatusAccepted, "")
	if v := viewOf(t, w); v.ID == orgScan || v.Reused || v.Report != nil {
		t.Errorf("an organization's scan was handed out: %s", w.Body)
	}

	// Excluded or opted out: the generic answer, never the stored result.
	a.doneScan(t, "excluded.example.com", nil, "basic", 5*time.Minute)
	if _, err := a.owner.Exec(context.Background(), "INSERT INTO scan_exclusions (name) VALUES ('excluded.example.com')"); err != nil {
		t.Fatal(err)
	}
	excluded := pub("198.51.100.6", "excluded.example.com")
	expect(t, excluded, http.StatusForbidden, "scan_not_allowed")
	d := addDomain(t, ana, "optedout.example.com")
	a.verify(t, d.ID, "verified", 0)
	expect(t, ana.do(t, "PATCH", "/api/org/domains/"+d.ID.String(), `{"block_public":true}`), http.StatusOK, "")
	a.doneScan(t, "optedout.example.com", nil, "basic", 5*time.Minute)
	optedOut := pub("198.51.100.7", "optedout.example.com")
	expect(t, optedOut, http.StatusForbidden, "scan_not_allowed")
	if optedOut.Body.String() != excluded.Body.String() || strings.Contains(optedOut.Body.String(), "report") {
		t.Errorf("blocked answers: %s / %s", excluded.Body, optedOut.Body)
	}
}

// Reusing still needs the proof of work.
func TestPublicReuseNeedsProof(t *testing.T) {
	v, _ := altcha.New([]byte(strings.Repeat("k", 32)), 50, time.Minute)
	a := newAuthAPI(t, AuthLimits{}, openPublic(PublicLimits{}), func(c *Config) { c.Altcha = v })
	a.doneScan(t, "www.example.org", nil, "basic", 10*time.Minute)
	c := a.client("198.51.100.9")
	expect(t, c.do(t, "POST", "/api/scans", `{"host":"www.example.org"}`), http.StatusBadRequest, "proof_failed")
	proof := solveAltcha(t, c.do(t, "GET", "/api/altcha", "").Body.Bytes())
	w := c.do(t, "POST", "/api/scans", js(map[string]string{"host": "www.example.org", "altcha": proof}))
	expect(t, w, http.StatusOK, "")
	if !viewOf(t, w).Reused {
		t.Errorf("not reused: %s", w.Body)
	}
}
