package httpapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	netmail "net/mail"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/auth"
	"github.com/fizsar/quantumready-saas/internal/mail"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// AccountStore is what the account endpoints need from internal/store.
type AccountStore interface {
	Register(ctx context.Context, r store.Registration) (store.User, store.Organization, error)
	UserByEmail(ctx context.Context, email string) (store.User, error)
	EnqueueEmail(ctx context.Context, e store.Email) error
	SendToken(ctx context.Context, userID uuid.UUID, purpose string, hash []byte, ttl time.Duration, e store.Email) error
	VerifyEmail(ctx context.Context, hash []byte) (store.User, error)
	ResetPassword(ctx context.Context, hash []byte, prepare func(store.User) (string, store.Email, error)) (store.User, error)
	ChangePassword(ctx context.Context, userID, keepSession uuid.UUID, newHash string, e store.Email) error
	UpdatePasswordHash(ctx context.Context, userID uuid.UUID, newHash string) error
	StartSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, csrf string, ttl time.Duration) (store.Session, error)
	SessionByToken(ctx context.Context, hash []byte, idle, touchAfter time.Duration) (store.GetSessionByTokenRow, error)
	EndSession(ctx context.Context, userID, sessionID uuid.UUID) error
	UserOrganizations(ctx context.Context, userID uuid.UUID) ([]store.ListUserOrganizationsRow, error)
	Audit(ctx context.Context, orgID, userID *uuid.UUID, action string, target, ipPrefix *string) error
}

// AuthLimits are the brute-force and email-flood limits. Nil fields take
// the defaults of NewRouter.
type AuthLimits struct {
	LoginIP       *Limiter // login attempts per client: 10/min, 50/day
	LoginFailures *Limiter // failed passwords per account email: 5 per 15 min
	EmailIP       *Limiter // register, verify, resend, forgot, reset per client: 5/min, 30/day
	PerAddress    *Limiter // emails sent to one address: 3/hour, silently skipped beyond
}

// Session cookie and lifetimes.
const (
	sessionCookie = "__Host-qr_session"
	sessionTTL    = 30 * 24 * time.Hour
	sessionIdle   = 7 * 24 * time.Hour
	sessionTouch  = 5 * time.Minute
	verifyTTL     = 24 * time.Hour
	resetTTL      = 30 * time.Minute
	csrfHeader    = "X-QR-CSRF"
	authBodyBytes = 4 << 10
	asyncTimeout  = 15 * time.Second
)

// Audit actions.
const (
	auditRegister       = "account.register"
	auditVerify         = "account.verify_email"
	auditLogin          = "account.login"
	auditLoginFailed    = "account.login_failed"
	auditLogout         = "account.logout"
	auditResetRequested = "account.reset_requested"
	auditReset          = "account.password_reset"
	auditChanged        = "account.password_changed"
)

func (a *api) authDefaults() {
	l := &a.cfg.AuthLimits
	if l.LoginIP == nil {
		l.LoginIP = NewLimiter(10, 50)
	}
	if l.LoginFailures == nil {
		l.LoginFailures = NewWindowLimiter(5, 15*time.Minute)
	}
	if l.EmailIP == nil {
		l.EmailIP = NewLimiter(5, 30)
	}
	if l.PerAddress == nil {
		l.PerAddress = NewWindowLimiter(3, time.Hour)
	}
	if a.cfg.Async == nil {
		a.cfg.Async = func(f func()) { go f() }
	}
	if a.cfg.Support == "" {
		a.cfg.Support = "soporte@fizsar.com"
	}
}

// --- Origin, session and CSRF middleware ---

