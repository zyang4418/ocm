package iam

import (
	"testing"
	"time"

	"ocm-backend/internal/authz"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func timePtr(t *testing.T, s string) *time.Time {
	v := mustTime(t, s)
	return &v
}

func role(id int64, perms ...string) Role {
	return Role{ID: id, Code: "r" + string(rune('0'+id)), Perms: perms}
}

func TestEffectiveUnion(t *testing.T) {
	now := mustTime(t, "2026-08-14T12:00:00Z")
	got := Effective(now,
		[]RoleGrant{{Role: role(1, authz.ClassroomRead, authz.CourseRead)}},
		[]RoleGrant{{Role: role(2, authz.ClassroomBook)}},
		[]PermGrant{{Permission: authz.RepairCreate}},
		[]int64{7, 7, 9},
	)

	if !got.Permissions[authz.ClassroomRead] || !got.Permissions[authz.CourseRead] ||
		!got.Permissions[authz.ClassroomBook] || !got.Permissions[authz.RepairCreate] {
		t.Fatalf("union missing permissions: %v", got.Permissions)
	}
	if len(got.Roles) != 2 || got.Roles[0].ID != 1 || got.Roles[1].ID != 2 {
		t.Fatalf("roles not de-duplicated in order: %+v", got.Roles)
	}
	if len(got.GroupIDs) != 2 || got.GroupIDs[0] != 7 || got.GroupIDs[1] != 9 {
		t.Fatalf("group ids not de-duplicated and sorted: %v", got.GroupIDs)
	}
}

func TestEffectiveExpiredGrantsDropped(t *testing.T) {
	now := mustTime(t, "2026-08-14T12:00:00Z")
	// Boundary: expiresAt == now counts as expired.
	atNow := timePtr(t, "2026-08-14T12:00:00Z")
	past := timePtr(t, "2026-08-13T12:00:00Z")
	future := timePtr(t, "2026-08-15T12:00:00Z")
	got := Effective(now,
		[]RoleGrant{
			{Role: role(1, authz.ClassroomRead), ExpiresAt: atNow},
			{Role: role(2, authz.CourseRead), ExpiresAt: past},
			{Role: role(3, authz.ClassroomBook), ExpiresAt: future},
		},
		nil,
		[]PermGrant{
			{Permission: authz.RepairCreate, ExpiresAt: past},
			{Permission: authz.CourseRead, ExpiresAt: future},
		},
		nil,
	)

	if got.Permissions[authz.ClassroomRead] || got.Permissions[authz.RepairCreate] {
		t.Fatalf("expired grants must be dropped: %v", got.Permissions)
	}
	if !got.Permissions[authz.ClassroomBook] || !got.Permissions[authz.CourseRead] {
		t.Fatalf("unexpired grants must survive: %v", got.Permissions)
	}
	if len(got.Roles) != 1 || got.Roles[0].ID != 3 {
		t.Fatalf("expired roles must be dropped from the role list: %+v", got.Roles)
	}
}

func TestEffectiveWildcardCollapses(t *testing.T) {
	now := mustTime(t, "2026-08-14T12:00:00Z")
	got := Effective(now,
		[]RoleGrant{{Role: role(1, authz.Wildcard)}},
		[]RoleGrant{{Role: role(2, authz.ClassroomRead)}},
		[]PermGrant{{Permission: authz.CourseRead}},
		nil,
	)

	if len(got.Permissions) != 1 || !got.Permissions[authz.Wildcard] {
		t.Fatalf("wildcard must collapse the set to {'*'}: %v", got.Permissions)
	}
	// Roles are still reported in full for display.
	if len(got.Roles) != 2 {
		t.Fatalf("wildcard must keep the role list: %+v", got.Roles)
	}
}

func TestEffectiveSameRoleViaUserAndGroup(t *testing.T) {
	now := mustTime(t, "2026-08-14T12:00:00Z")
	got := Effective(now,
		[]RoleGrant{{Role: role(1, authz.ClassroomRead)}},
		[]RoleGrant{{Role: role(1, authz.ClassroomRead)}},
		nil,
		[]int64{3},
	)
	if len(got.Roles) != 1 {
		t.Fatalf("role granted directly and via group must appear once: %+v", got.Roles)
	}
}

func TestEffectiveEmpty(t *testing.T) {
	now := mustTime(t, "2026-08-14T12:00:00Z")
	got := Effective(now, nil, nil, nil, nil)
	if len(got.Permissions) != 0 || len(got.Roles) != 0 || len(got.GroupIDs) != 0 {
		t.Fatalf("no grants must yield empty result: %+v", got)
	}
}
