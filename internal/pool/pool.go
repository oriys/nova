package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/pkg/singleflight"
)

// ErrConcurrencyLimit is returned when max concurrency is reached
var ErrConcurrencyLimit = errors.New("concurrency limit reached")

const (
	DefaultIdleTTL = 60 * time.Second

	// Maximum concurrent pre-warm goroutines
	maxPreWarmConcurrency = 8
)

type PooledVM struct {
	VM            *backend.VM
	Client        backend.Client
	Function      *domain.Function
	LastUsed      time.Time
	ColdStart     bool
	inflight      int
	maxConcurrent int
}

// functionPool holds VMs for a single function with its own lock
type functionPool struct {
	vms         []*PooledVM
	mu          sync.Mutex
	maxReplicas atomic.Int32 // max concurrent VMs (0 = unlimited)
	waiters     int          // number of goroutines waiting for a VM
	cond        *sync.Cond   // condition variable for waiting
	codeHash    atomic.Value // string: hash of code when VMs were created
}

// SnapshotCallback is called after a cold start to create a snapshot
type SnapshotCallback func(ctx context.Context, vmID, funcID string) error

type Pool struct {
	backend          backend.Backend
	pools            sync.Map // map[string]*functionPool - per-function pools
	group            singleflight.Group
	idleTTL          time.Duration
	ctx              context.Context
	cancel           context.CancelFunc
	snapshotCallback SnapshotCallback
	snapshotCache    sync.Map // funcID -> bool (true if snapshot exists)
	snapshotLocks    sync.Map // funcID -> *sync.Mutex
}

func NewPool(b backend.Backend, idleTTL time.Duration) *Pool {
	if idleTTL == 0 {
		idleTTL = DefaultIdleTTL
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		backend: b,
		idleTTL: idleTTL,
		ctx:     ctx,
		cancel:  cancel,
	}

	go p.cleanupLoop()
	go p.healthCheckLoop()
	return p
}

// SetSnapshotCallback sets the callback for creating snapshots after cold starts
func (p *Pool) SetSnapshotCallback(cb SnapshotCallback) {
	p.snapshotCallback = cb
}

// InvalidateSnapshotCache removes the cached snapshot status for a function
func (p *Pool) InvalidateSnapshotCache(funcID string) {
	p.snapshotCache.Delete(funcID)
}

// getOrCreatePool returns the function pool, creating it if needed
func (p *Pool) getOrCreatePool(funcID string) *functionPool {
	if fp, ok := p.pools.Load(funcID); ok {
		return fp.(*functionPool)
	}
	fp := &functionPool{}
	fp.cond = sync.NewCond(&fp.mu)
	actual, _ := p.pools.LoadOrStore(funcID, fp)
	return actual.(*functionPool)
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
	type expiredVM struct {
		client backend.Client
		vmID   string
	}
	var toStop []expiredVM

	now := time.Now()
	p.pools.Range(func(key, value interface{}) bool {
		funcID := key.(string)
		fp := value.(*functionPool)

		fp.mu.Lock()
		minReplicas := 0
		if len(fp.vms) > 0 {
			minReplicas = fp.vms[0].Function.MinReplicas
		}

		activeCount := len(fp.vms)
		var kept []*PooledVM

		for _, pvm := range fp.vms {
			if pvm.inflight > 0 {
				kept = append(kept, pvm)
				continue
			}

			if activeCount > minReplicas && now.Sub(pvm.LastUsed) > p.idleTTL {
				logging.Op().Info("VM expired",
					"vm_id", pvm.VM.ID,
					"function", funcID,
					"idle", now.Sub(pvm.LastUsed).Round(time.Second).String())
				toStop = append(toStop, expiredVM{client: pvm.Client, vmID: pvm.VM.ID})
				activeCount--
				continue
			}
			kept = append(kept, pvm)
		}
		fp.vms = kept
		fp.mu.Unlock()
		return true
	})

	// Stop VMs in parallel without holding any locks
	if len(toStop) > 0 {
		var wg sync.WaitGroup
		for _, e := range toStop {
			wg.Add(1)
			go func(client backend.Client, vmID string) {
				defer wg.Done()
				client.Close()
				p.backend.StopVM(vmID)
			}(e.client, e.vmID)
		}
		wg.Wait()
	}
}

