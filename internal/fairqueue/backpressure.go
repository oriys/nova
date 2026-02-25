package fairqueue

import (
	"sync"
	"sync/atomic"
	"time"
)

// BackpressureConfig defines per-tenant backpressure limits.
type BackpressureConfig struct {
	SoftLimit   int // In-flight count that triggers priority degradation
	HardLimit   int // In-flight count that triggers rejection (429)
	RetryAfterS int // Retry-After header value in seconds
	CooldownS   int // Seconds to wait after hitting hard limit before accepting again
}

// DefaultBackpressureConfig returns sensible defaults.
func DefaultBackpressureConfig() BackpressureConfig {
	return BackpressureConfig{
		SoftLimit:   50,
		HardLimit:   100,
		RetryAfterS: 5,
		CooldownS:   10,
	}
}

// BackpressureState tracks per-tenant execution pressure.
type BackpressureState struct {
	mu       sync.RWMutex
	tenants  map[string]*tenantPressure
	defaults BackpressureConfig
}

type tenantPressure struct {
	inflight    atomic.Int64
	config      BackpressureConfig
	lastReject  time.Time
	totalReject int64
	totalShed   int64
}

// NewBackpressureState creates a new backpressure tracker.
func NewBackpressureState(defaults BackpressureConfig) *BackpressureState {
	return &BackpressureState{
		tenants:  make(map[string]*tenantPressure),
		defaults: defaults,
	}
}

// SetConfig overrides backpressure config for a tenant.
func (bp *BackpressureState) SetConfig(tenantID string, cfg BackpressureConfig) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	tp := bp.getOrCreate(tenantID)
	tp.config = cfg
}

// Admit checks if a request from the tenant should be admitted.
// Returns (admitted, retryAfterS).
func (bp *BackpressureState) Admit(tenantID string) (bool, int) {
	bp.mu.RLock()
	tp := bp.getOrCreateRLocked(tenantID)
	bp.mu.RUnlock()

	current := int(tp.inflight.Load())
	cfg := tp.config

	if cfg.HardLimit > 0 && current >= cfg.HardLimit {
		tp.totalReject++
		tp.lastReject = time.Now()
		return false, cfg.RetryAfterS
	}

	return true, 0
}

// IsOverSoftLimit returns true if the tenant is over the soft inflight limit.
func (bp *BackpressureState) IsOverSoftLimit(tenantID string) bool {
	bp.mu.RLock()
	tp := bp.getOrCreateRLocked(tenantID)
	bp.mu.RUnlock()

	current := int(tp.inflight.Load())
	return tp.config.SoftLimit > 0 && current >= tp.config.SoftLimit
}

// Acquire increments the inflight counter for a tenant.
func (bp *BackpressureState) Acquire(tenantID string) {
	bp.mu.RLock()
	tp := bp.getOrCreateRLocked(tenantID)
	bp.mu.RUnlock()
	tp.inflight.Add(1)
}

// Release decrements the inflight counter for a tenant.
func (bp *BackpressureState) Release(tenantID string) {
	bp.mu.RLock()
	tp := bp.getOrCreateRLocked(tenantID)
	bp.mu.RUnlock()
	tp.inflight.Add(-1)
}

// TenantInflight returns the current inflight count for a tenant.
func (bp *BackpressureState) TenantInflight(tenantID string) int64 {
	bp.mu.RLock()
	tp := bp.getOrCreateRLocked(tenantID)
	bp.mu.RUnlock()
	return tp.inflight.Load()
}

// BackpressureStats contains backpressure statistics for a tenant.
type BackpressureStats struct {
	TenantID    string `json:"tenant_id"`
	Inflight    int64  `json:"inflight"`
	SoftLimit   int    `json:"soft_limit"`
	HardLimit   int    `json:"hard_limit"`
	TotalReject int64  `json:"total_reject"`
	OverSoft    bool   `json:"over_soft"`
}

// AllStats returns backpressure statistics for all tenants.
func (bp *BackpressureState) AllStats() []BackpressureStats {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	var stats []BackpressureStats
	for id, tp := range bp.tenants {
		inflight := tp.inflight.Load()
		stats = append(stats, BackpressureStats{
			TenantID:    id,
			Inflight:    inflight,
			SoftLimit:   tp.config.SoftLimit,
			HardLimit:   tp.config.HardLimit,
			TotalReject: tp.totalReject,
			OverSoft:    tp.config.SoftLimit > 0 && int(inflight) >= tp.config.SoftLimit,
		})
	}
	return stats
}

func (bp *BackpressureState) getOrCreate(tenantID string) *tenantPressure {
	tp, ok := bp.tenants[tenantID]
	if !ok {
		tp = &tenantPressure{config: bp.defaults}
		bp.tenants[tenantID] = tp
	}
	return tp
}

func (bp *BackpressureState) getOrCreateRLocked(tenantID string) *tenantPressure {
	tp, ok := bp.tenants[tenantID]
	if !ok {
		// Upgrade to write lock
		bp.mu.RUnlock()
		bp.mu.Lock()
		tp, ok = bp.tenants[tenantID]
		if !ok {
			tp = &tenantPressure{config: bp.defaults}
			bp.tenants[tenantID] = tp
		}
		bp.mu.Unlock()
		bp.mu.RLock()
	}
	return tp
}
