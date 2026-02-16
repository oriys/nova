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
	functionRefs    map[string]struct{}
}

// SnapshotCallback is called after a cold start to create a snapshot
type SnapshotCallback func(ctx context.Context, vmID, funcID string) error

type Pool struct {
	backend             backend.Backend
	pools               sync.Map // map[string]*functionPool - per-configuration pools
	functionPoolKeys    sync.Map // map[string]string - function ID -> pool key
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
