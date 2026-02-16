package pool

import (
	"fmt"
	"sync"
	"time"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
)

func (p *Pool) Release(pvm *PooledVM) {
	fp := p.getOrCreatePool(p.poolKeyForFunction(pvm.Function))
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
	poolKey, fp, ok := p.getPoolForFunctionID(funcID)
	if !ok {
		return
	}
	p.functionPoolKeys.Delete(funcID)

	fp.mu.Lock()
	if fp.functionRefs != nil {
		delete(fp.functionRefs, funcID)
		if len(fp.functionRefs) > 0 {
			fp.mu.Unlock()
			return
		}
	}
	vms := fp.vms
	evictedCount := int32(len(vms))
	fp.vms = nil
	fp.totalInflight = 0
	fp.readyVMs = nil
	fp.readySet = nil
	fp.mu.Unlock()
	p.pools.Delete(poolKey)
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

	fp := p.getOrCreatePool(p.poolKeyForFunction(target.Function))

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
	_, fp, ok := p.getPoolForFunctionID(funcID)
	if !ok {
		return nil // No active pool, nothing to reload
	}

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
		fp := value.(*functionPool)

		fp.mu.RLock()
		totalVMs += len(fp.vms)
		for _, pvm := range fp.vms {
			vmStats = append(vmStats, map[string]interface{}{
				"function_id":    pvm.Function.ID,
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

	if _, fp, ok := p.getPoolForFunctionID(funcID); ok {
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
