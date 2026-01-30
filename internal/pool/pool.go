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

	// Maximum concurrent pre-warm goroutines
	maxPreWarmConcurrency = 4
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

// Fix #3: Collect expired VMs under lock, then stop them without lock.
func (p *Pool) cleanupExpired() {
	type expiredVM struct {
		client *firecracker.VsockClient
		vmID   string
	}
	var toStop []expiredVM

	p.mu.Lock()
	now := time.Now()
	for funcID, vmList := range p.vms {
		minReplicas := 0
		if len(vmList) > 0 {
			minReplicas = vmList[0].Function.MinReplicas
		}

		activeCount := len(vmList)
		var kept []*PooledVM

		for _, pvm := range vmList {
			if pvm.busy {
				kept = append(kept, pvm)
				continue
			}

			if activeCount > minReplicas && now.Sub(pvm.LastUsed) > p.idleTTL {
				fmt.Printf("[pool] VM %s for function %s expired (idle %v)\n",
					pvm.VM.ID, funcID, now.Sub(pvm.LastUsed).Round(time.Second))
				toStop = append(toStop, expiredVM{client: pvm.Client, vmID: pvm.VM.ID})
				activeCount--
				continue
			}
			kept = append(kept, pvm)
		}
		p.vms[funcID] = kept
	}
	p.mu.Unlock()

	// Stop VMs without holding the lock
	for _, e := range toStop {
		e.client.Close()
		p.manager.StopVM(e.vmID)
	}
}

// Fix #6: EnsureReady with bounded concurrency.
func (p *Pool) EnsureReady(ctx context.Context, fn *domain.Function) error {
	p.mu.RLock()
	currentCount := len(p.vms[fn.ID])
	p.mu.RUnlock()

	needed := fn.MinReplicas - currentCount
	if needed <= 0 {
		return nil
	}

	fmt.Printf("[pool] Pre-warming %d VMs for function %s\n", needed, fn.Name)

	sem := make(chan struct{}, maxPreWarmConcurrency)
	var wg sync.WaitGroup
	for i := 0; i < needed; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			pvm, err := p.createVM(context.Background(), fn)
			if err != nil {
				fmt.Printf("[pool] Failed to pre-warm VM: %v\n", err)
				return
			}
			pvm.busy = false

			p.mu.Lock()
			p.vms[fn.ID] = append(p.vms[fn.ID], pvm)
			p.mu.Unlock()
		}()
	}
	wg.Wait()
	return nil
}

// Fix #5: Try all idle VMs, not just the first one.
// Fix #8: Skip ping on hot path; let execution failure trigger eviction.
// Fix #10: Use singleflight to deduplicate concurrent cold starts per function.
func (p *Pool) Acquire(ctx context.Context, fn *domain.Function) (*PooledVM, error) {
	p.mu.Lock()

	// Scan all idle VMs
	vmList := p.vms[fn.ID]
	for _, pvm := range vmList {
		if !pvm.busy {
			pvm.busy = true
			pvm.LastUsed = time.Now()
			pvm.ColdStart = false
			p.mu.Unlock()
			fmt.Printf("[pool] Reusing warm VM %s for %s\n", pvm.VM.ID, fn.Name)
			return pvm, nil
		}
	}
	p.mu.Unlock()

	// Cold start with singleflight to avoid thundering herd.
	// Each concurrent caller for the same function shares a single VM creation.
	// If one is already in-flight, subsequent callers wait for it.
	// But we only coalesce if there are NO idle VMs. Once the first VM is created,
	// the next Acquire will find it idle (if released quickly).
	val, err, shared := p.group.Do(fn.ID, func() (interface{}, error) {
		return p.createVM(ctx, fn)
	})
	if err != nil {
		return nil, err
	}

	pvm := val.(*PooledVM)
	if shared {
		// Another goroutine created this VM and got it. We need our own.
		// Fall back to direct creation.
		pvm, err = p.createVM(ctx, fn)
		if err != nil {
			return nil, err
		}
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

// Fix #4: Remove double-unlock panic. Use explicit lock/unlock without defer.
func (p *Pool) Evict(funcID string) {
	p.mu.Lock()
	list, ok := p.vms[funcID]
	if !ok {
		p.mu.Unlock()
		return
	}
	delete(p.vms, funcID)
	p.mu.Unlock()

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
