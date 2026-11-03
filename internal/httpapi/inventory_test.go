package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/fizsar/quantumready-saas/internal/altcha"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// routes lists every method and path the router serves.
func routes(t *testing.T, h http.Handler) []string {
	t.Helper()
	r, ok := h.(chi.Routes)
	if !ok {
		t.Fatalf("router %T is not a chi.Routes", h)
	}
	var out []string
	if err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		out = append(out, method+" "+strings.TrimSuffix(strings.ReplaceAll(route, "/*/", "/"), "/"))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// production is the configuration of the Apertura milestone: every store
// wired as cmd/api does, accounts off (ACCOUNTS_ENABLED=false), the demo off,
// the proof of work and PUBLIC_ORIGIN set.
func production(t *testing.T, accounts bool) Config {
	t.Helper()
	st := store.NewStore(nil) // never queried: only the routes are looked at
	v, err := altcha.New(bytes.Repeat([]byte{7}, 32), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Scans: st, Usage: st, Demos: st, DemoAltcha: v, Altcha: v, PublicOrigin: "https://quantumready.fizsar.com", PublicScanOpen: true}
	if accounts {
		cfg.Accounts, cfg.Orgs, cfg.Domains = st, st, st
	}
	return cfg
}

// The whole API surface with accounts off, exactly: a route added anywhere
// fails here until it is listed (and thought about).
func TestRouteInventoryAccountsOff(t *testing.T) {
	want := []string{
		"GET /api/altcha",
		"GET /api/health",
		"GET /api/scans/{id}",
		"GET /api/targets",
		"POST /api/e",
		"POST /api/scans",
	}
	if got := routes(t, NewRouter(production(t, false))); !slices.Equal(got, want) {
		t.Errorf("routes with accounts off:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	// Accounts alone decides: organization and domain stores wired by mistake
	// without it mount nothing.
	cfg := production(t, false)
	st := store.NewStore(nil)
	cfg.Orgs, cfg.Domains = st, st
	if got := routes(t, NewRouter(cfg)); !slices.Equal(got, want) {
		t.Errorf("routes with Orgs and Domains but no Accounts:\n  %s", strings.Join(got, "\n  "))
	}

	// And through HTTP: every account endpoint is a plain 404, sessions or not.
	h := NewRouter(production(t, false))
	for _, req := range []struct{ method, path string }{
		{"POST", "/api/auth/register"}, {"POST", "/api/auth/login"}, {"GET", "/api/auth/me"}, {"POST", "/api/auth/forgot-password"},
		{"GET", "/api/org"}, {"GET", "/api/org/members"}, {"POST", "/api/org/invitations"}, {"GET", "/api/org/domains"},
		{"POST", "/api/org/scans"}, {"GET", "/api/org/scans/x"}, {"POST", "/api/organizations"}, {"POST", "/api/invitations/preview"},
		{"POST", "/api/invitations/accept"}, {"POST", "/api/demo"}, {"GET", "/api/demo/challenge"},
	} {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(req.method, req.path, strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://quantumready.fizsar.com")
		r.AddCookie(&http.Cookie{Name: "__Host-qr_session", Value: "x"})
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: %d, want 404 (accounts off)", req.method, req.path, rec.Code)
		}
	}
}

// The inventory sees account routes when they exist: the test above is not
// passing because it cannot see them.
func TestRouteInventoryAccountsOn(t *testing.T) {
	got := routes(t, NewRouter(production(t, true)))
	for _, r := range []string{"POST /api/auth/register", "POST /api/auth/login", "GET /api/auth/me", "GET /api/org/members", "POST /api/org/scans", "POST /api/invitations/accept"} {
		if !slices.Contains(got, r) {
			t.Errorf("%s missing with accounts on (got %v)", r, got)
		}
	}
}

// GET /api/targets tells the web whether accounts are on.
// The AGPL's "Source code" link: only when QR_SOURCE_URL is set.
func TestTargetsSource(t *testing.T) {
	for _, src := range []string{"", "https://github.com/fizsar/quantumready-saas/tree/5637c0d"} {
		cfg := production(t, false)
		cfg.SourceURL = src
		rec := httptest.NewRecorder()
		NewRouter(cfg).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/targets", nil))
		has := strings.Contains(rec.Body.String(), `"source":`)
		if rec.Code != http.StatusOK || has != (src != "") || (src != "" && !strings.Contains(rec.Body.String(), `"source":"`+src+`"`)) {
			t.Errorf("source %q: %d %s", src, rec.Code, rec.Body)
		}
	}
}

func TestTargetsSayAccounts(t *testing.T) {
	for _, on := range []bool{false, true} {
		rec := httptest.NewRecorder()
		NewRouter(production(t, on)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/targets", nil))
		want := `"accounts":false`
		if on {
			want = `"accounts":true`
		}
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("accounts %v: %d %s", on, rec.Code, rec.Body)
		}
	}
}
