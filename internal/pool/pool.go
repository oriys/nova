package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/firecracker"
	"golang.org/x/sync/singleflight"
)

const (
	DefaultIdleTTL = 60 * time.Second
)

type PooledVM struct {
	VM        *firecracker.VM
	Client    *firecracker.VsockClient
	Function  *domain.Function
	LastUsed  time.Time
	ColdStart bool
	busy      bool // true while handling a request
}

type Pool struct {
	manager *firecracker.Manager
	vms     map[string]*PooledVM // key: functionID
	mu      sync.Mutex
	group   singleflight.Group // prevents duplicate VM creation
	idleTTL time.Duration
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewPool(manager *firecracker.Manager, idleTTL time.Duration) *Pool {
	if idleTTL == 0 {
		idleTTL = DefaultIdleTTL
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		manager: manager,
		vms:     make(map[string]*PooledVM),
		idleTTL: idleTTL,
		ctx:     ctx,
		cancel:  cancel,
	}

	go p.cleanupLoop()
	return p
}

func (p *Pool) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.cleanupExpired()
		}
	}
}

func (p *Pool) cleanupExpired() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	for funcID, pvm := range p.vms {
		// Don't evict a VM that's currently handling a request
		if pvm.busy {
			continue
		}
		if now.Sub(pvm.LastUsed) > p.idleTTL {
			fmt.Printf("[pool] VM %s for function %s expired (idle %v)\n",
				pvm.VM.ID, funcID, now.Sub(pvm.LastUsed).Round(time.Second))
			pvm.Client.Close()
			p.manager.StopVM(pvm.VM.ID)
			delete(p.vms, funcID)
		}
	}
}

// Acquire returns a PooledVM for the function, marked as busy.
// Uses singleflight to prevent duplicate VM creation for the same function.
func (p *Pool) Acquire(ctx context.Context, fn *domain.Function) (*PooledVM, error) {
	// Fast path: try to grab an existing idle VM
	p.mu.Lock()
	if pvm, ok := p.vms[fn.ID]; ok && !pvm.busy {
		pvm.busy = true
		pvm.LastUsed = time.Now()
		pvm.ColdStart = false
		p.mu.Unlock()

		// Verify health
		if err := pvm.Client.Ping(); err != nil {
			// Unhealthy, destroy and fall through to cold start
			p.mu.Lock()
			delete(p.vms, fn.ID)
			p.mu.Unlock()
			pvm.Client.Close()
			p.manager.StopVM(pvm.VM.ID)
		} else {
			fmt.Printf("[pool] Warm VM %s for %s\n", pvm.VM.ID, fn.Name)
			return pvm, nil
		}
	} else {
		p.mu.Unlock()
	}

	// Cold start: use singleflight so concurrent requests for the same
	// function don't each create a separate VM.
	result, err, _ := p.group.Do(fn.ID, func() (interface{}, error) {
		return p.createVM(ctx, fn)
	})
	if err != nil {
		return nil, err
	}

	pvm := result.(*PooledVM)

	// singleflight may return the same result to multiple callers.
	// Only the first one gets exclusive access; others must wait or create new.
	p.mu.Lock()
	existing, exists := p.vms[fn.ID]
	if exists && existing == pvm && pvm.busy {
		// Another caller already acquired this VM from singleflight.
		// This caller must create a dedicated VM.
		p.mu.Unlock()
		return p.createVM(ctx, fn)
	}
	pvm.busy = true
	p.vms[fn.ID] = pvm
	p.mu.Unlock()

	return pvm, nil
}

func (p *Pool) createVM(ctx context.Context, fn *domain.Function) (*PooledVM, error) {
	fmt.Printf("[pool] Cold start: %s (runtime: %s)\n", fn.Name, fn.Runtime)

	vm, err := p.manager.CreateVM(ctx, fn)
	if err != nil {
		return nil, fmt.Errorf("create VM: %w", err)
	}

	client, err := firecracker.NewVsockClient(vm)
	if err != nil {
		p.manager.StopVM(vm.ID)
		return nil, fmt.Errorf("connect vsock: %w", err)
	}

	if err := client.Init(fn); err != nil {
		client.Close()
		p.manager.StopVM(vm.ID)
		return nil, fmt.Errorf("init function: %w", err)
	}

	pvm := &PooledVM{
		VM:        vm,
		Client:    client,
		Function:  fn,
		LastUsed:  time.Now(),
		ColdStart: true,
		busy:      true,
	}

	fmt.Printf("[pool] VM %s ready for %s\n", vm.ID, fn.Name)
	return pvm, nil
}

// Release marks the VM as idle so it can be reused or cleaned up.
func (p *Pool) Release(pvm *PooledVM) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pvm.busy = false
	pvm.LastUsed = time.Now()
}

func (p *Pool) Evict(funcID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if pvm, ok := p.vms[funcID]; ok {
		pvm.Client.Close()
		p.manager.StopVM(pvm.VM.ID)
		delete(p.vms, funcID)
	}
}

func (p *Pool) Stats() map[string]interface{} {
	p.mu.Lock()
	defer p.mu.Unlock()

	vmStats := make([]map[string]interface{}, 0, len(p.vms))
	for funcID, pvm := range p.vms {
		vmStats = append(vmStats, map[string]interface{}{
			"function_id": funcID,
			"vm_id":       pvm.VM.ID,
			"runtime":     pvm.VM.Runtime,
			"busy":        pvm.busy,
			"idle_sec":    time.Since(pvm.LastUsed).Seconds(),
		})
	}

	return map[string]interface{}{
		"active_vms": len(p.vms),
		"idle_ttl":   p.idleTTL.String(),
		"vms":        vmStats,
	}
}

func (p *Pool) Shutdown() {
	p.cancel()

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, pvm := range p.vms {
		pvm.Client.Close()
		p.manager.StopVM(pvm.VM.ID)
	}
	p.vms = make(map[string]*PooledVM)
}