func (p *Pool) EnsureReady(ctx context.Context, fn *domain.Function, codeContent []byte) error {
	fp := p.getOrCreatePool(fn.ID)

	fp.mu.Lock()
	currentCount := len(fp.vms)
	fp.mu.Unlock()

	needed := fn.MinReplicas - currentCount
	if needed <= 0 {
		return nil
	}

	logging.Op().Info("pre-warming VMs", "count", needed, "function", fn.Name)

	sem := make(chan struct{}, maxPreWarmConcurrency)
	var wg sync.WaitGroup
	for i := 0; i < needed; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			pvm, err := p.createVM(context.Background(), fn, codeContent)
			if err != nil {
				logging.Op().Error("pre-warm failed", "error", err)
				return
			}
			pvm.inflight = 0

			fp.mu.Lock()
			fp.vms = append(fp.vms, pvm)
			fp.mu.Unlock()
		}()
	}
	wg.Wait()
	return nil
}

func (p *Pool) computeInstanceConcurrency(fn *domain.Function) int {
	// Firecracker backend keeps strong isolation: one request per VM.
	if p.backend.SnapshotDir() != "" {
		return 1
	}
	if fn.InstanceConcurrency > 0 {
		return fn.InstanceConcurrency
	}
	return 1
}

func (p *Pool) Acquire(ctx context.Context, fn *domain.Function, codeContent []byte) (*PooledVM, error) {
	fp := p.getOrCreatePool(fn.ID)

	// Check if code has changed using atomic load first
	if fn.CodeHash != "" {
		storedHash, _ := fp.codeHash.Load().(string)
		if storedHash != "" && storedHash != fn.CodeHash {
			// Double-check under lock before evicting
			fp.mu.Lock()
			storedHash2, _ := fp.codeHash.Load().(string)
			if storedHash2 != "" && storedHash2 != fn.CodeHash {
				logging.Op().Info("code change detected, evicting VMs",
					"function", fn.Name,
					"old_hash", storedHash2[:8],
					"new_hash", fn.CodeHash[:8])
				vmsToStop := fp.vms
				fp.vms = nil
				fp.codeHash.Store(fn.CodeHash)
				fp.mu.Unlock()

				// Stop all old VMs in background
				go func() {
					for _, pvm := range vmsToStop {
						pvm.Client.Close()
						p.backend.StopVM(pvm.VM.ID)
					}
				}()
			} else {
				fp.mu.Unlock()
			}
		} else if storedHash == "" {
			fp.codeHash.Store(fn.CodeHash)
		}
	}

	// Update max replicas atomically
	if fn.MaxReplicas > 0 {
		fp.maxReplicas.Store(int32(fn.MaxReplicas))
	}

	// Fast path: find an idle VM
	fp.mu.Lock()
	for _, pvm := range fp.vms {
		if pvm.inflight < pvm.maxConcurrent {
			pvm.inflight++
			pvm.LastUsed = time.Now()
			pvm.ColdStart = false
			fp.mu.Unlock()
			logging.Op().Debug("reusing warm VM", "vm_id", pvm.VM.ID, "function", fn.Name)
			return pvm, nil
		}
	}

	// Check concurrency limit
	maxReps := fp.maxReplicas.Load()
	if maxReps > 0 && len(fp.vms) >= int(maxReps) {
		// All VMs at capacity and at max replica limit - wait for one to become available
		logging.Op().Debug("concurrency limit reached, waiting", "limit", maxReps, "function", fn.Name)
		fp.waiters++

		// Wait with context timeout
		done := make(chan struct{})
		go func() {
			fp.cond.Wait()
			close(done)
		}()

		fp.mu.Unlock()

		select {
		case <-ctx.Done():
			fp.mu.Lock()
			fp.waiters--
			fp.mu.Unlock()
			return nil, ctx.Err()
		case <-done:
			fp.mu.Lock()
			fp.waiters--
			// Try to find an idle VM again
			for _, pvm := range fp.vms {
				if pvm.inflight < pvm.maxConcurrent {
					pvm.inflight++
					pvm.LastUsed = time.Now()
					pvm.ColdStart = false
					fp.mu.Unlock()
					logging.Op().Debug("got VM after waiting", "vm_id", pvm.VM.ID, "function", fn.Name)
					return pvm, nil
				}
			}
			fp.mu.Unlock()
			// No idle VM found, but we might be able to create one now
			// (another VM might have been removed)
		}
	} else {
		fp.mu.Unlock()
	}

	// Cold start with singleflight to avoid thundering herd
	val, err, shared := p.group.Do(fn.ID, func() (interface{}, error) {
		return p.createVM(ctx, fn, codeContent)
	})
	if err != nil {
		return nil, err
	}

	pvm := val.(*PooledVM)
	if shared {
		// Another goroutine created this VM and got it. We need our own.
		pvm, err = p.createVM(ctx, fn, codeContent)
		if err != nil {
			return nil, err
		}
	}

	fp.mu.Lock()
	fp.vms = append(fp.vms, pvm)
	fp.mu.Unlock()

	return pvm, nil
}

