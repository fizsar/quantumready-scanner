package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
var (
	volatile    = regexp.MustCompile(`"(id|created_at|host)":"[^"]*"`)
	volatileNum = regexp.MustCompile(`"(queue_position|queue_eta_seconds)":\d+`)
)

func shape(body string) string {
	return volatileNum.ReplaceAllString(volatile.ReplaceAllString(body, `"$1":"…"`), `"$1":0`)
}

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
	block := func(on bool) *httptestRecorder {
		return ana.do(t, "PATCH", "/api/org/domains/"+opt.ID.String(), js(map[string]bool{"block_public": on}))
	}
	// Nobody blocks a domain they have not proved they control.
	expect(t, block(true), http.StatusConflict, "optout_needs_verification")
	expect(t, pub("optout.example.com"), http.StatusAccepted, "")
	a.verify(t, opt.ID, "verified", 0)
	expect(t, block(true), http.StatusOK, "")
	optedOut := pub("shop.optout.example.com") // subdomains too, like the operator's exclusions
	expect(t, optedOut, http.StatusForbidden, "scan_not_allowed")
	if optedOut.Body.String() != excluded.Body.String() {
		t.Errorf("opt-out and exclusion answers differ:\n%s\n%s", optedOut.Body, excluded.Body)
	}
	// The block stays in force when the verification lapses, and a lapsed
	// owner cannot lift it; only verifying again (or the operator) does.
	a.verify(t, opt.ID, "lost", 15)
	expect(t, pub("optout.example.com"), http.StatusForbidden, "scan_not_allowed")
	if _, err := a.owner.Exec(ctx, "UPDATE domains SET status = 'pending', lost_at = NULL, verified_at = NULL WHERE id = $1", opt.ID); err != nil {
		t.Fatal(err)
	}
	expect(t, pub("optout.example.com"), http.StatusForbidden, "scan_not_allowed")
	expect(t, block(false), http.StatusConflict, "optout_needs_verification")
	a.verify(t, opt.ID, "verified", 0)
	expect(t, block(false), http.StatusOK, "")
	expect(t, pub("optout.example.com"), http.StatusOK, "") // the scan from before, reused
	expect(t, ana.do(t, "PATCH", "/api/org/domains/"+opt.ID.String(), `{}`), http.StatusBadRequest, "invalid_json")

	// Removing a blocked domain from the organization keeps the block, as an
	// operator exclusion.
	kept := addDomain(t, ana, "kept.example.com")
	a.verify(t, kept.ID, "verified", 0)
	expect(t, ana.do(t, "PATCH", "/api/org/domains/"+kept.ID.String(), `{"block_public":true}`), http.StatusOK, "")
	if w := ana.do(t, "DELETE", "/api/org/domains/"+kept.ID.String(), ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	expect(t, pub("www.kept.example.com"), http.StatusForbidden, "scan_not_allowed")
	var reason string
	_ = a.owner.QueryRow(ctx, "SELECT reason FROM scan_exclusions WHERE name = 'kept.example.com'").Scan(&reason)
	if !strings.Contains(reason, "owner opt-out kept") {
		t.Errorf("exclusion reason: %q", reason)
	}

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

// Every per-IP limit (open public scans, ALTCHA challenges, login) keys on
// the real client: an invented X-Forwarded-For is ignored unless the
// connection comes from the Caddy network, and then only the address Caddy
// appended (the rightmost untrusted one) counts.
func TestForgedForwardedFor(t *testing.T) {
	v, _ := altcha.New([]byte(strings.Repeat("k", 32)), 10, time.Minute)
	trusted, _ := ParsePrefixes("172.18.0.0/16")
	a := newAuthAPI(t, AuthLimits{LoginIP: NewLimiter(2, 100)},
		openPublic(PublicLimits{Challenge: NewLimiter(2, 100)}),
		func(c *Config) { c.Altcha = v; c.Limiter = NewLimiter(2, 100); c.TrustedProxies = trusted })
	forged := 0
	req := func(method, path, body, remote, xff string) *httptestRecorder {
		c := a.client("x")
		c.remote = remote
		return c.do(t, method, path, body, func(r *http.Request) {
			if xff == "" {
				forged++
				xff = fmt.Sprintf("198.51.100.%d", forged) // a new invented address every time
			}
			r.Header.Set("X-Forwarded-For", xff)
		})
	}
	for _, ep := range []struct{ method, path, body string }{
		{"GET", "/api/altcha", ""},
		{"POST", "/api/auth/login", `{"email":"x@example.com","password":"wrong-password-12"}`},
	} {
		// A direct client (not the Caddy network) cannot dodge its limit.
		for range 2 {
			req(ep.method, ep.path, ep.body, "203.0.113.50:1000", "")
		}
		expect(t, req(ep.method, ep.path, ep.body, "203.0.113.50:1001", ""), http.StatusTooManyRequests, "too_many_attempts")
		// Through Caddy: a client-supplied prefix is ignored; Caddy's entry counts.
		for range 2 {
			req(ep.method, ep.path, ep.body, "172.18.0.2:2000", "10.9.9.9, 203.0.113.60")
		}
		expect(t, req(ep.method, ep.path, ep.body, "172.18.0.2:2001", "1.2.3.4, 203.0.113.60"), http.StatusTooManyRequests, "too_many_attempts")
	}
	// Open public scans (without the proof of work, to reach the limiter).
	b := newAuthAPI(t, AuthLimits{}, openPublic(PublicLimits{}),
		func(c *Config) { c.Limiter = NewLimiter(2, 100); c.TrustedProxies = trusted })
	scan := func(host, remote, xff string) *httptestRecorder {
		c := b.client("x")
		c.remote = remote
		return c.do(t, "POST", "/api/scans", js(map[string]string{"host": host}), func(r *http.Request) { r.Header.Set("X-Forwarded-For", xff) })
	}
	expect(t, scan("a.example.org", "203.0.113.70:1", "198.51.100.201"), http.StatusAccepted, "")
	expect(t, scan("b.example.org", "203.0.113.70:2", "198.51.100.202"), http.StatusAccepted, "")
	expect(t, scan("c.example.org", "203.0.113.70:3", "198.51.100.203"), http.StatusTooManyRequests, "rate_limited")
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
