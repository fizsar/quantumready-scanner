package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Invitation errors.
var (
	ErrAlreadyMember   = errors.New("store: already a member")
	ErrInvitationEmail = errors.New("store: the invitation is for another email")
	ErrLimit           = errors.New("store: limit reached")
	ErrNewOrgsClosed   = errors.New("store: invited accounts cannot create organizations during the beta")
)

// InvitationTTL is how long an invitation link works.
const InvitationTTL = 7 * 24 * time.Hour

// InvitationInfo is what an invitation link shows before it is accepted.
type InvitationInfo struct {
	ID               uuid.UUID
	OrganizationID   uuid.UUID
	OrganizationName string
	Email, Role      string
	InviterEmail     *string
	ExpiresAt        time.Time
}

// invitationByToken returns a pending, unexpired invitation, or
// ErrInvalidToken (unknown, used, revoked or expired).
func invitationByToken(ctx context.Context, q *Queries, hash []byte) (InvitationInfo, error) {
	// The function returns only pending, unexpired invitations.
	var inv InvitationInfo
	err := q.db.QueryRow(ctx, `SELECT id, organization_id, organization_name, email, role, inviter_email, expires_at
		FROM invitation_by_token($1)`, hash).Scan(&inv.ID, &inv.OrganizationID, &inv.OrganizationName, &inv.Email, &inv.Role,
		&inv.InviterEmail, &inv.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return InvitationInfo{}, ErrInvalidToken
	}
	return inv, err
}

// setTenant switches the transaction's tenant (after proving the right to
// it: a valid invitation, a new organization).
func setTenant(ctx context.Context, q *Queries, org, user uuid.UUID) error {
	_, err := q.db.Exec(ctx, "SELECT set_config('app.org_id', $1, true), set_config('app.user_id', $2, true)", org.String(), user.String())
	return err
}

// InvitationByToken is the invitation behind a link (preview page).
func (s *Store) InvitationByToken(ctx context.Context, hash []byte) (inv InvitationInfo, err error) {
	err = s.Tx(ctx, Public, func(q *Queries) error {
		inv, err = invitationByToken(ctx, q, hash)
		return err
	})
	return inv, err
}

// NewInvitation is an invitation to create in the active organization.
type NewInvitation struct {
	Email, Role string
	TokenHash   []byte
	TTL         time.Duration
	Mail        Email // the invitation email, with the link
	MaxPending  int   // pending invitations per organization
	MaxMembers  int   // members plus pending invitations
}

// Invite creates an invitation (replacing a pending one to the same
// address) and queues its email. ErrAlreadyMember if the address is a
// member; ErrLimit beyond MaxPending.
func (s *Store) Invite(ctx context.Context, t Tenant, n NewInvitation) (row CreateInvitationRow, err error) {
	err = s.Tx(ctx, t, func(q *Queries) error {
		member, err := q.IsMemberByEmail(ctx, IsMemberByEmailParams{OrganizationID: *t.OrgID, Email: n.Email})
		if err != nil {
			return err
		}
		if member {
			return ErrAlreadyMember
		}
		if err := q.RevokePendingInvitationsFor(ctx, RevokePendingInvitationsForParams{OrganizationID: *t.OrgID, Email: n.Email}); err != nil {
			return err
		}
		pending, err := q.ListPendingInvitations(ctx, *t.OrgID)
		if err != nil {
			return err
		}
		members, err := q.ListMembers(ctx, *t.OrgID)
		if err != nil {
			return err
		}
		// Pending invitations count as members: accepting them can never
		// exceed MaxMembers.
		if (n.MaxPending > 0 && len(pending) >= n.MaxPending) || (n.MaxMembers > 0 && len(members)+len(pending) >= n.MaxMembers) {
			return ErrLimit
		}
		if row, err = q.CreateInvitation(ctx, CreateInvitationParams{OrganizationID: *t.OrgID, Email: n.Email, Role: n.Role,
			TokenHash: n.TokenHash, InvitedBy: t.UserID, ExpiresAt: time.Now().Add(n.TTL)}); err != nil {
			return err
		}
		return enqueue(ctx, q, n.Mail)
	})
	return row, err
}

// Invitations lists the active organization's pending invitations
// (expired ones included, for the owner to see and renew).
func (s *Store) Invitations(ctx context.Context, t Tenant) (rows []ListPendingInvitationsRow, err error) {
	err = s.Tx(ctx, t, func(q *Queries) error {
		rows, err = q.ListPendingInvitations(ctx, *t.OrgID)
		return err
	})
	return rows, err
}

// RevokeInvitation revokes a pending invitation of the active organization.
func (s *Store) RevokeInvitation(ctx context.Context, t Tenant, id uuid.UUID) error {
	return s.Tx(ctx, t, func(q *Queries) error {
		n, err := q.RevokeInvitation(ctx, RevokeInvitationParams{OrganizationID: *t.OrgID, ID: id})
		if err == nil && n != 1 {
			err = ErrNotFound
		}
		return err
	})
}

