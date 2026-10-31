package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/fizsar/quantumready-saas/internal/domains"
	"github.com/fizsar/quantumready-saas/internal/report"
	"github.com/fizsar/quantumready-saas/internal/scoring"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// ScanPort is the only port scanned in this phase.
const ScanPort = 443

const maxBodyBytes = 1 << 10

// ScanStore is what the scan endpoints need from internal/store.
type ScanStore interface {
	// Every call names the tenant: the API never reads or writes scans
	// outside it (store.Store.Tx, row-level security).
	EnqueueOrReuse(ctx context.Context, t store.Tenant, r store.ScanRequest) (store.Scan, bool, error)
	ScanFor(ctx context.Context, t store.Tenant, id uuid.UUID) (store.Scan, error)
	FindingsFor(ctx context.Context, t store.Tenant, scanID uuid.UUID) ([]store.Finding, error)
	// OrgScans lists an organization's scans (history).
	OrgScans(ctx context.Context, t store.Tenant, limit int32) ([]store.Scan, error)
	// PublicScanBlocked: excluded or opted out (a yes or no, nothing else).
	PublicScanBlocked(ctx context.Context, host string) (bool, error)
	// RecentPublicScan: a public basic scan to reuse (pgx.ErrNoRows if none),
	// judged (or to be judged) with scoringVersion.
	RecentPublicScan(ctx context.Context, host string, port int32, cooldown, doneWithin time.Duration, scoringVersion string) (store.Scan, error)
	// PublicQueueLength and PublicQueuePosition count public scans only.
	PublicQueueLength(ctx context.Context) (int64, error)
	PublicQueuePosition(ctx context.Context, createdAt time.Time) (int64, error)
}

type createScanRequest struct {
	Host   string `json:"host"`
	Altcha string `json:"altcha,omitempty"` // proof of work, when public scanning is open
}

// listTargets: GET /api/targets: the demo hosts (the frontend shows them as
// buttons), whether any domain may be scanned, whether a proof of work is
// required and whether the demo request form is on.
// demoOn: the demo request form is served (the web hides it otherwise).
func (a *api) demoOn() bool { return a.cfg.Demos != nil && a.cfg.DemoEnabled }

func (a *api) listTargets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"hosts": a.cfg.Allowlist.Hosts(), "port": ScanPort,
		"open": a.cfg.PublicScanOpen, "proof": a.cfg.PublicScanOpen && a.cfg.Altcha != nil, "demo": a.demoOn(),
		"accounts": a.cfg.Accounts != nil})
}

// slot is one rate-limit reservation, given back if no scan is started.
type slot struct {
	lim *Limiter
	key string
}

func cancelAll(slots []slot) {
	for _, s := range slots {
		s.lim.Cancel(s.key)
	}
}

