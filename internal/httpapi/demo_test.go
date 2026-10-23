package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/altcha"
	"github.com/fizsar/quantumready-saas/internal/store"
)

type fakeDemos struct {
	mu   sync.Mutex
	rows []store.InsertDemoRequestParams
}

func (f *fakeDemos) InsertDemoRequest(_ context.Context, p store.InsertDemoRequestParams) (store.InsertDemoRequestRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, p)
	return store.InsertDemoRequestRow{ID: uuid.New()}, nil
}

func (f *fakeDemos) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows)
}

func demoAPI(t *testing.T, cfg Config) (http.Handler, *fakeDemos) {
	t.Helper()
	d := &fakeDemos{}
	cfg.Demos = d
	cfg.DemoEnabled = true
	return NewRouter(cfg), d
}

// A valid request, as the form sends it.
func demoBody(change func(m map[string]any)) string {
	m := map[string]any{"name": " Ana García ", "email": "ana@example.org", "company": "Acme", "domain": "WWW.Example.ORG",
		"message": "Queremos ver la nota de nuestra web.", "privacy": true, "lang": "es", "website": ""}
	if change != nil {
		change(m)
	}
	b, _ := json.Marshal(m)
	return string(b)
}

type demoReq struct {
	body, ctype, fetchSite, ip string
}

