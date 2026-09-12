package modules

import "testing"

// Register panics on contract violations — both are startup-time errors that
// must never pass silently (a duplicate name would double-mount routes; an
// empty name would break the deterministic ordering contract).
func TestRegisterRejectsBadModules(t *testing.T) {
	cases := []struct {
		name    string
		module  Module
		wantDup bool
	}{
		{name: "empty name", module: Module{Name: ""}},
		{name: "first ok", module: Module{Name: "alpha"}},
		{name: "duplicate", module: Module{Name: "alpha"}, wantDup: true},
	}
	for _, tc := range cases {
		func() {
			defer func() {
				r := recover()
				if tc.wantDup || tc.module.Name == "" {
					if r == nil {
						t.Fatalf("%s: Register did not panic", tc.name)
					}
					return
				}
				if r != nil {
					t.Fatalf("%s: unexpected panic: %v", tc.name, r)
				}
			}()
			Register(tc.module)
		}()
	}
}

// All returns modules sorted by Name — the deterministic migration/mount
// order downstream modules rely on (name prefixes like "10-" exist so a
// module can control where it lands).
func TestAllSortedAndFrozen(t *testing.T) {
	Register(Module{Name: "zeta"})
	Register(Module{Name: "10-early"})
	Register(Module{Name: "mid"})

	all := All()
	if len(all) < 3 {
		t.Fatalf("expected at least 3 registered modules, got %d", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].Name >= all[i].Name {
			t.Fatalf("All() not sorted: %q before %q", all[i-1].Name, all[i].Name)
		}
	}

	// After All() the registry is frozen: late registration is a panic (the
	// main() wiring already consumed the list).
	defer func() {
		if recover() == nil {
			t.Fatal("Register after All() must panic")
		}
	}()
	Register(Module{Name: "too-late"})
}
