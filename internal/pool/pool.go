package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/pkg/singleflight"
)

var (
	// ErrConcurrencyLimit is returned when max replica count is reached and no warm slot is available.
	ErrConcurrencyLimit = errors.New("concurrency limit reached")
	// ErrInflightLimit is returned when function-level max in-flight policy is exceeded.
	ErrInflightLimit = errors.New("inflight limit reached")
	// ErrQueueFull is returned when function-level queue depth policy is exceeded.
	ErrQueueFull = errors.New("queue depth limit reached")
	// ErrQueueWaitTimeout is returned when waiting for a VM exceeds queue wait policy.
	ErrQueueWaitTimeout = errors.New("queue wait timeout")
	// ErrGlobalVMLimit is returned when the system-wide maximum VM count is reached.
	ErrGlobalVMLimit = errors.New("global VM limit reached")
)

const (
	DefaultIdleTTL = 60 * time.Second

	// Default values for pool settings
	DefaultCleanupInterval     = 10 * time.Second
	DefaultHealthCheckInterval = 30 * time.Second
	DefaultMaxPreWarmWorkers   = 8
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
	vms             []*PooledVM
	mu              sync.RWMutex // read-write lock reduces contention for read-heavy paths
	maxReplicas     atomic.Int32 // max concurrent VMs (0 = unlimited)
	totalInflight   int
	readyVMs        []*PooledVM
	readySet        map[*PooledVM]struct{}
	waiters         int          // number of goroutines waiting for a VM
	cond            *sync.Cond   // condition variable for waiting
	codeHash        atomic.Value // string: hash of code when VMs were created
	desiredReplicas atomic.Int32 // desired replica count set by autoscaler
	lastQueueWaitMs atomic.Int64
}

// SnapshotCallback is called after a cold start to create a snapshot
type SnapshotCallback func(ctx context.Context, vmID, funcID string) error

type Pool struct {
	backend             backend.Backend
	pools               sync.Map // map[string]*functionPool - per-function pools
	group               singleflight.Group
	idleTTL             time.Duration
	cleanupInterval     time.Duration
	healthCheckInterval time.Duration
	maxPreWarmWorkers   int
	maxGlobalVMs        atomic.Int32 // system-wide max VM count (0 = unlimited)
	totalVMs            atomic.Int32 // current total VM count across all pools
	ctx                 context.Context
	cancel              context.CancelFunc
	snapshotCallback    SnapshotCallback
	snapshotCache       sync.Map             // funcID -> bool (true if snapshot exists)
	snapshotLocks       sync.Map             // funcID -> *sync.Mutex
	templatePool        *RuntimeTemplatePool // optional pre-warmed runtime template pool
}

// PoolConfig holds pool configuration options
type PoolConfig struct {
	IdleTTL             time.Duration
	CleanupInterval     time.Duration
	HealthCheckInterval time.Duration
	MaxPreWarmWorkers   int
}

func NewPool(b backend.Backend, cfg PoolConfig) *Pool {
	if cfg.IdleTTL == 0 {
		cfg.IdleTTL = DefaultIdleTTL
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = DefaultCleanupInterval
	}
	if cfg.HealthCheckInterval == 0 {
		cfg.HealthCheckInterval = DefaultHealthCheckInterval
	}
	if cfg.MaxPreWarmWorkers == 0 {
		cfg.MaxPreWarmWorkers = DefaultMaxPreWarmWorkers
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		backend:             b,
		idleTTL:             cfg.IdleTTL,
		cleanupInterval:     cfg.CleanupInterval,
		healthCheckInterval: cfg.HealthCheckInterval,
		maxPreWarmWorkers:   cfg.MaxPreWarmWorkers,
		ctx:                 ctx,
		cancel:              cancel,
	}

	go p.cleanupLoop()
	go p.healthCheckLoop()
	return p
}

// SetSnapshotCallback sets the callback for creating snapshots after cold starts
func (p *Pool) SetSnapshotCallback(cb SnapshotCallback) {
	p.snapshotCallback = cb
}

// SetTemplatePool attaches a RuntimeTemplatePool to the pool.
// When set, Acquire will attempt to claim a pre-warmed template VM
// before falling back to creating a new VM from scratch.
func (p *Pool) SetTemplatePool(tp *RuntimeTemplatePool) {
	p.templatePool = tp
}

// TemplatePool returns the attached RuntimeTemplatePool, or nil.
func (p *Pool) TemplatePool() *RuntimeTemplatePool {
	return p.templatePool
}