// createScan: POST /api/scans {"host": "...", "altcha": "..."}: a public,
// basic scan. Closed (default): only the allowlist. Open
// (PUBLIC_SCAN_OPEN): any domain, with a proof of work, unless excluded or
// opted out by its verified owner, within per-IP, per-domain and global
// limits. A public scan is never an organization's: the tenant is always
// store.Public.
func (a *api) createScan(w http.ResponseWriter, r *http.Request) {
	var req createScanRequest
	if !decodeJSON(w, r, maxBodyBytes, &req) {
		return
	}
	host, err := NormalizeHost(req.Host)
	if errors.Is(err, errPortNotAllowed) {
		writeError(w, r, http.StatusBadRequest, "port_not_allowed")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_host")
		return
	}
	open := a.cfg.PublicScanOpen && !a.cfg.Allowlist.Allows(host)
	if !a.cfg.PublicScanOpen && !a.cfg.Allowlist.Allows(host) {
		a.countBlocked(r, host) // M1; with open scanning there is no such 403

		writeError(w, r, http.StatusForbidden, "host_not_allowed")
		return
	}
	if open {
		if _, err := domains.Normalize(host); err != nil {
			code := "invalid_host"
			for e, c := range domainErrors {
				if errors.Is(err, e) {
					code = c
				}
			}
			writeError(w, r, http.StatusBadRequest, code)
			return
		}
	}
	if a.cfg.PublicScanOpen && a.cfg.Altcha != nil {
		if err := a.cfg.Altcha.Verify(req.Altcha); err != nil {
			writeError(w, r, http.StatusBadRequest, "proof_failed")
			return
		}
	}
	if open {
		// Excluded and opted-out domains get the same answer: nobody learns
		// which, nor whether an organization has the domain verified.
		blocked, err := a.cfg.Scans.PublicScanBlocked(r.Context(), host)
		if err != nil {
			a.internal(w, r, "public scan blocked", err)
			return
		}
		if blocked {
			writeError(w, r, http.StatusForbidden, "scan_not_allowed")
			return
		}
	}

	// Reuse before the limits: a recent public result costs nothing, so it is
	// served even when this client or this domain has used up its quota.
	// Read with the public tenant: an organization's scan is never handed out.
	if recent, err := a.cfg.Scans.RecentPublicScan(r.Context(), host, ScanPort, a.cfg.TargetCooldown, PublicReuseWindow, scoring.Version); err == nil {
		a.writeScan(w, r, recent, true)
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		a.internal(w, r, "recent public scan", err)
		return
	}

	// A full public queue (about five minutes of work) refuses new scans
	// before any quota is spent.
	if queued, err := a.cfg.Scans.PublicQueueLength(r.Context()); err != nil {
		a.internal(w, r, "public queue length", err)
		return
	} else if queued >= int64(a.cfg.PublicQueueMax) {
		w.Header().Set("Retry-After", "60")
		writeError(w, r, http.StatusTooManyRequests, "busy")
		return
	}

	var slots []slot
	take := func(lim *Limiter, key, code string) bool {
		ok, retry := lim.Reserve(key)
		if !ok {
			cancelAll(slots)
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			writeError(w, r, http.StatusTooManyRequests, code)
			return false
		}
		slots = append(slots, slot{lim, key})
		return true
	}
	if !take(a.cfg.Limiter, rateKey(clientAddr(r, a.cfg.TrustedProxies)), "rate_limited") {
		return
	}
	if open && (!take(a.cfg.PublicLimits.PerDomain, domains.Registrable(host), "domain_rate_limited") ||
		!take(a.cfg.PublicLimits.Global, "all", "busy")) {
		return
	}
	sc, reused, err := a.cfg.Scans.EnqueueOrReuse(r.Context(), store.Public,
		store.ScanRequest{Host: host, Port: ScanPort, Intensity: store.IntensityBasic, Cooldown: a.cfg.TargetCooldown, ScoringVersion: scoring.Version})
	if err != nil {
		cancelAll(slots)
		a.internal(w, r, "enqueue scan", err)
		return
	}
	if reused {
		cancelAll(slots) // a concurrent request enqueued it first: it does not count
	}
	a.writeScan(w, r, sc, reused)
}

// PublicReuseWindow is how recent a finished public result must be to be handed out
// instead of scanning again (and without spending any quota).
const PublicReuseWindow = time.Hour

// writeScan answers a public scan request: 202 for a new scan, 200 with
// reused and the result's age for an existing one.
func (a *api) writeScan(w http.ResponseWriter, r *http.Request, sc store.Scan, reused bool) {
	view, err := a.view(r, store.Public, sc)
	if err != nil {
		a.internal(w, r, "scan view", err)
		return
	}
	status := http.StatusAccepted
	if reused {
		status = http.StatusOK
		view.Reused = true
		since := sc.CreatedAt
		if sc.FinishedAt != nil {
			since = *sc.FinishedAt
		}
		age := max(0, int(time.Since(since).Seconds()))
		view.AgeSeconds = &age
	}
	w.Header().Set("Location", "/api/scans/"+sc.ID.String())
	writeView(w, status, view)
}

// getScan: GET /api/scans/{id}.
func (a *api) getScan(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found")
		return
	}
	sc, err := a.cfg.Scans.ScanFor(r.Context(), store.Public, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		a.internal(w, r, "get scan", err)
		return
	}
	view, err := a.view(r, store.Public, sc)
	if err != nil {
		a.internal(w, r, "scan view", err)
		return
	}
	writeView(w, http.StatusOK, view)
}

func (a *api) internal(w http.ResponseWriter, r *http.Request, what string, err error) {
	a.cfg.Logger.ErrorContext(r.Context(), "api: "+what, "err", err)
	writeError(w, r, http.StatusInternalServerError, "internal")
}