func postDemo(h http.Handler, d demoReq) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/demo", strings.NewReader(d.body))
	r.Host = "quantumready.fizsar.com"
	r.Header.Set("Content-Type", "application/json")
	if d.ctype != "" {
		r.Header.Set("Content-Type", d.ctype)
	}
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if d.fetchSite != "" {
		r.Header.Set("Sec-Fetch-Site", d.fetchSite)
	}
	if d.ip != "" {
		r.RemoteAddr = d.ip + ":1234"
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestDemoStored(t *testing.T) {
	h, d := demoAPI(t, Config{})
	w := postDemo(h, demoReq{body: demoBody(nil)})
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"received"`) {
		t.Fatalf("valid request: %d %s", w.Code, w.Body)
	}
	want := store.InsertDemoRequestParams{Name: "Ana García", Email: "ana@example.org", Company: "Acme", Domain: "www.example.org",
		Message: "Queremos ver la nota de nuestra web.", Lang: "es", PrivacyVersion: PrivacyVersion}
	if d.count() != 1 || d.rows[0] != want {
		t.Errorf("stored %+v, want %+v", d.rows, want)
	}
}

// Every field is checked; nothing invalid is stored.
func TestDemoValidation(t *testing.T) {
	cases := map[string]struct {
		change func(map[string]any)
		code   string
	}{
		"privacy not accepted":   {func(m map[string]any) { m["privacy"] = false }, "privacy_required"},
		"privacy missing":        {func(m map[string]any) { delete(m, "privacy") }, "privacy_required"},
		"empty name":             {func(m map[string]any) { m["name"] = "  " }, "invalid_name"},
		"name of 101":            {func(m map[string]any) { m["name"] = strings.Repeat("a", 101) }, "invalid_name"},
		"name with a newline":    {func(m map[string]any) { m["name"] = "Ana\nBcc: x" }, "invalid_name"},
		"empty company":          {func(m map[string]any) { m["company"] = "" }, "invalid_company"},
		"company of 101":         {func(m map[string]any) { m["company"] = strings.Repeat("á", 101) }, "invalid_company"},
		"email without @":        {func(m map[string]any) { m["email"] = "ana.example.org" }, "invalid_email"},
		"email with a name":      {func(m map[string]any) { m["email"] = "Ana <ana@example.org>" }, "invalid_email"},
		"email without a dot":    {func(m map[string]any) { m["email"] = "ana@localhost" }, "invalid_email"},
		"email of 255":           {func(m map[string]any) { m["email"] = strings.Repeat("a", 243) + "@example.org" }, "invalid_email"},
		"domain is an IP":        {func(m map[string]any) { m["domain"] = "192.0.2.1" }, "invalid_domain"},
		"domain with a space":    {func(m map[string]any) { m["domain"] = "exa mple.org" }, "invalid_domain"},
		"domain empty":           {func(m map[string]any) { m["domain"] = "" }, "invalid_domain"},
		"message of 2001":        {func(m map[string]any) { m["message"] = strings.Repeat("é", 2001) }, "invalid_message"},
		"message with a NUL":     {func(m map[string]any) { m["message"] = "hola\x00" }, "invalid_message"},
		"unknown field":          {func(m map[string]any) { m["phone"] = "600000000" }, "invalid_json"},
		"privacy as text":        {func(m map[string]any) { m["privacy"] = "yes" }, "invalid_json"},
		"message of 2000 is ok":  {func(m map[string]any) { m["message"] = strings.Repeat("é", 2000) }, ""},
		"no message is ok":       {func(m map[string]any) { delete(m, "message") }, ""},
		"other language is ok":   {func(m map[string]any) { m["lang"] = "fr" }, ""},
		"name of 100 is ok":      {func(m map[string]any) { m["name"] = strings.Repeat("ñ", 100) }, ""},
		"bare domain is ok":      {func(m map[string]any) { m["domain"] = "example.co.uk" }, ""},
		"url-ish domain refused": {func(m map[string]any) { m["domain"] = "https://example.org/" }, "invalid_domain"},
	}
	for name, tc := range cases {
		h, d := demoAPI(t, Config{})
		w := postDemo(h, demoReq{body: demoBody(tc.change)})
		if tc.code == "" {
			if w.Code != http.StatusAccepted || d.count() != 1 {
				t.Errorf("%s: %d %s, stored %d", name, w.Code, w.Body, d.count())
			}
			continue
		}
		if w.Code != http.StatusBadRequest || errCode(w) != tc.code || d.count() != 0 {
			t.Errorf("%s: %d %s (want 400 %s), stored %d", name, w.Code, w.Body, tc.code, d.count())
		}
	}
	// Spanish answers by default, English when asked.
	h, _ := demoAPI(t, Config{})
	w := postDemo(h, demoReq{body: demoBody(func(m map[string]any) { m["privacy"] = false })})
	if !strings.Contains(w.Body.String(), "política de privacidad") {
		t.Errorf("message: %s", w.Body)
	}
}

// A bot that fills the hidden field gets the same answer as a person, and
// nothing is stored.
func TestDemoHoneypot(t *testing.T) {
	h, d := demoAPI(t, Config{})
	w := postDemo(h, demoReq{body: demoBody(func(m map[string]any) { m["website"] = "http://spam.example" })})
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"received"`) {
		t.Errorf("honeypot: %d %s, want the normal 202", w.Code, w.Body)
	}
	if d.count() != 0 {
		t.Error("honeypot request stored")
	}
	// Whitespace only is a person whose browser autofilled nothing.
	if w := postDemo(h, demoReq{body: demoBody(func(m map[string]any) { m["website"] = " " })}); w.Code != http.StatusAccepted || d.count() != 1 {
		t.Errorf("blank honeypot: %d, stored %d", w.Code, d.count())
	}
}

// Per client, in memory: the third request in a minute is refused with
// Retry-After; another client is not affected.
func TestDemoRateLimit(t *testing.T) {
	h, d := demoAPI(t, Config{DemoLimiter: NewLimiter(2, 10)})
	for i := range 2 {
		if w := postDemo(h, demoReq{body: demoBody(nil), ip: "198.51.100.7"}); w.Code != http.StatusAccepted {
			t.Fatalf("request %d: %d", i+1, w.Code)
		}
	}
	w := postDemo(h, demoReq{body: demoBody(nil), ip: "198.51.100.7"})
	if w.Code != http.StatusTooManyRequests || errCode(w) != "too_many_requests" {
		t.Errorf("third request: %d %s", w.Code, w.Body)
	}
	if s, err := strconv.Atoi(w.Header().Get("Retry-After")); err != nil || s < 1 {
		t.Errorf("Retry-After %q", w.Header().Get("Retry-After"))
	}
	if w := postDemo(h, demoReq{body: demoBody(nil), ip: "198.51.100.8"}); w.Code != http.StatusAccepted {
		t.Errorf("another client: %d", w.Code)
	}
	if d.count() != 3 {
		t.Errorf("stored %d, want 3", d.count())
	}
	// The default limit exists.
	h, _ = demoAPI(t, Config{})
	got := 0
	for range 20 {
		if postDemo(h, demoReq{body: demoBody(nil), ip: "203.0.113.9"}).Code == http.StatusTooManyRequests {
			got++
		}
	}
	if got != 17 {
		t.Errorf("default limit: %d of 20 refused, want 17 (3 per minute)", got)
	}
}