// SetMaxGlobalVMs sets the system-wide maximum number of VMs (0 = unlimited).
func (p *Pool) SetMaxGlobalVMs(n int) {
	p.maxGlobalVMs.Store(int32(n))
}

// TotalVMCount returns the total number of active VMs across all function pools.
func (p *Pool) TotalVMCount() int {
	return int(p.totalVMs.Load())
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
	ticker := time.NewTicker(p.cleanupInterval)
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
			minReplicas = max(fp.vms[0].Function.MinReplicas, int(fp.desiredReplicas.Load()))
		}

		activeCount := len(fp.vms)
		var kept []*PooledVM
		removed := 0

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
				removed++
				continue
			}
			kept = append(kept, pvm)
		}
		fp.vms = kept
		rebuildReadyVMLocked(fp)
		if removed > 0 {
			p.totalVMs.Add(int32(-removed))
		}
		fp.mu.Unlock()
		return true
	})

	// Update Prometheus active VMs metric
	metrics.SetActiveVMs(p.TotalVMCount())

	// Stop expired VMs asynchronously — they are already removed from pools
	for _, e := range toStop {
		go func(client backend.Client, vmID string) {
			defer func() {
				if r := recover(); r != nil {
					logging.Op().Error("recovered panic in async VM cleanup", "panic", r)
				}
			}()
			client.Close()
			p.backend.StopVM(vmID)
		}(e.client, e.vmID)
	}
}

func (p *Pool) EnsureReady(ctx context.Context, fn *domain.Function, codeContent []byte) error {
	fp := p.getOrCreatePool(fn.ID)

	fp.mu.RLock()
	currentCount := len(fp.vms)
	fp.mu.RUnlock()

	needed := max(fn.MinReplicas, int(fp.desiredReplicas.Load())) - currentCount
	if needed <= 0 {
		return nil
	}

	logging.Op().Info("pre-warming VMs", "count", needed, "function", fn.Name)

	sem := make(chan struct{}, p.maxPreWarmWorkers)
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
			addReadyVMLocked(fp, pvm)
			if fp.waiters > 0 {
				fp.cond.Signal()
			}
			fp.mu.Unlock()
			p.totalVMs.Add(1)
		}()
	}
	wg.Wait()

	// Update metric after all VMs are created
	metrics.SetActiveVMs(p.TotalVMCount())
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

func (p *Pool) preparePoolForFunction(fn *domain.Function) *functionPool {
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
				evictedCount := int32(len(vmsToStop))
				fp.vms = nil
				fp.totalInflight = 0
				fp.readyVMs = nil
				fp.readySet = nil
				fp.codeHash.Store(fn.CodeHash)
				fp.mu.Unlock()
				if evictedCount > 0 {
					p.totalVMs.Add(-evictedCount)
					metrics.SetActiveVMs(p.TotalVMCount())
				}

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

	// Update max replicas atomically (0 means unlimited).
	fp.maxReplicas.Store(int32(fn.MaxReplicas))

	return fp
}

func getCapacityLimits(fn *domain.Function) (maxInflight, maxQueueDepth int, maxQueueWait time.Duration) {
	if fn.CapacityPolicy == nil || !fn.CapacityPolicy.Enabled {
		return 0, 0, 0
	}
	maxInflight = fn.CapacityPolicy.MaxInflight
	maxQueueDepth = fn.CapacityPolicy.MaxQueueDepth
	if fn.CapacityPolicy.MaxQueueWaitMs > 0 {
		maxQueueWait = time.Duration(fn.CapacityPolicy.MaxQueueWaitMs) * time.Millisecond
	}
	return
}

func ensurePoolStateLocked(fp *functionPool) {
	if fp.readySet == nil {
		fp.readySet = make(map[*PooledVM]struct{})
	}
}

func addReadyVMLocked(fp *functionPool, pvm *PooledVM) {
	if pvm == nil || pvm.inflight >= pvm.maxConcurrent {
		return
	}
	ensurePoolStateLocked(fp)
	if _, ok := fp.readySet[pvm]; ok {
		return
	}
	fp.readySet[pvm] = struct{}{}
	fp.readyVMs = append(fp.readyVMs, pvm)
}

func removeReadyVMLocked(fp *functionPool, pvm *PooledVM) {
	if fp.readySet == nil || pvm == nil {
		return
	}
	delete(fp.readySet, pvm)
}

