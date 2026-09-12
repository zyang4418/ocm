package observation

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"
)

// snapshotFactory returns the current factory so a test can restore it
// afterwards. RegisterRendererFactory mutates package-level state, which is
// accepted because registration is a boot-time-only, single-threaded step.
func snapshotFactory() RendererFactory {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	return rendererFactory
}

func restoreFactory(f RendererFactory) {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	rendererFactory = f
}

type stubRenderer struct{}

func (stubRenderer) Templates() (any, error) { return nil, nil }
func (stubRenderer) Validate(*Observation) ([]string, error) {
	return nil, nil
}
func (stubRenderer) Render(ctx context.Context, obs *Observation, w io.Writer) error {
	return nil
}

func TestResolveRendererWithoutRegistration(t *testing.T) {
	defer restoreFactory(snapshotFactory())
	restoreFactory(nil)

	renderer, err := ResolveRenderer((*sql.DB)(nil))
	if err != nil {
		t.Fatalf("unregistered deployment must resolve without error, got %v", err)
	}
	if renderer != nil {
		t.Fatalf("unregistered deployment must resolve to nil, got %T", renderer)
	}
}

func TestResolveRendererCallsFactoryWithDB(t *testing.T) {
	defer restoreFactory(snapshotFactory())

	var got *sql.DB
	RegisterRendererFactory(func(db *sql.DB) (Renderer, error) {
		got = db
		return stubRenderer{}, nil
	})

	db := &sql.DB{}
	renderer, err := ResolveRenderer(db)
	if err != nil {
		t.Fatalf("factory error must be nil, got %v", err)
	}
	if renderer == nil {
		t.Fatalf("factory result must be returned")
	}
	if got != db {
		t.Fatalf("factory must receive the db handle passed to ResolveRenderer")
	}
}

func TestResolveRendererPropagatesFactoryError(t *testing.T) {
	defer restoreFactory(snapshotFactory())

	want := errors.New("templates broken")
	RegisterRendererFactory(func(db *sql.DB) (Renderer, error) {
		return nil, want
	})

	renderer, err := ResolveRenderer((*sql.DB)(nil))
	if !errors.Is(err, want) {
		t.Fatalf("factory error must propagate, got %v", err)
	}
	if renderer != nil {
		t.Fatalf("no renderer must be returned on factory error, got %T", renderer)
	}
}

func TestRegisterRendererFactoryDuplicatePanics(t *testing.T) {
	defer restoreFactory(snapshotFactory())

	RegisterRendererFactory(func(db *sql.DB) (Renderer, error) { return stubRenderer{}, nil })

	defer func() {
		if recover() == nil {
			t.Fatalf("double registration must panic")
		}
	}()
	RegisterRendererFactory(func(db *sql.DB) (Renderer, error) { return stubRenderer{}, nil })
}

func TestRegisterRendererFactoryNilPanics(t *testing.T) {
	defer restoreFactory(snapshotFactory())

	defer func() {
		if recover() == nil {
			t.Fatalf("nil factory registration must panic")
		}
	}()
	RegisterRendererFactory(nil)
}
