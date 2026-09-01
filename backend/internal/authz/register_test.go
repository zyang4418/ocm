package authz

import (
	"reflect"
	"sort"
	"testing"
)

// snapshot returns the current Catalog contents so a test can restore them
// afterwards. RegisterPermissions mutates the package-level Catalog, which is
// accepted because registration is a boot-time-only, single-threaded step.
func snapshot() []Permission {
	return append([]Permission(nil), Catalog...)
}

func restore(perms []Permission) {
	Catalog = perms
}

func catalogSorted() bool {
	return sort.SliceIsSorted(Catalog, func(i, j int) bool {
		if Catalog[i].Category != Catalog[j].Category {
			return Catalog[i].Category < Catalog[j].Category
		}
		return Catalog[i].Code < Catalog[j].Code
	})
}

func TestRegisterPermissionsAddsAndSorts(t *testing.T) {
	defer restore(snapshot())

	RegisterPermissions(
		Permission{Code: "zzz:last", Name: "Last", Category: "zzz", CategoryName: "ZZZ"},
		Permission{Code: "aaa:first", Name: "First", Category: "aaa", CategoryName: "AAA"},
	)

	if !PermissionExists("zzz:last") || !PermissionExists("aaa:first") {
		t.Fatalf("registered permissions must be recognized by PermissionExists")
	}

	// The merged catalog must stay sorted by category, then code.
	if !catalogSorted() {
		t.Fatalf("catalog is not sorted after registration")
	}
}

func TestRegisterPermissionsIdempotentAndFirstWins(t *testing.T) {
	defer restore(snapshot())

	RegisterPermissions(Permission{Code: "mod:read", Name: "First", Category: "mod", CategoryName: "Mod"})
	RegisterPermissions(Permission{Code: "mod:read", Name: "Second", Category: "mod", CategoryName: "Mod"})

	for _, p := range Catalog {
		if p.Code == "mod:read" && p.Name != "First" {
			t.Fatalf("re-registering a code must keep the first definition, got %q", p.Name)
		}
	}

	// Registering an existing upstream code must not change the entry either.
	RegisterPermissions(Permission{Code: UserRead, Name: "Hijacked", Category: "user", CategoryName: "用户管理"})
	for _, p := range Catalog {
		if p.Code == UserRead && p.Name == "Hijacked" {
			t.Fatalf("upstream permission %s was overwritten by registration", UserRead)
		}
	}
}

func TestRegisterPermissionsIgnoresInvalid(t *testing.T) {
	defer restore(snapshot())
	before := snapshot()

	RegisterPermissions(
		Permission{Code: "", Name: "Empty", Category: "x", CategoryName: "X"},     // empty code
		Permission{Code: Wildcard, Name: "All", Category: "*", CategoryName: "*"}, // reserved
	)

	if !reflect.DeepEqual(before, Catalog) {
		t.Fatalf("empty and wildcard registrations must leave the catalog unchanged")
	}
}

func TestRegisterPermissionsEmptyCall(t *testing.T) {
	defer restore(snapshot())
	before := snapshot()

	RegisterPermissions()

	if !reflect.DeepEqual(before, Catalog) {
		t.Fatalf("RegisterPermissions() with no arguments must be a no-op")
	}
}