// checkOrigin refuses state-changing requests that a browser marks as coming
// from another site. Requests without these headers (curl, scripts) carry no
// ambient cookies of a victim, so they are not CSRF.
func checkOrigin(publicOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				site := r.Header.Get("Sec-Fetch-Site")
				origin := r.Header.Get("Origin")
				if (site != "" && site != "same-origin" && site != "none") || (origin != "" && origin != publicOrigin) {
					writeError(w, r, http.StatusForbidden, "bad_origin")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

type sessionKey struct{}

// session is the logged-in user of the request, or nil.
func session(r *http.Request) *store.GetSessionByTokenRow {
	s, _ := r.Context().Value(sessionKey{}).(*store.GetSessionByTokenRow)
	return s
}

// loadSession attaches the session of the cookie, if live. A dead cookie is
// cleared and the request goes on anonymous.
func (a *api) loadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		s, err := a.cfg.Accounts.SessionByToken(r.Context(), auth.HashToken(c.Value), sessionIdle, sessionTouch)
		if errors.Is(err, store.ErrSessionExpired) {
			clearSessionCookie(w)
			next.ServeHTTP(w, r)
			return
		}
		if err != nil {
			a.internal(w, r, "load session", err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, &s)))
	})
}

// requireSession answers 401 without a session and, for state-changing
// methods, 403 unless the X-QR-CSRF header carries the session's token.
func requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := session(r)
		if s == nil {
			writeError(w, r, http.StatusUnauthorized, "unauthenticated")
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if subtle.ConstantTimeCompare([]byte(r.Header.Get(csrfHeader)), []byte(s.CsrfToken)) != 1 {
				writeError(w, r, http.StatusForbidden, "csrf_failed")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

// --- Helpers ---

// normalizeEmail lowercases and checks a bare address (no display name).
func normalizeEmail(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) < 3 || len(s) > 254 || strings.ContainsAny(s, "\r\n") {
		return "", false
	}
	a, err := netmail.ParseAddress(s)
	if err != nil || a.Name != "" || a.Address != s {
		return "", false
	}
	at := strings.LastIndexByte(s, '@')
	if at < 1 || !strings.Contains(s[at+1:], ".") {
		return "", false
	}
	return s, true
}

// cleanText trims s and checks it has at most max runes and no control
// characters.
func cleanText(s string, maxRunes int) (string, bool) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxRunes || !utf8.ValidString(s) {
		return "", false
	}
	for _, c := range s {
		if unicode.IsControl(c) {
			return "", false
		}
	}
	return s, true
}

// ipPrefix truncates the client address for the audit log: /24 (IPv4) or
// /48 (IPv6).
func ipPrefix(a netip.Addr) *string {
	if !a.IsValid() {
		return nil
	}
	bits := 24
	if a.Is6() {
		bits = 48
	}
	p, _ := a.Prefix(bits)
	s := p.String()
	return &s
}

func (a *api) audit(r *http.Request, orgID, userID *uuid.UUID, action string) {
	ip := ipPrefix(clientAddr(r, a.cfg.TrustedProxies))
	if err := a.cfg.Accounts.Audit(r.Context(), orgID, userID, action, nil, ip); err != nil {
		a.cfg.Logger.ErrorContext(r.Context(), "api: audit", "action", action, "err", err)
	}
}

