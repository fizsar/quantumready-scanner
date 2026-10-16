package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/org"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// OrgStore is what the organization endpoints need from internal/store.
// The organization is always the session's active one: IDs in the URL name
// members (or invitations) inside it, never the organization.
type OrgStore interface {
	ActiveMembership(ctx context.Context, sessionID, userID uuid.UUID, active *uuid.UUID) (store.Member, error)
	Members(ctx context.Context, t store.Tenant) ([]store.ListMembersRow, error)
	RenameOrganization(ctx context.Context, t store.Tenant, name string) error
	ChangeMember(ctx context.Context, t store.Tenant, target uuid.UUID, decide store.MemberDecision) error
}

// Audit actions.
const (
	auditOrgRenamed  = "org.renamed"
	auditRoleChanged = "member.role_changed"
	auditRemoved     = "member.removed"
	auditLeft        = "member.left"
)

var errForbidden = errors.New("forbidden")

type memberKey struct{}

// member is the signed-in user's place in their active organization.
func member(r *http.Request) store.Member {
	m, _ := r.Context().Value(memberKey{}).(store.Member)
	return m
}

// tenant is who an organization request acts for.
func tenant(r *http.Request) store.Tenant {
	m, s := member(r), session(r)
	return store.Tenant{OrgID: &m.OrgID, UserID: &s.UserID}
}

// requireOrg (after requireSession) finds the user's role in the active
// organization; 403 no_organization if they belong to none.
func (a *api) requireOrg(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := session(r)
		m, err := a.cfg.Orgs.ActiveMembership(r.Context(), s.ID, s.UserID, s.ActiveOrgID)
		if errors.Is(err, store.ErrNoOrganization) {
			writeError(w, r, http.StatusForbidden, "no_organization")
			return
		}
		if err != nil {
			a.internal(w, r, "active membership", err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), memberKey{}, m)))
	})
}

// allow answers 403 forbidden unless the member may perform action.
func allow(w http.ResponseWriter, r *http.Request, action org.Action) bool {
	if org.Can(member(r).Role, action) {
		return true
	}
	writeError(w, r, http.StatusForbidden, "forbidden")
	return false
}

func (a *api) orgAudit(r *http.Request, action string, target *string) {
	t := tenant(r)
	ip := ipPrefix(clientAddr(r, a.cfg.TrustedProxies))
	if err := a.cfg.Accounts.Audit(r.Context(), t.OrgID, t.UserID, action, target, ip); err != nil {
		a.cfg.Logger.ErrorContext(r.Context(), "api: audit", "action", action, "err", err)
	}
}

type orgDetail struct {
	ID          uuid.UUID    `json:"id"`
	Name        string       `json:"name"`
	Role        string       `json:"role"`
	Permissions []org.Action `json:"permissions"`
}

// getOrg: GET /api/org, the active organization and what the user may do in it.
func (a *api) getOrg(w http.ResponseWriter, r *http.Request) {
	m := member(r)
	writeJSON(w, http.StatusOK, orgDetail{ID: m.OrgID, Name: m.OrgName, Role: m.Role, Permissions: org.Actions(m.Role)})
}

type renameRequest struct {
	Name string `json:"name"`
}

// renameOrg: PATCH /api/org {"name"}.
func (a *api) renameOrg(w http.ResponseWriter, r *http.Request) {
	var req renameRequest
	if !decodeJSON(w, r, authBodyBytes, &req) || !allow(w, r, org.Rename) {
		return
	}
	name, ok := cleanText(req.Name, 100)
	if !ok || name == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_organization")
		return
	}
	if err := a.cfg.Orgs.RenameOrganization(r.Context(), tenant(r), name); err != nil {
		a.internal(w, r, "rename organization", err)
		return
	}
	a.orgAudit(r, auditOrgRenamed, nil)
	w.WriteHeader(http.StatusNoContent)
}

