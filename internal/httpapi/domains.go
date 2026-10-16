package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/domains"
	"github.com/fizsar/quantumready-saas/internal/org"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// DomainStore is what the domain endpoints need from internal/store.
type DomainStore interface {
	AddDomain(ctx context.Context, t store.Tenant, name, token string, maxDomains int) (store.Domain, error)
	Domains(ctx context.Context, t store.Tenant) ([]store.Domain, error)
	DomainFor(ctx context.Context, t store.Tenant, id uuid.UUID) (store.Domain, error)
	DeleteDomain(ctx context.Context, t store.Tenant, id uuid.UUID) error
	RequestDomainCheck(ctx context.Context, t store.Tenant, id uuid.UUID) (store.Domain, error)
}

const maxDomainsPerOrg = 20

// Audit actions.
const (
	auditDomainAdded   = "domain.added"
	auditDomainDeleted = "domain.deleted"
	auditDomainCheck   = "domain.check_requested"
)

type txtRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type domainView struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`   // pending, verified, lost
	Verified       bool       `json:"verified"` // full scans allowed now (lost: during the grace period)
	VerifiedAt     *time.Time `json:"verified_at,omitempty"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	LostAt         *time.Time `json:"lost_at,omitempty"`
	GraceUntil     *time.Time `json:"grace_until,omitempty"`
	CheckPending   bool       `json:"check_pending"`
	LastCheckError *apiError  `json:"last_check_error,omitempty"`
	Record         txtRecord  `json:"record"`
}

func viewDomain(r *http.Request, d store.Domain) domainView {
	v := domainView{ID: d.ID, Name: d.Name, Status: d.Status, Verified: d.Verified(time.Now()), VerifiedAt: d.VerifiedAt,
		LastCheckedAt: d.LastCheckedAt, LostAt: d.LostAt, CheckPending: d.CheckRequestedAt != nil,
		Record: txtRecord{Type: "TXT", Name: domains.RecordName(d.Name), Value: domains.RecordValue(d.Token)}}
	if d.LostAt != nil {
		g := d.LostAt.Add(store.DomainGrace)
		v.GraceUntil = &g
	}
	if d.LastCheckError != nil {
		lang := pickLang(r)
		v.LastCheckError = &apiError{Code: *d.LastCheckError, Message: message(*d.LastCheckError, lang)}
	}
	return v
}

// newDomainToken: 128 random bits, lower-case base32 (26 characters).
func newDomainToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)), nil
}

var domainErrors = map[error]string{
	domains.ErrInvalid:      "invalid_domain",
	domains.ErrIP:           "domain_ip",
	domains.ErrSpecialUse:   "domain_reserved",
	domains.ErrUnknownTLD:   "domain_unknown_tld",
	domains.ErrPublicSuffix: "domain_public_suffix",
}

type addDomainRequest struct {
	Name string `json:"name"`
}

// addDomain: POST /api/org/domains {"name"}.
func (a *api) addDomain(w http.ResponseWriter, r *http.Request) {
	var req addDomainRequest
	if !decodeJSON(w, r, authBodyBytes, &req) || !allow(w, r, org.ManageDomains) {
		return
	}
	name, err := domains.Normalize(req.Name)
	if err != nil {
		code := "invalid_domain"
		for e, c := range domainErrors {
			if errors.Is(err, e) {
				code = c
			}
		}
		writeError(w, r, http.StatusBadRequest, code)
		return
	}
	token, err := newDomainToken()
	if err != nil {
		a.internal(w, r, "token", err)
		return
	}
	d, err := a.cfg.Domains.AddDomain(r.Context(), tenant(r), name, token, maxDomainsPerOrg)
	switch {
	case errors.Is(err, store.ErrDomainExists):
		writeError(w, r, http.StatusConflict, "domain_exists")
		return
	case errors.Is(err, store.ErrLimit):
		writeError(w, r, http.StatusConflict, "limit_reached")
		return
	case err != nil:
		a.internal(w, r, "add domain", err)
		return
	}
	a.orgAudit(r, auditDomainAdded, &d.Name)
	w.Header().Set("Location", "/api/org/domains/"+d.ID.String())
	writeJSON(w, http.StatusCreated, viewDomain(r, d))
}

// listDomains: GET /api/org/domains.
func (a *api) listDomains(w http.ResponseWriter, r *http.Request) {
	if !allow(w, r, org.View) {
		return
	}
	ds, err := a.cfg.Domains.Domains(r.Context(), tenant(r))
	if err != nil {
		a.internal(w, r, "list domains", err)
		return
	}
	out := []domainView{}
	for _, d := range ds {
		out = append(out, viewDomain(r, d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": out})
}

// getDomain: GET /api/org/domains/{domainID}.
func (a *api) getDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "domainID")
	if !ok || !allow(w, r, org.View) {
		return
	}
	d, err := a.cfg.Domains.DomainFor(r.Context(), tenant(r), id)
	if a.domainError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, viewDomain(r, d))
}

// deleteDomain: DELETE /api/org/domains/{domainID}. Its scans stay in the history.
func (a *api) deleteDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "domainID")
	if !ok || !allow(w, r, org.ManageDomains) {
		return
	}
	d, err := a.cfg.Domains.DomainFor(r.Context(), tenant(r), id)
	if a.domainError(w, r, err) {
		return
	}
	if a.domainError(w, r, a.cfg.Domains.DeleteDomain(r.Context(), tenant(r), id)) {
		return
	}
	a.orgAudit(r, auditDomainDeleted, &d.Name)
	w.WriteHeader(http.StatusNoContent)
}

// verifyDomain: POST /api/org/domains/{domainID}/verify. The worker checks
// the TXT record within seconds; poll GET /api/org/domains/{domainID}.
func (a *api) verifyDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "domainID")
	if !ok || !allow(w, r, org.ManageDomains) {
		return
	}
	if limited(w, r, a.cfg.AuthLimits.DomainChecks.Reserve, id.String()) {
		return
	}
	d, err := a.cfg.Domains.RequestDomainCheck(r.Context(), tenant(r), id)
	if a.domainError(w, r, err) {
		return
	}
	a.orgAudit(r, auditDomainCheck, &d.Name)
	writeJSON(w, http.StatusAccepted, viewDomain(r, d))
}

func (a *api) domainError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, store.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "not_found")
	default:
		a.internal(w, r, "domain", err)
	}
	return true
}