func (p *Pool) createVM(ctx context.Context, fn *domain.Function, codeContent []byte) (*PooledVM, error) {
	logging.Op().Info("creating VM", "function", fn.Name, "runtime", fn.Runtime)

	bootStart := time.Now()
	vm, err := p.backend.CreateVM(ctx, fn, codeContent)
	if err != nil {
		return nil, err
	}
	bootDurationMs := time.Since(bootStart).Milliseconds()

	client, err := p.backend.NewClient(vm)
	if err != nil {
		p.backend.StopVM(vm.ID)
		return nil, err
	}

	if err := client.Init(fn); err != nil {
		client.Close()
		p.backend.StopVM(vm.ID)
		return nil, err
	}

	// Record boot duration metric
	// Check if this was a snapshot restore (boot time < 1000ms typically indicates snapshot)
	fromSnapshot := bootDurationMs < 1000
	metrics.RecordVMBootDuration(fn.Name, string(fn.Runtime), bootDurationMs, fromSnapshot)
	if fromSnapshot {
		metrics.RecordSnapshotRestoreTime(fn.Name, bootDurationMs)
	}

	pvm := &PooledVM{
		VM:            vm,
		Client:        client,
		Function:      fn,
		LastUsed:      time.Now(),
		ColdStart:     true,
		inflight:      1,
		maxConcurrent: p.computeInstanceConcurrency(fn),
	}

	// Create snapshot if callback is set and no snapshot exists for this function
	if p.snapshotCallback != nil {
		if _, hasSnapshot := p.snapshotCache.Load(fn.ID); !hasSnapshot {
			lock := p.getSnapshotLock(fn.ID)
			lock.Lock()
			if _, hasSnapshot := p.snapshotCache.Load(fn.ID); !hasSnapshot {
				logging.Op().Info("creating snapshot after cold start", "function", fn.Name, "vm_id", vm.ID)
				if err := p.snapshotCallback(ctx, vm.ID, fn.ID); err != nil {
					logging.Op().Error("snapshot creation failed", "function", fn.Name, "error", err)
				} else {
					p.snapshotCache.Store(fn.ID, true)
				}
			}
			lock.Unlock()
		}
	}

	logging.Op().Info("VM ready", "vm_id", vm.ID, "function", fn.Name)
	return pvm, nil
}

func (p *Pool) getSnapshotLock(funcID string) *sync.Mutex {
	if lock, ok := p.snapshotLocks.Load(funcID); ok {
		return lock.(*sync.Mutex)
	}
	lock := &sync.Mutex{}
	actual, _ := p.snapshotLocks.LoadOrStore(funcID, lock)
	return actual.(*sync.Mutex)
}

func (p *Pool) Release(pvm *PooledVM) {
	fp := p.getOrCreatePool(pvm.Function.ID)
	fp.mu.Lock()
	if pvm.inflight > 0 {
		pvm.inflight--
	}
	pvm.LastUsed = time.Now()
	hasWaiters := fp.waiters > 0
	fp.mu.Unlock()

	// Signal one waiting goroutine if any
	if hasWaiters {
		fp.cond.Signal()
	}
}

func (p *Pool) Evict(funcID string) {
	val, ok := p.pools.LoadAndDelete(funcID)
	if !ok {
		return
	}
	fp := val.(*functionPool)

	fp.mu.Lock()
	vms := fp.vms
	fp.vms = nil
	fp.mu.Unlock()

	// Stop VMs in parallel
	var wg sync.WaitGroup
	for _, pvm := range vms {
		wg.Add(1)
		go func(pvm *PooledVM) {
			defer wg.Done()
			pvm.Client.Close()
			p.backend.StopVM(pvm.VM.ID)
		}(pvm)
	}
	wg.Wait()
}

