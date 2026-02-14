package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
)

// RuntimeTemplatePool maintains a pool of pre-warmed "zygote" VMs for each
// runtime. These VMs have the agent loaded and initialized, but have no
// function-specific code. When a cold-start request arrives, a template VM
// is claimed from this pool and the function code is injected via the Reload
// protocol, reducing cold-start latency from "sub-second" to "tens of
// milliseconds" by skipping VM boot and kernel initialization.
type RuntimeTemplatePool struct {
	backend   backend.Backend
	templates sync.Map // runtime -> *templateEntry
	cfg       RuntimePoolConfig
	ctx       context.Context
	cancel    context.CancelFunc
}

// templateEntry holds pre-warmed VMs for a single runtime.
type templateEntry struct {
	mu  sync.Mutex
	vms []*TemplateVM
}

// TemplateVM is a pre-warmed VM that has the agent loaded but no function code.
type TemplateVM struct {
	VM     *backend.VM
	Client backend.Client
}

// RuntimePoolConfig configures the runtime template pool.
type RuntimePoolConfig struct {
	Enabled     bool          `json:"enabled"`      // Enable the runtime template pool
	PoolSize    int           `json:"pool_size"`    // Number of pre-warmed VMs per runtime (default: 2)
	RefillInterval time.Duration `json:"refill_interval"` // How often to check and refill pools (default: 30s)
	Runtimes    []string      `json:"runtimes"`     // Runtimes to pre-warm (e.g. ["python", "node"])
}

// DefaultRuntimePoolConfig returns default runtime pool configuration.
func DefaultRuntimePoolConfig() RuntimePoolConfig {
	return RuntimePoolConfig{
		Enabled:        false,
		PoolSize:       2,
		RefillInterval: 30 * time.Second,
	}
}

// NewRuntimeTemplatePool creates a new runtime template pool.
func NewRuntimeTemplatePool(b backend.Backend, cfg RuntimePoolConfig) *RuntimeTemplatePool {
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 2
	}
	if cfg.RefillInterval <= 0 {
		cfg.RefillInterval = 30 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	rtp := &RuntimeTemplatePool{
		backend: b,
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
	}
	if cfg.Enabled {
		go rtp.refillLoop()
	}
	return rtp
}

// Acquire attempts to claim a pre-warmed template VM for the given runtime.
// If a template VM is available it is removed from the pool and returned.
// The caller must inject function code via Client.Reload before using it.
// Returns nil, nil when no template VM is available (caller should fall back
// to the normal cold-start path).
func (rtp *RuntimeTemplatePool) Acquire(runtime domain.Runtime) (*TemplateVM, error) {
	val, ok := rtp.templates.Load(string(runtime))
	if !ok {
		return nil, nil
	}
	entry := val.(*templateEntry)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if len(entry.vms) == 0 {
		return nil, nil
	}

	// Take the last VM (LIFO for cache warmth)
	tvm := entry.vms[len(entry.vms)-1]
	entry.vms = entry.vms[:len(entry.vms)-1]

	logging.Op().Info("acquired template VM",
		"runtime", string(runtime),
		"remaining", len(entry.vms))
	return tvm, nil
}

// Return returns a template VM back to the pool (e.g. if code injection failed).
func (rtp *RuntimeTemplatePool) Return(runtime domain.Runtime, tvm *TemplateVM) {
	val, _ := rtp.templates.LoadOrStore(string(runtime), &templateEntry{})
	entry := val.(*templateEntry)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	entry.vms = append(entry.vms, tvm)
}

// PreWarm fills the template pool for the specified runtimes up to PoolSize.
func (rtp *RuntimeTemplatePool) PreWarm(runtimes []string) {
	for _, rt := range runtimes {
		rtp.fillRuntime(domain.Runtime(rt))
	}
}

func (rtp *RuntimeTemplatePool) fillRuntime(runtime domain.Runtime) {
	val, _ := rtp.templates.LoadOrStore(string(runtime), &templateEntry{})
	entry := val.(*templateEntry)

	entry.mu.Lock()
	current := len(entry.vms)
	needed := rtp.cfg.PoolSize - current
	entry.mu.Unlock()

	if needed <= 0 {
		return
	}

	logging.Op().Info("pre-warming runtime template VMs",
		"runtime", string(runtime),
		"current", current,
		"target", rtp.cfg.PoolSize)

	// Create a stub function for the template VM — runtime-only, no code
	stubFn := &domain.Function{
		ID:      fmt.Sprintf("_template_%s", runtime),
		Name:    fmt.Sprintf("_template_%s", runtime),
		Runtime: runtime,
	}

	for i := 0; i < needed; i++ {
		select {
		case <-rtp.ctx.Done():
			return
		default:
		}

		vm, err := rtp.backend.CreateVM(rtp.ctx, stubFn, nil)
		if err != nil {
			logging.Op().Warn("failed to create template VM",
				"runtime", string(runtime),
				"error", err)
			continue
		}

		client, err := rtp.backend.NewClient(vm)
		if err != nil {
			rtp.backend.StopVM(vm.ID)
			logging.Op().Warn("failed to create client for template VM",
				"runtime", string(runtime),
				"error", err)
			continue
		}

		if err := client.Init(stubFn); err != nil {
			client.Close()
			rtp.backend.StopVM(vm.ID)
			logging.Op().Warn("failed to init template VM",
				"runtime", string(runtime),
				"error", err)
			continue
		}

		tvm := &TemplateVM{VM: vm, Client: client}
		entry.mu.Lock()
		entry.vms = append(entry.vms, tvm)
		entry.mu.Unlock()

		logging.Op().Debug("template VM ready",
			"runtime", string(runtime),
			"vm_id", vm.ID)
	}

	metrics.SetActiveVMs(rtp.activeCount())
}

func (rtp *RuntimeTemplatePool) refillLoop() {
	ticker := time.NewTicker(rtp.cfg.RefillInterval)
	defer ticker.Stop()

	// Initial fill
	rtp.PreWarm(rtp.cfg.Runtimes)

	for {
		select {
		case <-rtp.ctx.Done():
			return
		case <-ticker.C:
			rtp.PreWarm(rtp.cfg.Runtimes)
		}
	}
}

func (rtp *RuntimeTemplatePool) activeCount() int {
	count := 0
	rtp.templates.Range(func(_, value interface{}) bool {
		entry := value.(*templateEntry)
		entry.mu.Lock()
		count += len(entry.vms)
		entry.mu.Unlock()
		return true
	})
	return count
}

// Stats returns statistics about the runtime template pool.
func (rtp *RuntimeTemplatePool) Stats() map[string]interface{} {
	stats := make(map[string]interface{})
	rtp.templates.Range(func(key, value interface{}) bool {
		rt := key.(string)
		entry := value.(*templateEntry)
		entry.mu.Lock()
		stats[rt] = len(entry.vms)
		entry.mu.Unlock()
		return true
	})
	return map[string]interface{}{
		"enabled":   rtp.cfg.Enabled,
		"pool_size": rtp.cfg.PoolSize,
		"runtimes":  stats,
	}
}

// Shutdown stops the refill loop and cleans up all template VMs.
func (rtp *RuntimeTemplatePool) Shutdown() {
	rtp.cancel()

	rtp.templates.Range(func(key, value interface{}) bool {
		entry := value.(*templateEntry)
		entry.mu.Lock()
		for _, tvm := range entry.vms {
			tvm.Client.Close()
			rtp.backend.StopVM(tvm.VM.ID)
		}
		entry.vms = nil
		entry.mu.Unlock()
		return true
	})
}
