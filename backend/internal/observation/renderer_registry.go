package observation

import (
	"database/sql"
	"sync"
)

// RendererFactory builds the deployment's document backend from the shared
// database handle. It runs once during main's wiring, immediately after the
// observation schema exists; a factory needing its own tables must migrate
// them itself (CREATE TABLE IF NOT EXISTS, the repo-wide convention) — the
// internal/modules migration loop has not run yet.
type RendererFactory func(db *sql.DB) (Renderer, error)

var (
	rendererMu      sync.Mutex
	rendererFactory RendererFactory
)

// RegisterRendererFactory installs the customization layer's document backend.
// Call it from package init in a package the binary actually imports — for a
// downstream fork that is the internal/modules file-level assembly, where a
// new file runs its init without needing to call modules.Register. Do NOT put
// the registration in a file inside this package: implementations must import
// observation for the Renderer contract, so that would create an import
// cycle. A double registration is a wiring error, not a recoverable condition.
func RegisterRendererFactory(factory RendererFactory) {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	if factory == nil {
		panic("observation: RegisterRendererFactory with nil factory")
	}
	if rendererFactory != nil {
		panic("observation: renderer factory already registered")
	}
	rendererFactory = factory
}

// ResolveRenderer builds the registered Renderer, or returns (nil, nil) when
// the deployment ships none — the open-source default that keeps the module
// fully functional for CRUD/submit with templates/export disabled.
func ResolveRenderer(db *sql.DB) (Renderer, error) {
	rendererMu.Lock()
	factory := rendererFactory
	rendererMu.Unlock()
	if factory == nil {
		return nil, nil
	}
	return factory(db)
}