func rebuildReadyVMLocked(fp *functionPool) {
	ensurePoolStateLocked(fp)
	clear(fp.readySet)
	fp.readyVMs = fp.readyVMs[:0]
	fp.totalInflight = 0
	for _, pvm := range fp.vms {
		fp.totalInflight += pvm.inflight
		addReadyVMLocked(fp, pvm)
	}
}

func takeWarmVMLocked(fp *functionPool) *PooledVM {
	ensurePoolStateLocked(fp)
	for len(fp.readyVMs) > 0 {
		last := len(fp.readyVMs) - 1
		pvm := fp.readyVMs[last]
		fp.readyVMs = fp.readyVMs[:last]
		if _, ok := fp.readySet[pvm]; !ok {
			continue
		}
		delete(fp.readySet, pvm)
		if pvm.inflight >= pvm.maxConcurrent {
			continue
		}
		pvm.inflight++
		fp.totalInflight++
		pvm.LastUsed = time.Now()
		pvm.ColdStart = false
		addReadyVMLocked(fp, pvm)
		return pvm
	}
	return nil
}

func inflightCountLocked(fp *functionPool) int {
	return fp.totalInflight
}

func waitForVMLocked(ctx context.Context, fp *functionPool, waitFor time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fp.waiters++
	defer func() {
		fp.waiters--
	}()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			fp.mu.Lock()
			fp.cond.Broadcast()
			fp.mu.Unlock()
		case <-done:
		}
	}()

	var timer *time.Timer
	if waitFor > 0 {
		timer = time.AfterFunc(waitFor, func() {
			fp.mu.Lock()
			fp.cond.Broadcast()
			fp.mu.Unlock()
		})
	}

	fp.cond.Wait()
	close(done)
	if timer != nil {
		timer.Stop()
	}
	return ctx.Err()
}

func (p *Pool) acquireGeneric(
	ctx context.Context,
	fn *domain.Function,
	createVM func(context.Context, *domain.Function) (*PooledVM, error),
) (*PooledVM, error) {
	fp := p.preparePoolForFunction(fn)
	maxInflight, maxQueueDepth, maxQueueWait := getCapacityLimits(fn)

	var waitStart time.Time
	recordQueueWait := func() {
		if waitStart.IsZero() {
			fp.lastQueueWaitMs.Store(0)
			return
		}
		waitMs := time.Since(waitStart).Milliseconds()
		if waitMs < 0 {
			waitMs = 0
		}
		fp.lastQueueWaitMs.Store(waitMs)
	}
	for {
		fp.mu.Lock()
		if pvm := takeWarmVMLocked(fp); pvm != nil {
			fp.mu.Unlock()
			recordQueueWait()
			logging.Op().Debug("reusing warm VM", "vm_id", pvm.VM.ID, "function", fn.Name)
			return pvm, nil
		}

		maxReps := fp.maxReplicas.Load()
		canCreate := maxReps == 0 || len(fp.vms) < int(maxReps)
		currentInflight := inflightCountLocked(fp)

		if maxInflight > 0 && currentInflight >= maxInflight {
			fp.mu.Unlock()
			recordQueueWait()
			return nil, ErrInflightLimit
		}
		if canCreate {
			// Check system-wide VM limit before allowing creation
			globalMax := p.maxGlobalVMs.Load()
			if globalMax > 0 && p.TotalVMCount() >= int(globalMax) {
				fp.mu.Unlock()
				recordQueueWait()
				return nil, ErrGlobalVMLimit
			}
			fp.mu.Unlock()
			break
		}

		if maxQueueDepth > 0 && fp.waiters >= maxQueueDepth {
			fp.mu.Unlock()
			recordQueueWait()
			return nil, ErrQueueFull
		}
		if waitStart.IsZero() {
			waitStart = time.Now()
		}
		if maxQueueWait > 0 && time.Since(waitStart) >= maxQueueWait {
			fp.mu.Unlock()
			recordQueueWait()
			return nil, ErrQueueWaitTimeout
		}

		waitFor := time.Duration(0)
		if maxQueueWait > 0 {
			remaining := maxQueueWait - time.Since(waitStart)
			if remaining <= 0 {
				fp.mu.Unlock()
				recordQueueWait()
				return nil, ErrQueueWaitTimeout
			}
			waitFor = remaining
		}

		if err := waitForVMLocked(ctx, fp, waitFor); err != nil {
			fp.mu.Unlock()
			recordQueueWait()
			return nil, err
		}
		fp.mu.Unlock()
	}

	val, err, shared := p.group.Do(fn.ID, func() (interface{}, error) {
		return createVM(ctx, fn)
	})
	if err != nil {
		return nil, err
	}

	pvm := val.(*PooledVM)
	if shared {
		fp.mu.Lock()
		if existing := takeWarmVMLocked(fp); existing != nil {
			fp.mu.Unlock()
			recordQueueWait()
			return existing, nil
		}
		maxReps := fp.maxReplicas.Load()
		canCreate := maxReps == 0 || len(fp.vms) < int(maxReps)
		currentInflight := inflightCountLocked(fp)
		fp.mu.Unlock()

		if maxInflight > 0 && currentInflight >= maxInflight {
			recordQueueWait()
			return nil, ErrInflightLimit
		}
		if !canCreate {
			recordQueueWait()
			return nil, ErrConcurrencyLimit
		}

		pvm, err = createVM(ctx, fn)
		if err != nil {
			return nil, err
		}
	}

	fp.mu.Lock()
	fp.vms = append(fp.vms, pvm)
	fp.totalInflight += pvm.inflight
	addReadyVMLocked(fp, pvm)
	if fp.waiters > 0 {
		fp.cond.Signal()
	}
	fp.mu.Unlock()
	p.totalVMs.Add(1)
	metrics.SetActiveVMs(p.TotalVMCount())
	recordQueueWait()
	return pvm, nil
}