func TestDemoOriginTypeAndSize(t *testing.T) {
	h, d := demoAPI(t, Config{})
	for name, tc := range map[string]struct {
		req    demoReq
		status int
		code   string
	}{
		"cross-site":       {demoReq{body: demoBody(nil), fetchSite: "cross-site"}, http.StatusForbidden, "bad_origin"},
		"same-site":        {demoReq{body: demoBody(nil), fetchSite: "same-site"}, http.StatusForbidden, "bad_origin"},
		"form post":        {demoReq{body: "name=x", ctype: "application/x-www-form-urlencoded"}, http.StatusUnsupportedMediaType, "unsupported_media_type"},
		"too large":        {demoReq{body: demoBody(func(m map[string]any) { m["message"] = strings.Repeat("a", 9000) })}, http.StatusRequestEntityTooLarge, "body_too_large"},
		"not json":         {demoReq{body: "{"}, http.StatusBadRequest, "invalid_json"},
		"json with params": {demoReq{body: demoBody(nil), ctype: "application/json; charset=utf-8"}, http.StatusAccepted, ""},
	} {
		w := postDemo(h, tc.req)
		if w.Code != tc.status || (tc.code != "" && errCode(w) != tc.code) {
			t.Errorf("%s: %d %s, want %d %s", name, w.Code, w.Body, tc.status, tc.code)
		}
	}
	if d.count() != 1 {
		t.Errorf("stored %d, want only the valid one", d.count())
	}
}

func solveChallenge(t *testing.T, c altcha.Challenge) string {
	t.Helper()
	for n := int64(0); n <= c.MaxNumber; n++ {
		sum := sha256.Sum256([]byte(c.Salt + strconv.FormatInt(n, 10)))
		if hex.EncodeToString(sum[:]) == c.Challenge {
			b, _ := json.Marshal(map[string]any{"algorithm": c.Algorithm, "challenge": c.Challenge, "number": n, "salt": c.Salt, "signature": c.Signature})
			return base64.StdEncoding.EncodeToString(b)
		}
	}
	t.Fatal("no solution")
	return ""
}

// With ALTCHA_HMAC_KEY: a solved challenge is required, once.
func TestDemoProofOfWork(t *testing.T) {
	v, err := altcha.New([]byte(strings.Repeat("k", 32)), 1000, 0)
	if err != nil {
		t.Fatal(err)
	}
	h, d := demoAPI(t, Config{DemoAltcha: v})
	if w := postDemo(h, demoReq{body: demoBody(nil)}); w.Code != http.StatusBadRequest || errCode(w) != "invalid_proof" {
		t.Errorf("without proof: %d %s", w.Code, w.Body)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/demo/challenge", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var c altcha.Challenge
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &c) != nil || c.Challenge == "" {
		t.Fatalf("challenge: %d %s", w.Code, w.Body)
	}
	proof := solveChallenge(t, c)
	body := demoBody(func(m map[string]any) { m["altcha"] = proof })
	if w := postDemo(h, demoReq{body: body}); w.Code != http.StatusAccepted || d.count() != 1 {
		t.Errorf("with proof: %d %s", w.Code, w.Body)
	}
	if w := postDemo(h, demoReq{body: body}); w.Code != http.StatusBadRequest || errCode(w) != "invalid_proof" {
		t.Errorf("the same proof twice: %d %s", w.Code, w.Body)
	}
	// Without the key there is no challenge endpoint (and no proof needed).
	h, _ = demoAPI(t, Config{})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/demo/challenge", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("challenge without a key: %d", w.Code)
	}
}

