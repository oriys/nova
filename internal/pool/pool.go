package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/pkg/singleflight"
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
	vms     map[string][]*PooledVM // key: functionID, val: list of VMs
	mu      sync.RWMutex
	group   singleflight.Group
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
		vms:     make(map[string][]*PooledVM),
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
	for funcID, vmList := range p.vms {
		// We need to know MinReplicas. Since all VMs for a function share the same
		// config, we can check the first one. If empty, it doesn't matter.
		minReplicas := 0
		if len(vmList) > 0 {
			minReplicas = vmList[0].Function.MinReplicas
		}

		activeCount := len(vmList)
		var newParams []*PooledVM

		// Filter loop
		for _, pvm := range vmList {
			// Always keep busy VMs
			if pvm.busy {
				newParams = append(newParams, pvm)
				continue
			}

			// If we have more than needed, check expiry
			if activeCount > minReplicas {
				if now.Sub(pvm.LastUsed) > p.idleTTL {
					fmt.Printf("[pool] VM %s for function %s expired (idle %v)\n",
						pvm.VM.ID, funcID, now.Sub(pvm.LastUsed).Round(time.Second))
					pvm.Client.Close()
					p.manager.StopVM(pvm.VM.ID)
					activeCount--
					continue // Drop from list
				}
			}
			newParams = append(newParams, pvm)
		}
		p.vms[funcID] = newParams
	}
}

// EnsureReady provision warm VMs up to minReplicas
func (p *Pool) EnsureReady(ctx context.Context, fn *domain.Function) error {
	p.mu.Lock()
	currentCount := len(p.vms[fn.ID])
	p.mu.Unlock()

	needed := fn.MinReplicas - currentCount
	if needed <= 0 {
		return nil
	}

	fmt.Printf("[pool] Pre-warming %d VMs for function %s\n", needed, fn.Name)
	for i := 0; i < needed; i++ {
		// Launch in parallel (or sequential, keeps it simple for now)
		go func() {
			// We manually create and add to pool
			// Note: This bypasses singleflight, which is fine for provisioning
			pvm, err := p.createVM(context.Background(), fn)
			if err != nil {
				fmt.Printf("[pool] Failed to pre-warm VM: %v\n", err)
				return
			}
			pvm.busy = false // Ready to take requests

			p.mu.Lock()
			p.vms[fn.ID] = append(p.vms[fn.ID], pvm)
			p.mu.Unlock()
		}()
	}
	return nil
}

// Acquire returns a PooledVM for the function, marked as busy.
func (p *Pool) Acquire(ctx context.Context, fn *domain.Function) (*PooledVM, error) {
	p.mu.Lock()
	
	// Scan for idle VM
	vmList := p.vms[fn.ID]
	for _, pvm := range vmList {
		if !pvm.busy {
			pvm.busy = true
			pvm.LastUsed = time.Now()
			pvm.ColdStart = false
			p.mu.Unlock()

			// Verify health
			if err := pvm.Client.Ping(); err != nil {
				// Bad VM, remove it (requires lock)
				p.removeVM(fn.ID, pvm)
				pvm.Client.Close()
				p.manager.StopVM(pvm.VM.ID)
				// Try again (recursive or loop? simpler to fall through to create)
			} else {
				fmt.Printf("[pool] Reusing warm VM %s for %s\n", pvm.VM.ID, fn.Name)
				return pvm, nil
			}
			break // Break to fall through
		}
	}
	p.mu.Unlock()

	// Cold start
	// Use singleflight to deduplicate concurrent "Cold Start" attempts for same function?
	// Actually, with multiple VMs allowed, we might NOT want to deduplicate strictly if
	// we want to scale up. But to avoid "thundering herd" creating 100 VMs for 100 requests,
	// singleflight per functionID is still a reasonable throttle.
	// But here, if we need a NEW VM, we just make one.
	
	// Let's create a new VM
	pvm, err := p.createVM(ctx, fn)
	if err != nil {
		return nil, err
	}
	
	p.mu.Lock()
	p.vms[fn.ID] = append(p.vms[fn.ID], pvm)
	p.mu.Unlock()
	
	return pvm, nil
}

func (p *Pool) removeVM(funcID string, target *PooledVM) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	list := p.vms[funcID]
	newList := make([]*PooledVM, 0, len(list))
	for _, vm := range list {
		if vm != target {
			newList = append(newList, vm)
		}
	}
	p.vms[funcID] = newList
}

func (p *Pool) createVM(ctx context.Context, fn *domain.Function) (*PooledVM, error) {
	fmt.Printf("[pool] Creating VM: %s (runtime: %s)\n", fn.Name, fn.Runtime)

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

	list, ok := p.vms[funcID]
	if !ok {
		return
	}
	delete(p.vms, funcID)
	p.mu.Unlock() // Unlock early to avoid holding lock during slow stops

	for _, pvm := range list {
		pvm.Client.Close()
		p.manager.StopVM(pvm.VM.ID)
	}
}

func (p *Pool) Stats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	vmStats := make([]map[string]interface{}, 0)
	totalVMs := 0
	
	for funcID, list := range p.vms {
		totalVMs += len(list)
		for _, pvm := range list {
			vmStats = append(vmStats, map[string]interface{}{
				"function_id": funcID,
				"vm_id":       pvm.VM.ID,
				"runtime":     pvm.VM.Runtime,
				"busy":        pvm.busy,
				"idle_sec":    time.Since(pvm.LastUsed).Seconds(),
			})
		}
	}

	return map[string]interface{}{
		"active_vms": totalVMs,
		"idle_ttl":   p.idleTTL.String(),
		"vms":        vmStats,
	}
}

func (p *Pool) Shutdown() {
	p.cancel()

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, list := range p.vms {
		for _, pvm := range list {
			pvm.Client.Close()
			p.manager.StopVM(pvm.VM.ID)
		}
	}
	p.vms = make(map[string][]*PooledVM)
}