func (p *Pool) Acquire(ctx context.Context, fn *domain.Function, codeContent []byte) (*PooledVM, error) {
	return p.acquireGeneric(ctx, fn, func(ctx context.Context, fn *domain.Function) (*PooledVM, error) {
		return p.createVM(ctx, fn, codeContent)
	})
}

func (p *Pool) createVM(ctx context.Context, fn *domain.Function, codeContent []byte) (*PooledVM, error) {
	// Try to acquire a pre-warmed template VM from the runtime template pool.
	// This skips VM boot and kernel initialization, reducing cold-start latency.
	if p.templatePool != nil {
		if pvm, err := p.createVMFromTemplate(ctx, fn, codeContent); err == nil && pvm != nil {
			return pvm, nil
		}
		// Template acquisition failed or unavailable — fall back to full cold start.
	}

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

	// Create snapshot asynchronously — not needed for the current invocation
	if p.snapshotCallback != nil {
		if _, hasSnapshot := p.snapshotCache.Load(fn.ID); !hasSnapshot {
			snapshotCb := p.snapshotCallback
			funcName := fn.Name
			funcID := fn.ID
			vmID := vm.ID
			go func() {
				defer func() {
					if r := recover(); r != nil {
						logging.Op().Error("recovered panic in async snapshot creation", "panic", r)
					}
				}()
				lock := p.getSnapshotLock(funcID)
				lock.Lock()
				defer lock.Unlock()
				if _, hasSnapshot := p.snapshotCache.Load(funcID); !hasSnapshot {
					logging.Op().Info("creating snapshot after cold start", "function", funcName, "vm_id", vmID)
					snapshotCtx, snapshotCancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer snapshotCancel()
					if err := snapshotCb(snapshotCtx, vmID, funcID); err != nil {
						logging.Op().Error("snapshot creation failed", "function", funcName, "error", err)
					} else {
						p.snapshotCache.Store(funcID, true)
					}
				}
			}()
		}
	}

	logging.Op().Info("VM ready", "vm_id", vm.ID, "function", fn.Name)
	return pvm, nil
}

