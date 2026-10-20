package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fizsar/quantumready-saas/internal/domains"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// M1 usage metrics: daily aggregate counters only. The web sends events to
// POST /api/e; the API keeps no IP (the rate limiter holds it in memory, as
// for scans), no user agent, no time of day and no identifier.

// UsageStore records the counters; *store.Store satisfies it.
type UsageStore interface {
	CountUsage(ctx context.Context, arg store.CountUsageParams) error
	CountBlockedTarget(ctx context.Context, arg store.CountBlockedTargetParams) error
}

const (
	maxEventBody   = 256
	maxDimsPerDay  = 200  // different dims per event and day; then "other"
	maxBlockedHost = 1000 // different 403 domains per day; then "other"
)

// The closed lists. Anything else is dropped without a trace.
var (
	usageRoutes = []string{"/", "/examples", "/architecture", "/privacy", "/demo", "scan_report", "not_found"}
	usagePages  = []string{"scan_report", "examples"}
	usageLangs  = []string{"es", "en"}
	// demo_requested: where the "Request a demo" button was, never the form.
	demoSources = []string{"home", "host_not_allowed", "examples", "direct"}
)

// botUA matches crawlers, link previews, monitors, scripts and headless
// browsers (our own Chromium checks included). An empty user agent is a bot too.
var botUA = regexp.MustCompile(`(?i)bot|crawl|spider|slurp|preview|facebookexternalhit|embedly|curl|wget|python|go-http-client|java/|okhttp|axios|node-fetch|headless|lighthouse|pingdom|uptime|monitor|phantom|selenium|puppeteer|playwright`)

func isBot(r *http.Request) bool {
	ua := r.UserAgent()
	return ua == "" || botUA.MatchString(ua)
}

// sameOrigin: only our own pages count. Sec-Fetch-Site when the browser sends
// it, else the Origin header against the host the request came to.
func sameOrigin(r *http.Request) bool {
	if s := r.Header.Get("Sec-Fetch-Site"); s != "" {
		return s == "same-origin"
	}
	u, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host)
}

type usageEvent struct {
	E     string `json:"e"`     // event
	P     string `json:"p"`     // page_view: route
	D     string `json:"d"`     // other events: their dimension
	Entry bool   `json:"entry"` // page_view: first view of a page load (a visit)
	R     string `json:"r"`     // entry: referrer host, if another site
}

// usageEvent handles POST /api/e. It always answers 204, recorded or not, so
// probing it teaches nothing.
func (a *api) usageEvent(w http.ResponseWriter, r *http.Request) {
	defer w.WriteHeader(http.StatusNoContent)
	if isBot(r) || !sameOrigin(r) {
		return
	}
	if ok, _ := a.usageLimiter.Reserve(rateKey(clientAddr(r, a.cfg.TrustedProxies))); !ok {
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxEventBody))
	dec.DisallowUnknownFields()
	var ev usageEvent
	if err := dec.Decode(&ev); err != nil {
		return
	}
	for _, c := range a.usageCounts(ev, r.Host) {
		a.countUsage(r.Context(), c[0], c[1])
	}
}

// usageCounts validates an event against the closed lists and returns the
// (event, dim) pairs to count; nothing if anything is off.
func (a *api) usageCounts(ev usageEvent, ownHost string) [][2]string {
	switch ev.E {
	case "page_view":
		if ev.D != "" || !slices.Contains(usageRoutes, ev.P) || (!ev.Entry && ev.R != "") || (ev.P == "/demo" && !a.demoOn()) {
			return nil
		}
		out := [][2]string{{"page_view", ev.P}}
		if ev.Entry {
			src, valid := visitSource(ev.R, ownHost)
			if !valid {
				return nil // not a host name: the whole event is junk
			}
			if src != "" {
				out = append(out, [2]string{"visit_source", src})
			}
		}
		return out
	case "scan_started":
		if ev.P != "" || ev.Entry || ev.R != "" || a.cfg.Allowlist == nil || !a.cfg.Allowlist.Allows(ev.D) {
			return nil
		}
	case "under_the_hood_open", "json_download":
		if ev.P != "" || ev.Entry || ev.R != "" || !slices.Contains(usagePages, ev.D) {
			return nil
		}
	case "lang_switch":
		if ev.P != "" || ev.Entry || ev.R != "" || !slices.Contains(usageLangs, ev.D) {
			return nil
		}
	case "demo_requested":
		if !a.demoOn() || ev.P != "" || ev.Entry || ev.R != "" || !slices.Contains(demoSources, ev.D) {
			return nil
		}
	default: // scan_blocked_403 is counted by the API itself, never from the web
		return nil
	}
	return [][2]string{{ev.E, ev.D}}
}

// visitSource reduces a referrer host to its registrable domain ("direct"
// without one). Our own host is no source (a reload or a back button): "".
// valid is false when ref is not a host name at all.
func visitSource(ref, ownHost string) (src string, valid bool) {
	if ref == "" {
		return "direct", true
	}
	host, err := domains.Normalize(ref)
	if err != nil {
		return "", false
	}
	if own, err := domains.Normalize(ownHost); err == nil && host == own {
		return "", true
	}
	return domains.Registrable(host), true
}

func today() pgtype.Date {
	y, m, d := time.Now().UTC().Date()
	return pgtype.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Valid: true}
}

// countBlocked counts a scan request refused with 403 host_not_allowed, and
// the registrable domain it pointed at (never an IP address). Nothing about
// who asked is kept. Bots are not counted, and the per-client limit of
// POST /api/e applies, since the 403 comes before the scan rate limit.
func (a *api) countBlocked(r *http.Request, host string) {
	if a.cfg.Usage == nil || isBot(r) {
		return
	}
	if ok, _ := a.usageLimiter.Reserve(rateKey(clientAddr(r, a.cfg.TrustedProxies))); !ok {
		return
	}
	a.countUsage(r.Context(), "scan_blocked_403", "")
	h, err := domains.Normalize(host)
	if err != nil { // an IP address, a special-use name…: the count alone
		return
	}
	d := domains.Registrable(h)
	if err := a.cfg.Usage.CountBlockedTarget(r.Context(), store.CountBlockedTargetParams{Day: today(), Domain: d, MaxDomains: maxBlockedHost}); err != nil {
		a.cfg.Logger.Warn("usage: count blocked target", "err", err)
	}
}

// countUsage adds one; a failure is logged and never reaches the user.
func (a *api) countUsage(ctx context.Context, event, dim string) {
	if a.cfg.Usage == nil {
		return
	}
	if err := a.cfg.Usage.CountUsage(ctx, store.CountUsageParams{Day: today(), Event: event, Dim: dim, MaxDims: maxDimsPerDay}); err != nil {
		a.cfg.Logger.Warn("usage: count", "event", event, "err", err)
	}
}