// limited answers 429 when take (a Limiter's Reserve or Check) has no slot
// for key.
func limited(w http.ResponseWriter, r *http.Request, take func(string) (bool, time.Duration), key string) bool {
	ok, retry := take(key)
	if ok {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	writeError(w, r, http.StatusTooManyRequests, "too_many_attempts")
	return true
}

// later runs f after the response, so that its duration (does the account
// exist? is an email queued?) does not show in the response time.
func (a *api) later(r *http.Request, what string, f func(ctx context.Context) error) {
	ctx := context.WithoutCancel(r.Context())
	a.cfg.Async(func() {
		ctx, cancel := context.WithTimeout(ctx, asyncTimeout)
		defer cancel()
		if err := f(ctx); err != nil {
			a.cfg.Logger.ErrorContext(ctx, "api: "+what, "err", err)
		}
	})
}

func (a *api) render(template, lang, to, link string, validFor time.Duration) (store.Email, error) {
	return a.renderData(template, lang, to, validFor, mail.Data{Link: link})
}

// renderData fills d with the validity, site and support address, and renders.
func (a *api) renderData(template, lang, to string, validFor time.Duration, d mail.Data) (store.Email, error) {
	const day = 24 * time.Hour
	switch {
	case validFor > day && validFor%day == 0:
		d.ValidFor = strconv.Itoa(int(validFor/day)) + map[string]string{"es": " días", "en": " days"}[lang]
	case validFor >= time.Hour:
		d.ValidFor = mail.ValidFor(lang, int(validFor.Hours()), 0)
	case validFor > 0:
		d.ValidFor = mail.ValidFor(lang, 0, int(validFor.Minutes()))
	}
	d.SiteURL, d.Support = a.cfg.PublicOrigin, a.cfg.Support
	return mail.Render(template, lang, to, d)
}

// link builds an email link. The token goes in the fragment, which browsers
// never send: it stays out of proxy logs and Referer headers.
func (a *api) link(page, token string) string {
	return a.cfg.PublicOrigin + "/" + page + "#token=" + token
}

func passwordError(err error) (string, bool) {
	for _, e := range []error{auth.ErrPasswordTooShort, auth.ErrPasswordTooLong, auth.ErrPasswordCommon, auth.ErrPasswordEmail} {
		if errors.Is(err, e) {
			return e.Error(), true
		}
	}
	return "", false
}

func requestLang(r *http.Request, asked string) string {
	if asked == "es" || asked == "en" {
		return asked
	}
	return pickLang(r)
}

// --- Views ---

type userView struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  *string   `json:"name,omitempty"`
	Lang  string    `json:"lang"`
}

type orgView struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Role string    `json:"role"`
}

type meView struct {
	User          userView   `json:"user"`
	Organizations []orgView  `json:"organizations"`
	ActiveOrgID   *uuid.UUID `json:"active_org_id"`
	CSRFToken     string     `json:"csrf_token"`
}

type statusView struct {
	Status string `json:"status"`
}

func (a *api) writeMe(w http.ResponseWriter, r *http.Request, u userView, activeOrg *uuid.UUID, csrf string) {
	orgs, err := a.cfg.Accounts.UserOrganizations(r.Context(), u.ID)
	if err != nil {
		a.internal(w, r, "list organizations", err)
		return
	}
	v := meView{User: u, Organizations: []orgView{}, ActiveOrgID: activeOrg, CSRFToken: csrf}
	for _, o := range orgs {
		v.Organizations = append(v.Organizations, orgView{ID: o.ID, Name: o.Name, Role: o.Role})
	}
	writeJSON(w, http.StatusOK, v)
}

// --- Handlers ---

type registerRequest struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	Name         string `json:"name"`
	Organization string `json:"organization"`
	Lang         string `json:"lang"`
	SignupCode   string `json:"signup_code"`
	// InvitationToken replaces the signup code: the user joins the inviting
	// organization (no own one) and the address counts as verified.
	InvitationToken string `json:"invitation_token"`
}

