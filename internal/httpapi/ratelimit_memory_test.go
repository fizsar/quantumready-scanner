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
// POST /api/e reserve before any proof of work. A million different clients
// keep memory bounded (at most MaxLimiterKeys) and none of them is refused
// for it: a full limiter makes room, it never closes the door.
func TestLimiterMemoryBounded(t *testing.T) {
	const keys = 1_000_000
	now := time.Unix(1_800_000_000, 0)
	l := NewLimiter(60, 2000)
	l.now = func() time.Time { return now }
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := range keys {
		if ok, _ := l.Reserve(key6(i)); !ok {
			t.Fatalf("client %d refused (the limiter was full)", i)
		}
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	heap := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("%d keys held after %d clients, %.1f MB of heap", len(l.clients), keys, float64(heap)/1e6)
	if len(l.clients) > MaxLimiterKeys || l.lru.Len() != len(l.clients) {
		t.Errorf("%d keys (list %d), want at most %d", len(l.clients), l.lru.Len(), MaxLimiterKeys)
	}
	if heap > 40<<20 {
		t.Errorf("%.1f MB for one limiter", float64(heap)/1e6)
	}
}

// A full limiter never answers 429 to a new client because it is full: GET
// /api/altcha with 100,000 other clients known gives a challenge.
func TestLimiterFullServesNewClients(t *testing.T) {
	l := NewLimiter(60, 2000)
	for i := range MaxLimiterKeys {
		l.Reserve(key6(i))
	}
	if ok, retry := l.Reserve(key6(MaxLimiterKeys + 1)); !ok {
		t.Errorf("new client refused by a full limiter (retry %v)", retry)
	}
	cfg := production(t, false)
	cfg.PublicLimits.Challenge = l
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/altcha", nil)
	r.RemoteAddr = "[2001:db8:ffff:ffff::1]:1234"
	NewRouter(cfg).ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/altcha with a full limiter: %d %s", rec.Code, rec.Body)
	}
}

// A client that used up its quota stays limited while others rotate
// addresses: the oldest idle clients make room first, never a recent one,
// and every request (refused ones too) counts as recent.
func TestLimiterRecentClientSurvivesRotation(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	l := NewLimiter(5, 30)
	l.now = func() time.Time { return now }
	const abuser = "203.0.113.7"
	for range 5 {
		l.Reserve(abuser)
	}
	// Half the capacity of rotation after its last request: still known.
	for i := range MaxLimiterKeys / 2 {
		l.Reserve(key6(i))
	}
	if ok, _ := l.Reserve(abuser); ok {
		t.Fatal("limited client allowed again after 50,000 other clients")
	}
	// A million more, while it keeps trying every 10,000: never forgotten.
	for i := range 1_000_000 {
		l.Reserve(key6(MaxLimiterKeys + i))
		if i%10_000 == 0 {
			if ok, _ := l.Reserve(abuser); ok {
				t.Fatalf("limited client allowed again after %d rotated clients", i)
			}
		}
	}
	// Expired clients go before anyone else: after the windows pass, the
	// abuser and the flood are all expired, and a new client gets in.
	now = now.Add(25 * time.Hour)
	if ok, _ := l.Reserve("198.51.100.1"); !ok {
		t.Error("new client refused after every key expired")
	}
	if ok, _ := l.Reserve(abuser); !ok {
		t.Error("the abuser is still limited a day later")
	}
}

// Clients with nothing left to remember (no hit in the windows: seen only by
// Check, or expired) make room before anyone with live hits, even if they
// were seen more recently.
func TestLimiterExpiredGoFirst(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	l := NewLimiter(5, 30)
	l.now = func() time.Time { return now }
	const abuser = "203.0.113.7"
	for range 5 {
		l.Reserve(abuser)
	}
	for i := range MaxLimiterKeys - 1 { // seen after the abuser, but no hits
		l.Check(key6(i))
	}
	l.Reserve("198.51.100.1") // full: room must come from the hit-less ones
	if ok, _ := l.Check(abuser); ok {
		t.Error("the limited client was dropped while hit-less clients stayed")
	}
}