// The M1 event of the form: only where the button was, from a closed list.
// usageDemoAPI: the M1 counters with the demo form on or off.
func usageDemoAPI(t *testing.T, on bool) (http.Handler, *fakeUsage) {
	t.Helper()
	allow, err := ParseAllowlist("fizsar.com quantumready.fizsar.com")
	if err != nil {
		t.Fatal(err)
	}
	u := &fakeUsage{}
	st := &fixtureStore{scans: map[uuid.UUID]store.Scan{}, findings: map[uuid.UUID][]store.Finding{}}
	return NewRouter(Config{Usage: u, Scans: st, Allowlist: allow, Demos: &fakeDemos{}, DemoEnabled: on}), u
}

func TestDemoRequestedEvent(t *testing.T) {
	h, u := usageDemoAPI(t, true)
	for _, body := range []string{
		`{"e":"demo_requested","d":"home"}`,
		`{"e":"demo_requested","d":"host_not_allowed"}`,
		`{"e":"demo_requested","d":"examples"}`,
		`{"e":"demo_requested","d":"direct"}`,
		`{"e":"demo_requested","d":"ana@example.org"}`,
		`{"e":"demo_requested","d":"acme.com"}`,
		`{"e":"demo_requested","d":""}`,
		`{"e":"demo_requested","d":"home","p":"/"}`,
		`{"e":"page_view","p":"/demo"}`,
	} {
		postEvent(h, evReq{body: body, origin: ownOrigin})
	}
	want := "demo_requested/home demo_requested/host_not_allowed demo_requested/examples demo_requested/direct page_view//demo"
	if got := strings.Join(u.counts, " "); got != want {
		t.Errorf("counted %q, want %q", got, want)
	}
}

// DEMO_ENABLED off (the default): no form endpoints, the web is told so, and
// neither the event nor /demo views are counted. On: all of it.
func TestDemoDisabled(t *testing.T) {
	v, err := altcha.New([]byte(strings.Repeat("k", 32)), 1000, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, on := range []bool{false, true} {
		d := &fakeDemos{}
		allow, _ := ParseAllowlist("fizsar.com")
		st := &fixtureStore{scans: map[uuid.UUID]store.Scan{}, findings: map[uuid.UUID][]store.Finding{}}
		h := NewRouter(Config{Scans: st, Allowlist: allow, Demos: d, DemoAltcha: v, DemoEnabled: on})
		want := map[bool]int{false: http.StatusNotFound, true: http.StatusBadRequest}[on] // on: no proof given
		if w := postDemo(h, demoReq{body: demoBody(nil)}); w.Code != want {
			t.Errorf("enabled=%v: POST /api/demo %d, want %d", on, w.Code, want)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/demo/challenge", nil))
		if want := map[bool]int{false: http.StatusNotFound, true: http.StatusOK}[on]; w.Code != want {
			t.Errorf("enabled=%v: GET /api/demo/challenge %d, want %d", on, w.Code, want)
		}
		if d.count() != 0 {
			t.Errorf("enabled=%v: stored %d", on, d.count())
		}
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/targets", nil))
		var targets struct{ Demo *bool }
		if err := json.Unmarshal(w.Body.Bytes(), &targets); err != nil || targets.Demo == nil || *targets.Demo != on {
			t.Errorf("enabled=%v: GET /api/targets %s", on, w.Body)
		}

		hu, u := usageDemoAPI(t, on)
		postEvent(hu, evReq{body: `{"e":"demo_requested","d":"home"}`, origin: ownOrigin})
		postEvent(hu, evReq{body: `{"e":"page_view","p":"/demo"}`, origin: ownOrigin})
		postEvent(hu, evReq{body: `{"e":"page_view","p":"/"}`, origin: ownOrigin})
		want2 := map[bool]string{false: "page_view//", true: "demo_requested/home page_view//demo page_view//"}[on]
		if got := strings.Join(u.counts, " "); got != want2 {
			t.Errorf("enabled=%v: counted %q, want %q", on, got, want2)
		}
	}
	// Without a store there is nothing to turn on.
	allow, _ := ParseAllowlist("fizsar.com")
	h := NewRouter(Config{Scans: &fixtureStore{scans: map[uuid.UUID]store.Scan{}, findings: map[uuid.UUID][]store.Finding{}}, Allowlist: allow, DemoEnabled: true})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/targets", nil))
	if !strings.Contains(w.Body.String(), `"demo":false`) {
		t.Errorf("no store: %s", w.Body)
	}
}
