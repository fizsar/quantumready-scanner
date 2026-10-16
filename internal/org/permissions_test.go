package org

import (
	"fmt"
	"strings"
	"testing"
)

// The whole matrix, written out: a change to a permission has to change
// this table too (and docs/security.md).
func TestPermissionMatrix(t *testing.T) {
	want := `
view:              owner admin viewer
rename:            owner admin
invite:            owner admin
revoke_invitation: owner admin
change_role:       owner
remove_member:     owner admin
manage_domains:    owner admin`
	var got strings.Builder
	for _, a := range []Action{View, Rename, Invite, RevokeInvitation, ChangeRole, RemoveMember, ManageDomains} {
		var roles []string
		for _, r := range []string{Owner, Admin, Viewer, "", "root"} {
			if Can(r, a) {
				roles = append(roles, r)
			}
		}
		fmt.Fprintf(&got, "\n%-19s%s", string(a)+":", strings.Join(roles, " "))
	}
	if got.String() != want {
		t.Errorf("matrix:%s\nwant:%s", got.String(), want)
	}
	if len(Actions(Viewer)) != 1 || len(Actions(Owner)) != 7 || Actions("nobody") != nil {
		t.Errorf("Actions: viewer %v owner %v", Actions(Viewer), Actions(Owner))
	}
}

func TestCanRemove(t *testing.T) {
	for _, tc := range []struct {
		actor, target string
		self, want    bool
	}{
		{Owner, Owner, false, true}, {Owner, Admin, false, true}, {Owner, Viewer, false, true},
		{Admin, Owner, false, false}, {Admin, Admin, false, false}, {Admin, Viewer, false, true},
		{Viewer, Viewer, false, false}, {Viewer, Admin, false, false},
		{Viewer, Viewer, true, true}, {Admin, Admin, true, true}, {Owner, Owner, true, true},
	} {
		if got := CanRemove(tc.actor, tc.target, tc.self); got != tc.want {
			t.Errorf("CanRemove(%s, %s, self=%v) = %v", tc.actor, tc.target, tc.self, got)
		}
	}
}

func TestCanInviteAs(t *testing.T) {
	for _, tc := range []struct {
		actor, role string
		want        bool
	}{
		{Owner, Admin, true}, {Owner, Viewer, true}, {Owner, Owner, false},
		{Admin, Admin, true}, {Admin, Viewer, true}, {Admin, Owner, false},
		{Viewer, Viewer, false}, {Owner, "root", false},
	} {
		if got := CanInviteAs(tc.actor, tc.role); got != tc.want {
			t.Errorf("CanInviteAs(%s, %s) = %v", tc.actor, tc.role, got)
		}
	}
	if !ValidRole(Viewer) || ValidRole("root") || ValidRole("") {
		t.Error("ValidRole")
	}
}
