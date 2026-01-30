package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/firecracker"
)

const (
	DefaultIdleTTL = 60 * time.Second // 1 minute idle timeout
)

type PooledVM struct {
	VM        *firecracker.VM
	Client    *firecracker.VsockClient
	Function  *domain.Function
	LastUsed  time.Time
	ColdStart bool
}

type Pool struct {
	manager   *firecracker.Manager
	vms       map[string]*PooledVM // key: functionID
	mu        sync.RWMutex
	idleTTL   time.Duration
	ctx       context.Context
	cancel    context.CancelFunc
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
		if now.Sub(pvm.LastUsed) > p.idleTTL {
			fmt.Printf("[pool] VM %s for function %s expired (idle %v)\n",
				pvm.VM.ID, funcID, now.Sub(pvm.LastUsed).Round(time.Second))
			pvm.Client.Close()
			p.manager.StopVM(pvm.VM.ID)
			delete(p.vms, funcID)
		}
	}
}

func (p *Pool) Acquire(ctx context.Context, fn *domain.Function) (*PooledVM, error) {
	p.mu.Lock()

	// Check for existing warm VM
	if pvm, ok := p.vms[fn.ID]; ok {
		pvm.LastUsed = time.Now()
		pvm.ColdStart = false
		p.mu.Unlock()

		// Ping to check health
		if err := pvm.Client.Ping(); err != nil {
			p.mu.Lock()
			delete(p.vms, fn.ID)
			p.mu.Unlock()
			pvm.Client.Close()
			p.manager.StopVM(pvm.VM.ID)
			// Fall through to create new VM
		} else {
			fmt.Printf("[pool] Reusing warm VM %s for function %s\n", pvm.VM.ID, fn.Name)
			return pvm, nil
		}
	} else {
		p.mu.Unlock()
	}

	// Cold start: create new VM
	fmt.Printf("[pool] Cold start: creating VM for function %s (runtime: %s)\n", fn.Name, fn.Runtime)

	vm, err := p.manager.CreateVM(ctx, fn.Runtime)
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
	}

	p.mu.Lock()
	p.vms[fn.ID] = pvm
	p.mu.Unlock()

	fmt.Printf("[pool] VM %s created for function %s\n", vm.ID, fn.Name)
	return pvm, nil
}

func (p *Pool) Release(pvm *PooledVM) {
	p.mu.Lock()
	defer p.mu.Unlock()
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
	p.mu.RLock()
	defer p.mu.RUnlock()

	vmStats := make([]map[string]interface{}, 0, len(p.vms))
	for funcID, pvm := range p.vms {
		vmStats = append(vmStats, map[string]interface{}{
			"function_id": funcID,
			"vm_id":       pvm.VM.ID,
			"runtime":     pvm.VM.Runtime,
			"idle_sec":    time.Since(pvm.LastUsed).Seconds(),
			"created_at":  pvm.VM.CreatedAt.Format(time.RFC3339),
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
