package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusMetrics wraps prometheus collectors for Nova metrics
type PrometheusMetrics struct {
	registry *prometheus.Registry

	// Counters
	invocationsTotal *prometheus.CounterVec
	coldStartsTotal  prometheus.Counter
	warmStartsTotal  prometheus.Counter
	vmsCreated       prometheus.Counter
	vmsStopped       prometheus.Counter
	vmsCrashed       prometheus.Counter
	snapshotsHit     prometheus.Counter

	// Histograms
	invocationDuration  *prometheus.HistogramVec
	vmBootDuration      *prometheus.HistogramVec
	snapshotRestoreTime *prometheus.HistogramVec
	vsockLatency        *prometheus.HistogramVec

	// Gauges
	uptime          prometheus.GaugeFunc
	vmPool          *prometheus.GaugeVec
	poolUtilization *prometheus.GaugeVec
	activeRequests  prometheus.Gauge
	activeVMs       prometheus.Gauge

	// Autoscaling
	autoscaleDesiredReplicas *prometheus.GaugeVec
	autoscaleDecisionsTotal  *prometheus.CounterVec

	// Admission control
	admissionTotal *prometheus.CounterVec
	shedTotal      *prometheus.CounterVec
	queueDepth     *prometheus.GaugeVec
	queueWaitMs    *prometheus.GaugeVec

	// Circuit breaker
	circuitBreakerState       *prometheus.GaugeVec
	circuitBreakerTripsTotal  *prometheus.CounterVec
}

// Default histogram buckets for invocation duration (in milliseconds)
var defaultBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

var promMetrics *PrometheusMetrics

// InitPrometheus initializes the Prometheus metrics subsystem
func InitPrometheus(namespace string, buckets []float64) {
	if buckets == nil || len(buckets) == 0 {
		buckets = defaultBuckets
	}

	registry := prometheus.NewRegistry()
	// Register default Go and process collectors
	registry.MustRegister(prometheus.NewGoCollector())
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	pm := &PrometheusMetrics{
		registry: registry,

		invocationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "invocations_total",
				Help:      "Total number of function invocations",
			},
			[]string{"function", "runtime", "status"},
		),

		coldStartsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cold_starts_total",
				Help:      "Total number of cold starts",
			},
		),

		warmStartsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "warm_starts_total",
				Help:      "Total number of warm starts",
			},
		),

		vmsCreated: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "vms_created_total",
				Help:      "Total VMs created",
			},
		),

		vmsStopped: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "vms_stopped_total",
				Help:      "Total VMs stopped",
			},
		),

		vmsCrashed: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "vms_crashed_total",
				Help:      "Total VMs that crashed unexpectedly",
			},
		),

		snapshotsHit: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "snapshots_hit_total",
				Help:      "Total snapshot restores",
			},
		),

		invocationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "invocation_duration_milliseconds",
				Help:      "Duration of function invocations in milliseconds",
				Buckets:   buckets,
			},
			[]string{"function", "runtime", "cold_start"},
		),

		vmBootDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "vm_boot_duration_milliseconds",
				Help:      "Duration of VM boot (cold start) in milliseconds",
				Buckets:   []float64{100, 250, 500, 1000, 2000, 3000, 5000, 10000},
			},
			[]string{"function", "runtime", "from_snapshot"},
		),

		snapshotRestoreTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "snapshot_restore_milliseconds",
				Help:      "Duration of snapshot restore in milliseconds",
				Buckets:   []float64{50, 100, 200, 500, 1000, 2000},
			},
			[]string{"function"},
		),

		vsockLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "vsock_latency_milliseconds",
				Help:      "Latency of vsock operations in milliseconds",
				Buckets:   []float64{0.5, 1, 2, 5, 10, 25, 50, 100},
			},
			[]string{"operation"}, // connect, send, receive
		),

		vmPool: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "vm_pool_size",
				Help:      "Current VM pool size by function and state",
			},
			[]string{"function", "state"},
		),

		poolUtilization: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "pool_utilization_ratio",
				Help:      "Pool utilization ratio (busy / total) by function",
			},
			[]string{"function"},
		),

		activeRequests: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "active_requests",
				Help:      "Number of currently active invocation requests",
			},
		),

		activeVMs: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "active_vms",
				Help:      "Total number of active VMs across all function pools",
			},
		),

		autoscaleDesiredReplicas: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "autoscale_desired_replicas",
				Help:      "Current desired replica count set by autoscaler",
			},
			[]string{"function"},
		),

		autoscaleDecisionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "autoscale_decisions_total",
				Help:      "Total auto-scaling decisions",
			},
			[]string{"function", "direction"},
		),

		admissionTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "admission_total",
				Help:      "Admission decisions by result and reason",
			},
			[]string{"function", "result", "reason"},
		),

		shedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "shed_total",
				Help:      "Load shedding events",
			},
			[]string{"function", "reason"},
		),

		queueDepth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "queue_depth",
				Help:      "Current queue depth by function",
			},
			[]string{"function"},
		),

		queueWaitMs: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "queue_wait_milliseconds",
				Help:      "Last observed queue wait in milliseconds by function",
			},
			[]string{"function"},
		),

		circuitBreakerState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "circuit_breaker_state",
				Help:      "Current circuit breaker state (0=closed, 1=open, 2=half_open)",
			},
			[]string{"function"},
		),

		circuitBreakerTripsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "circuit_breaker_trips_total",
				Help:      "Total circuit breaker state transitions",
			},
			[]string{"function", "to_state"},
		),
	}

	pm.uptime = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "uptime_seconds",
			Help:      "Time since Nova daemon started",
		},
		func() float64 {
			return time.Since(StartTime()).Seconds()
		},
	)

	registry.MustRegister(
		pm.invocationsTotal,
		pm.coldStartsTotal,
		pm.warmStartsTotal,
		pm.vmsCreated,
		pm.vmsStopped,
		pm.vmsCrashed,
		pm.snapshotsHit,
		pm.invocationDuration,
		pm.vmBootDuration,
		pm.snapshotRestoreTime,
		pm.vsockLatency,
		pm.uptime,
		pm.vmPool,
		pm.poolUtilization,
		pm.activeRequests,
		pm.activeVMs,
		pm.autoscaleDesiredReplicas,
		pm.autoscaleDecisionsTotal,
		pm.admissionTotal,
		pm.shedTotal,
		pm.queueDepth,
		pm.queueWaitMs,
		pm.circuitBreakerState,
		pm.circuitBreakerTripsTotal,
	)

	promMetrics = pm
}