func (p *Pool) EvictVM(funcID string, target *PooledVM) {
	if target == nil {
		return
	}

	fp := p.getOrCreatePool(funcID)

	fp.mu.Lock()
	newList := make([]*PooledVM, 0, len(fp.vms))
	for _, pvm := range fp.vms {
		if pvm != target {
			newList = append(newList, pvm)
		}
	}
	fp.vms = newList
	fp.mu.Unlock()

	target.Client.Close()
	p.backend.StopVM(target.VM.ID)
}

func (p *Pool) Stats() map[string]interface{} {
	vmStats := make([]map[string]interface{}, 0)
	totalVMs := 0

	p.pools.Range(func(key, value interface{}) bool {
		funcID := key.(string)
		fp := value.(*functionPool)

		fp.mu.Lock()
		totalVMs += len(fp.vms)
		for _, pvm := range fp.vms {
			vmStats = append(vmStats, map[string]interface{}{
				"function_id":    funcID,
				"vm_id":          pvm.VM.ID,
				"runtime":        pvm.VM.Runtime,
				"inflight":       pvm.inflight,
				"max_concurrent": pvm.maxConcurrent,
				"idle_sec":       time.Since(pvm.LastUsed).Seconds(),
			})
		}
		fp.mu.Unlock()
		return true
	})

	return map[string]interface{}{
		"active_vms": totalVMs,
		"idle_ttl":   p.idleTTL.String(),
		"vms":        vmStats,
	}
}

// FunctionStats returns pool stats for a specific function
func (p *Pool) FunctionStats(funcID string) map[string]interface{} {
	result := map[string]interface{}{
		"active_vms": 0,
		"busy_vms":   0,
		"idle_vms":   0,
	}

	if value, ok := p.pools.Load(funcID); ok {
		fp := value.(*functionPool)
		fp.mu.Lock()
		busyCount := 0
		idleCount := 0
		for _, pvm := range fp.vms {
			if pvm.inflight > 0 {
				busyCount++
			} else {
				idleCount++
			}
		}
		result["active_vms"] = len(fp.vms)
		result["busy_vms"] = busyCount
		result["idle_vms"] = idleCount
		fp.mu.Unlock()
	}

	return result
}

func (p *Pool) Shutdown() {
	p.cancel()

	type vmToStop struct {
		client backend.Client
		vmID   string
	}
	var toStop []vmToStop

	p.pools.Range(func(key, value interface{}) bool {
		fp := value.(*functionPool)
		fp.mu.Lock()
		for _, pvm := range fp.vms {
			toStop = append(toStop, vmToStop{client: pvm.Client, vmID: pvm.VM.ID})
		}
		fp.vms = nil
		fp.mu.Unlock()
		return true
	})

	// Stop all VMs in parallel with a 10s timeout
	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		for _, e := range toStop {
			wg.Add(1)
			go func(client backend.Client, vmID string) {
				defer wg.Done()
				client.Close()
				p.backend.StopVM(vmID)
			}(e.client, e.vmID)
		}
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		logging.Op().Warn("pool shutdown timed out after 10s")
	}
}

// healthCheckLoop periodically pings idle VMs and evicts unresponsive ones
func (p *Pool) healthCheckLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.healthCheck()
		}
	}
}

func (p *Pool) healthCheck() {
	type checkTarget struct {
		funcID string
		pvm    *PooledVM
	}
	var targets []checkTarget

	// Collect idle VMs under lock
	p.pools.Range(func(key, value interface{}) bool {
		funcID := key.(string)
		fp := value.(*functionPool)

		fp.mu.Lock()
		for _, pvm := range fp.vms {
			if pvm.inflight == 0 {
				targets = append(targets, checkTarget{funcID: funcID, pvm: pvm})
			}
		}
		fp.mu.Unlock()
		return true
	})

	// Ping outside lock
	for _, t := range targets {
		if err := t.pvm.Client.Ping(); err != nil {
			logging.Op().Warn("health check failed, evicting VM",
				"vm_id", t.pvm.VM.ID,
				"function", t.funcID,
				"error", err)
			p.EvictVM(t.funcID, t.pvm)
		}
	}
}