func writeView(w http.ResponseWriter, status int, v *scanView) {
	w.Header().Set("Content-Language", v.Lang)
	writeJSON(w, status, v)
}

// scanView is the public representation of a scan, texts in Lang.
type scanView struct {
	ID         uuid.UUID  `json:"id"`
	Host       string     `json:"host"`
	Port       int32      `json:"port"`
	Intensity  string     `json:"intensity,omitempty"` // basic (public) or full (verified domains)
	Status     string     `json:"status"`              // queued, running, done, failed
	Retries    int32      `json:"retries"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Error      *apiError  `json:"error,omitempty"`       // failed (or waiting to retry)
	Reused     bool       `json:"reused,omitempty"`      // POST returned a recent scan of the same target
	AgeSeconds *int       `json:"age_seconds,omitempty"` // with reused: how old the result is (since it finished)
	// A queued public scan: its place among public scans (1: next) and a
	// rough wait from the worker's pace. Organizations' scans are not counted.
	QueuePosition   *int64           `json:"queue_position,omitempty"`
	QueueETASeconds *int64           `json:"queue_eta_seconds,omitempty"`
	Lang            string           `json:"lang"`
	Report          *report.Document `json:"report,omitempty"` // done (internal/report)
}

func (a *api) view(r *http.Request, t store.Tenant, sc store.Scan) (*scanView, error) {
	lang := pickLang(r)
	v := &scanView{
		ID: sc.ID, Host: sc.Host, Port: sc.Port, Intensity: sc.Intensity, Status: sc.Status, Retries: sc.Retries,
		CreatedAt: sc.CreatedAt, StartedAt: sc.StartedAt, FinishedAt: sc.FinishedAt, Lang: lang,
	}
	if sc.Error != nil {
		v.Error = &apiError{Code: *sc.Error, Message: message(*sc.Error, lang)}
	}
	if sc.Status == "queued" && sc.OrganizationID == nil && t.OrgID == nil {
		pos, err := a.cfg.Scans.PublicQueuePosition(r.Context(), sc.CreatedAt)
		if err != nil {
			return nil, err
		}
		eta := (pos*60 + int64(a.cfg.ScansPerMinute) - 1) / int64(a.cfg.ScansPerMinute)
		v.QueuePosition, v.QueueETASeconds = &pos, &eta
	}
	if sc.Status != "done" {
		return v, nil
	}
	in, err := a.reportInput(r.Context(), t, sc, lang)
	if err != nil {
		return nil, err
	}
	if v.Report, err = report.Build(in); err != nil {
		return nil, err
	}
	return v, nil
}

// reportInput gathers what is stored about a finished scan for report.Build.
func (a *api) reportInput(ctx context.Context, t store.Tenant, sc store.Scan, lang string) (report.Input, error) {
	in := report.Input{Result: sc.Result, ScanTime: sc.CreatedAt, Lang: lang}
	if sc.StartedAt != nil {
		in.ScanTime = *sc.StartedAt
	}
	if err := json.Unmarshal(sc.Scoring, &in.Scoring); err != nil {
		return in, err
	}
	rows, err := a.cfg.Scans.FindingsFor(ctx, t, sc.ID)
	if err != nil {
		return in, err
	}
	in.Findings = make([]report.Finding, 0, len(rows))
	for _, f := range rows {
		var params map[string]string
		if err := json.Unmarshal(f.Params, &params); err != nil {
			return in, err
		}
		in.Findings = append(in.Findings, report.Finding{Code: f.Code, Axis: f.Axis, Severity: f.Severity, Category: f.Category, Params: params})
	}
	return in, nil
}

// altchaChallenge: GET /api/altcha, a proof-of-work challenge for POST
// /api/scans (open public scanning).
func (a *api) altchaChallenge(w http.ResponseWriter, r *http.Request) {
	if limited(w, r, a.cfg.PublicLimits.Challenge.Reserve, rateKey(clientAddr(r, a.cfg.TrustedProxies))) {
		return
	}
	c, err := a.cfg.Altcha.Challenge()
	if err != nil {
		a.internal(w, r, "altcha challenge", err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}