// register: POST /api/auth/register. A taken email gets the same 202 as a
// new one (the owner receives an "account exists" email instead).
func (a *api) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeJSON(w, r, authBodyBytes, &req) {
		return
	}
	if limited(w, r, a.cfg.AuthLimits.EmailIP.Reserve, a.ipKey(r)) {
		return
	}
	email, ok := normalizeEmail(req.Email)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "invalid_email")
		return
	}
	lang := requestLang(r, req.Lang)
	name, ok := cleanText(req.Name, 100)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "invalid_name")
		return
	}
	orgName, ok := cleanText(req.Organization, 100)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "invalid_organization")
		return
	}
	if orgName == "" {
		orgName = map[string]string{"es": "Mi organización", "en": "My organization"}[lang]
	}
	if err := auth.CheckPassword(req.Password, email); err != nil {
		code, _ := passwordError(err)
		writeError(w, r, http.StatusBadRequest, code)
		return
	}
	if req.InvitationToken != "" && a.cfg.Orgs != nil {
		a.registerInvited(w, r, email, req.Password, lang, name, req.InvitationToken)
		return
	}
	code := strings.TrimSpace(req.SignupCode)
	if code == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_signup_code")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		a.internal(w, r, "hash password", err)
		return
	}
	token, tokenHash, err := auth.NewToken()
	if err != nil {
		a.internal(w, r, "token", err)
		return
	}
	verify, err := a.render(mail.VerifyEmail, lang, email, a.link("verify", token), verifyTTL)
	if err != nil {
		a.internal(w, r, "render email", err)
		return
	}
	reg := store.Registration{Email: email, PasswordHash: hash, Lang: lang, SignupCodeHash: auth.HashToken(code),
		OrgName: orgName, VerifyTokenHash: tokenHash, VerifyTTL: verifyTTL, VerifyEmail: verify}
	if name != "" {
		reg.Name = &name
	}
	u, org, err := a.cfg.Accounts.Register(r.Context(), reg)
	switch {
	case errors.Is(err, store.ErrBadSignupCode):
		writeError(w, r, http.StatusBadRequest, "invalid_signup_code")
		return
	case errors.Is(err, store.ErrEmailTaken):
		a.accountExists(r, email)
	case err != nil:
		a.internal(w, r, "register", err)
		return
	default:
		a.cfg.AuthLimits.PerAddress.Reserve(email)
		a.audit(r, &org.ID, &u.ID, auditRegister)
	}
	writeJSON(w, http.StatusAccepted, statusView{"check_email"})
}

// accountExists tells the owner of email, after the response, that someone
// tried to sign up with it (the response itself is the same as for a new
// account).
func (a *api) accountExists(r *http.Request, email string) {
	a.later(r, "account exists email", func(ctx context.Context) error {
		if ok, _ := a.cfg.AuthLimits.PerAddress.Reserve(email); !ok {
			return nil
		}
		u, err := a.cfg.Accounts.UserByEmail(ctx, email)
		if err != nil {
			return err
		}
		e, err := a.render(mail.AccountExists, u.Lang, email, "", 0)
		if err != nil {
			return err
		}
		return a.cfg.Accounts.EnqueueEmail(ctx, e)
	})
}

