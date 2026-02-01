package metrics

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// TimeSeriesBucket stores metrics for a single time bucket
type TimeSeriesBucket struct {
	Timestamp    time.Time
	Invocations  int64
	Errors       int64
	TotalLatency int64
	Count        int64 // for calculating avg
}

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

	// Time-series data (hourly buckets for last 24 hours)
	timeSeriesMu sync.RWMutex
	timeSeries   []*TimeSeriesBucket

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
	global.initTimeSeries()
}

// initTimeSeries initializes time series buckets for the last 24 hours
func (m *Metrics) initTimeSeries() {
	m.timeSeriesMu.Lock()
	defer m.timeSeriesMu.Unlock()

	now := time.Now().Truncate(time.Hour)
	m.timeSeries = make([]*TimeSeriesBucket, 24)
	for i := 0; i < 24; i++ {
		m.timeSeries[i] = &TimeSeriesBucket{
			Timestamp: now.Add(time.Duration(i-23) * time.Hour),
		}
	}
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

	// Time series recording
	m.recordTimeSeries(durationMs, !success)

	// Prometheus bridge
	RecordPrometheusInvocation(funcName, runtime, durationMs, coldStart, success)
}

// recordTimeSeries adds an invocation to the current time bucket
func (m *Metrics) recordTimeSeries(durationMs int64, isError bool) {
	m.timeSeriesMu.Lock()
	defer m.timeSeriesMu.Unlock()

	now := time.Now().Truncate(time.Hour)

	// Check if we need to rotate buckets
	if len(m.timeSeries) > 0 {
		lastBucket := m.timeSeries[len(m.timeSeries)-1]
		hoursDiff := int(now.Sub(lastBucket.Timestamp).Hours())

		if hoursDiff > 0 {
			// Rotate buckets
			if hoursDiff >= 24 {
				// Reset all buckets
				m.timeSeries = make([]*TimeSeriesBucket, 24)
				for i := 0; i < 24; i++ {
					m.timeSeries[i] = &TimeSeriesBucket{
						Timestamp: now.Add(time.Duration(i-23) * time.Hour),
					}
				}
			} else {
				// Shift and add new buckets
				m.timeSeries = m.timeSeries[hoursDiff:]
				for i := 0; i < hoursDiff; i++ {
					m.timeSeries = append(m.timeSeries, &TimeSeriesBucket{
						Timestamp: lastBucket.Timestamp.Add(time.Duration(i+1) * time.Hour),
					})
				}
			}
		}
	}

	// Record to current bucket
	if len(m.timeSeries) > 0 {
		bucket := m.timeSeries[len(m.timeSeries)-1]
		bucket.Invocations++
		bucket.TotalLatency += durationMs
		bucket.Count++
		if isError {
			bucket.Errors++
		}
	}
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

// TimeSeries returns the time-series data for the last 24 hours
func (m *Metrics) TimeSeries() []map[string]interface{} {
	m.timeSeriesMu.RLock()
	defer m.timeSeriesMu.RUnlock()

	result := make([]map[string]interface{}, len(m.timeSeries))
	for i, bucket := range m.timeSeries {
		avgDuration := float64(0)
		if bucket.Count > 0 {
			avgDuration = float64(bucket.TotalLatency) / float64(bucket.Count)
		}
		result[i] = map[string]interface{}{
			"timestamp":    bucket.Timestamp.Format(time.RFC3339),
			"invocations":  bucket.Invocations,
			"errors":       bucket.Errors,
			"avg_duration": avgDuration,
		}
	}
	return result
}

// TimeSeriesHandler returns an HTTP handler for time-series metrics
func (m *Metrics) TimeSeriesHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m.TimeSeries())
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
