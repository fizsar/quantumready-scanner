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
	// RecentPublicScan: a public basic scan to reuse (pgx.ErrNoRows if none).
	RecentPublicScan(ctx context.Context, host string, port int32, cooldown, doneWithin time.Duration) (store.Scan, error)
	// PublicQueueLength and PublicQueuePosition count public scans only.
	PublicQueueLength(ctx context.Context) (int64, error)
	PublicQueuePosition(ctx context.Context, createdAt time.Time) (int64, error)
}

type createScanRequest struct {
	Host   string `json:"host"`
	Altcha string `json:"altcha,omitempty"` // proof of work, when public scanning is open
}

// listTargets: GET /api/targets: the demo hosts (the frontend shows them as
// buttons), whether any domain may be scanned and whether a proof of work
// is required.
func (a *api) listTargets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"hosts": a.cfg.Allowlist.Hosts(), "port": ScanPort,
		"open": a.cfg.PublicScanOpen, "proof": a.cfg.PublicScanOpen && a.cfg.Altcha != nil})
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
	if recent, err := a.cfg.Scans.RecentPublicScan(r.Context(), host, ScanPort, a.cfg.TargetCooldown, PublicReuseWindow); err == nil {
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
		store.ScanRequest{Host: host, Port: ScanPort, Intensity: store.IntensityBasic, Cooldown: a.cfg.TargetCooldown})
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
	QueuePosition   *int64  `json:"queue_position,omitempty"`
	QueueETASeconds *int64  `json:"queue_eta_seconds,omitempty"`
	Lang            string  `json:"lang"`
	Report          *report `json:"report,omitempty"` // done
}

type report struct {
	ScoringVersion string          `json:"scoring_version"`
	Score          int             `json:"score"`
	Grade          string          `json:"grade"`
	ScoreGrade     string          `json:"score_grade"`
	FloorPoints    int             `json:"floor_points,omitempty"`
	Cap            *capView        `json:"cap,omitempty"`
	PQC            bool            `json:"pqc"`
	SelectedGroup  string          `json:"selected_group,omitempty"`
	TLSVersion     string          `json:"tls_version,omitempty"`
	QuantumRisk    labeled         `json:"quantum_risk"`
	CurrentRisk    labeled         `json:"current_risk"`
	Blocks         []blockView     `json:"blocks"`
	Findings       []findingView   `json:"findings"`
	NotEvaluated   []labeled       `json:"not_evaluated"`
	Result         json.RawMessage `json:"result"` // raw scanner facts
}

type labeled struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type capView struct {
	Grade string    `json:"grade"`
	Codes []labeled `json:"findings"`
}

type blockView struct {
	ID     string     `json:"id"`
	Label  string     `json:"label"`
	Points int        `json:"points"`
	Max    int        `json:"max"`
	Items  []itemView `json:"items,omitempty"`
}

type itemView struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Points int    `json:"points"`
	Max    int    `json:"max"`
}

type findingView struct {
	Code          string `json:"code"`
	Axis          string `json:"axis"`
	AxisLabel     string `json:"axis_label"`
	Severity      string `json:"severity"`
	SeverityLabel string `json:"severity_label"`
	Category      string `json:"category"`
	scoring.Text
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

	var s scoring.Result
	if err := json.Unmarshal(sc.Scoring, &s); err != nil {
		return nil, err
	}
	var facts struct {
		Default struct {
			Version string `json:"version"`
		} `json:"default"`
		KeyExchange struct {
			Selected string `json:"selected"`
		} `json:"key_exchange"`
	}
	if err := json.Unmarshal(sc.Result, &facts); err != nil {
		return nil, err
	}
	rows, err := a.cfg.Scans.FindingsFor(r.Context(), t, sc.ID)
	if err != nil {
		return nil, err
	}

	rep := &report{
		ScoringVersion: s.ScoringVersion, Score: s.Score, Grade: s.Grade, ScoreGrade: s.ScoreGrade,
		FloorPoints: s.FloorPoints, PQC: s.PQC,
		SelectedGroup: facts.KeyExchange.Selected, TLSVersion: facts.Default.Version,
		QuantumRisk: labeled{s.QuantumRisk, scoring.Label("severity."+s.QuantumRisk, lang)},
		CurrentRisk: labeled{s.CurrentRisk, scoring.Label("severity."+s.CurrentRisk, lang)},
		Result:      sc.Result,
	}
	// Finding parameters by code: some titles use them, e.g. "({bits} bits)".
	params := make(map[string]map[string]string, len(rows))
	// Some texts depend on the scan: its date and its other findings.
	tctx := scoring.Context{ScanTime: sc.CreatedAt}
	if sc.StartedAt != nil {
		tctx.ScanTime = *sc.StartedAt
	}
	for _, f := range rows {
		var p map[string]string
		if err := json.Unmarshal(f.Params, &p); err != nil {
			return nil, err
		}
		params[f.Code] = p
		tctx.Codes = append(tctx.Codes, f.Code)
	}
	if s.Cap != nil {
		rep.Cap = &capView{Grade: s.Cap.Grade}
		for _, c := range s.Cap.Codes {
			rep.Cap.Codes = append(rep.Cap.Codes, labeled{c, scoring.DescribeIn(c, params[c], lang, tctx).Title})
		}
	}
	for _, b := range s.Blocks {
		bv := blockView{ID: b.ID, Label: scoring.Label("block."+b.ID, lang), Points: b.Points, Max: b.Max}
		for _, it := range b.Items {
			bv.Items = append(bv.Items, itemView{it.ID, scoring.Label("item."+it.ID, lang), it.Points, it.Max})
		}
		rep.Blocks = append(rep.Blocks, bv)
	}
	rep.Findings = make([]findingView, 0, len(rows))
	for _, f := range rows {
		var params map[string]string
		if err := json.Unmarshal(f.Params, &params); err != nil {
			return nil, err
		}
		rep.Findings = append(rep.Findings, findingView{
			Code: f.Code, Axis: f.Axis, AxisLabel: scoring.Label("axis."+f.Axis, lang),
			Severity: f.Severity, SeverityLabel: scoring.Label("severity."+f.Severity, lang),
			Category: f.Category, Text: scoring.DescribeIn(f.Code, params, lang, tctx),
		})
	}
	rep.NotEvaluated = make([]labeled, 0, len(s.NotEvaluated))
	for _, c := range s.NotEvaluated {
		rep.NotEvaluated = append(rep.NotEvaluated, labeled{c, scoring.Label("noteval."+c, lang)})
	}
	v.Report = rep
	return v, nil
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
