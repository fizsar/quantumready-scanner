package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fizsar/quantumready-saas/internal/auth"
	"github.com/fizsar/quantumready-saas/internal/store"
	"github.com/fizsar/quantumready-saas/internal/testdb"
	"github.com/fizsar/quantumready-saas/migrations"
)

const (
	testOrigin = "https://q.example"
	goodPass   = "tranvia-azul-1987-luna"
	otherPass  = "faro-de-niebla-2031-sur"
)

type authAPI struct {
	h     http.Handler
	owner *pgxpool.Pool // test owner: reads what the API wrote
}

func newAuthAPI(t *testing.T, limits AuthLimits) *authAPI {
	t.Helper()
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	// The API runs as qr_app, as in production.
	app := store.NewStore(testdb.AsRole(t, pool, store.RoleApp))
	return &authAPI{owner: pool, h: NewRouter(Config{
		DB: fakePinger{}, Accounts: app, Orgs: app, PublicOrigin: testOrigin, AuthLimits: limits,
		Async:  func(f func()) { f() },
		Logger: slog.New(slog.NewTextHandler(testLog{t}, nil)),
	})}
}

// testLog shows the API's error logs in the test output.
type testLog struct{ t *testing.T }

func (l testLog) Write(p []byte) (int, error) {
	l.t.Helper()
	l.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}

