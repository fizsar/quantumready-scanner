package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/altcha"
)

func openPublic(limits PublicLimits) func(*Config) {
	return func(c *Config) {
		c.PublicScanOpen = true
		c.Limiter = NewLimiter(100, 1000)
		c.PublicLimits = limits
	}
}

// shape removes what differs between two scans of different hosts (ids,
// times, the host itself) to compare the rest of two answers.
var volatile = regexp.MustCompile(`"(id|created_at|host)":"[^"]*"`)

func shape(body string) string { return volatile.ReplaceAllString(body, `"$1":"…"`) }

func TestOpenPublicScans(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100)}, openPublic(PublicLimits{}))
	anon := a.client("198.51.100.7")
	pub := func(host string) *httptestRecorder {
		return anon.do(t, "POST", "/api/scans", js(map[string]string{"host": host}))
	}

	w := anon.do(t, "GET", "/api/targets", "")
	if !strings.Contains(w.Body.String(), `"open":true`) {
		t.Errorf("targets: %s", w.Body)
	}
	w = pub("https://Example.org/path")
	expect(t, w, http.StatusAccepted, "")
	var v scanView
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if v.Host != "example.org" || v.Intensity != "basic" || w.Header().Get("Location") != "/api/scans/"+v.ID.String() {
		t.Fatalf("public scan: %s", w.Body)
	}
	expect(t, pub("example.org"), http.StatusOK, "") // reused within the cooldown
	for host, code := range map[string]string{
		"192.0.2.10": "domain_ip", "co.uk": "domain_public_suffix", "printer.local": "domain_reserved",
		"x.notatld": "domain_unknown_tld", "example.org:8443": "port_not_allowed",
	} {
		expect(t, pub(host), http.StatusBadRequest, code)
	}

	// Exclusion list and the verified owner's opt-out: the same answer.
	ctx := context.Background()
	if _, err := a.owner.Exec(ctx, "INSERT INTO scan_exclusions (name, reason) VALUES ('excluded.example.net', 'asked by mail')"); err != nil {
		t.Fatal(err)
	}
	excluded := pub("www.excluded.example.net") // subdomains too
	expect(t, excluded, http.StatusForbidden, "scan_not_allowed")

	ana, _ := a.signedIn(t, "ana@example.com", "203.0.113.1")
	opt := addDomain(t, ana, "optout.example.com")
	expect(t, ana.do(t, "PATCH", "/api/org/domains/"+opt.ID.String(), `{"block_public":true}`), http.StatusOK, "")
	expect(t, pub("optout.example.com"), http.StatusAccepted, "") // not verified yet: the opt-out does not apply
	a.verify(t, opt.ID, "verified", 0)
	optedOut := pub("shop.optout.example.com")
	expect(t, optedOut, http.StatusForbidden, "scan_not_allowed")
	if optedOut.Body.String() != excluded.Body.String() {
		t.Errorf("opt-out and exclusion answers differ:\n%s\n%s", optedOut.Body, excluded.Body)
	}
	a.verify(t, opt.ID, "lost", 15) // verification lost after the grace: the opt-out lapses
	expect(t, pub("optout.example.com"), http.StatusOK, "")
	expect(t, ana.do(t, "PATCH", "/api/org/domains/"+opt.ID.String(), `{}`), http.StatusBadRequest, "invalid_json")

	// A domain verified by an organization, with a recent full scan, looks
	// exactly like any other domain to a public request.
	ver := addDomain(t, ana, "verified.example.com")
	a.verify(t, ver.ID, "verified", 0)
	w = ana.do(t, "POST", "/api/org/scans", `{"host":"verified.example.com"}`)
	expect(t, w, http.StatusAccepted, "")
	var full scanView
	_ = json.Unmarshal(w.Body.Bytes(), &full)
	verified, plain := pub("verified.example.com"), pub("plain.example.com")
	if verified.Code != http.StatusAccepted || plain.Code != http.StatusAccepted || shape(verified.Body.String()) != shape(plain.Body.String()) {
		t.Errorf("a verified domain is distinguishable:\n%d %s\n%d %s", verified.Code, verified.Body, plain.Code, plain.Body)
	}
	var pv scanView
	_ = json.Unmarshal(verified.Body.Bytes(), &pv)
	if pv.ID == full.ID || pv.Intensity != "basic" || pv.Reused {
		t.Errorf("the public request got the organization's scan: %s", verified.Body)
	}
	expect(t, anon.do(t, "GET", "/api/scans/"+full.ID.String(), ""), http.StatusNotFound, "not_found")
	var orgID *string
	_ = a.owner.QueryRow(ctx, "SELECT organization_id::text FROM scans WHERE id = $1", pv.ID).Scan(&orgID)
	if orgID != nil {
		t.Error("the public scan belongs to an organization")
	}

	// Viewers cannot opt out.
	carl, _ := a.signedIn(t, "carl@example.com", "203.0.113.3")
	var orgA uuid.UUID
	if err := a.owner.QueryRow(ctx, "SELECT organization_id FROM domains WHERE id = $1", ver.ID).Scan(&orgA); err != nil {
		t.Fatal(err)
	}
	a.join(t, "carl@example.com", orgA, "viewer")
	expect(t, carl.do(t, "PATCH", "/api/org/domains/"+ver.ID.String(), `{"block_public":true}`), http.StatusForbidden, "forbidden")
}

