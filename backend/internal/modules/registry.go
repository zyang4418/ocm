// Package modules is the registration seam for modules contributed by a
// customization layer (downstream fork).
//
// Built-in modules are wired explicitly in main.go — they have
// interdependencies (course needs classroom+schedule, ai needs all four, ...)
// and that order is upstream's to own. This package exists for everything
// downstream adds: because the customization layer assembles the backend tree
// at file level (overlay wins, new paths compile in), a downstream module can
// join the binary WITHOUT touching main.go:
//
//  1. Add the module's own package anywhere, e.g.
//     backend/internal/myspace/handler.go (new paths compile as-is).
//
//  2. Add ONE new file to THIS directory (package modules), e.g.
//     backend/internal/modules/zz_myspace.go:
//
//     package modules
//
//     import (
//     "context"
//     "database/sql"
//     "net/http"
//
//     "ocm-backend/internal/myspace"
//     )
//
//     func init() {
//     Register(Module{
//     Name: "myspace",
//     Migrate: func(ctx context.Context, db *sql.DB) error {
//     return myspace.NewStore(db).Migrate(ctx)
//     },
//     Mount: func(ctx context.Context, db *sql.DB, mux *http.ServeMux, authenticate func(http.Handler) http.Handler) {
//     myspace.NewHandler(myspace.NewStore(db)).RegisterRoutes(ctx, mux, authenticate)
//     },
//     })
//     }
//
//     main.go runs every registered module's Migrate (after all built-in
//     migrations, sorted by Name for determinism) and then Mount (after the
//     built-in routes, with the standard authenticate chain), so the only
//     upstream file a downstream module ever touches is the new file itself.
//
// Permissions contributed by downstream modules keep using
// authz.RegisterPermissions from their own package init (before main runs, so
// the orphan-permission check in main still sees the final catalog).
package modules

import (
	"context"
	"database/sql"
	"net/http"
	"sort"
	"sync"
)

// Module is one downstream-contributed module. Migrate and Mount are both
// optional; a module that only registers background workers can keep Migrate
// nil and start its goroutines from Mount (ctx is the application lifetime).
type Module struct {
	// Name identifies the module in logs and orders registration (All is
	// sorted by Name). Must be unique and non-empty.
	Name string
	// Migrate creates the module's tables (CREATE TABLE IF NOT EXISTS, the
	// repo-wide migration convention). Runs before ANY Mount.
	Migrate func(ctx context.Context, db *sql.DB) error
	// Mount registers routes on mux. db is the shared database handle and
	// authenticate the standard auth chain (JWT -> subject -> audit); wrap
	// handlers with it exactly like the built-in modules do. Background
	// workers started here should take ctx and stop when it is canceled.
	Mount func(ctx context.Context, db *sql.DB, mux *http.ServeMux, authenticate func(http.Handler) http.Handler)
}

var (
	mu      sync.Mutex
	modules []Module
	byName  = make(map[string]bool)
	frozen  bool
)

// Register adds a downstream module. Call it from an init() in this package
// (see the package comment). Registering after the registry was consumed is a
// programming error and panics, as does an empty/duplicate name — both are
// startup-time contract violations, not runtime conditions.
func Register(m Module) {
	mu.Lock()
	defer mu.Unlock()
	if frozen {
		panic("modules: Register after All(): modules must register from init()")
	}
	if m.Name == "" {
		panic("modules: Register with empty Name")
	}
	if byName[m.Name] {
		panic("modules: duplicate module name " + m.Name)
	}
	byName[m.Name] = true
	modules = append(modules, m)
}

// All returns the registered modules sorted by Name. main.go consumes this
// once (freezing the registry) in two passes: Migrate for every module first,
// then Mount.
func All() []Module {
	mu.Lock()
	defer mu.Unlock()
	frozen = true
	out := make([]Module, len(modules))
	copy(out, modules)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