// RecordPrometheusInvocation records an invocation in Prometheus collectors
func RecordPrometheusInvocation(funcName, runtime string, durationMs int64, coldStart bool, success bool) {
	if promMetrics == nil {
		return
	}

	status := "success"
	if !success {
		status = "failed"
	}
	promMetrics.invocationsTotal.WithLabelValues(funcName, runtime, status).Inc()

	if coldStart {
		promMetrics.coldStartsTotal.Inc()
	} else {
		promMetrics.warmStartsTotal.Inc()
	}

	coldLabel := "false"
	if coldStart {
		coldLabel = "true"
	}
	promMetrics.invocationDuration.WithLabelValues(funcName, runtime, coldLabel).Observe(float64(durationMs))
}

// RecordPrometheusVMCreated records a VM creation in Prometheus
func RecordPrometheusVMCreated() {
	if promMetrics == nil {
		return
	}
	promMetrics.vmsCreated.Inc()
}

// RecordPrometheusVMStopped records a VM stop in Prometheus
func RecordPrometheusVMStopped() {
	if promMetrics == nil {
		return
	}
	promMetrics.vmsStopped.Inc()
}

// RecordPrometheusVMCrashed records a VM crash in Prometheus
func RecordPrometheusVMCrashed() {
	if promMetrics == nil {
		return
	}
	promMetrics.vmsCrashed.Inc()
}

// RecordPrometheusSnapshotHit records a snapshot restore in Prometheus
func RecordPrometheusSnapshotHit() {
	if promMetrics == nil {
		return
	}
	promMetrics.snapshotsHit.Inc()
}

// SetVMPoolSize sets the current VM pool size for a function
func SetVMPoolSize(funcName string, idle, busy int) {
	if promMetrics == nil {
		return
	}
	promMetrics.vmPool.WithLabelValues(funcName, "idle").Set(float64(idle))
	promMetrics.vmPool.WithLabelValues(funcName, "busy").Set(float64(busy))

	// Calculate and set utilization ratio
	total := idle + busy
	if total > 0 {
		promMetrics.poolUtilization.WithLabelValues(funcName).Set(float64(busy) / float64(total))
	}
}

