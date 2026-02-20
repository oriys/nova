package pool

import (
	"context"
	"sync"
	"time"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
)

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

// cleanupExpired scans all active function pools and evicts VMs that have
// been idle for longer than IdleTTL, subject to the MinReplicas floor.
//
// # Tiered eviction strategy
//
// When SuspendTTL is configured (> 0), idle VMs pass through an intermediate
// "suspended" state before destruction:
//
//  1. Active → Idle: VM finishes all in-flight requests (via Release).
//  2. Idle → Suspended: idle longer than SuspendTTL. A snapshot is created
//     asynchronously so the next cold start can restore from it.
//  3. Idle/Suspended → Destroyed: idle longer than IdleTTL. VM is stopped
//     and all resources are released.
//
// When SuspendTTL is 0 (default), VMs skip the suspend step and go directly
// from Idle to Destroyed after IdleTTL.
//
// # Side effects
//
// VM stop operations are dispatched asynchronously (goroutine per VM)
// after the pool lock is released to avoid holding the lock during I/O.
// Prometheus active-VM gauge is updated before the async stops begin.
func (p *Pool) cleanupExpired() {
	type expiredVM struct {
		client backend.Client
		vmID   string
	}
	type suspendTarget struct {
		pvm    *PooledVM
		funcID string
	}
	var toStop []expiredVM
	var toSuspend []suspendTarget

	now := time.Now()
	p.pools.Range(func(key, value interface{}) bool {
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
				pvm.State = VMStateActive
				kept = append(kept, pvm)
				continue
			}

			idleDur := now.Sub(pvm.LastUsed)

			// Destroy: idle longer than IdleTTL and above min replicas
			if activeCount > minReplicas && idleDur > p.idleTTL {
				logging.Op().Info("VM expired",
					"vm_id", pvm.VM.ID,
					"function", pvm.Function.Name,
					"state", string(pvm.State),
					"idle", idleDur.Round(time.Second).String())
				pvm.State = VMStateDestroyed
				toStop = append(toStop, expiredVM{client: pvm.Client, vmID: pvm.VM.ID})
				activeCount--
				removed++
				continue
			}

			// Suspend: idle longer than SuspendTTL but less than IdleTTL
			if p.suspendTTL > 0 && p.snapshotCallback != nil &&
				pvm.State == VMStateIdle && idleDur > p.suspendTTL {
				pvm.State = VMStateSuspended
				toSuspend = append(toSuspend, suspendTarget{pvm: pvm, funcID: pvm.Function.ID})
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

	// Trigger snapshot creation for suspended VMs asynchronously.
	// The snapshot callback creates a snapshot that can be restored on
	// the next cold start, giving near-instant boot times.
	for _, s := range toSuspend {
		if p.snapshotCallback != nil {
			if _, hasSnapshot := p.snapshotCache.Load(s.funcID); hasSnapshot {
				continue // snapshot already exists
			}
			cb := p.snapshotCallback
			funcID := s.funcID
			vmID := s.pvm.VM.ID
			funcName := s.pvm.Function.Name
			go func() {
				defer func() {
					if r := recover(); r != nil {
						logging.Op().Error("recovered panic in suspend snapshot", "panic", r)
					}
				}()
				lock := p.getSnapshotLock(funcID)
				lock.Lock()
				defer lock.Unlock()
				if _, has := p.snapshotCache.Load(funcID); has {
					return
				}
				logging.Op().Info("suspending idle VM via snapshot",
					"function", funcName, "vm_id", vmID)
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				if err := cb(ctx, vmID, funcID); err != nil {
					logging.Op().Error("suspend snapshot failed",
						"function", funcName, "error", err)
				} else {
					p.snapshotCache.Store(funcID, true)
				}
			}()
		}
	}
}

// EnsureReady pre-warms VMs up to the function's MinReplicas or the
// autoscaler's desired replica count, whichever is higher.
//
// # When to call
//
// This is called by the pre-warm scheduler after function creation or
// update and by the autoscaler when it decides to scale up. It is NOT
// on the hot invocation path; the pool's Acquire method handles
// just-in-time cold starts.
//
// # Concurrency
//
// VM creation is parallelised up to maxPreWarmWorkers. Each goroutine
// appends to fp.vms under the write lock after creation succeeds.
// EnsureReady waits for all goroutines to finish before returning, so
// the caller can assert that MinReplicas VMs are available.
//
// # Edge cases
//
// If needed <= 0 (pool already at or above the target), the function
// returns immediately without creating any VMs.
func (p *Pool) EnsureReady(ctx context.Context, fn *domain.Function, codeContent []byte) error {
	fp := p.preparePoolForFunction(fn)

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
	poolKey := p.poolKeyForFunction(fn)
	fp := p.getOrCreatePool(poolKey)
	if oldPoolKey, ok := p.functionPoolKeys.Load(fn.ID); ok && oldPoolKey.(string) != poolKey {
		if oldVal, ok := p.pools.Load(oldPoolKey.(string)); ok {
			oldFP := oldVal.(*functionPool)
			oldFP.mu.Lock()
			if oldFP.functionRefs != nil {
				delete(oldFP.functionRefs, fn.ID)
			}
			oldFP.mu.Unlock()
		}
	}
	p.functionPoolKeys.Store(fn.ID, poolKey)
	fp.mu.Lock()
	if fp.functionRefs == nil {
		fp.functionRefs = make(map[string]struct{})
	}
	fp.functionRefs[fn.ID] = struct{}{}
	if desired, ok := p.desiredByFunction.Load(fn.ID); ok {
		fp.desiredReplicas.Store(desired.(int32))
	}
	fp.mu.Unlock()

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
		fp := value.(*functionPool)

		fp.mu.RLock()
		for _, pvm := range fp.vms {
			if pvm.inflight == 0 {
				targets = append(targets, checkTarget{funcID: pvm.Function.ID, pvm: pvm})
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
