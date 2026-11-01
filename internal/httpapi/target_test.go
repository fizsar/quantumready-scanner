package httpapi

import (
	"errors"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestNormalizeHost(t *testing.T) {
	good := map[string]string{
		"fizsar.com":                       "fizsar.com",
		"  QuantumReady.Fizsar.COM.  ":     "quantumready.fizsar.com",
		"https://fizsar.com/path?q=1#frag": "fizsar.com",
		"fizsar.com:443":                   "fizsar.com",
		"user:secret@fizsar.com/x":         "fizsar.com",
		"bücher.de":                        "xn--bcher-kva.de",
		"https://BÜCHER.de:443/página":     "xn--bcher-kva.de",
		"xn--bcher-kva.de":                 "xn--bcher-kva.de",
		"[2606:4700:4700::1111]:443":       "2606:4700:4700::1111",
		"https://203.0.113.7/":             "203.0.113.7",
		"münchen.example.com":              "xn--mnchen-3ya.example.com",
	}
	for in, want := range good {
		got, err := NormalizeHost(in)
		if err != nil || got != want {
			t.Errorf("NormalizeHost(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"fizsar.com:8443", "HTTP://fizsar.com:80/", "[2606:4700:4700::1111]:8443", "fizsar.com:0"} {
		if _, err := NormalizeHost(in); !errors.Is(err, errPortNotAllowed) {
			t.Errorf("NormalizeHost(%q) error = %v, want errPortNotAllowed", in, err)
		}
	}
	for _, in := range []string{"", "   ", "localhost", "https://", "exa mple.com", "-a.com", "a-.com", "a..com",
		"exa_mple.com", "fizsar.com\x00", strings.Repeat("a", 64) + ".com", strings.Repeat("a.", 130) + "com"} {
		if got, err := NormalizeHost(in); err == nil {
			t.Errorf("NormalizeHost(%q) = %q, want error", in, got)
		}
	}
}

func TestAllowlistExactMatch(t *testing.T) {
	a, err := ParseAllowlist("fizsar.com, QuantumReady.fizsar.com\nhttps://bücher.de/")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"fizsar.com", "quantumready.fizsar.com", "xn--bcher-kva.de"} {
		if !a.Allows(h) {
			t.Errorf("%s not allowed", h)
		}
	}
	for _, h := range []string{"www.fizsar.com", "sub.quantumready.fizsar.com", "fizsar.com.evil.example", "izsar.com", ""} {
		if a.Allows(h) {
			t.Errorf("%s allowed, want exact match only", h)
		}
	}
	if _, err := ParseAllowlist("fizsar.com, not a host"); err == nil {
		t.Error("invalid allowlist entry accepted")
	}
	var none *Allowlist
	if none.Allows("fizsar.com") {
		t.Error("nil allowlist allows")
	}
}

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	l := NewLimiter(5, 30)
	l.now = func() time.Time { return now }

	for i := range 5 {
		if ok, _ := l.Reserve("a"); !ok {
			t.Fatalf("request %d denied", i+1)
		}
		now = now.Add(time.Second)
	}
	ok, retry := l.Reserve("a")
	if ok || retry < 50*time.Second || retry > 56*time.Second {
		t.Fatalf("6th in a minute = %v retry %v, want denied with ~55s", ok, retry)
	}
	if ok, _ := l.Reserve("b"); !ok {
		t.Error("another client was limited")
	}
	l.Cancel("b")
	if n := len(l.clients["b"].hits); n != 0 {
		t.Errorf("Cancel left %d hits", n)
	}

	// Day limit: 5 per minute, 30 per day.
	for range 5 { // 25 more over five minutes
		now = now.Add(time.Minute)
		for range 5 {
			if ok, _ := l.Reserve("a"); !ok {
				t.Fatal("denied before 30 a day")
			}
		}
	}
	now = now.Add(time.Minute)
	if ok, retry := l.Reserve("a"); ok || retry < 23*time.Hour {
		t.Fatalf("31st in a day = %v retry %v, want denied for most of a day", ok, retry)
	}
	now = now.Add(24 * time.Hour)
	if ok, _ := l.Reserve("a"); !ok {
		t.Error("still denied a day later")
	}

	// Idle keys are swept.
	now = now.Add(48 * time.Hour)
	l.Reserve("c")
	if _, ok := l.clients["a"]; ok {
		t.Error("idle key not swept")
	}
}

func TestClientAddr(t *testing.T) {
	trusted, err := ParsePrefixes("172.18.0.0/16, 10.0.0.9")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, remote, xff, want string
	}{
		{"direct client ignores XFF", "203.0.113.7:5000", "1.2.3.4", "203.0.113.7"},
		{"from Caddy uses XFF", "172.18.0.2:5000", "198.51.100.1", "198.51.100.1"},
		{"rightmost untrusted wins", "172.18.0.2:5000", "6.6.6.6, 198.51.100.1", "198.51.100.1"},
		{"skips trusted hops", "172.18.0.2:5000", "198.51.100.1, 10.0.0.9", "198.51.100.1"},
		{"garbage falls back to the proxy", "172.18.0.2:5000", "not-an-ip", "172.18.0.2"},
		{"no header from proxy", "172.18.0.2:5000", "", "172.18.0.2"},
		{"single trusted IP", "10.0.0.9:1", "2001:db8::5", "2001:db8::5"},
		{"IPv6 client", "[2001:db8::1]:443", "1.2.3.4", "2001:db8::1"},
	} {
		r := httptest.NewRequest("POST", "/api/scans", nil)
		r.RemoteAddr = tt.remote
		if tt.xff != "" {
			r.Header.Set("X-Forwarded-For", tt.xff)
		}
		if got := clientAddr(r, trusted).String(); got != tt.want {
			t.Errorf("%s: clientAddr = %s, want %s", tt.name, got, tt.want)
		}
	}
	if _, err := ParsePrefixes("172.18.0.0/16, nope"); err == nil {
		t.Error("invalid prefix accepted")
	}
	// Empty items (a trailing comma, an unset variable) are ignored.
	for _, s := range []string{"172.18.0.0/16,", ",172.18.0.0/16, ,"} {
		if ps, err := ParsePrefixes(s); err != nil || len(ps) != 1 || ps[0].String() != "172.18.0.0/16" {
			t.Errorf("ParsePrefixes(%q) = %v, %v", s, ps, err)
		}
	}
	if ps, err := ParsePrefixes(""); err != nil || len(ps) != 0 {
		t.Errorf("ParsePrefixes(\"\") = %v, %v", ps, err)
	}
}