// RecordVMBootDuration records VM boot time in Prometheus
func RecordVMBootDuration(funcName, runtime string, durationMs int64, fromSnapshot bool) {
	if promMetrics == nil {
		return
	}
	snapshotLabel := "false"
	if fromSnapshot {
		snapshotLabel = "true"
	}
	promMetrics.vmBootDuration.WithLabelValues(funcName, runtime, snapshotLabel).Observe(float64(durationMs))
}

// RecordSnapshotRestoreTime records snapshot restore duration
func RecordSnapshotRestoreTime(funcName string, durationMs int64) {
	if promMetrics == nil {
		return
	}
	promMetrics.snapshotRestoreTime.WithLabelValues(funcName).Observe(float64(durationMs))
}

// RecordVsockLatency records vsock operation latency
func RecordVsockLatency(operation string, durationMs float64) {
	if promMetrics == nil {
		return
	}
	promMetrics.vsockLatency.WithLabelValues(operation).Observe(durationMs)
}

// IncActiveRequests increments the active requests counter
func IncActiveRequests() {
	if promMetrics == nil {
		return
	}
	promMetrics.activeRequests.Inc()
}

// DecActiveRequests decrements the active requests counter
func DecActiveRequests() {
	if promMetrics == nil {
		return
	}
	promMetrics.activeRequests.Dec()
}

// SetActiveVMs sets the total number of active VMs across all pools
func SetActiveVMs(count int) {
	if promMetrics == nil {
		return
	}
	promMetrics.activeVMs.Set(float64(count))
}

// PrometheusHandler returns an HTTP handler for Prometheus metrics scraping
func PrometheusHandler() http.Handler {
	if promMetrics == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("prometheus metrics not initialized"))
		})
	}
	return promhttp.HandlerFor(promMetrics.registry, promhttp.HandlerOpts{})
}

// PrometheusRegistry returns the prometheus registry (for custom collectors)
func PrometheusRegistry() *prometheus.Registry {
	if promMetrics == nil {
		return nil
	}
	return promMetrics.registry
}

// SetAutoscaleDesiredReplicas sets the desired replica gauge
func SetAutoscaleDesiredReplicas(funcName string, desired int) {
	if promMetrics == nil {
		return
	}
	promMetrics.autoscaleDesiredReplicas.WithLabelValues(funcName).Set(float64(desired))
}

// RecordAutoscaleDecision records an autoscale decision
func RecordAutoscaleDecision(funcName, direction string) {
	if promMetrics == nil {
		return
	}
	promMetrics.autoscaleDecisionsTotal.WithLabelValues(funcName, direction).Inc()
}

// RecordAdmissionResult records request admission/rejection decisions.
func RecordAdmissionResult(funcName, result, reason string) {
	if promMetrics == nil {
		return
	}
	promMetrics.admissionTotal.WithLabelValues(funcName, result, reason).Inc()
}

// RecordShed records load-shedding events for a function.
func RecordShed(funcName, reason string) {
	if promMetrics == nil {
		return
	}
	promMetrics.shedTotal.WithLabelValues(funcName, reason).Inc()
}

// SetQueueDepth sets the queue depth gauge for a function.
func SetQueueDepth(funcName string, depth int) {
	if promMetrics == nil {
		return
	}
	promMetrics.queueDepth.WithLabelValues(funcName).Set(float64(depth))
}

// SetQueueWaitMs sets the latest queue wait duration gauge for a function.
func SetQueueWaitMs(funcName string, waitMs int64) {
	if promMetrics == nil {
		return
	}
	promMetrics.queueWaitMs.WithLabelValues(funcName).Set(float64(waitMs))
}

// SetCircuitBreakerState sets the circuit breaker state gauge for a function.
// state: 0=closed, 1=open, 2=half_open
func SetCircuitBreakerState(funcName string, state int) {
	if promMetrics == nil {
		return
	}
	promMetrics.circuitBreakerState.WithLabelValues(funcName).Set(float64(state))
}

// RecordCircuitBreakerTrip records a circuit breaker state transition.
func RecordCircuitBreakerTrip(funcName, toState string) {
	if promMetrics == nil {
		return
	}
	promMetrics.circuitBreakerTripsTotal.WithLabelValues(funcName, toState).Inc()
}