func (a *api) ipKey(r *http.Request) string {
	return rateKey(clientAddr(r, a.cfg.TrustedProxies))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// login: POST /api/auth/login. Unknown email and wrong password are the same
// 401 (and take the same argon2 time); "not verified" is only told to
// whoever knows the password.
func (a *api) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, authBodyBytes, &req) {
		return
	}
	if limited(w, r, a.cfg.AuthLimits.LoginIP.Reserve, a.ipKey(r)) {
		return
	}
	email, ok := normalizeEmail(req.Email)
	if !ok {
		auth.VerifyDummy(req.Password)
		writeError(w, r, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	fails := a.cfg.AuthLimits.LoginFailures
	if limited(w, r, fails.Check, email) {
		return
	}
	u, err := a.cfg.Accounts.UserByEmail(r.Context(), email)
	if errors.Is(err, store.ErrNoUser) {
		auth.VerifyDummy(req.Password)
		fails.Add(email)
		a.audit(r, nil, nil, auditLoginFailed)
		writeError(w, r, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if err != nil {
		a.internal(w, r, "user by email", err)
		return
	}
	ok, rehash, err := auth.VerifyPassword(req.Password, u.PasswordHash)
	if err != nil {
		a.internal(w, r, "verify password", err)
		return
	}
	if !ok {
		fails.Add(email)
		a.audit(r, nil, &u.ID, auditLoginFailed)
		writeError(w, r, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if u.EmailVerifiedAt == nil {
		writeError(w, r, http.StatusForbidden, "email_not_verified")
		return
	}
	fails.Reset(email)
	if rehash {
		if h, err := auth.HashPassword(req.Password); err == nil {
			if err := a.cfg.Accounts.UpdatePasswordHash(r.Context(), u.ID, h); err != nil {
				a.cfg.Logger.WarnContext(r.Context(), "api: rehash password", "err", err)
			}
		}
	}
	token, tokenHash, err := auth.NewToken()
	if err != nil {
		a.internal(w, r, "token", err)
		return
	}
	csrf, _, err := auth.NewToken()
	if err != nil {
		a.internal(w, r, "token", err)
		return
	}
	s, err := a.cfg.Accounts.StartSession(r.Context(), u.ID, tokenHash, csrf, sessionTTL)
	if err != nil {
		a.internal(w, r, "start session", err)
		return
	}
	setSessionCookie(w, token)
	a.audit(r, nil, &u.ID, auditLogin)
	a.writeMe(w, r, userView{ID: u.ID, Email: u.Email, Name: u.Name, Lang: u.Lang}, s.ActiveOrgID, csrf)
}

// me: GET /api/auth/me.
func (a *api) me(w http.ResponseWriter, r *http.Request) {
	s := session(r)
	a.writeMe(w, r, userView{ID: s.UserID, Email: s.Email, Name: s.Name, Lang: s.Lang}, s.ActiveOrgID, s.CsrfToken)
}

// logout: POST /api/auth/logout.
func (a *api) logout(w http.ResponseWriter, r *http.Request) {
	s := session(r)
	if err := a.cfg.Accounts.EndSession(r.Context(), s.UserID, s.ID); err != nil {
		a.internal(w, r, "end session", err)
		return
	}
	clearSessionCookie(w)
	a.audit(r, nil, &s.UserID, auditLogout)
	w.WriteHeader(http.StatusNoContent)
}

type tokenRequest struct {
	Token string `json:"token"`
}

// verifyEmail: POST /api/auth/verify-email {"token"}.
func (a *api) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if !decodeJSON(w, r, authBodyBytes, &req) {
		return
	}
	if limited(w, r, a.cfg.AuthLimits.EmailIP.Reserve, a.ipKey(r)) {
		return
	}
	if req.Token == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_token")
		return
	}
	u, err := a.cfg.Accounts.VerifyEmail(r.Context(), auth.HashToken(req.Token))
	if errors.Is(err, store.ErrInvalidToken) {
		writeError(w, r, http.StatusBadRequest, "invalid_token")
		return
	}
	if err != nil {
		a.internal(w, r, "verify email", err)
		return
	}
	a.audit(r, nil, &u.ID, auditVerify)
	writeJSON(w, http.StatusOK, statusView{"verified"})
}

type emailRequest struct {
	Email string `json:"email"`
	Lang  string `json:"lang"`
}

// sendLink answers 202 at once and, later, sends a token link to the
// account if it exists and wants returns true for it.
func (a *api) sendLink(w http.ResponseWriter, r *http.Request, what string, wants func(store.User) bool,
	purpose, template, page string, ttl time.Duration, action string) {
	var req emailRequest
	if !decodeJSON(w, r, authBodyBytes, &req) {
		return
	}
	if limited(w, r, a.cfg.AuthLimits.EmailIP.Reserve, a.ipKey(r)) {
		return
	}
	email, ok := normalizeEmail(req.Email)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "invalid_email")
		return
	}
	ip := ipPrefix(clientAddr(r, a.cfg.TrustedProxies))
	a.later(r, what, func(ctx context.Context) error {
		u, err := a.cfg.Accounts.UserByEmail(ctx, email)
		if errors.Is(err, store.ErrNoUser) {
			return nil
		}
		if err != nil {
			return err
		}
		if !wants(u) {
			return nil
		}
		if ok, _ := a.cfg.AuthLimits.PerAddress.Reserve(email); !ok {
			return nil
		}
		token, hash, err := auth.NewToken()
		if err != nil {
			return err
		}
		e, err := a.render(template, u.Lang, email, a.link(page, token), ttl)
		if err != nil {
			return err
		}
		if err := a.cfg.Accounts.SendToken(ctx, u.ID, purpose, hash, ttl, e); err != nil {
			return err
		}
		if action != "" {
			return a.cfg.Accounts.Audit(ctx, nil, &u.ID, action, nil, ip)
		}
		return nil
	})
	writeJSON(w, http.StatusAccepted, statusView{"check_email"})
}

// resendVerification: POST /api/auth/resend-verification {"email"}; always 202.
func (a *api) resendVerification(w http.ResponseWriter, r *http.Request) {
	a.sendLink(w, r, "resend verification", func(u store.User) bool { return u.EmailVerifiedAt == nil },
		store.PurposeVerifyEmail, mail.VerifyEmail, "verify", verifyTTL, "")
}

// forgotPassword: POST /api/auth/forgot-password {"email"}; always 202.
func (a *api) forgotPassword(w http.ResponseWriter, r *http.Request) {
	a.sendLink(w, r, "forgot password", func(store.User) bool { return true },
		store.PurposeResetPassword, mail.ResetPassword, "reset", resetTTL, auditResetRequested)
}

type resetRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// resetPassword: POST /api/auth/reset-password {"token", "password"}. Ends
// every session of the account.
func (a *api) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetRequest
	if !decodeJSON(w, r, authBodyBytes, &req) {
		return
	}
	if limited(w, r, a.cfg.AuthLimits.EmailIP.Reserve, a.ipKey(r)) {
		return
	}
	if req.Token == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_token")
		return
	}
	// A length check first, so a bad password does not need the database.
	if err := auth.CheckPassword(req.Password, ""); err != nil {
		code, _ := passwordError(err)
		writeError(w, r, http.StatusBadRequest, code)
		return
	}
	u, err := a.cfg.Accounts.ResetPassword(r.Context(), auth.HashToken(req.Token), func(u store.User) (string, store.Email, error) {
		if err := auth.CheckPassword(req.Password, u.Email); err != nil {
			return "", store.Email{}, err
		}
		h, err := auth.HashPassword(req.Password)
		if err != nil {
			return "", store.Email{}, err
		}
		e, err := a.render(mail.PasswordChanged, u.Lang, u.Email, "", 0)
		return h, e, err
	})
	if code, ok := passwordError(err); ok {
		writeError(w, r, http.StatusBadRequest, code)
		return
	}
	if errors.Is(err, store.ErrInvalidToken) {
		writeError(w, r, http.StatusBadRequest, "invalid_token")
		return
	}
	if err != nil {
		a.internal(w, r, "reset password", err)
		return
	}
	a.cfg.AuthLimits.LoginFailures.Reset(u.Email)
	clearSessionCookie(w)
	a.audit(r, nil, &u.ID, auditReset)
	writeJSON(w, http.StatusOK, statusView{"password_reset"})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// changePassword: POST /api/auth/change-password. Ends the other sessions.
