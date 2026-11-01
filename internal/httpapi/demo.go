package httpapi

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/fizsar/quantumready-saas/internal/domains"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// "Request a free demo" (POST /api/demo): stored in demo_requests, kept 12
// months (cmd/api purges them), read with `qradmin demos`. Against spam: a
// honeypot field, a per-client limit in memory (no IP is stored) and, when
// ALTCHA_HMAC_KEY is set, a proof of work (GET /api/demo/challenge).

// DemoStore stores demo requests; *store.Store satisfies it.
type DemoStore interface {
	InsertDemoRequest(ctx context.Context, arg store.InsertDemoRequestParams) (store.InsertDemoRequestRow, error)
}

// DemoNotifier tells support about a new request. Nil: nobody is told (no
// mailer on this branch yet; DEMO_NOTIFY in cmd/api).
type DemoNotifier func(ctx context.Context, id string, req store.InsertDemoRequestParams) error

// PrivacyVersion identifies the /privacy text a sender accepts (stored with
// the request). Change it together with web/src/i18n.showcase.ts privacy.*.
const PrivacyVersion = "2026-10-20"

const (
	maxDemoBody    = 8 << 10
	maxDemoName    = 100
	maxDemoCompany = 100
	maxDemoMessage = 2000
)

type demoRequest struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Company string `json:"company"`
	Domain  string `json:"domain"`
	Message string `json:"message"`
	Privacy bool   `json:"privacy"`
	Lang    string `json:"lang"`
	Altcha  string `json:"altcha"`
	// Website is the honeypot: hidden from people, filled in by bots.
	Website string `json:"website"`
}

// createDemo: POST /api/demo.
func (a *api) createDemo(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeError(w, r, http.StatusForbidden, "bad_origin")
		return
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type")
		return
	}
	if ok, retry := a.demoLimiter.Reserve(rateKey(clientAddr(r, a.cfg.TrustedProxies))); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeError(w, r, http.StatusTooManyRequests, "too_many_requests")
		return
	}
	var req demoRequest
	if err := decodeStrict(http.MaxBytesReader(w, r.Body, maxDemoBody), &req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, r, http.StatusRequestEntityTooLarge, "body_too_large")
		} else {
			writeError(w, r, http.StatusBadRequest, "invalid_json")
		}
		return
	}
	// A bot filled in the hidden field: answer as if it worked, keep nothing.
	if strings.TrimSpace(req.Website) != "" {
		a.cfg.Logger.Info("demo: honeypot filled, request dropped")
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "received"})
		return
	}
	if a.cfg.DemoAltcha != nil {
		if err := a.cfg.DemoAltcha.Verify(req.Altcha); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_proof")
			return
		}
	}
	p, code := validDemo(req, pickLang(r))
	if code != "" {
		writeError(w, r, http.StatusBadRequest, code)
		return
	}
	row, err := a.cfg.Demos.InsertDemoRequest(r.Context(), p)
	if err != nil {
		a.internal(w, r, "demo: store", err)
		return
	}
	if a.cfg.DemoNotify != nil {
		if err := a.cfg.DemoNotify(r.Context(), row.ID.String(), p); err != nil {
			a.cfg.Logger.Warn("demo: notify", "id", row.ID, "err", err)
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "received"})
}

// validDemo checks and normalizes a request; code is the error, if any.
func validDemo(req demoRequest, fallbackLang string) (store.InsertDemoRequestParams, string) {
	p := store.InsertDemoRequestParams{PrivacyVersion: PrivacyVersion}
	if !req.Privacy {
		return p, "privacy_required"
	}
	var ok bool
	if p.Name, ok = cleanLine(req.Name, maxDemoName); !ok {
		return p, "invalid_name"
	}
	if p.Company, ok = cleanLine(req.Company, maxDemoCompany); !ok {
		return p, "invalid_company"
	}
	email := strings.TrimSpace(req.Email)
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || addr.Name != "" || len(email) > 254 || !strings.Contains(email[strings.LastIndexByte(email, '@')+1:], ".") {
		return p, "invalid_email"
	}
	p.Email = email
	d, err := domains.Normalize(strings.TrimSpace(req.Domain))
	if err != nil {
		return p, "invalid_domain"
	}
	p.Domain = d
	msg := strings.TrimSpace(req.Message)
	if utf8.RuneCountInString(msg) > maxDemoMessage || !utf8.ValidString(msg) || strings.ContainsFunc(msg, func(c rune) bool {
		return unicode.IsControl(c) && c != '\n' && c != '\r' && c != '\t'
	}) {
		return p, "invalid_message"
	}
	p.Message = msg
	p.Lang = fallbackLang // the language support should answer in
	if req.Lang == "es" || req.Lang == "en" {
		p.Lang = req.Lang
	}
	return p, ""
}

// cleanLine trims s and accepts 1..max characters of valid UTF-8 without
// control characters (one line: no newlines in a name).
func cleanLine(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n == 0 || n > max || !utf8.ValidString(s) || strings.ContainsFunc(s, unicode.IsControl) {
		return "", false
	}
	return s, true
}

// demoChallenge: GET /api/demo/challenge, the proof of work for POST
// /api/demo (only when ALTCHA_HMAC_KEY is set; otherwise 404).
func (a *api) demoChallenge(w http.ResponseWriter, r *http.Request) {
	if ok, retry := a.demoChallengeLimiter.Reserve(rateKey(clientAddr(r, a.cfg.TrustedProxies))); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeError(w, r, http.StatusTooManyRequests, "too_many_requests")
		return
	}
	c, err := a.cfg.DemoAltcha.Challenge()
	if err != nil {
		a.internal(w, r, "demo: altcha challenge", err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}
