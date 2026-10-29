package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/fizsar/quantumready-saas/internal/domains"
	"github.com/fizsar/quantumready-saas/internal/org"
	"github.com/fizsar/quantumready-saas/internal/scoring"
	"github.com/fizsar/quantumready-saas/internal/store"
)

const orgScanListLimit = 200

// Audit action.
const auditFullScan = "scan.full_requested"

// createOrgScan: POST /api/org/scans {"host"}: a full scan of a verified
// domain of the active organization (or a subdomain of one). The allowlist
// of public scans does not apply: verification proves control.
func (a *api) createOrgScan(w http.ResponseWriter, r *http.Request) {
	var req createScanRequest
	if !decodeJSON(w, r, maxBodyBytes, &req) || !allow(w, r, org.ManageDomains) {
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
	t := tenant(r)
	ds, err := a.cfg.Domains.Domains(r.Context(), t)
	if err != nil {
		a.internal(w, r, "list domains", err)
		return
	}
	covered, now := false, time.Now()
	for _, d := range ds {
		if d.Verified(now) && domains.Covers(d.Name, host) {
			covered = true
		}
	}
	if !covered {
		writeError(w, r, http.StatusForbidden, "domain_not_verified")
		return
	}
	key := member(r).OrgID.String()
	if limited(w, r, a.cfg.AuthLimits.OrgScans.Reserve, key) {
		return
	}
	sc, reused, err := a.cfg.Scans.EnqueueOrReuse(r.Context(), t,
		store.ScanRequest{Host: host, Port: ScanPort, Intensity: store.IntensityFull, Cooldown: a.cfg.TargetCooldown, ScoringVersion: scoring.Version})
	if err != nil {
		a.cfg.AuthLimits.OrgScans.Cancel(key)
		a.internal(w, r, "enqueue scan", err)
		return
	}
	status := http.StatusAccepted
	if reused {
		a.cfg.AuthLimits.OrgScans.Cancel(key)
		status = http.StatusOK
	} else {
		a.orgAudit(r, auditFullScan, &host)
	}
	view, err := a.view(r, t, sc)
	if err != nil {
		a.internal(w, r, "scan view", err)
		return
	}
	view.Reused = reused
	w.Header().Set("Location", "/api/org/scans/"+sc.ID.String())
	writeView(w, status, view)
}

type scanSummary struct {
	ID         uuid.UUID  `json:"id"`
	Host       string     `json:"host"`
	Status     string     `json:"status"`
	Intensity  string     `json:"intensity"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Score      *int       `json:"score,omitempty"`
	Grade      string     `json:"grade,omitempty"`
}

// listOrgScans: GET /api/org/scans, the organization's history.
func (a *api) listOrgScans(w http.ResponseWriter, r *http.Request) {
	if !allow(w, r, org.View) {
		return
	}
	scs, err := a.cfg.Scans.OrgScans(r.Context(), tenant(r), orgScanListLimit)
	if err != nil {
		a.internal(w, r, "list scans", err)
		return
	}
	out := []scanSummary{}
	for _, sc := range scs {
		s := scanSummary{ID: sc.ID, Host: sc.Host, Status: sc.Status, Intensity: sc.Intensity, CreatedAt: sc.CreatedAt, FinishedAt: sc.FinishedAt}
		var res struct {
			Score int    `json:"score"`
			Grade string `json:"grade"`
		}
		if sc.Status == "done" && json.Unmarshal(sc.Scoring, &res) == nil {
			s.Score, s.Grade = &res.Score, res.Grade
		}
		out = append(out, s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"scans": out})
}

// getOrgScan: GET /api/org/scans/{scanID}: one of the organization's own
// scans (public scans have their own URL).
func (a *api) getOrgScan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "scanID")
	if !ok || !allow(w, r, org.View) {
		return
	}
	t := tenant(r)
	sc, err := a.cfg.Scans.ScanFor(r.Context(), t, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (sc.OrganizationID == nil || *sc.OrganizationID != *t.OrgID)) {
		writeError(w, r, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		a.internal(w, r, "get scan", err)
		return
	}
	view, err := a.view(r, t, sc)
	if err != nil {
		a.internal(w, r, "scan view", err)
		return
	}
	writeView(w, http.StatusOK, view)
}