// createVMFromTemplate attempts to acquire a pre-warmed template VM for the
// function's runtime, injects the function code via Reload, and re-initializes
// the agent with the correct function identity. Returns nil, nil when no
// template VM is available (caller should fall back to full cold start).
func (p *Pool) createVMFromTemplate(ctx context.Context, fn *domain.Function, codeContent []byte) (*PooledVM, error) {
	tvm, err := p.templatePool.Acquire(fn.Runtime)
	if err != nil {
		logging.Op().Warn("template pool acquire error", "runtime", string(fn.Runtime), "error", err)
		return nil, err
	}
	if tvm == nil {
		return nil, nil // no template available
	}

	bootStart := time.Now()

	// Build the code files map for Reload. The handler file path follows
	// the convention used by the agent: the primary entry point is "handler".
	files := map[string][]byte{
		"handler": codeContent,
	}

	// Inject the function code into the template VM via hot-reload.
	// This also clears /tmp inside the VM to prevent cross-function pollution.
	if err := tvm.Client.Reload(files); err != nil {
		logging.Op().Warn("template VM reload failed, returning to pool",
			"runtime", string(fn.Runtime),
			"vm_id", tvm.VM.ID,
			"error", err)
		p.templatePool.Return(fn.Runtime, tvm)
		return nil, err
	}

	// Re-initialize the agent with the real function identity so it picks
	// up the correct runtime config, env vars, handler path, etc.
	if err := tvm.Client.Init(fn); err != nil {
		logging.Op().Warn("template VM re-init failed, stopping VM",
			"runtime", string(fn.Runtime),
			"vm_id", tvm.VM.ID,
			"error", err)
		tvm.Client.Close()
		p.backend.StopVM(tvm.VM.ID)
		return nil, err
	}

	bootDurationMs := time.Since(bootStart).Milliseconds()
	metrics.RecordVMBootDuration(fn.Name, string(fn.Runtime), bootDurationMs, false)

	pvm := &PooledVM{
		VM:            tvm.VM,
		Client:        tvm.Client,
		Function:      fn,
		LastUsed:      time.Now(),
		ColdStart:     true,
		inflight:      1,
		maxConcurrent: p.computeInstanceConcurrency(fn),
	}

	logging.Op().Info("VM ready from template",
		"vm_id", tvm.VM.ID,
		"function", fn.Name,
		"runtime", string(fn.Runtime),
		"boot_ms", bootDurationMs)
	return pvm, nil
}

// AcquireWithFiles acquires a VM for a multi-file function.
// files is a map of relative path -> content.
func (p *Pool) AcquireWithFiles(ctx context.Context, fn *domain.Function, files map[string][]byte) (*PooledVM, error) {
	return p.acquireGeneric(ctx, fn, func(ctx context.Context, fn *domain.Function) (*PooledVM, error) {
		return p.createVMWithFiles(ctx, fn, files)
	})
}

// createVMWithFiles creates a VM with multiple code files
func (p *Pool) createVMWithFiles(ctx context.Context, fn *domain.Function, files map[string][]byte) (*PooledVM, error) {
	logging.Op().Info("creating VM with files", "function", fn.Name, "runtime", fn.Runtime, "file_count", len(files))

	bootStart := time.Now()
	vm, err := p.backend.CreateVMWithFiles(ctx, fn, files)
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

	// Note: snapshot creation for multi-file VMs is deferred for now

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
		fp.totalInflight--
		if fp.totalInflight < 0 {
			fp.totalInflight = 0
		}
	}
	pvm.LastUsed = time.Now()
	addReadyVMLocked(fp, pvm)
	if fp.waiters > 0 {
		fp.cond.Signal()
	}
	fp.mu.Unlock()
}

func (p *Pool) Evict(funcID string) {
	val, ok := p.pools.LoadAndDelete(funcID)
	if !ok {
		return
	}
	fp := val.(*functionPool)

	fp.mu.Lock()
	vms := fp.vms
	evictedCount := int32(len(vms))
	fp.vms = nil
	fp.totalInflight = 0
	fp.readyVMs = nil
	fp.readySet = nil
	fp.mu.Unlock()
	if evictedCount > 0 {
		p.totalVMs.Add(-evictedCount)
		metrics.SetActiveVMs(p.TotalVMCount())
	}

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
	prevLen := len(fp.vms)
	newList := make([]*PooledVM, 0, len(fp.vms))
	removedInflight := 0
	for _, pvm := range fp.vms {
		if pvm != target {
			newList = append(newList, pvm)
		} else {
			removedInflight += pvm.inflight
			removeReadyVMLocked(fp, pvm)
		}
	}
	fp.vms = newList
	fp.totalInflight -= removedInflight
	if fp.totalInflight < 0 {
		fp.totalInflight = 0
	}
	removed := int32(prevLen - len(newList))
	fp.mu.Unlock()
	if removed > 0 {
		p.totalVMs.Add(-removed)
		metrics.SetActiveVMs(p.TotalVMCount())
	}

	// Stop VM asynchronously — it is already removed from the pool
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.Op().Error("recovered panic in async VM eviction", "panic", r)
			}
		}()
		target.Client.Close()
		p.backend.StopVM(target.VM.ID)
	}()
}