func TestRateKeyGroupsIPv6By64(t *testing.T) {
	a := rateKey(netip.MustParseAddr("2001:db8:1:2::1"))
	b := rateKey(netip.MustParseAddr("2001:db8:1:2:ffff::9"))
	c := rateKey(netip.MustParseAddr("2001:db8:1:3::1"))
	if a != b || a == c {
		t.Errorf("keys %s %s %s: want the same /64 grouped, others apart", a, b, c)
	}
	if k := rateKey(netip.MustParseAddr("203.0.113.7")); k != "203.0.113.7" {
		t.Errorf("IPv4 key = %s", k)
	}
}

func TestPickLang(t *testing.T) {
	for _, tt := range []struct{ query, header, want string }{
		{"", "", "es"},
		{"en", "es-ES", "en"},
		{"es", "en-US", "es"},
		{"fr", "en-GB,en;q=0.9", "en"},
		{"", "de-DE,de;q=0.9,en;q=0.5", "en"},
		{"", "es-MX,es;q=0.9,en;q=0.8", "es"},
		{"", "ja", "es"},
		{"", "garbage;;", "es"},
	} {
		r := httptest.NewRequest("GET", "/api/scans/x?lang="+tt.query, nil)
		if tt.header != "" {
			r.Header.Set("Accept-Language", tt.header)
		}
		if got := pickLang(r); got != tt.want {
			t.Errorf("lang=%q Accept-Language=%q: %s, want %s", tt.query, tt.header, got, tt.want)
		}
	}
}
