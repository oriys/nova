package metrics

import (
	"encoding/json"
	"fmt"
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

// RecordInvocation records an invocation result
func (m *Metrics) RecordInvocation(funcID string, durationMs int64, coldStart bool, success bool) {
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
}

// RecordVMCreated records a new VM creation
func (m *Metrics) RecordVMCreated() {
	m.VMsCreated.Add(1)
}

// RecordVMStopped records a VM being stopped
func (m *Metrics) RecordVMStopped() {
	m.VMsStopped.Add(1)
}

// RecordVMCrashed records a VM crash
func (m *Metrics) RecordVMCrashed() {
	m.VMsCrashed.Add(1)
}

// RecordSnapshotHit records a snapshot being used instead of cold boot
func (m *Metrics) RecordSnapshotHit() {
	m.SnapshotsHit.Add(1)
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

// PrometheusHandler returns an HTTP handler that exposes metrics in Prometheus format
func (m *Metrics) PrometheusHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		total := m.TotalInvocations.Load()
		avgLatency := float64(0)
		if total > 0 {
			avgLatency = float64(m.TotalLatencyMs.Load()) / float64(total)
		}

		// Global metrics
		lines := []string{
			"# HELP nova_uptime_seconds Time since Nova daemon started",
			"# TYPE nova_uptime_seconds gauge",
			formatMetric("nova_uptime_seconds", nil, int64(time.Since(m.startTime).Seconds())),

			"# HELP nova_invocations_total Total number of function invocations",
			"# TYPE nova_invocations_total counter",
			formatMetric("nova_invocations_total", map[string]string{"status": "success"}, m.SuccessInvocations.Load()),
			formatMetric("nova_invocations_total", map[string]string{"status": "failed"}, m.FailedInvocations.Load()),

			"# HELP nova_cold_starts_total Total number of cold starts",
			"# TYPE nova_cold_starts_total counter",
			formatMetric("nova_cold_starts_total", nil, m.ColdStarts.Load()),

			"# HELP nova_warm_starts_total Total number of warm starts",
			"# TYPE nova_warm_starts_total counter",
			formatMetric("nova_warm_starts_total", nil, m.WarmStarts.Load()),

			"# HELP nova_latency_ms_avg Average invocation latency in milliseconds",
			"# TYPE nova_latency_ms_avg gauge",
			formatMetricFloat("nova_latency_ms_avg", nil, avgLatency),

			"# HELP nova_vms_created_total Total VMs created",
			"# TYPE nova_vms_created_total counter",
			formatMetric("nova_vms_created_total", nil, m.VMsCreated.Load()),

			"# HELP nova_vms_crashed_total Total VMs that crashed unexpectedly",
			"# TYPE nova_vms_crashed_total counter",
			formatMetric("nova_vms_crashed_total", nil, m.VMsCrashed.Load()),

			"# HELP nova_snapshots_hit_total Total snapshot restores (vs cold boots)",
			"# TYPE nova_snapshots_hit_total counter",
			formatMetric("nova_snapshots_hit_total", nil, m.SnapshotsHit.Load()),
		}

		for _, line := range lines {
			w.Write([]byte(line + "\n"))
		}

		// Per-function metrics
		w.Write([]byte("\n# HELP nova_function_invocations_total Invocations per function\n"))
		w.Write([]byte("# TYPE nova_function_invocations_total counter\n"))

		m.funcMetrics.Range(func(key, value interface{}) bool {
			funcID := key.(string)
			fm := value.(*FunctionMetrics)

			w.Write([]byte(formatMetric("nova_function_invocations_total",
				map[string]string{"function": funcID, "status": "success"},
				fm.Successes.Load()) + "\n"))
			w.Write([]byte(formatMetric("nova_function_invocations_total",
				map[string]string{"function": funcID, "status": "failed"},
				fm.Failures.Load()) + "\n"))
			return true
		})
	})
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

func formatMetric(name string, labels map[string]string, value int64) string {
	if len(labels) == 0 {
		return name + " " + formatInt(value)
	}
	labelStr := ""
	for k, v := range labels {
		if labelStr != "" {
			labelStr += ","
		}
		labelStr += k + "=\"" + v + "\""
	}
	return name + "{" + labelStr + "} " + formatInt(value)
}

func formatMetricFloat(name string, labels map[string]string, value float64) string {
	if len(labels) == 0 {
		return name + " " + formatFloat(value)
	}
	labelStr := ""
	for k, v := range labels {
		if labelStr != "" {
			labelStr += ","
		}
		labelStr += k + "=\"" + v + "\""
	}
	return name + "{" + labelStr + "} " + formatFloat(value)
}

func formatInt(v int64) string {
	return fmt.Sprintf("%d", v)
}

func formatFloat(v float64) string {
	return fmt.Sprintf("%.2f", v)
}
