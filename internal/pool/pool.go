// Package pool manages the lifecycle of warm VM instances that are shared
// across invocations of the same function.
//
// # Design rationale
//
// Cold-starting a Firecracker microVM takes 100–500 ms. To amortise this
// cost across many requests the pool keeps VMs alive between invocations.
// A VM is returned to the warm set after each successful execution and is
// only evicted when it becomes idle for longer than IdleTTL, fails a health
// check ping, or the code hash changes (indicating a function update).
//
// # Pool topology
//
// One functionPool is maintained per unique "pool key" (a hash of function
// configuration fields that affect the VM image). Functions that share
// identical configuration (same runtime, memory, backend, layers) reuse the
// same pool. The functionPoolKeys map tracks which key is active for each
// function ID so the old pool can be cleaned up on config change.
//
// # Concurrency model
//
// Each functionPool has its own sync.RWMutex. Reads (takeWarmVMLocked,
// Stats) take a read lock; writes (add/remove VM, code-change eviction)
// take the write lock. A sync.Cond on the write lock is used to wake
// goroutines that are waiting for a VM to become available.
//
// Atomic operations (maxReplicas, totalVMs, codeHash) are used for fields
// that are read frequently on the hot path to avoid lock contention.
//
// The Pool itself uses sync.Map for its top-level pools and
// functionPoolKeys maps because these are read-heavy and written rarely.
//
// # Invariants
//
//   - totalVMs always equals the sum of len(fp.vms) across all function pools.
//   - fp.totalInflight always equals the sum of pvm.inflight for pvm in fp.vms.
//   - A PooledVM is in fp.readySet if and only if pvm.inflight < pvm.maxConcurrent.
//   - Once closing is set (via Shutdown), no new VMs are created.
//
// # Failure behaviour
//
// If VM creation fails, the error is returned to the caller directly; no
// VM is added to the pool. The singleflight group ensures that concurrent
// cold-start requests for the same function share a single creation attempt
// rather than racing to create N identical VMs simultaneously.
package pool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
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

// PooledVM is a handle to a live VM that has been acquired from the pool.
// It must be returned via Pool.Release when the invocation is complete, or
// removed via Pool.EvictVM when the VM is known to be unhealthy.
//
// The inflight and maxConcurrent fields are owned by the pool and must only
// be read or written while holding the enclosing functionPool's mutex.
type PooledVM struct {
	VM            *backend.VM
	Client        backend.Client
	Function      *domain.Function
	LastUsed      time.Time
	ColdStart     bool
	inflight      int
	maxConcurrent int
}

// functionPool holds all VMs for a single pool key (a unique combination of
// function configuration fields). Multiple function IDs may share the same
// functionPool when their configuration is identical.
//
// # Locking discipline
//
// All fields except the atomic ones (maxReplicas, codeHash, desiredReplicas,
// lastQueueWaitMs) must be accessed under mu. readyVMs and readySet are
// derived views over vms and must be kept consistent with it; use
// rebuildReadyVMLocked after any bulk modification.
//
// cond is bound to the write side of mu. Callers must hold mu.Lock() when
// calling cond.Wait or cond.Signal/Broadcast.
type functionPool struct {
	vms             []*PooledVM
	mu              sync.RWMutex // read-write lock reduces contention for read-heavy paths
	maxReplicas     atomic.Int32 // max concurrent VMs (0 = unlimited)
	totalInflight   int
	readyVMs        []*PooledVM
	readySet        map[*PooledVM]struct{}
	waiters         int          // number of goroutines waiting for a VM
	cond            *sync.Cond   // condition variable for waiting; bound to mu (write lock)
	codeHash        atomic.Value // stores a string: SHA-256 of code when VMs were created; always Store/Load as string
	desiredReplicas atomic.Int32 // desired replica count set by autoscaler
	lastQueueWaitMs atomic.Int64
	functionRefs    map[string]struct{} // function IDs sharing this pool key
}

// SnapshotCallback is called after a cold start to create a VM snapshot that
// can be restored on subsequent cold starts to reduce boot latency.
// The callback is invoked asynchronously; failures are logged but not fatal.
type SnapshotCallback func(ctx context.Context, vmID, funcID string) error

