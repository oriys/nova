package metrics

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects and exposes Nova runtime metrics
type Metrics struct {
	// Invocation metrics
	TotalInvocations   atomic.Int64
	SuccessInvocations atomic.Int64
	FailedInvocations  atomic.Int64
	ColdStarts         atomic.Int64
	WarmStarts         atomic.Int64

	// Latency metrics (in milliseconds)
	TotalLatencyMs atomic.Int64
	MinLatencyMs   atomic.Int64
	MaxLatencyMs   atomic.Int64

	// VM metrics
	VMsCreated   atomic.Int64
	VMsStopped   atomic.Int64
	VMsCrashed   atomic.Int64
	SnapshotsHit atomic.Int64

	// Per-function metrics
	funcMetrics sync.Map // funcID -> *FunctionMetrics

	startTime time.Time
}

// FunctionMetrics tracks metrics for a single function
type FunctionMetrics struct {
	Invocations atomic.Int64
	Successes   atomic.Int64
	Failures    atomic.Int64
	ColdStarts  atomic.Int64
	WarmStarts  atomic.Int64
	TotalMs     atomic.Int64
	MinMs       atomic.Int64
	MaxMs       atomic.Int64
}

// Global metrics instance
var global = &Metrics{startTime: time.Now()}

func init() {
	global.MinLatencyMs.Store(int64(^uint64(0) >> 1)) // Max int64
}

// Global returns the global metrics instance
func Global() *Metrics {
	return global
}

// StartTime returns the time when the metrics system was initialized
func StartTime() time.Time {
	return global.startTime
}

// RecordInvocation records an invocation result
func (m *Metrics) RecordInvocation(funcID string, durationMs int64, coldStart bool, success bool) {
	m.RecordInvocationWithDetails(funcID, "", "", durationMs, coldStart, success)
}

// RecordInvocationWithDetails records an invocation with function name and runtime for Prometheus labels
func (m *Metrics) RecordInvocationWithDetails(funcID, funcName, runtime string, durationMs int64, coldStart bool, success bool) {
	m.TotalInvocations.Add(1)

	if success {
		m.SuccessInvocations.Add(1)
	} else {
		m.FailedInvocations.Add(1)
	}

	if coldStart {
		m.ColdStarts.Add(1)
	} else {
		m.WarmStarts.Add(1)
	}

	m.TotalLatencyMs.Add(durationMs)
	updateMin(&m.MinLatencyMs, durationMs)
	updateMax(&m.MaxLatencyMs, durationMs)

	// Per-function metrics
	fm := m.getFunctionMetrics(funcID)
	fm.Invocations.Add(1)
	if success {
		fm.Successes.Add(1)
	} else {
		fm.Failures.Add(1)
	}
	if coldStart {
		fm.ColdStarts.Add(1)
	} else {
		fm.WarmStarts.Add(1)
	}
	fm.TotalMs.Add(durationMs)
	updateMin(&fm.MinMs, durationMs)
	updateMax(&fm.MaxMs, durationMs)

	// Prometheus bridge
	RecordPrometheusInvocation(funcName, runtime, durationMs, coldStart, success)
}

// RecordVMCreated records a new VM creation
func (m *Metrics) RecordVMCreated() {
	m.VMsCreated.Add(1)
	RecordPrometheusVMCreated()
}

// RecordVMStopped records a VM being stopped
func (m *Metrics) RecordVMStopped() {
	m.VMsStopped.Add(1)
	RecordPrometheusVMStopped()
}

// RecordVMCrashed records a VM crash
func (m *Metrics) RecordVMCrashed() {
	m.VMsCrashed.Add(1)
	RecordPrometheusVMCrashed()
}

// RecordSnapshotHit records a snapshot being used instead of cold boot
func (m *Metrics) RecordSnapshotHit() {
	m.SnapshotsHit.Add(1)
	RecordPrometheusSnapshotHit()
}

func (m *Metrics) getFunctionMetrics(funcID string) *FunctionMetrics {
	if v, ok := m.funcMetrics.Load(funcID); ok {
		return v.(*FunctionMetrics)
	}

	fm := &FunctionMetrics{}
	fm.MinMs.Store(int64(^uint64(0) >> 1))
	actual, _ := m.funcMetrics.LoadOrStore(funcID, fm)
	return actual.(*FunctionMetrics)
}

// Snapshot returns a point-in-time snapshot of all metrics
func (m *Metrics) Snapshot() map[string]interface{} {
	total := m.TotalInvocations.Load()
	avgLatency := float64(0)
	if total > 0 {
		avgLatency = float64(m.TotalLatencyMs.Load()) / float64(total)
	}

	minLatency := m.MinLatencyMs.Load()
	if minLatency == int64(^uint64(0)>>1) {
		minLatency = 0
	}

	result := map[string]interface{}{
		"uptime_seconds": int64(time.Since(m.startTime).Seconds()),
		"invocations": map[string]interface{}{
			"total":    total,
			"success":  m.SuccessInvocations.Load(),
			"failed":   m.FailedInvocations.Load(),
			"cold":     m.ColdStarts.Load(),
			"warm":     m.WarmStarts.Load(),
			"cold_pct": coldStartPercentage(m.ColdStarts.Load(), total),
		},
		"latency_ms": map[string]interface{}{
			"avg": avgLatency,
			"min": minLatency,
			"max": m.MaxLatencyMs.Load(),
		},
		"vms": map[string]interface{}{
			"created":       m.VMsCreated.Load(),
			"stopped":       m.VMsStopped.Load(),
			"crashed":       m.VMsCrashed.Load(),
			"snapshots_hit": m.SnapshotsHit.Load(),
		},
	}

	return result
}

// FunctionStats returns per-function metrics
func (m *Metrics) FunctionStats() map[string]interface{} {
	result := make(map[string]interface{})

	m.funcMetrics.Range(func(key, value interface{}) bool {
		funcID := key.(string)
		fm := value.(*FunctionMetrics)

		total := fm.Invocations.Load()
		avgMs := float64(0)
		if total > 0 {
			avgMs = float64(fm.TotalMs.Load()) / float64(total)
		}

		minMs := fm.MinMs.Load()
		if minMs == int64(^uint64(0)>>1) {
			minMs = 0
		}

		result[funcID] = map[string]interface{}{
			"invocations": total,
			"successes":   fm.Successes.Load(),
			"failures":    fm.Failures.Load(),
			"cold_starts": fm.ColdStarts.Load(),
			"warm_starts": fm.WarmStarts.Load(),
			"avg_ms":      avgMs,
			"min_ms":      minMs,
			"max_ms":      fm.MaxMs.Load(),
		}
		return true
	})

	return result
}

// JSONHandler returns an HTTP handler that exposes metrics in JSON format
func (m *Metrics) JSONHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		result := m.Snapshot()
		result["functions"] = m.FunctionStats()
		json.NewEncoder(w).Encode(result)
	})
}

// Helper functions

func updateMin(target *atomic.Int64, value int64) {
	for {
		old := target.Load()
		if value >= old {
			return
		}
		if target.CompareAndSwap(old, value) {
			return
		}
	}
}

func updateMax(target *atomic.Int64, value int64) {
	for {
		old := target.Load()
		if value <= old {
			return
		}
		if target.CompareAndSwap(old, value) {
			return
		}
	}
}

func coldStartPercentage(cold, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(cold) / float64(total) * 100
}