// solveAltcha does what the browser does with GET /api/altcha.
func solveAltcha(t *testing.T, body []byte) string {
	t.Helper()
	var c altcha.Challenge
	if err := json.Unmarshal(body, &c); err != nil {
		t.Fatal(err)
	}
	for n := int64(0); n <= c.MaxNumber; n++ {
		h := sha256.Sum256([]byte(c.Salt + strconv.FormatInt(n, 10)))
		if hex.EncodeToString(h[:]) == c.Challenge {
			b, _ := json.Marshal(map[string]any{"algorithm": c.Algorithm, "challenge": c.Challenge, "number": n, "salt": c.Salt, "signature": c.Signature})
			return base64.StdEncoding.EncodeToString(b)
		}
	}
	t.Fatal("unsolvable challenge")
	return ""
}

func TestPublicScanProofOfWork(t *testing.T) {
	v, _ := altcha.New([]byte(strings.Repeat("k", 32)), 300, time.Minute)
	a := newAuthAPI(t, AuthLimits{}, openPublic(PublicLimits{Challenge: NewLimiter(3, 100)}), func(c *Config) { c.Altcha = v })
	c := a.client("198.51.100.9")
	if w := c.do(t, "GET", "/api/targets", ""); !strings.Contains(w.Body.String(), `"proof":true`) {
		t.Errorf("targets: %s", w.Body)
	}
	scan := func(host, proof string) *httptestRecorder {
		return c.do(t, "POST", "/api/scans", js(map[string]string{"host": host, "altcha": proof}))
	}
	expect(t, scan("example.org", ""), http.StatusBadRequest, "proof_failed")
	w := c.do(t, "GET", "/api/altcha", "")
	expect(t, w, http.StatusOK, "")
	proof := solveAltcha(t, w.Body.Bytes())
	expect(t, scan("example.org", proof), http.StatusAccepted, "")
	expect(t, scan("example.net", proof), http.StatusBadRequest, "proof_failed") // one scan per solution
	// Demo (allowlisted) hosts need it too once scanning is open.
	expect(t, scan("fizsar.com", "x"), http.StatusBadRequest, "proof_failed")
	// Challenges are rate limited per client.
	c.do(t, "GET", "/api/altcha", "")
	c.do(t, "GET", "/api/altcha", "")
	expect(t, c.do(t, "GET", "/api/altcha", ""), http.StatusTooManyRequests, "too_many_attempts")
}

func TestPublicScanLimits(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{}, openPublic(PublicLimits{PerDomain: NewWindowLimiter(2, time.Hour), Global: NewLimiter(3, 100)}))
	c := a.client("198.51.100.8")
	pub := func(host string) *httptestRecorder {
		return c.do(t, "POST", "/api/scans", js(map[string]string{"host": host}))
	}
	expect(t, pub("a.example.org"), http.StatusAccepted, "")
	expect(t, pub("a.example.org"), http.StatusOK, "") // reused: does not count
	expect(t, pub("b.example.org"), http.StatusAccepted, "")
	w := pub("c.example.org") // third new scan of example.org
	expect(t, w, http.StatusTooManyRequests, "domain_rate_limited")
	if w.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After")
	}
	expect(t, pub("other.example.net"), http.StatusAccepted, "") // the refused one gave its global slot back
	expect(t, pub("third.example.com"), http.StatusTooManyRequests, "busy")
}
