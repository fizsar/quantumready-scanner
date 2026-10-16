// Package org holds the organization roles and what each one may do. It has
// no dependencies: the API asks it before every organization action, and the
// table in docs/security.md is this file in words.
package org

// Roles of a member.
const (
	Owner  = "owner"
	Admin  = "admin"
	Viewer = "viewer"
)

// ValidRole reports whether r is a role.
func ValidRole(r string) bool { return r == Owner || r == Admin || r == Viewer }

// Action is something a member may be allowed to do in their organization.
type Action string

// Actions.
const (
	View             Action = "view"              // the organization, its members, scans and domains
	Rename           Action = "rename"            // the organization
	Invite           Action = "invite"            // create invitations (admin or viewer)
	RevokeInvitation Action = "revoke_invitation" // and list pending invitations
	ChangeRole       Action = "change_role"       // of any member, to any role
	RemoveMember     Action = "remove_member"     // see CanRemove for admins
	ManageDomains    Action = "manage_domains"    // add, verify, delete domains; full scans (milestone 3.4)
)

var allowed = map[Action][]string{
	View:             {Owner, Admin, Viewer},
	Rename:           {Owner, Admin},
	Invite:           {Owner, Admin},
	RevokeInvitation: {Owner, Admin},
	ChangeRole:       {Owner},
	RemoveMember:     {Owner, Admin},
	ManageDomains:    {Owner, Admin},
}

// Can reports whether role may perform a.
func Can(role string, a Action) bool {
	for _, r := range allowed[a] {
		if r == role {
			return true
		}
	}
	return false
}

// Actions lists what role may do (for the web, which hides what it cannot).
func Actions(role string) []Action {
	var out []Action
	for _, a := range []Action{View, Rename, Invite, RevokeInvitation, ChangeRole, RemoveMember, ManageDomains} {
		if Can(role, a) {
			out = append(out, a)
		}
	}
	return out
}

// CanRemove reports whether actor may remove a member with role target:
// owners remove anyone, admins only viewers. Leaving (removing yourself) is
// always allowed, subject to the last-owner rule.
func CanRemove(actor, target string, self bool) bool {
	if self {
		return true
	}
	switch actor {
	case Owner:
		return true
	case Admin:
		return target == Viewer
	}
	return false
}

// CanInviteAs reports whether actor may invite someone as role. Owners are
// made by promotion, never by invitation.
func CanInviteAs(actor, role string) bool {
	return Can(actor, Invite) && (role == Admin || role == Viewer)
}