// joinByInvitation spends inv for user (whose address must be the invited
// one) and adds them with its role, unless already a member.
func joinByInvitation(ctx context.Context, q *Queries, inv InvitationInfo, userID uuid.UUID, email string) (Member, error) {
	if inv.Email != email {
		return Member{}, ErrInvitationEmail
	}
	if err := setTenant(ctx, q, inv.OrganizationID, userID); err != nil {
		return Member{}, err
	}
	n, err := q.MarkInvitationAccepted(ctx, MarkInvitationAcceptedParams{ID: inv.ID, AcceptedBy: &userID})
	if err != nil {
		return Member{}, err
	}
	if n != 1 {
		return Member{}, ErrInvalidToken // spent concurrently
	}
	m := Member{OrgID: inv.OrganizationID, OrgName: inv.OrganizationName, Role: inv.Role}
	existing, err := q.GetMembership(ctx, GetMembershipParams{OrganizationID: inv.OrganizationID, UserID: userID})
	switch {
	case err == nil:
		m.Role = existing.Role // never a downgrade
		return m, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return Member{}, err
	}
	return m, q.AddMembership(ctx, AddMembershipParams{OrganizationID: inv.OrganizationID, UserID: userID, Role: inv.Role})
}

// AcceptInvitation adds the signed-in user to the invitation's organization
// and makes it the session's active one.
func (s *Store) AcceptInvitation(ctx context.Context, sessionID, userID uuid.UUID, email string, hash []byte) (m Member, err error) {
	err = s.Tx(ctx, Public, func(q *Queries) error {
		inv, err := invitationByToken(ctx, q, hash)
		if err != nil {
			return err
		}
		if m, err = joinByInvitation(ctx, q, inv, userID, email); err != nil {
			return err
		}
		return q.SetActiveOrganization(ctx, SetActiveOrganizationParams{ID: sessionID, UserID: userID, OrganizationID: &m.OrgID})
	})
	return m, err
}

// InvitedRegistration signs up someone with an invitation instead of a
// signup code: no organization of their own, and the address is verified
// (the link reached it).
type InvitedRegistration struct {
	Email, PasswordHash, Lang string
	Name                      *string
	InvitationHash            []byte
}

// RegisterInvited creates the user and joins them to the invitation's
// organization. ErrEmailTaken (nothing spent) or ErrInvalidToken /
// ErrInvitationEmail.
func (s *Store) RegisterInvited(ctx context.Context, r InvitedRegistration) (u User, m Member, err error) {
	err = s.Tx(ctx, Public, func(q *Queries) error {
		inv, err := invitationByToken(ctx, q, r.InvitationHash)
		if err != nil {
			return err
		}
		if inv.Email != r.Email {
			return ErrInvitationEmail
		}
		u, err = q.CreateUser(ctx, CreateUserParams{Email: r.Email, PasswordHash: r.PasswordHash, Name: r.Name, Lang: r.Lang})
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return ErrEmailTaken
		}
		if err != nil {
			return err
		}
		if err := q.MarkEmailVerified(ctx, u.ID); err != nil {
			return err
		}
		if err := q.MarkSignupViaInvitation(ctx, u.ID); err != nil {
			return err
		}
		u.SignupVia = SignupViaInvitation
		m, err = joinByInvitation(ctx, q, inv, u.ID, r.Email)
		return err
	})
	return u, m, err
}

// How an account was created (users.signup_via).
const (
	SignupViaCode       = "code"
	SignupViaInvitation = "invitation"
)

// NewOrgRules limit who creates organizations.
type NewOrgRules struct {
	MaxOrgs          int  // memberships per user; 0 = no limit
	InvitedMayCreate bool // accounts created by invitation may create organizations (off during the beta)
}

// CreateOrganization creates an organization owned by the user and makes it
// the session's active one. ErrNewOrgsClosed for an invited account while
// the beta rule holds; ErrLimit beyond MaxOrgs.
func (s *Store) CreateOrganization(ctx context.Context, sessionID, userID uuid.UUID, name string, rules NewOrgRules) (m Member, err error) {
	err = s.Tx(ctx, Tenant{UserID: &userID}, func(q *Queries) error {
		u, err := q.GetUserByID(ctx, userID)
		if err != nil {
			return err
		}
		if u.SignupVia == SignupViaInvitation && !rules.InvitedMayCreate {
			return ErrNewOrgsClosed
		}
		maxOrgs := rules.MaxOrgs
		orgs, err := q.ListUserOrganizations(ctx, userID)
		if err != nil {
			return err
		}
		if maxOrgs > 0 && len(orgs) >= maxOrgs {
			return ErrLimit
		}
		id := uuid.New()
		if err := setTenant(ctx, q, id, userID); err != nil {
			return err
		}
		o, err := q.CreateOrganization(ctx, CreateOrganizationParams{ID: id, Name: name})
		if err != nil {
			return err
		}
		if err := q.AddMembership(ctx, AddMembershipParams{OrganizationID: id, UserID: userID, Role: "owner"}); err != nil {
			return err
		}
		m = Member{OrgID: o.ID, OrgName: o.Name, Role: "owner"}
		return q.SetActiveOrganization(ctx, SetActiveOrganizationParams{ID: sessionID, UserID: userID, OrganizationID: &id})
	})
	return m, err
}

// SwitchOrganization makes org the session's active organization;
// ErrNotFound unless the user is a member of it.
func (s *Store) SwitchOrganization(ctx context.Context, sessionID, userID, org uuid.UUID) (m Member, err error) {
	err = s.Tx(ctx, Tenant{OrgID: &org, UserID: &userID}, func(q *Queries) error {
		ms, err := q.GetMembership(ctx, GetMembershipParams{OrganizationID: org, UserID: userID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		o, err := q.GetOrganization(ctx, org)
		if err != nil {
			return err
		}
		m = Member{OrgID: o.ID, OrgName: o.Name, Role: ms.Role}
		return q.SetActiveOrganization(ctx, SetActiveOrganizationParams{ID: sessionID, UserID: userID, OrganizationID: &org})
	})
	return m, err
}