// ReloadCode sends new code files to all active VMs for a function.
// Returns nil if no VMs are active. Falls back to eviction on failure.
func (p *Pool) ReloadCode(funcID string, files map[string][]byte) error {
	val, ok := p.pools.Load(funcID)
	if !ok {
		return nil // No active pool, nothing to reload
	}
	fp := val.(*functionPool)

	fp.mu.RLock()
	vms := make([]*PooledVM, len(fp.vms))
	copy(vms, fp.vms)
	fp.mu.RUnlock()

	if len(vms) == 0 {
		return nil
	}

	logging.Op().Info("hot reloading code", "function", funcID, "vm_count", len(vms))

	var failedVMs []*PooledVM
	for _, pvm := range vms {
		if err := pvm.Client.Reload(files); err != nil {
			logging.Op().Warn("reload failed on VM, will evict",
				"vm_id", pvm.VM.ID,
				"function", funcID,
				"error", err)
			failedVMs = append(failedVMs, pvm)
		}
	}

	// Evict VMs that failed to reload
	for _, pvm := range failedVMs {
		p.EvictVM(funcID, pvm)
	}

	if len(failedVMs) > 0 {
		return fmt.Errorf("reload failed on %d/%d VMs", len(failedVMs), len(vms))
	}

	return nil
}

func (p *Pool) Stats() map[string]interface{} {
	vmStats := make([]map[string]interface{}, 0)
	totalVMs := 0

	p.pools.Range(func(key, value interface{}) bool {
		funcID := key.(string)
		fp := value.(*functionPool)

		fp.mu.RLock()
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
		fp.mu.RUnlock()
		return true
	})

	stats := map[string]interface{}{
		"active_vms": totalVMs,
		"idle_ttl":   p.idleTTL.String(),
		"vms":        vmStats,
	}
	if p.templatePool != nil {
		stats["template_pool"] = p.templatePool.Stats()
	}
	return stats
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
		fp.mu.RLock()
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
		fp.mu.RUnlock()
	}

	return result
}

func (p *Pool) Shutdown() {
	p.cancel()

	// Shutdown the template pool first
	if p.templatePool != nil {
		p.templatePool.Shutdown()
	}

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
		fp.totalInflight = 0
		fp.readyVMs = nil
		fp.readySet = nil
		fp.mu.Unlock()
		return true
	})
	p.totalVMs.Store(0)

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

// QueueDepth returns the number of goroutines waiting for a VM for the given function
func (p *Pool) QueueDepth(funcID string) int {
	if value, ok := p.pools.Load(funcID); ok {
		fp := value.(*functionPool)
		fp.mu.RLock()
		depth := fp.waiters
		fp.mu.RUnlock()
		return depth
	}
	return 0
}

// FunctionQueueWaitMs returns the most recent queue wait duration observed for a function.
func (p *Pool) FunctionQueueWaitMs(funcID string) int64 {
	if value, ok := p.pools.Load(funcID); ok {
		fp := value.(*functionPool)
		return fp.lastQueueWaitMs.Load()
	}
	return 0
}

// SetDesiredReplicas sets the autoscaler-driven desired replica count for a function
func (p *Pool) SetDesiredReplicas(funcID string, desired int) {
	fp := p.getOrCreatePool(funcID)
	fp.desiredReplicas.Store(int32(desired))
}

// FunctionPoolStats returns total, busy, and idle VM counts for a function
func (p *Pool) FunctionPoolStats(funcID string) (total, busy, idle int) {
	if value, ok := p.pools.Load(funcID); ok {
		fp := value.(*functionPool)
		fp.mu.RLock()
		total = len(fp.vms)
		for _, pvm := range fp.vms {
			if pvm.inflight > 0 {
				busy++
			} else {
				idle++
			}
		}
		fp.mu.RUnlock()
	}
	return
}

// healthCheckLoop periodically pings idle VMs and evicts unresponsive ones
func (p *Pool) healthCheckLoop() {
	ticker := time.NewTicker(p.healthCheckInterval)
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

	// Collect idle VMs under read lock
	p.pools.Range(func(key, value interface{}) bool {
		funcID := key.(string)
		fp := value.(*functionPool)

		fp.mu.RLock()
		for _, pvm := range fp.vms {
			if pvm.inflight == 0 {
				targets = append(targets, checkTarget{funcID: funcID, pvm: pvm})
			}
		}
		fp.mu.RUnlock()
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
