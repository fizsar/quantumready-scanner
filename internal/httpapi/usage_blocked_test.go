package httpapi

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/store"
)

func blockedAPI(t *testing.T, limit *Limiter) (http.Handler, *fakeUsage) {
	t.Helper()
	allow, err := ParseAllowlist("fizsar.com quantumready.fizsar.com")
	if err != nil {
		t.Fatal(err)
	}
	u := &fakeUsage{}
	st := &fixtureStore{scans: map[uuid.UUID]store.Scan{}, findings: map[uuid.UUID][]store.Finding{}}
	return NewRouter(Config{Scans: st, Usage: u, Allowlist: allow, UsageLimiter: limit}), u
}

func postScan(h http.Handler, host, ua string) int {
	r := httptest.NewRequest(http.MethodPost, "/api/scans", strings.NewReader(`{"host":"`+host+`"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("User-Agent", ua)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

// A scan request outside the allowlist counts once, with the registrable
// domain it pointed at; nothing about who asked.
func TestBlockedScanCounted(t *testing.T) {
	h, u := blockedAPI(t, nil)
	for _, host := range []string{"www.shop.example.co.uk", "EXAMPLE.org", "203.0.113.5"} {
		if code := postScan(h, host, browserUA); code != http.StatusForbidden {
			t.Fatalf("%s: %d, want 403", host, code)
		}
	}
	if want := []string{"scan_blocked_403/", "scan_blocked_403/", "scan_blocked_403/"}; !slices.Equal(u.counts, want) {
		t.Errorf("counts %v, want %v", u.counts, want)
	}
	if want := []string{"example.co.uk", "example.org"}; !slices.Equal(u.blocked, want) { // never the IP
		t.Errorf("domains %v, want %v", u.blocked, want)
	}
	// Bots are not counted.
	postScan(h, "example.net", "curl/8.9.1")
	postScan(h, "example.net", "")
	if len(u.counts) != 3 || slices.Contains(u.blocked, "example.net") {
		t.Errorf("bots counted: %v %v", u.counts, u.blocked)
	}
}

// The 403 comes before the scan rate limit, so the usage limit applies:
// nobody can fill the table with invented domains.
func TestBlockedScanCountLimited(t *testing.T) {
	h, u := blockedAPI(t, NewLimiter(2, 100))
	for i := range 5 {
		postScan(h, "d"+string(rune('a'+i))+".org", browserUA)
	}
	if len(u.blocked) != 2 || len(u.counts) != 2 {
		t.Errorf("counted %v %v, want 2 of each", u.counts, u.blocked)
	}
}
