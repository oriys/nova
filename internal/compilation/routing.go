// Package compilation handles cross-compilation routing for multi-arch deployments.
package compilation

import (
	"context"
	"fmt"
	"sync"

	"github.com/oriys/nova/internal/domain"
)

// Target describes a compilation target.
type Target struct {
	Runtime domain.Runtime `json:"runtime"`
	Arch    domain.Arch    `json:"arch"`
	FuncID  string         `json:"func_id"`
}

// Result describes the outcome of a compilation.
type Result struct {
	Target     Target `json:"target"`
	BinaryHash string `json:"binary_hash"`
	SizeBytes  int64  `json:"size_bytes"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

// Compiler can compile code for a given target.
type Compiler interface {
	Compile(ctx context.Context, target Target, sourceCode []byte) (*Result, []byte, error)
}

// Router decides how to compile for a given architecture.
type Router struct {
	mu              sync.RWMutex
	compilers       map[domain.Arch]Compiler
	crossCompileMap map[crossKey]bool // (from, to) -> supports cross-compile
}

type crossKey struct {
	from, to domain.Arch
}

// NewRouter creates a new compilation router.
func NewRouter() *Router {
	return &Router{
		compilers: make(map[domain.Arch]Compiler),
		crossCompileMap: map[crossKey]bool{
			{domain.ArchAMD64, domain.ArchARM64}: true, // Go, Rust support cross-compile
			{domain.ArchARM64, domain.ArchAMD64}: true,
		},
	}
}

// RegisterCompiler registers a compiler for a specific architecture.
func (r *Router) RegisterCompiler(arch domain.Arch, c Compiler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.compilers[arch] = c
}

// CanCrossCompile returns true if cross-compilation from hostArch to targetArch is supported.
func (r *Router) CanCrossCompile(hostArch, targetArch domain.Arch) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.crossCompileMap[crossKey{hostArch, targetArch}]
}

// Route determines the best compilation strategy for a target.
func (r *Router) Route(hostArch domain.Arch, target Target) (CompileStrategy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if target.Arch == "" || target.Arch == hostArch {
		// Native compilation
		if c, ok := r.compilers[hostArch]; ok {
			return CompileStrategy{Compiler: c, Arch: hostArch, CrossCompile: false}, nil
		}
		return CompileStrategy{}, fmt.Errorf("no compiler for native arch %s", hostArch)
	}

	// Try cross-compilation on current host
	if r.crossCompileMap[crossKey{hostArch, target.Arch}] {
		if c, ok := r.compilers[hostArch]; ok {
			return CompileStrategy{Compiler: c, Arch: target.Arch, CrossCompile: true}, nil
		}
	}

	// Try native compilation on target-arch node
	if c, ok := r.compilers[target.Arch]; ok {
		return CompileStrategy{Compiler: c, Arch: target.Arch, CrossCompile: false, RemoteNode: true}, nil
	}

	return CompileStrategy{}, fmt.Errorf("no compilation path from %s to %s for runtime %s",
		hostArch, target.Arch, target.Runtime)
}

// CompileStrategy describes how to compile for a target.
type CompileStrategy struct {
	Compiler     Compiler
	Arch         domain.Arch
	CrossCompile bool
	RemoteNode   bool // Needs routing to a different node
}

// SupportsCrossCompile returns true if the runtime supports cross-compilation.
func SupportsCrossCompile(rt domain.Runtime) bool {
	switch rt {
	case domain.RuntimeGo, domain.RuntimeRust, domain.RuntimeZig, domain.RuntimeC, domain.RuntimeCpp:
		return true
	default:
		return false
	}
}
