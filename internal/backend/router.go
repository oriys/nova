package backend

import (
	"context"
	"fmt"
	"sync"

	"github.com/oriys/nova/internal/domain"
)

// BackendFactory lazily initializes a backend implementation.
type BackendFactory func() (Backend, error)

type routerEntry struct {
	mu      sync.Mutex
	backend Backend
	factory BackendFactory
}

func (e *routerEntry) get(backendType domain.BackendType) (Backend, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.backend != nil {
		return e.backend, nil
	}
	if e.factory == nil {
		return nil, fmt.Errorf("backend %q is not configured", backendType)
	}

	b, err := e.factory()
	if err != nil {
		return nil, err
	}
	e.backend = b
	return b, nil
}

// Router dispatches backend operations using function-level backend preference.
//
// Backend selection rules:
// - fn.Backend == "" or "auto" => use default backend
// - fn.Backend is explicit       => use that backend
//
// VM -> backend mapping is tracked so StopVM/NewClient are routed to the
// backend that created the VM.
type Router struct {
	defaultBackend domain.BackendType
	entries        map[domain.BackendType]*routerEntry
	vmBackend      sync.Map // map[vmID]string -> domain.BackendType
}

// NewRouter creates a backend router with lazy factories.
func NewRouter(defaultBackend domain.BackendType, factories map[domain.BackendType]BackendFactory) (*Router, error) {
	if defaultBackend == "" || defaultBackend == domain.BackendAuto {
		return nil, fmt.Errorf("default backend must be explicit, got %q", defaultBackend)
	}
	if len(factories) == 0 {
		return nil, fmt.Errorf("at least one backend factory is required")
	}

	entries := make(map[domain.BackendType]*routerEntry, len(factories))
	for backendType, factory := range factories {
		if backendType == "" || backendType == domain.BackendAuto {
			return nil, fmt.Errorf("invalid backend key %q", backendType)
		}
		entries[backendType] = &routerEntry{factory: factory}
	}
	if _, ok := entries[defaultBackend]; !ok {
		return nil, fmt.Errorf("default backend %q is not configured", defaultBackend)
	}

	return &Router{
		defaultBackend: defaultBackend,
		entries:        entries,
	}, nil
}

// DefaultBackend returns the default backend type used for "auto".
func (r *Router) DefaultBackend() domain.BackendType {
	return r.defaultBackend
}

// EnsureReady eagerly initializes the given backend (or default for auto/empty).
func (r *Router) EnsureReady(backendType domain.BackendType) error {
	_, err := r.backendForType(backendType)
	return err
}

func (r *Router) resolveBackendType(fn *domain.Function) domain.BackendType {
	if fn == nil {
		return r.defaultBackend
	}
	if fn.Backend == "" || fn.Backend == domain.BackendAuto {
		return r.defaultBackend
	}
	return fn.Backend
}

func (r *Router) backendForType(backendType domain.BackendType) (Backend, error) {
	if backendType == "" || backendType == domain.BackendAuto {
		backendType = r.defaultBackend
	}

	entry, ok := r.entries[backendType]
	if !ok {
		return nil, fmt.Errorf("backend %q is not enabled on this node", backendType)
	}

	b, err := entry.get(backendType)
	if err != nil {
		return nil, fmt.Errorf("init backend %q: %w", backendType, err)
	}
	return b, nil
}

func (r *Router) backendForFunction(fn *domain.Function) (domain.BackendType, Backend, error) {
	backendType := r.resolveBackendType(fn)
	b, err := r.backendForType(backendType)
	if err != nil {
		return "", nil, err
	}
	return backendType, b, nil
}

func (r *Router) CreateVM(ctx context.Context, fn *domain.Function, codeContent []byte) (*VM, error) {
	backendType, b, err := r.backendForFunction(fn)
	if err != nil {
		return nil, err
	}
	vm, err := b.CreateVM(ctx, fn, codeContent)
	if err != nil {
		return nil, err
	}
	if vm != nil {
		r.vmBackend.Store(vm.ID, backendType)
	}
	return vm, nil
}

func (r *Router) CreateVMWithFiles(ctx context.Context, fn *domain.Function, files map[string][]byte) (*VM, error) {
	backendType, b, err := r.backendForFunction(fn)
	if err != nil {
		return nil, err
	}
	vm, err := b.CreateVMWithFiles(ctx, fn, files)
	if err != nil {
		return nil, err
	}
	if vm != nil {
		r.vmBackend.Store(vm.ID, backendType)
	}
	return vm, nil
}

func (r *Router) StopVM(vmID string) error {
	if vmID == "" {
		return fmt.Errorf("vm id is required")
	}

	backendType := r.defaultBackend
	if stored, ok := r.vmBackend.Load(vmID); ok {
		if bt, ok := stored.(domain.BackendType); ok && bt != "" {
			backendType = bt
		}
	}

	b, err := r.backendForType(backendType)
	if err != nil {
		return err
	}

	stopErr := b.StopVM(vmID)
	r.vmBackend.Delete(vmID)
	return stopErr
}

func (r *Router) NewClient(vm *VM) (Client, error) {
	if vm == nil {
		return nil, fmt.Errorf("vm is required")
	}

	backendType := r.defaultBackend
	if stored, ok := r.vmBackend.Load(vm.ID); ok {
		if bt, ok := stored.(domain.BackendType); ok && bt != "" {
			backendType = bt
		}
	}

	b, err := r.backendForType(backendType)
	if err != nil {
		return nil, err
	}
	return b.NewClient(vm)
}

func (r *Router) Shutdown() {
	for _, entry := range r.entries {
		entry.mu.Lock()
		b := entry.backend
		entry.mu.Unlock()
		if b != nil {
			b.Shutdown()
		}
	}
}

func (r *Router) SnapshotDir() string {
	b, err := r.backendForType(r.defaultBackend)
	if err != nil {
		return ""
	}
	return b.SnapshotDir()
}
