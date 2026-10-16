package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/auth"
	"github.com/fizsar/quantumready-saas/internal/mail"
	"github.com/fizsar/quantumready-saas/internal/org"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// Organization limits.
const (
	maxPendingInvitations = 20 // per organization
	maxMembers            = 50 // per organization, pending invitations included
	maxOrgsPerUser        = 10 // memberships of one user, own organizations included
)

// Audit actions.
const (
	auditOrgCreated         = "org.created"
	auditInvitationCreated  = "invitation.created"
	auditInvitationRevoked  = "invitation.revoked"
	auditInvitationAccepted = "invitation.accepted"
)

var roleNames = map[string]map[string]string{
	"es": {org.Owner: "propietario", org.Admin: "administrador", org.Viewer: "lectura"},
	"en": {org.Owner: "owner", org.Admin: "admin", org.Viewer: "read-only"},
}

type inviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	Lang  string `json:"lang"` // of the email; default the request's
}

type invitationView struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	InviterEmail *string   `json:"inviter_email,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	Expired      bool      `json:"expired"`
}

// createInvitation: POST /api/org/invitations {"email", "role"}.
func (a *api) createInvitation(w http.ResponseWriter, r *http.Request) {
	var req inviteRequest
	if !decodeJSON(w, r, authBodyBytes, &req) || !allow(w, r, org.Invite) {
		return
	}
	email, ok := normalizeEmail(req.Email)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "invalid_email")
		return
	}
	m := member(r)
	if !org.ValidRole(req.Role) || req.Role == org.Owner {
		writeError(w, r, http.StatusBadRequest, "invalid_role")
		return
	}
	if !org.CanInviteAs(m.Role, req.Role) {
		writeError(w, r, http.StatusForbidden, "forbidden")
		return
	}
	if limited(w, r, a.cfg.AuthLimits.OrgInvitations.Reserve, m.OrgID.String()) ||
		limited(w, r, a.cfg.AuthLimits.PerAddress.Reserve, email) {
		return
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		a.internal(w, r, "token", err)
		return
	}
	lang := requestLang(r, req.Lang)
	e, err := a.renderData(mail.Invitation, lang, email, store.InvitationTTL, mail.Data{
		Link: a.link("invite", token), Inviter: session(r).Email, OrgName: m.OrgName, Role: roleNames[lang][req.Role]})
	if err != nil {
		a.internal(w, r, "render email", err)
		return
	}
	row, err := a.cfg.Orgs.Invite(r.Context(), tenant(r), store.NewInvitation{Email: email, Role: req.Role, TokenHash: hash,
		TTL: store.InvitationTTL, Mail: e, MaxPending: maxPendingInvitations, MaxMembers: maxMembers})
	switch {
	case errors.Is(err, store.ErrAlreadyMember):
		writeError(w, r, http.StatusConflict, "already_member")
		return
	case errors.Is(err, store.ErrLimit):
		writeError(w, r, http.StatusConflict, "limit_reached")
		return
	case err != nil:
		a.internal(w, r, "invite", err)
		return
	}
	id := row.ID.String()
	a.orgAudit(r, auditInvitationCreated, &id)
	writeJSON(w, http.StatusCreated, invitationView{ID: row.ID, Email: row.Email, Role: row.Role, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt})
}

// listInvitations: GET /api/org/invitations, the pending ones.
func (a *api) listInvitations(w http.ResponseWriter, r *http.Request) {
	if !allow(w, r, org.RevokeInvitation) {
		return
	}
	rows, err := a.cfg.Orgs.Invitations(r.Context(), tenant(r))
	if err != nil {
		a.internal(w, r, "list invitations", err)
		return
	}
	out := []invitationView{}
	now := time.Now()
	for _, i := range rows {
		out = append(out, invitationView{ID: i.ID, Email: i.Email, Role: i.Role, InviterEmail: i.InviterEmail,
			CreatedAt: i.CreatedAt, ExpiresAt: i.ExpiresAt, Expired: !i.ExpiresAt.After(now)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"invitations": out})
}

// revokeInvitation: DELETE /api/org/invitations/{invitationID}.
func (a *api) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "invitationID")
	if !ok || !allow(w, r, org.RevokeInvitation) {
		return
	}
	err := a.cfg.Orgs.RevokeInvitation(r.Context(), tenant(r), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		a.internal(w, r, "revoke invitation", err)
		return
	}
	t := id.String()
	a.orgAudit(r, auditInvitationRevoked, &t)
	w.WriteHeader(http.StatusNoContent)
}

type invitationPreview struct {
	Organization string    `json:"organization"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	InviterEmail *string   `json:"inviter_email,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// previewInvitation: POST /api/invitations/preview {"token"}, what the link
// offers (no session needed: the invitee may not have an account yet).
func (a *api) previewInvitation(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if !decodeJSON(w, r, authBodyBytes, &req) || limited(w, r, a.cfg.AuthLimits.EmailIP.Reserve, a.ipKey(r)) {
		return
	}
	if req.Token == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_token")
		return
	}
	inv, err := a.cfg.Orgs.InvitationByToken(r.Context(), auth.HashToken(req.Token))
	if errors.Is(err, store.ErrInvalidToken) {
		writeError(w, r, http.StatusBadRequest, "invalid_token")
		return
	}
	if err != nil {
		a.internal(w, r, "invitation by token", err)
		return
	}
	writeJSON(w, http.StatusOK, invitationPreview{Organization: inv.OrganizationName, Email: inv.Email, Role: inv.Role,
		InviterEmail: inv.InviterEmail, ExpiresAt: inv.ExpiresAt})
}

// acceptInvitation: POST /api/invitations/accept {"token"}, signed in as the
// invited address. The organization becomes the active one.
func (a *api) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if !decodeJSON(w, r, authBodyBytes, &req) || limited(w, r, a.cfg.AuthLimits.EmailIP.Reserve, a.ipKey(r)) {
		return
	}
	s := session(r)
	m, err := a.cfg.Orgs.AcceptInvitation(r.Context(), s.ID, s.UserID, s.Email, auth.HashToken(req.Token))
	if a.invitationError(w, r, err) {
		return
	}
	a.auditIn(r, m.OrgID, s.UserID, auditInvitationAccepted)
	writeJSON(w, http.StatusOK, orgDetail{ID: m.OrgID, Name: m.OrgName, Role: m.Role, Permissions: org.Actions(m.Role)})
}

// invitationError writes the answer for an invitation error; false if none.
func (a *api) invitationError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, store.ErrInvalidToken):
		writeError(w, r, http.StatusBadRequest, "invalid_token")
	case errors.Is(err, store.ErrInvitationEmail):
		writeError(w, r, http.StatusForbidden, "invitation_other_email")
	default:
		a.internal(w, r, "accept invitation", err)
	}
	return true
}

// auditIn records an action in an organization other than the request's.
func (a *api) auditIn(r *http.Request, orgID, userID uuid.UUID, action string) {
	ip := ipPrefix(clientAddr(r, a.cfg.TrustedProxies))
	if err := a.cfg.Accounts.Audit(r.Context(), &orgID, &userID, action, nil, ip); err != nil {
		a.cfg.Logger.ErrorContext(r.Context(), "api: audit", "action", action, "err", err)
	}
}

// registerInvited is the signup path with an invitation instead of a code.
func (a *api) registerInvited(w http.ResponseWriter, r *http.Request, email, password, lang, name, token string) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		a.internal(w, r, "hash password", err)
		return
	}
	reg := store.InvitedRegistration{Email: email, PasswordHash: hash, Lang: lang, InvitationHash: auth.HashToken(strings.TrimSpace(token))}
	if name != "" {
		reg.Name = &name
	}
	u, m, err := a.cfg.Orgs.RegisterInvited(r.Context(), reg)
	if errors.Is(err, store.ErrEmailTaken) {
		a.accountExists(r, email)
		writeJSON(w, http.StatusAccepted, statusView{"check_email"})
		return
	}
	if a.invitationError(w, r, err) {
		return
	}
	a.cfg.AuthLimits.PerAddress.Reserve(email)
	a.auditIn(r, m.OrgID, u.ID, auditRegister)
	writeJSON(w, http.StatusCreated, statusView{"registered"})
}

type createOrgRequest struct {
	Name string `json:"name"`
}

// createOrganization: POST /api/organizations {"name"}; the user is its
// owner and it becomes the active one.
func (a *api) createOrganization(w http.ResponseWriter, r *http.Request) {
	var req createOrgRequest
	if !decodeJSON(w, r, authBodyBytes, &req) {
		return
	}
	name, ok := cleanText(req.Name, 100)
	if !ok || name == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_organization")
		return
	}
	s := session(r)
	m, err := a.cfg.Orgs.CreateOrganization(r.Context(), s.ID, s.UserID, name,
		store.NewOrgRules{MaxOrgs: maxOrgsPerUser, InvitedMayCreate: a.cfg.InvitedCanCreateOrgs})
	if errors.Is(err, store.ErrNewOrgsClosed) {
		writeError(w, r, http.StatusForbidden, "new_organizations_closed")
		return
	}
	if errors.Is(err, store.ErrLimit) {
		writeError(w, r, http.StatusConflict, "limit_reached")
		return
	}
	if err != nil {
		a.internal(w, r, "create organization", err)
		return
	}
	a.auditIn(r, m.OrgID, s.UserID, auditOrgCreated)
	writeJSON(w, http.StatusCreated, orgDetail{ID: m.OrgID, Name: m.OrgName, Role: m.Role, Permissions: org.Actions(m.Role)})
}

// switchOrganization: POST /api/organizations/{orgID}/switch. 404 unless a member.
func (a *api) switchOrganization(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "orgID")
	if !ok {
		return
	}
	s := session(r)
	m, err := a.cfg.Orgs.SwitchOrganization(r.Context(), s.ID, s.UserID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		a.internal(w, r, "switch organization", err)
		return
	}
	writeJSON(w, http.StatusOK, orgDetail{ID: m.OrgID, Name: m.OrgName, Role: m.Role, Permissions: org.Actions(m.Role)})
}