type memberView struct {
	UserID   uuid.UUID `json:"user_id"`
	Email    string    `json:"email"`
	Name     *string   `json:"name,omitempty"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
	You      bool      `json:"you"`
}

// listMembers: GET /api/org/members.
func (a *api) listMembers(w http.ResponseWriter, r *http.Request) {
	if !allow(w, r, org.View) {
		return
	}
	ms, err := a.cfg.Orgs.Members(r.Context(), tenant(r))
	if err != nil {
		a.internal(w, r, "list members", err)
		return
	}
	me := session(r).UserID
	out := []memberView{}
	for _, m := range ms {
		out = append(out, memberView{UserID: m.UserID, Email: m.Email, Name: m.Name, Role: m.Role, JoinedAt: m.CreatedAt, You: m.UserID == me})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": out})
}

type roleRequest struct {
	Role string `json:"role"`
}

// changeRole: PATCH /api/org/members/{userID} {"role"}. Owners only.
func (a *api) changeRole(w http.ResponseWriter, r *http.Request) {
	target, ok := pathID(w, r, "userID")
	if !ok {
		return
	}
	var req roleRequest
	if !decodeJSON(w, r, authBodyBytes, &req) {
		return
	}
	if !org.ValidRole(req.Role) {
		writeError(w, r, http.StatusBadRequest, "invalid_role")
		return
	}
	err := a.cfg.Orgs.ChangeMember(r.Context(), tenant(r), target, func(actor, _ string) (string, bool, error) {
		if !org.Can(actor, org.ChangeRole) {
			return "", false, errForbidden
		}
		return req.Role, false, nil
	})
	if a.memberError(w, r, err) {
		return
	}
	t := target.String() + ":" + req.Role
	a.orgAudit(r, auditRoleChanged, &t)
	w.WriteHeader(http.StatusNoContent)
}

// removeMember: DELETE /api/org/members/{userID}. Removing yourself is
// leaving.
func (a *api) removeMember(w http.ResponseWriter, r *http.Request) {
	target, ok := pathID(w, r, "userID")
	if ok {
		a.remove(w, r, target)
	}
}

// leaveOrg: POST /api/org/leave.
func (a *api) leaveOrg(w http.ResponseWriter, r *http.Request) {
	a.remove(w, r, session(r).UserID)
}

func (a *api) remove(w http.ResponseWriter, r *http.Request, target uuid.UUID) {
	s := session(r)
	self := target == s.UserID
	tn := tenant(r)
	err := a.cfg.Orgs.ChangeMember(r.Context(), tn, target, func(actor, targetRole string) (string, bool, error) {
		if !org.CanRemove(actor, targetRole, self) {
			return "", false, errForbidden
		}
		return "", true, nil
	})
	if a.memberError(w, r, err) {
		return
	}
	if self {
		a.orgAudit(r, auditLeft, nil)
		// Move this session to another organization (or none).
		if _, err := a.cfg.Orgs.ActiveMembership(r.Context(), s.ID, s.UserID, nil); err != nil && !errors.Is(err, store.ErrNoOrganization) {
			a.cfg.Logger.WarnContext(r.Context(), "api: switch after leaving", "err", err)
		}
	} else {
		t := target.String()
		a.orgAudit(r, auditRemoved, &t)
	}
	w.WriteHeader(http.StatusNoContent)
}

// memberError writes the answer for a ChangeMember error; false if none.
func (a *api) memberError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, errForbidden), errors.Is(err, store.ErrNotMember):
		writeError(w, r, http.StatusForbidden, "forbidden")
	case errors.Is(err, store.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "not_found")
	case errors.Is(err, store.ErrLastOwner):
		writeError(w, r, http.StatusConflict, "last_owner")
	default:
		a.internal(w, r, "change member", err)
	}
	return true
}

// pathID reads a UUID URL parameter; anything else is 404.
func pathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found")
		return uuid.UUID{}, false
	}
	return id, true
}
