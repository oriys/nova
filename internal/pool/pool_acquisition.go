// pool_acquisition.go contains the VM acquisition path: the hot path that
// every invocation traverses to obtain a warm VM or trigger a cold start.
package pool

import (
	"context"
	"sync"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
)

func getCapacityLimits(fn *domain.Function) (maxInflight, maxQueueDepth int, maxQueueWait time.Duration) {
	// Zero values mean "no limit" throughout the acquisition loop, so
	// returning zeros when the policy is absent is the correct default.
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

// takeWarmVMLocked returns a VM that has capacity for one more in-flight
// request, or nil if none is available.
//
// Must be called with fp.mu held (write lock). Increments pvm.inflight and
// fp.totalInflight before returning so that the caller cannot accidentally
// double-count the slot.
//
// The readyVMs slice is used as a stack (LIFO) so that the most recently
// used VM is preferred, maximising the chance that its process cache is
// warm. VMs that are in readyVMs but no longer in readySet (stale pointers
// from removeReadyVMLocked) are silently skipped.
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

// waitForVMLocked suspends the calling goroutine until either a VM becomes
// available (signalled via fp.cond), the context is cancelled, or the
// optional waitFor deadline elapses.
//
// Must be called with fp.mu held (write lock). Releases the lock
// via cond.Wait and re-acquires it before returning.
//
// The goroutine spawned here exists solely to translate channel-based
// cancellation signals (ctx.Done) into a Broadcast on the condition variable.
// This is necessary because sync.Cond has no native context-awareness.
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

// acquireGeneric is the shared acquisition loop used by Acquire and
// AcquireWithFiles. It implements the following admission control policy:
//
//  1. If a warm VM has capacity, return it immediately (fast path).
//  2. If a new VM can be created (below MaxReplicas and MaxGlobalVMs),
//     break out of the loop and call createVM.
//  3. Otherwise, apply capacity policy checks:
//     - Reject immediately if MaxInflight is exceeded.
//     - Reject immediately if MaxQueueDepth is exceeded.
//     - Wait on the condition variable until a VM is released or the
//       MaxQueueWait deadline expires.
//
// The singleflight group deduplicates concurrent cold-start attempts for
// the same function so that N waiting goroutines result in exactly one VM
// creation. When the shared result arrives, the group re-checks whether
// the caller can use the new VM or must create another one.
//
// # Concurrency
//
// The inner loop acquires and releases fp.mu on every iteration to allow
// other goroutines to release VMs concurrently. The singleflight call
// happens outside the lock to avoid blocking the whole pool.
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

	poolKey := p.poolKeyForFunction(fn)
	val, err, shared := p.group.Do(poolKey, func() (interface{}, error) {
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
