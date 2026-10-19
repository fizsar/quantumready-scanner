package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fizsar/quantumready-saas/internal/store"
)

type fakeUsage struct {
	mu      sync.Mutex
	counts  []string // "event/dim"
	blocked []string
}

func (f *fakeUsage) CountUsage(_ context.Context, p store.CountUsageParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts = append(f.counts, p.Event+"/"+p.Dim)
	return nil
}

func (f *fakeUsage) CountBlockedTarget(_ context.Context, p store.CountBlockedTargetParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blocked = append(f.blocked, p.Domain)
	return nil
}

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0 Safari/537.36"

func usageAPI(t *testing.T, limit *Limiter) (http.Handler, *fakeUsage) {
	t.Helper()
	allow, err := ParseAllowlist("fizsar.com quantumready.fizsar.com")
	if err != nil {
		t.Fatal(err)
	}
	u := &fakeUsage{}
	return NewRouter(Config{Usage: u, Allowlist: allow, UsageLimiter: limit}), u
}

type evReq struct {
	body, ua, origin, fetchSite, ip string
}

func postEvent(h http.Handler, e evReq) int {
	r := httptest.NewRequest(http.MethodPost, "/api/e", strings.NewReader(e.body))
	r.Host = "quantumready.fizsar.com"
	r.Header.Set("Content-Type", "application/json")
	if e.ua != "-" {
		r.Header.Set("User-Agent", browserUA)
		if e.ua != "" {
			r.Header.Set("User-Agent", e.ua)
		}
	}
	if e.origin != "" {
		r.Header.Set("Origin", e.origin)
	}
	if e.fetchSite != "" {
		r.Header.Set("Sec-Fetch-Site", e.fetchSite)
	}
	if e.ip != "" {
		r.RemoteAddr = e.ip + ":1234"
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

const ownOrigin = "https://quantumready.fizsar.com"

func TestUsageEventsCounted(t *testing.T) {
	h, u := usageAPI(t, nil)
	for _, body := range []string{
		`{"e":"page_view","p":"/examples"}`,
		`{"e":"page_view","p":"/privacy"}`,
		`{"e":"page_view","p":"/","entry":true,"r":"www.linkedin.com"}`,
		`{"e":"page_view","p":"scan_report","entry":true}`,
		`{"e":"page_view","p":"/architecture","entry":true,"r":"quantumready.fizsar.com"}`, // a reload: no source
		`{"e":"scan_started","d":"quantumready.fizsar.com"}`,
		`{"e":"under_the_hood_open","d":"examples"}`,
		`{"e":"json_download","d":"scan_report"}`,
		`{"e":"lang_switch","d":"en"}`,
	} {
		if code := postEvent(h, evReq{body: body, origin: ownOrigin}); code != http.StatusNoContent {
			t.Errorf("%s: %d", body, code)
		}
	}
	want := []string{"page_view//examples", "page_view//privacy", "page_view//", "visit_source/linkedin.com", "page_view/scan_report", "visit_source/direct",
		"page_view//architecture", "scan_started/quantumready.fizsar.com", "under_the_hood_open/examples", "json_download/scan_report", "lang_switch/en"}
	if !slices.Equal(u.counts, want) {
		t.Errorf("counted\n %v\nwant\n %v", u.counts, want)
	}
}

// The closed lists: anything outside them is dropped (and still 204).
func TestUsageClosedLists(t *testing.T) {
	h, u := usageAPI(t, nil)
	for _, body := range []string{
		`{"e":"page_view","p":"/admin"}`,                                   // route not in the list
		`{"e":"page_view","p":"/scans/123"}`,                               // raw scan URL, never stored
		`{"e":"page_view","p":"/","r":"evil.example"}`,                     // a referrer without entry
		`{"e":"page_view","p":"/","entry":true,"r":"not a host"}`,          // a referrer that is not a host
		`{"e":"scan_started","d":"example.com"}`,                           // not a demo domain
		`{"e":"scan_blocked_403","d":""}`,                                  // counted by the API only
		`{"e":"under_the_hood_open","d":"home"}`,                           // page not in the list
		`{"e":"json_download","d":"examples","p":"/"}`,                     // field that does not belong
		`{"e":"lang_switch","d":"fr"}`,                                     // language not in the list
		`{"e":"signup"}`,                                                   // event not in the list
		`{"e":"page_view","p":"/","x":1}`,                                  // unknown field
		`{"e":"page_view","p":"/"`,                                         // broken JSON
		`{"e":"page_view","p":"/","r":"` + strings.Repeat("a", 300) + `"}`, // too long
	} {
		if code := postEvent(h, evReq{body: body, origin: ownOrigin}); code != http.StatusNoContent {
			t.Errorf("%s: %d, want 204 always", body, code)
		}
	}
	if len(u.counts) != 0 {
		t.Errorf("counted junk: %v", u.counts)
	}
}

func TestUsageOnlyFromOurPagesAndBrowsers(t *testing.T) {
	h, u := usageAPI(t, nil)
	ok := `{"e":"page_view","p":"/"}`
	for name, e := range map[string]evReq{
		"other site (Origin)":         {body: ok, origin: "https://evil.example"},
		"other site (Sec-Fetch-Site)": {body: ok, origin: ownOrigin, fetchSite: "cross-site"},
		"no Origin":                   {body: ok},
		"Googlebot":                   {body: ok, origin: ownOrigin, ua: "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"},
		"curl":                        {body: ok, origin: ownOrigin, ua: "curl/8.9.1"},
		"headless Chrome":             {body: ok, origin: ownOrigin, ua: "Mozilla/5.0 (X11; Linux x86_64) HeadlessChrome/124.0 Safari/537.36"},
		"Lighthouse":                  {body: ok, origin: ownOrigin, ua: browserUA + " Chrome-Lighthouse"},
		"LinkedIn preview":            {body: ok, origin: ownOrigin, ua: "LinkedInBot/1.0 (compatible; Mozilla/5.0)"},
		"no user agent":               {body: ok, origin: ownOrigin, ua: "-"},
	} {
		if code := postEvent(h, e); code != http.StatusNoContent {
			t.Errorf("%s: %d", name, code)
		}
	}
	if len(u.counts) != 0 {
		t.Errorf("counted: %v", u.counts)
	}
	postEvent(h, evReq{body: ok, origin: ownOrigin, fetchSite: "same-origin"})
	postEvent(h, evReq{body: ok, origin: ownOrigin})
	if len(u.counts) != 2 {
		t.Errorf("a browser on our pages: %v, want 2 counts", u.counts)
	}
}

func TestUsageRateLimitPerClient(t *testing.T) {
	h, u := usageAPI(t, NewLimiter(3, 100))
	for range 5 {
		postEvent(h, evReq{body: `{"e":"page_view","p":"/"}`, origin: ownOrigin, ip: "203.0.113.7"})
	}
	postEvent(h, evReq{body: `{"e":"page_view","p":"/"}`, origin: ownOrigin, ip: "203.0.113.8"})
	if len(u.counts) != 4 {
		t.Errorf("%d counted, want 3 from the first client and 1 from the second", len(u.counts))
	}
}