// Pool is the central resource manager for VM instances.
//
// It is safe for concurrent use by multiple goroutines. The zero value is
// not usable; always construct via NewPool.
type Pool struct {
	backend             backend.Backend
	pools               sync.Map // map[string]*functionPool - per-configuration pools
	functionPoolKeys    sync.Map // map[string]string - function ID -> pool key
	desiredByFunction   sync.Map // map[string]int32 - desired replicas keyed by function ID
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

// NewPool creates a Pool and starts the background cleanup and health-check
// loops. The caller must call Shutdown to stop those loops and release VM
// resources when the pool is no longer needed.
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

// SnapshotDir exposes the backend snapshot directory for snapshot-aware
// scheduling and autoscaling decisions.
func (p *Pool) SnapshotDir() string {
	return p.backend.SnapshotDir()
}

// TotalVMCount returns the total number of active VMs across all function pools.
func (p *Pool) TotalVMCount() int {
	return int(p.totalVMs.Load())
}

// InvalidateSnapshotCache removes the cached snapshot status for a function
func (p *Pool) InvalidateSnapshotCache(funcID string) {
	p.snapshotCache.Delete(funcID)
}

type poolKeyPayload struct {
	Runtime             domain.Runtime        `json:"runtime"`
	MemoryMB            int                   `json:"memory_mb"`
	Mode                domain.ExecutionMode  `json:"mode,omitempty"`
	Backend             domain.BackendType    `json:"backend,omitempty"`
	InstanceConcurrency int                   `json:"instance_concurrency,omitempty"`
	CodeHash            string                `json:"code_hash,omitempty"`
	Handler             string                `json:"handler,omitempty"`
	Limits              domain.ResourceLimits `json:"limits"`
	NetworkPolicy       *domain.NetworkPolicy `json:"network_policy,omitempty"`
	Layers              []string              `json:"layers,omitempty"`
	Mounts              []domain.VolumeMount  `json:"mounts,omitempty"`
	EnvVars             []string              `json:"env_vars,omitempty"`
	RuntimeCommand      []string              `json:"runtime_command,omitempty"`
	RuntimeExtension    string                `json:"runtime_extension,omitempty"`
	RuntimeImageName    string                `json:"runtime_image_name,omitempty"`
}

func (p *Pool) poolKeyForFunction(fn *domain.Function) string {
	if fn == nil {
		return ""
	}
	limits := domain.ResourceLimits{}
	if fn.Limits != nil {
		limits = *fn.Limits
	}
	envVars := make([]string, 0, len(fn.EnvVars))
	for k, v := range fn.EnvVars {
		envVars = append(envVars, k+"="+v)
	}
	sort.Strings(envVars)

	payload := poolKeyPayload{
		Runtime:             fn.Runtime,
		MemoryMB:            fn.MemoryMB,
		Mode:                fn.Mode,
		Backend:             fn.Backend,
		InstanceConcurrency: fn.InstanceConcurrency,
		CodeHash:            fn.CodeHash,
		Handler:             fn.Handler,
		Limits:              limits,
		NetworkPolicy:       fn.NetworkPolicy,
		Layers:              fn.Layers,
		Mounts:              fn.Mounts,
		EnvVars:             envVars,
		RuntimeCommand:      fn.RuntimeCommand,
		RuntimeExtension:    fn.RuntimeExtension,
		RuntimeImageName:    fn.RuntimeImageName,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		logging.Op().Warn("failed to build pool key payload, using fallback key", "function", fn.ID, "error", err)
		return fmt.Sprintf("fallback|%s|%d|%s|%s", fn.Runtime, fn.MemoryMB, fn.Backend, fn.CodeHash)
	}
	return string(b)
}

func (p *Pool) getPoolForFunctionID(funcID string) (string, *functionPool, bool) {
	if poolKey, ok := p.functionPoolKeys.Load(funcID); ok {
		if val, ok := p.pools.Load(poolKey.(string)); ok {
			return poolKey.(string), val.(*functionPool), true
		}
	}
	// Backward compatibility for any legacy per-function keyed entries.
	if val, ok := p.pools.Load(funcID); ok {
		return funcID, val.(*functionPool), true
	}
	return "", nil, false
}

// getOrCreatePool returns the function pool by key, creating it if needed.
func (p *Pool) getOrCreatePool(poolKey string) *functionPool {
	if fp, ok := p.pools.Load(poolKey); ok {
		return fp.(*functionPool)
	}
	fp := &functionPool{
		functionRefs: make(map[string]struct{}),
	}
	fp.cond = sync.NewCond(&fp.mu)
	actual, _ := p.pools.LoadOrStore(poolKey, fp)
	return actual.(*functionPool)
}