// code creates a beta signup code (as qradmin would).
func (a *authAPI) code(t *testing.T) string {
	t.Helper()
	tok, hash, _ := auth.NewToken()
	if _, err := store.NewStore(a.owner).CreateSignupCode(context.Background(), hash, nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	return tok
}

type sentEmail struct{ to, template, body string }

func (a *authAPI) outbox(t *testing.T) []sentEmail {
	t.Helper()
	rows, err := a.owner.Query(context.Background(), "SELECT to_email, template, body_text FROM email_outbox ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []sentEmail
	for rows.Next() {
		var e sentEmail
		if err := rows.Scan(&e.to, &e.template, &e.body); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	return out
}

// lastToken is the token of the newest email of template to whom.
func (a *authAPI) lastToken(t *testing.T, to, template string) string {
	t.Helper()
	re := regexp.MustCompile(regexp.QuoteMeta(testOrigin) + `/(verify|reset|invite)#token=([A-Za-z0-9_-]+)`)
	tok := ""
	for _, e := range a.outbox(t) {
		if e.to == to && e.template == template {
			if m := re.FindStringSubmatch(e.body); m != nil {
				tok = m[2]
			}
		}
	}
	if tok == "" {
		t.Fatalf("no %s email with a link to %s", template, to)
	}
	return tok
}

func (a *authAPI) count(t *testing.T, to, template string) int {
	n := 0
	for _, e := range a.outbox(t) {
		if e.to == to && e.template == template {
			n++
		}
	}
	return n
}

// client is one browser: its address, session cookie and CSRF token.
type client struct {
	a      *authAPI
	remote string
	cookie string
	csrf   string
}

func (a *authAPI) client(ip string) *client { return &client{a: a, remote: ip + ":40000"} }

func (c *client) do(t *testing.T, method, path, body string, edit ...func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = c.remote
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("Origin", testOrigin)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if c.cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: c.cookie})
	}
	if c.csrf != "" {
		r.Header.Set(csrfHeader, c.csrf)
	}
	for _, e := range edit {
		e(r)
	}
	w := httptest.NewRecorder()
	c.a.h.ServeHTTP(w, r)
	for _, ck := range w.Result().Cookies() {
		if ck.Name == sessionCookie {
			c.cookie = ck.Value
			if ck.MaxAge < 0 {
				c.cookie, c.csrf = "", ""
			}
		}
	}
	return w
}

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func errCode(w *httptest.ResponseRecorder) string {
	var b struct {
		Error apiError `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	return b.Error.Code
}

func expect(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status || errCode(w) != code {
		t.Fatalf("got %d %s, want %d %q", w.Code, w.Body.String(), status, code)
	}
}

func (c *client) register(t *testing.T, email, password, code string) *httptest.ResponseRecorder {
	t.Helper()
	return c.do(t, "POST", "/api/auth/register", js(map[string]string{"email": email, "password": password, "signup_code": code}))
}

func (c *client) login(t *testing.T, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	w := c.do(t, "POST", "/api/auth/login", js(map[string]string{"email": email, "password": password}))
	if w.Code == http.StatusOK {
		var me meView
		_ = json.Unmarshal(w.Body.Bytes(), &me)
		c.csrf = me.CSRFToken
	}
	return w
}

// verifiedUser registers and verifies email with goodPass.
func (a *authAPI) verifiedUser(t *testing.T, email string) {
	t.Helper()
	c := a.client("198.51.100.200")
	expect(t, c.register(t, email, goodPass, a.code(t)), http.StatusAccepted, "")
	expect(t, c.do(t, "POST", "/api/auth/verify-email", js(map[string]string{"token": a.lastToken(t, email, "verify_email")})), http.StatusOK, "")
}

func TestAccountLifecycle(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{})
	c := a.client("203.0.113.10")

	expect(t, c.register(t, "Ana@Example.com", goodPass, a.code(t)), http.StatusAccepted, "")
	if got := a.outbox(t); len(got) != 1 || got[0].to != "ana@example.com" || got[0].template != "verify_email" ||
		!strings.Contains(got[0].body, testOrigin+"/verify#token=") {
		t.Fatalf("outbox: %+v", got)
	}
	expect(t, c.login(t, "ana@example.com", "wrong-password-123"), http.StatusUnauthorized, "invalid_credentials")
	expect(t, c.login(t, "ana@example.com", goodPass), http.StatusForbidden, "email_not_verified")

	tok := a.lastToken(t, "ana@example.com", "verify_email")
	expect(t, c.do(t, "POST", "/api/auth/verify-email", js(map[string]string{"token": tok})), http.StatusOK, "")
	expect(t, c.do(t, "POST", "/api/auth/verify-email", js(map[string]string{"token": tok})), http.StatusBadRequest, "invalid_token")

	w := c.login(t, "ANA@example.com ", goodPass)
	expect(t, w, http.StatusOK, "")
	sc := w.Header().Get("Set-Cookie")
	for _, want := range []string{sessionCookie + "=", "Path=/", "Max-Age=2592000", "HttpOnly", "Secure", "SameSite=Lax"} {
		if !strings.Contains(sc, want) {
			t.Errorf("Set-Cookie %q lacks %q", sc, want)
		}
	}
	var me meView
	_ = json.Unmarshal(w.Body.Bytes(), &me)
	if me.User.Email != "ana@example.com" || len(me.Organizations) != 1 || me.Organizations[0].Role != "owner" ||
		me.ActiveOrgID == nil || *me.ActiveOrgID != me.Organizations[0].ID || me.CSRFToken == "" {
		t.Fatalf("login view: %s", w.Body)
	}
	// Only the token's hash is stored.
	var n int
	_ = a.owner.QueryRow(context.Background(), "SELECT count(*) FROM sessions WHERE token_hash = $1", auth.HashToken(c.cookie)).Scan(&n)
	if n != 1 {
		t.Errorf("session rows with the cookie's hash: %d", n)
	}
	_ = a.owner.QueryRow(context.Background(), "SELECT count(*) FROM sessions WHERE convert_to($1, 'UTF8') IN (token_hash, csrf_token::bytea)", c.cookie).Scan(&n)
	if n != 0 {
		t.Error("the raw session token is stored")
	}

	expect(t, c.do(t, "GET", "/api/auth/me", ""), http.StatusOK, "")
	csrf := c.csrf
	c.csrf = ""
	expect(t, c.do(t, "POST", "/api/auth/logout", ""), http.StatusForbidden, "csrf_failed")
	c.csrf = csrf[:len(csrf)-1] + "!" // never in base64url
	expect(t, c.do(t, "POST", "/api/auth/logout", ""), http.StatusForbidden, "csrf_failed")
	c.csrf = csrf
	old := c.cookie
	if w := c.do(t, "POST", "/api/auth/logout", ""); w.Code != http.StatusNoContent || !strings.Contains(w.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("logout: %d %v", w.Code, w.Header())
	}
	c.cookie = old // a copied cookie is dead too
	expect(t, c.do(t, "GET", "/api/auth/me", ""), http.StatusUnauthorized, "unauthenticated")

	rows, err := a.owner.Query(context.Background(), "SELECT action, coalesce(ip_prefix, ''), organization_id IS NOT NULL FROM audit_log ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var action, ip string
		var org bool
		_ = rows.Scan(&action, &ip, &org)
		if ip != "203.0.113.0/24" {
			t.Errorf("%s: ip_prefix %q", action, ip)
		}
		if org != (action == auditRegister) {
			t.Errorf("%s: organization set = %v", action, org)
		}
		got = append(got, action)
	}
	want := []string{auditRegister, auditLoginFailed, auditVerify, auditLogin, auditLogout}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("audit = %v, want %v", got, want)
	}
}

func TestRegisterValidation(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100)})
	c := a.client("203.0.113.11")
	code := a.code(t)
	cases := []struct {
		body map[string]string
		want string
	}{
		{map[string]string{"email": "not-an-email", "password": goodPass, "signup_code": code}, "invalid_email"},
		{map[string]string{"email": "Ana <ana@example.com>", "password": goodPass, "signup_code": code}, "invalid_email"},
		{map[string]string{"email": "ana@localhost", "password": goodPass, "signup_code": code}, "invalid_email"},
		{map[string]string{"email": "ana@example.com", "password": "short-pass", "signup_code": code}, "password_too_short"},
		{map[string]string{"email": "ana@example.com", "password": strings.Repeat("x", 129), "signup_code": code}, "password_too_long"},
		{map[string]string{"email": "ana@example.com", "password": "password1234", "signup_code": code}, "password_common"},
		{map[string]string{"email": "martina@example.com", "password": "MARTINA-2026-azul", "signup_code": code}, "password_contains_email"},
		{map[string]string{"email": "ana@example.com", "password": goodPass, "signup_code": ""}, "invalid_signup_code"},
		{map[string]string{"email": "ana@example.com", "password": goodPass, "signup_code": "made-up"}, "invalid_signup_code"},
		{map[string]string{"email": "ana@example.com", "password": goodPass, "signup_code": code, "name": strings.Repeat("n", 101)}, "invalid_name"},
		{map[string]string{"email": "ana@example.com", "password": goodPass, "signup_code": code, "organization": "Acme\nInc"}, "invalid_organization"},
	}
	for _, tc := range cases {
		expect(t, c.do(t, "POST", "/api/auth/register", js(tc.body)), http.StatusBadRequest, tc.want)
	}
	expect(t, c.do(t, "POST", "/api/auth/register", `{"email":"a@b.example","extra":1}`), http.StatusBadRequest, "invalid_json")
	expect(t, c.do(t, "POST", "/api/auth/register", `{}`, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }),
		http.StatusUnsupportedMediaType, "unsupported_media_type")
	if n := len(a.outbox(t)); n != 0 {
		t.Errorf("%d emails queued by refused registrations", n)
	}

	// A code works once.
	expect(t, c.register(t, "ana@example.com", goodPass, code), http.StatusAccepted, "")
	expect(t, c.register(t, "bob@example.com", goodPass, code), http.StatusBadRequest, "invalid_signup_code")
}

// The API never tells whether an email has an account (docs/security.md).
func TestNoAccountEnumeration(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100), LoginIP: NewLimiter(100, 100)})
	a.verifiedUser(t, "ana@example.com")
	c := a.client("203.0.113.12")

	same := func(what string, x, y *httptest.ResponseRecorder) {
		t.Helper()
		if x.Code != y.Code || x.Body.String() != y.Body.String() {
			t.Errorf("%s differs: %d %s / %d %s", what, x.Code, x.Body, y.Code, y.Body)
		}
	}

	// Register: a taken email answers like a new one; its owner gets an email
	// and the code is not spent.
	code := a.code(t)
	taken := c.register(t, "ana@example.com", otherPass, code)
	fresh := c.register(t, "bob@example.com", otherPass, a.code(t))
	same("register", taken, fresh)
	if a.count(t, "ana@example.com", "account_exists") != 1 {
		t.Errorf("no account_exists email: %+v", a.outbox(t))
	}
	expect(t, c.register(t, "carla@example.com", otherPass, code), http.StatusAccepted, "")

	// Login: unknown email and wrong password.
	same("login", c.login(t, "nobody@example.com", otherPass), c.login(t, "ana@example.com", otherPass))

	// Forgot password and resend verification: always 202.
	before := len(a.outbox(t))
	same("forgot", c.do(t, "POST", "/api/auth/forgot-password", `{"email":"nobody@example.com"}`),
		c.do(t, "POST", "/api/auth/forgot-password", `{"email":"ana@example.com"}`))
	same("resend", c.do(t, "POST", "/api/auth/resend-verification", `{"email":"nobody@example.com"}`),
		c.do(t, "POST", "/api/auth/resend-verification", `{"email":"ana@example.com"}`))
	expect(t, c.do(t, "POST", "/api/auth/resend-verification", `{"email":"bob@example.com"}`), http.StatusAccepted, "")
	got := a.outbox(t)[before:]
	if len(got) != 2 || got[0].to != "ana@example.com" || got[0].template != "reset_password" ||
		got[1].to != "bob@example.com" || got[1].template != "verify_email" {
		t.Errorf("emails: %+v (want a reset to ana and a verification to bob only)", got)
	}
}

func TestLoginBruteForce(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{LoginIP: NewLimiter(100, 100)})
	a.verifiedUser(t, "ana@example.com")
	for i := range 5 {
		// Spread over addresses: the per-account limit holds anyway.
		c := a.client(fmt.Sprintf("203.0.113.%d", 20+i))
		expect(t, c.login(t, "ana@example.com", otherPass), http.StatusUnauthorized, "invalid_credentials")
	}
	c := a.client("203.0.113.30")
	w := c.login(t, "ana@example.com", goodPass) // right password, still locked
	expect(t, w, http.StatusTooManyRequests, "too_many_attempts")
	if ra := w.Header().Get("Retry-After"); ra == "" || ra == "0" {
		t.Errorf("Retry-After = %q", ra)
	}
	// Unknown emails lock the same way: the 429 tells nothing.
	for range 5 {
		c.login(t, "nobody@example.com", otherPass)
	}
	expect(t, c.login(t, "nobody@example.com", otherPass), http.StatusTooManyRequests, "too_many_attempts")
	// Other accounts are not affected.
	expect(t, c.login(t, "bob@example.com", otherPass), http.StatusUnauthorized, "invalid_credentials")

	// Per address.
	b := newAuthAPI(t, AuthLimits{LoginIP: NewLimiter(3, 50)})
	d := b.client("203.0.113.40")
	for range 3 {
		expect(t, d.login(t, "x@example.com", otherPass), http.StatusUnauthorized, "invalid_credentials")
	}
	expect(t, d.login(t, "y@example.com", otherPass), http.StatusTooManyRequests, "too_many_attempts")
	expect(t, b.client("203.0.113.41").login(t, "y@example.com", otherPass), http.StatusUnauthorized, "invalid_credentials")
}

func TestPasswordReset(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100)})
	a.verifiedUser(t, "marta@example.com")
	c1, c2 := a.client("203.0.113.50"), a.client("203.0.113.51")
	expect(t, c1.login(t, "marta@example.com", goodPass), http.StatusOK, "")
	expect(t, c2.do(t, "POST", "/api/auth/forgot-password", `{"email":"marta@example.com"}`), http.StatusAccepted, "")
	first := a.lastToken(t, "marta@example.com", "reset_password")
	expect(t, c2.do(t, "POST", "/api/auth/forgot-password", `{"email":"marta@example.com"}`), http.StatusAccepted, "")
	tok := a.lastToken(t, "marta@example.com", "reset_password")

	reset := func(tok, pw string) *httptest.ResponseRecorder {
		return c2.do(t, "POST", "/api/auth/reset-password", js(map[string]string{"token": tok, "password": pw}))
	}
	expect(t, reset(first, otherPass), http.StatusBadRequest, "invalid_token") // replaced by the newer link
	expect(t, reset(tok, "short"), http.StatusBadRequest, "password_too_short")
	expect(t, reset(tok, "marta-y-su-gato-2026"), http.StatusBadRequest, "password_contains_email")
	expect(t, reset(tok, otherPass), http.StatusOK, "") // the refused tries did not spend it
	expect(t, reset(tok, otherPass), http.StatusBadRequest, "invalid_token")

	expect(t, c1.do(t, "GET", "/api/auth/me", ""), http.StatusUnauthorized, "unauthenticated")
	expect(t, c2.login(t, "marta@example.com", goodPass), http.StatusUnauthorized, "invalid_credentials")
	expect(t, c2.login(t, "marta@example.com", otherPass), http.StatusOK, "")
	if a.count(t, "marta@example.com", "password_changed") != 1 {
		t.Errorf("no password_changed email: %+v", a.outbox(t))
	}

	// Resetting proves the address: an unverified account can sign in after.
	expect(t, c2.register(t, "nuria@example.com", goodPass, a.code(t)), http.StatusAccepted, "")
	expect(t, c2.do(t, "POST", "/api/auth/forgot-password", `{"email":"nuria@example.com"}`), http.StatusAccepted, "")
	expect(t, reset(a.lastToken(t, "nuria@example.com", "reset_password"), otherPass), http.StatusOK, "")
	expect(t, c2.login(t, "nuria@example.com", otherPass), http.StatusOK, "")
}

func TestChangePassword(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{})
	a.verifiedUser(t, "ana@example.com")
	c1, c2 := a.client("203.0.113.60"), a.client("203.0.113.61")
	expect(t, c1.login(t, "ana@example.com", goodPass), http.StatusOK, "")
	expect(t, c2.login(t, "ana@example.com", goodPass), http.StatusOK, "")

	// A reset link asked for before the change must die with it.
	expect(t, c2.do(t, "POST", "/api/auth/forgot-password", `{"email":"ana@example.com"}`), http.StatusAccepted, "")
	pending := a.lastToken(t, "ana@example.com", "reset_password")

	change := func(cur, next string) *httptest.ResponseRecorder {
		return c1.do(t, "POST", "/api/auth/change-password", js(map[string]string{"current_password": cur, "new_password": next}))
	}
	expect(t, change(otherPass, otherPass), http.StatusBadRequest, "wrong_password")
	expect(t, change(goodPass, "password1234"), http.StatusBadRequest, "password_common")
	csrf := c1.csrf
	c1.csrf = ""
	expect(t, change(goodPass, otherPass), http.StatusForbidden, "csrf_failed")
	c1.csrf = csrf
	if w := change(goodPass, otherPass); w.Code != http.StatusNoContent {
		t.Fatalf("change: %d %s", w.Code, w.Body)
	}
	expect(t, c1.do(t, "GET", "/api/auth/me", ""), http.StatusOK, "")                          // this session stays
	expect(t, c2.do(t, "GET", "/api/auth/me", ""), http.StatusUnauthorized, "unauthenticated") // the others end
	expect(t, c2.login(t, "ana@example.com", otherPass), http.StatusOK, "")
	if a.count(t, "ana@example.com", "password_changed") != 1 {
		t.Error("no password_changed email")
	}
	expect(t, c2.do(t, "POST", "/api/auth/reset-password", js(map[string]string{"token": pending, "password": goodPass})),
		http.StatusBadRequest, "invalid_token")
	anon := a.client("203.0.113.62")
	expect(t, anon.do(t, "POST", "/api/auth/change-password", `{}`), http.StatusUnauthorized, "unauthenticated")
}

func TestOriginAndSessionChecks(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{})
	c := a.client("203.0.113.70")
	body := js(map[string]string{"email": "ana@example.com", "password": goodPass})
	for name, edit := range map[string]func(*http.Request){
		"other origin":      func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
		"cross-site":        func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		"same-site sibling": func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-site") },
		"null origin":       func(r *http.Request) { r.Header.Set("Origin", "null") },
	} {
		if w := c.do(t, "POST", "/api/auth/login", body, edit); w.Code != http.StatusForbidden || errCode(w) != "bad_origin" {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	// No browser headers (curl): no ambient cookies, so allowed.
	w := c.do(t, "POST", "/api/auth/login", body, func(r *http.Request) { r.Header.Del("Origin"); r.Header.Del("Sec-Fetch-Site") })
	expect(t, w, http.StatusUnauthorized, "invalid_credentials")

	// A made-up cookie is cleared.
	c.cookie = "made-up"
	w = c.do(t, "GET", "/api/auth/me", "")
	expect(t, w, http.StatusUnauthorized, "unauthenticated")
	if !strings.Contains(w.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Errorf("dead cookie not cleared: %v", w.Header())
	}
}

// The Origin check guards every route, POST /api/scans included, and lets
// non-browser clients (curl, scripts: no Origin, no Sec-Fetch-Site) through.
func TestCheckOrigin(t *testing.T) {
	h := checkOrigin(testOrigin)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	cases := []struct {
		method string
		hdr    map[string]string
		want   int
	}{
		{"POST", nil, http.StatusNoContent},                                         // curl
		{"POST", map[string]string{"Sec-Fetch-Site": "none"}, http.StatusNoContent}, // typed by the user
		{"POST", map[string]string{"Origin": testOrigin, "Sec-Fetch-Site": "same-origin"}, http.StatusNoContent},
		{"POST", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"POST", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"POST", map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden}, // fizsar.com is not this origin
		{"DELETE", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"GET", map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"}, http.StatusNoContent},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(tc.method, "/api/scans", strings.NewReader(`{"host":"fizsar.com"}`))
		for k, v := range tc.hdr {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s %v: %d, want %d", tc.method, tc.hdr, w.Code, tc.want)
		}
	}
}

// Expired and idle sessions are refused by the database query itself.
func TestSessionLifetime(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{})
	a.verifiedUser(t, "ana@example.com")
	c := a.client("203.0.113.80")
	for _, q := range []string{
		"UPDATE sessions SET last_seen_at = now() - interval '7 days 1 minute'",
		"UPDATE sessions SET expires_at = now() - interval '1 second'",
	} {
		expect(t, c.login(t, "ana@example.com", goodPass), http.StatusOK, "")
		if _, err := a.owner.Exec(context.Background(), q); err != nil {
			t.Fatal(err)
		}
		expect(t, c.do(t, "GET", "/api/auth/me", ""), http.StatusUnauthorized, "unauthenticated")
	}
}

func TestEmailsPerAddressAreLimited(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100)})
	a.verifiedUser(t, "ana@example.com")
	c := a.client("203.0.113.90")
	for range 5 {
		expect(t, c.do(t, "POST", "/api/auth/forgot-password", `{"email":"ana@example.com"}`), http.StatusAccepted, "")
	}
	// verifiedUser sent 1 (verification); 2 more resets fit in 3/hour.
	if n := a.count(t, "ana@example.com", "reset_password"); n != 2 {
		t.Errorf("%d reset emails, want 2", n)
	}
	b := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(2, 100)})
	d := b.client("203.0.113.91")
	for range 2 {
		expect(t, d.do(t, "POST", "/api/auth/forgot-password", `{"email":"x@example.com"}`), http.StatusAccepted, "")
	}
	expect(t, d.do(t, "POST", "/api/auth/forgot-password", `{"email":"x@example.com"}`), http.StatusTooManyRequests, "too_many_attempts")
}

func TestNormalizeEmail(t *testing.T) {
	for in, want := range map[string]string{
		" Ana@Example.COM ": "ana@example.com", "a.b+c@sub.example.org": "a.b+c@sub.example.org",
		"ana": "", "@example.com": "", "ana@example": "", "Ana <ana@example.com>": "", "ana@example.com\r\nBcc: x@y.z": "",
		strings.Repeat("a", 250) + "@example.com": "",
	} {
		if got, _ := normalizeEmail(in); got != want {
			t.Errorf("normalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIPPrefix(t *testing.T) {
	for in, want := range map[string]string{"203.0.113.77": "203.0.113.0/24", "2001:db8:1:2:3::4": "2001:db8:1::/48"} {
		if got := ipPrefix(netip.MustParseAddr(in)); got == nil || *got != want {
			t.Errorf("ipPrefix(%s) = %v", in, got)
		}
	}
	if ipPrefix(netip.Addr{}) != nil {
		t.Error("invalid address got a prefix")
	}
}

func TestLimiterCheckAddReset(t *testing.T) {
	l := NewWindowLimiter(2, time.Minute)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }
	for range 3 {
		if ok, _ := l.Check("k"); !ok {
			t.Fatal("Check took a slot")
		}
	}
	l.Add("k")
	l.Add("k")
	if ok, retry := l.Check("k"); ok || retry != time.Minute {
		t.Errorf("after 2 Add: ok=%v retry=%v", ok, retry)
	}
	now = now.Add(time.Minute + time.Second)
	if ok, _ := l.Check("k"); !ok {
		t.Error("window did not slide")
	}
	l.Add("k")
	l.Add("k")
	l.Reset("k")
	if ok, _ := l.Check("k"); !ok {
		t.Error("Reset did not clear")
	}
}
