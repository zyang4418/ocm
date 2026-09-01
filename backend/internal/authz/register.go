package authz

import "sort"

// RegisterPermissions extends the built-in permission catalog (Catalog) at
// startup. Deployments that add their own modules can register permissions
// without forking catalog.go: the IAM console (GET /api/permissions) lists
// them and role grant validation (PermissionExists) accepts them
// automatically.
//
// Registration is intentionally idempotent: empty codes and the reserved
// wildcard are ignored, and a code that already exists keeps its first
// definition, so upstream entries can never be redefined. Call this during
// startup only, before concurrent requests are accepted — Catalog is plain
// data guarded by the single-threaded boot sequence. It must also run before
// any handler calls PermissionExists, i.e. before the HTTP server starts
// serving requests.
func RegisterPermissions(perms ...Permission) {
	merged := append([]Permission(nil), Catalog...)
	seen := make(map[string]struct{}, len(merged)+len(perms))
	for _, permission := range merged {
		seen[permission.Code] = struct{}{}
	}
	for _, permission := range perms {
		if permission.Code == "" || permission.Code == Wildcard {
			continue
		}
		if _, exists := seen[permission.Code]; exists {
			continue
		}
		merged = append(merged, permission)
		seen[permission.Code] = struct{}{}
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Category != merged[j].Category {
			return merged[i].Category < merged[j].Category
		}
		return merged[i].Code < merged[j].Code
	})
	Catalog = merged
}