func (a *api) changePassword(w http.ResponseWriter, r *http.Request) {
	s := session(r)
	var req changePasswordRequest
	if !decodeJSON(w, r, authBodyBytes, &req) {
		return
	}
	fails := a.cfg.AuthLimits.LoginFailures
	if limited(w, r, fails.Check, s.Email) {
		return
	}
	u, err := a.cfg.Accounts.UserByEmail(r.Context(), s.Email)
	if err != nil {
		a.internal(w, r, "user by email", err)
		return
	}
	ok, _, err := auth.VerifyPassword(req.CurrentPassword, u.PasswordHash)
	if err != nil {
		a.internal(w, r, "verify password", err)
		return
	}
	if !ok {
		fails.Add(s.Email)
		a.audit(r, nil, &u.ID, auditLoginFailed)
		writeError(w, r, http.StatusBadRequest, "wrong_password")
		return
	}
	if err := auth.CheckPassword(req.NewPassword, u.Email); err != nil {
		code, _ := passwordError(err)
		writeError(w, r, http.StatusBadRequest, code)
		return
	}
	h, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		a.internal(w, r, "hash password", err)
		return
	}
	e, err := a.render(mail.PasswordChanged, u.Lang, u.Email, "", 0)
	if err != nil {
		a.internal(w, r, "render email", err)
		return
	}
	if err := a.cfg.Accounts.ChangePassword(r.Context(), u.ID, s.ID, h, e); err != nil {
		a.internal(w, r, "change password", err)
		return
	}
	a.audit(r, nil, &u.ID, auditChanged)
	w.WriteHeader(http.StatusNoContent)
}
