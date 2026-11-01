package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"runtime"
	"testing"
	"time"
)

// key6 is the rate key of the i-th IPv6 /64 of 2001:db8::/32.
func key6(i int) string {
	a := netip.MustParseAddr("2001:db8::").As16()
	a[4], a[5], a[6], a[7] = byte(i>>24), byte(i>>16), byte(i>>8), byte(i)
	return rateKey(netip.AddrFrom16(a))
}

// One request per IPv6 /64 is one key that stays for 24 hours (the longest
// window): a client with a /48 has 65,536 of them, and GET /api/altcha and
// POST /api/e reserve before any proof of work. Each limiter holds at most
// MaxLimiterKeys clients; beyond that a new one is refused (429), a known
// one is not, and room comes back as keys expire.
func TestLimiterKeysBounded(t *testing.T) {
	const keys = 3 * MaxLimiterKeys
	now := time.Unix(1_800_000_000, 0)
	l := NewLimiter(60, 2000)
	l.now = func() time.Time { return now }
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	refused := 0
	for i := range keys {
		if ok, retry := l.Reserve(key6(i)); !ok {
			refused++
			if retry <= 0 {
				t.Fatalf("key %d refused without a Retry-After", i)
			}
		}
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	heap := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("%d keys held, %d refused, %.1f MB of heap", len(l.hits), refused, float64(heap)/1e6)
	if len(l.hits) > MaxLimiterKeys || refused != keys-MaxLimiterKeys {
		t.Errorf("%d keys kept and %d refused, want at most %d kept", len(l.hits), refused, MaxLimiterKeys)
	}
	if heap > 40<<20 {
		t.Errorf("%.1f MB for one limiter", float64(heap)/1e6)
	}
	if ok, _ := l.Reserve(key6(0)); !ok {
		t.Error("a known client refused because the limiter is full")
	}
	now = now.Add(24*time.Hour + time.Second) // every hit expired: room for new clients
	if ok, _ := l.Reserve(key6(keys + 1)); !ok {
		t.Error("new client refused after every key expired")
	}
	if len(l.hits) > 2 {
		t.Errorf("%d keys after the sweep", len(l.hits))
	}
}

// Full, GET /api/altcha answers 429 with Retry-After to a new client.
func TestAltchaChallengeLimiterFull(t *testing.T) {
	cfg := production(t, false)
	cfg.PublicLimits.Challenge = NewLimiter(60, 2000)
	for i := range MaxLimiterKeys {
		cfg.PublicLimits.Challenge.Reserve(key6(i))
	}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/altcha", nil)
	r.RemoteAddr = "[2001:db8:ffff:ffff::1]:1234"
	NewRouter(cfg).ServeHTTP(rec, r)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("full limiter: %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}
