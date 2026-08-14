package iam

import (
	"sort"
	"time"

	"ocm-backend/internal/authz"
)

// Effective merges direct role grants ∪ group role grants ∪ direct
// permission grants into one permission set. Grants whose ExpiresAt is
// non-nil and not after now are dropped (the store already filters expired
// rows in SQL; the filter here is a second line of defense and the only one
// exercised by unit tests). If any surviving role holds the "*" wildcard the
// whole result collapses to {"*": true} — the wildcard grants everything,
// and extra permissions would only mislead callers. Roles are de-duplicated
// by ID (direct roles first, then group-derived, stable order); GroupIDs are
// de-duplicated.
//
// Pure function with no DB dependency, so it is unit-testable in isolation.
func Effective(now time.Time, directRoles, groupRoles []RoleGrant, directPerms []PermGrant, groupIDs []int64) EffectiveResult {
	alive := func(expiresAt *time.Time) bool {
		return expiresAt == nil || expiresAt.After(now)
	}

	permissions := make(map[string]bool)
	var roles []Role
	seenRoles := make(map[int64]bool)
	addRole := func(role Role) {
		if seenRoles[role.ID] {
			return
		}
		seenRoles[role.ID] = true
		roles = append(roles, role)
	}

	// Direct role grants before group-derived ones: a user's own roles are
	// the primary identity, group roles the inherited part.
	for _, g := range append(append([]RoleGrant{}, directRoles...), groupRoles...) {
		if !alive(g.ExpiresAt) {
			continue
		}
		addRole(g.Role)
		for _, perm := range g.Role.Perms {
			permissions[perm] = true
		}
	}
	for _, g := range directPerms {
		if !alive(g.ExpiresAt) {
			continue
		}
		permissions[g.Permission] = true
	}

	if permissions[authz.Wildcard] {
		permissions = map[string]bool{authz.Wildcard: true}
	}

	seenGroups := make(map[int64]bool)
	groups := make([]int64, 0, len(groupIDs))
	for _, id := range groupIDs {
		if seenGroups[id] {
			continue
		}
		seenGroups[id] = true
		groups = append(groups, id)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i] < groups[j] })

	return EffectiveResult{Permissions: permissions, Roles: roles, GroupIDs: groups}
}
