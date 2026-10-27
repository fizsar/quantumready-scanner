package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/fizsar/quantumready-saas/internal/report"
	"github.com/fizsar/quantumready-saas/internal/scoring"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// ScanPort is the only port scanned in this phase.
const ScanPort = 443

const maxBodyBytes = 1 << 10

// ScanStore is what the scan endpoints need from internal/store.
type ScanStore interface {
	// EnqueueOrReuse reuses a recent scan only if judged (or to be judged)
	// with scoringVersion.
	EnqueueOrReuse(ctx context.Context, host string, port int32, cooldown time.Duration, scoringVersion string) (store.Scan, bool, error)
	GetScan(ctx context.Context, id uuid.UUID) (store.Scan, error)
	ListFindings(ctx context.Context, scanID uuid.UUID) ([]store.Finding, error)
}

type createScanRequest struct {
	Host string `json:"host"`
}

// listTargets: GET /api/targets, the hosts that may be scanned (the
// frontend shows them as buttons instead of keeping its own copy), and
// whether the demo request form is on.
// demoOn: the demo request form is served (the web hides it otherwise).
func (a *api) demoOn() bool { return a.cfg.Demos != nil && a.cfg.DemoEnabled }

func (a *api) listTargets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"hosts": a.cfg.Allowlist.Hosts(), "port": ScanPort, "demo": a.demoOn()})
}

// createScan: POST /api/scans {"host": "..."}.
func (a *api) createScan(w http.ResponseWriter, r *http.Request) {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		// Also keeps cross-site HTML forms out: they cannot send JSON.
		writeError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type")
		return
	}
	var req createScanRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, r, http.StatusRequestEntityTooLarge, "body_too_large")
		} else {
			writeError(w, r, http.StatusBadRequest, "invalid_json")
		}
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
	if !a.cfg.Allowlist.Allows(host) {
		a.countBlocked(r, host)
		writeError(w, r, http.StatusForbidden, "host_not_allowed")
		return
	}

	key := rateKey(clientAddr(r, a.cfg.TrustedProxies))
	if ok, retry := a.cfg.Limiter.Reserve(key); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeError(w, r, http.StatusTooManyRequests, "rate_limited")
		return
	}
	sc, reused, err := a.cfg.Scans.EnqueueOrReuse(r.Context(), host, ScanPort, a.cfg.TargetCooldown, scoring.Version)
	if err != nil {
		a.cfg.Limiter.Cancel(key)
		a.internal(w, r, "enqueue scan", err)
		return
	}
	if reused {
		a.cfg.Limiter.Cancel(key) // no new scan: it does not count
	}

	view, err := a.view(r, sc)
	if err != nil {
		a.internal(w, r, "scan view", err)
		return
	}
	view.Reused = reused
	status := http.StatusAccepted
	if reused {
		status = http.StatusOK
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
	sc, err := a.cfg.Scans.GetScan(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		a.internal(w, r, "get scan", err)
		return
	}
	view, err := a.view(r, sc)
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
	ID         uuid.UUID        `json:"id"`
	Host       string           `json:"host"`
	Port       int32            `json:"port"`
	Status     string           `json:"status"` // queued, running, done, failed
	Retries    int32            `json:"retries"`
	CreatedAt  time.Time        `json:"created_at"`
	StartedAt  *time.Time       `json:"started_at,omitempty"`
	FinishedAt *time.Time       `json:"finished_at,omitempty"`
	Error      *apiError        `json:"error,omitempty"`  // failed (or waiting to retry)
	Reused     bool             `json:"reused,omitempty"` // POST returned a recent scan of the same target
	Lang       string           `json:"lang"`
	Report     *report.Document `json:"report,omitempty"` // done (internal/report)
}

func (a *api) view(r *http.Request, sc store.Scan) (*scanView, error) {
	lang := pickLang(r)
	v := &scanView{
		ID: sc.ID, Host: sc.Host, Port: sc.Port, Status: sc.Status, Retries: sc.Retries,
		CreatedAt: sc.CreatedAt, StartedAt: sc.StartedAt, FinishedAt: sc.FinishedAt, Lang: lang,
	}
	if sc.Error != nil {
		v.Error = &apiError{Code: *sc.Error, Message: message(*sc.Error, lang)}
	}
	if sc.Status != "done" {
		return v, nil
	}
	in, err := a.reportInput(r.Context(), sc, lang)
	if err != nil {
		return nil, err
	}
	if v.Report, err = report.Build(in); err != nil {
		return nil, err
	}
	return v, nil
}

// reportInput gathers what is stored about a finished scan for report.Build.
func (a *api) reportInput(ctx context.Context, sc store.Scan, lang string) (report.Input, error) {
	in := report.Input{Result: sc.Result, ScanTime: sc.CreatedAt, Lang: lang}
	if sc.StartedAt != nil {
		in.ScanTime = *sc.StartedAt
	}
	if err := json.Unmarshal(sc.Scoring, &in.Scoring); err != nil {
		return in, err
	}
	rows, err := a.cfg.Scans.ListFindings(ctx, sc.ID)
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
