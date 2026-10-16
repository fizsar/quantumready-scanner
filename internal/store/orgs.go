package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Organization errors.
var (
	ErrNoOrganization = errors.New("store: the user belongs to no organization")
	ErrNotMember      = errors.New("store: not a member of the organization")
	ErrNotFound       = errors.New("store: not found")
	ErrLastOwner      = errors.New("store: an organization needs at least one owner")
)

// Member is the signed-in user's place in their active organization.
type Member struct {
	OrgID   uuid.UUID
	OrgName string
	Role    string
}

// lastOwner turns the memberships_keep_owner trigger error into ErrLastOwner.
func lastOwner(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.ConstraintName == "memberships_last_owner" {
		return ErrLastOwner
	}
	return err
}

// ActiveMembership returns the user's role in the session's active
// organization. If they are no longer a member (removed, or they left), the
// session moves to their first organization; with none, ErrNoOrganization.
func (s *Store) ActiveMembership(ctx context.Context, sessionID, userID uuid.UUID, active *uuid.UUID) (m Member, err error) {
	err = s.Tx(ctx, Tenant{OrgID: active, UserID: &userID}, func(q *Queries) error {
		if active != nil {
			ms, err := q.GetMembership(ctx, GetMembershipParams{OrganizationID: *active, UserID: userID})
			if err == nil {
				o, err := q.GetOrganization(ctx, *active)
				m = Member{OrgID: o.ID, OrgName: o.Name, Role: ms.Role}
				return err
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		orgs, err := q.ListUserOrganizations(ctx, userID)
		if err != nil {
			return err
		}
		var next *uuid.UUID
		if len(orgs) > 0 {
			next = &orgs[0].ID
			m = Member{OrgID: orgs[0].ID, OrgName: orgs[0].Name, Role: orgs[0].Role}
		}
		if err := q.SetActiveOrganization(ctx, SetActiveOrganizationParams{ID: sessionID, UserID: userID, OrganizationID: next}); err != nil {
			return err
		}
		if next == nil {
			return ErrNoOrganization
		}
		return nil
	})
	return m, err
}

// Members lists the active organization's members.
func (s *Store) Members(ctx context.Context, t Tenant) (ms []ListMembersRow, err error) {
	err = s.Tx(ctx, t, func(q *Queries) error {
		ms, err = q.ListMembers(ctx, *t.OrgID)
		return err
	})
	return ms, err
}

// RenameOrganization renames the active organization.
func (s *Store) RenameOrganization(ctx context.Context, t Tenant, name string) error {
	return s.Tx(ctx, t, func(q *Queries) error {
		n, err := q.RenameOrganization(ctx, RenameOrganizationParams{ID: *t.OrgID, Name: name})
		if err == nil && n != 1 {
			err = ErrNotFound
		}
		return err
	})
}

// MemberDecision says what happens to a member, given the actor's and the
// target's current roles: a new role, removal, or an error (not allowed).
type MemberDecision func(actorRole, targetRole string) (newRole string, remove bool, err error)

// ChangeMember changes or removes target in the active organization. Both
// memberships stay locked from the decision to the change, so roles cannot
// change in between. ErrNotMember: the actor is not (any more) a member;
// ErrNotFound: the target is not a member; ErrLastOwner: no owner would be
// left.
func (s *Store) ChangeMember(ctx context.Context, t Tenant, target uuid.UUID, decide MemberDecision) error {
	actor := *t.UserID
	return s.Tx(ctx, t, func(q *Queries) error {
		rows, err := q.LockMemberships(ctx, LockMembershipsParams{OrganizationID: *t.OrgID, UserIds: []uuid.UUID{actor, target}})
		if err != nil {
			return err
		}
		var actorRole, targetRole string
		for _, r := range rows {
			if r.UserID == actor {
				actorRole = r.Role
			}
			if r.UserID == target {
				targetRole = r.Role
			}
		}
		if actorRole == "" {
			return ErrNotMember
		}
		if targetRole == "" {
			return ErrNotFound
		}
		role, remove, err := decide(actorRole, targetRole)
		if err != nil {
			return err
		}
		switch {
		case remove:
			_, err = q.RemoveMember(ctx, RemoveMemberParams{OrganizationID: *t.OrgID, UserID: target})
		case role != targetRole:
			_, err = q.SetMemberRole(ctx, SetMemberRoleParams{OrganizationID: *t.OrgID, UserID: target, Role: role})
		}
		return lastOwner(err)
	})
}
