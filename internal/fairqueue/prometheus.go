package fairqueue

import (
	"fmt"
	"strings"
	"time"
)

// PrometheusExporter exports QoS metrics in Prometheus text format.
type PrometheusExporter struct {
	metrics *QoSMetrics
	bp      *BackpressureState
}

// NewPrometheusExporter creates a metrics exporter.
func NewPrometheusExporter(metrics *QoSMetrics, bp *BackpressureState) *PrometheusExporter {
	return &PrometheusExporter{metrics: metrics, bp: bp}
}

// Export returns Prometheus-formatted metrics text.
func (pe *PrometheusExporter) Export() string {
	var b strings.Builder
	snap := pe.metrics.Snapshot()

	// Enqueue/dequeue totals
	b.WriteString("# HELP nova_qos_enqueue_total Total items enqueued\n")
	b.WriteString("# TYPE nova_qos_enqueue_total counter\n")
	fmt.Fprintf(&b, "nova_qos_enqueue_total %d\n", snap.TotalEnqueued)

	b.WriteString("# HELP nova_qos_dequeue_total Total items dequeued\n")
	b.WriteString("# TYPE nova_qos_dequeue_total counter\n")
	fmt.Fprintf(&b, "nova_qos_dequeue_total %d\n", snap.TotalDequeued)

	b.WriteString("# HELP nova_qos_rejected_total Total items rejected\n")
	b.WriteString("# TYPE nova_qos_rejected_total counter\n")
	fmt.Fprintf(&b, "nova_qos_rejected_total %d\n", snap.TotalRejected)

	b.WriteString("# HELP nova_qos_expired_total Total items expired\n")
	b.WriteString("# TYPE nova_qos_expired_total counter\n")
	fmt.Fprintf(&b, "nova_qos_expired_total %d\n", snap.TotalExpired)

	// Budget exhaustion
	b.WriteString("# HELP nova_qos_budget_exhausted_total Requests that exhausted timeout budget\n")
	b.WriteString("# TYPE nova_qos_budget_exhausted_total counter\n")
	fmt.Fprintf(&b, "nova_qos_budget_exhausted_total %d\n", snap.TotalBudgetExhausted)

	// Priority class distribution
	b.WriteString("# HELP nova_qos_priority_enqueued_total Items enqueued per priority class\n")
	b.WriteString("# TYPE nova_qos_priority_enqueued_total counter\n")
	for class, count := range snap.PriorityEnqueued {
		fmt.Fprintf(&b, "nova_qos_priority_enqueued_total{class=%q} %d\n", class, count)
	}

	b.WriteString("# HELP nova_qos_priority_dequeued_total Items dequeued per priority class\n")
	b.WriteString("# TYPE nova_qos_priority_dequeued_total counter\n")
	for class, count := range snap.PriorityDequeued {
		fmt.Fprintf(&b, "nova_qos_priority_dequeued_total{class=%q} %d\n", class, count)
	}

	// Wait time per tenant
	b.WriteString("# HELP nova_qos_wait_seconds Average time items spend waiting in queue\n")
	b.WriteString("# TYPE nova_qos_wait_seconds gauge\n")
	for tenant, avgMs := range snap.TenantAvgWaitMs {
		fmt.Fprintf(&b, "nova_qos_wait_seconds{tenant=%q} %.6f\n", tenant, avgMs/1000)
	}

	// Backpressure status
	if pe.bp != nil {
		bpStats := pe.bp.AllStats()
		if len(bpStats) > 0 {
			b.WriteString("# HELP nova_qos_inflight Current in-flight requests per tenant\n")
			b.WriteString("# TYPE nova_qos_inflight gauge\n")
			b.WriteString("# HELP nova_qos_backpressure_rejections_total Requests rejected by backpressure\n")
			b.WriteString("# TYPE nova_qos_backpressure_rejections_total counter\n")

			for _, st := range bpStats {
				fmt.Fprintf(&b, "nova_qos_inflight{tenant=%q} %d\n", st.TenantID, st.Inflight)
				fmt.Fprintf(&b, "nova_qos_backpressure_rejections_total{tenant=%q} %d\n", st.TenantID, st.TotalReject)
			}
		}
	}

	// Timestamp
	fmt.Fprintf(&b, "# nova_qos_metrics_timestamp %d\n", time.Now().UnixMilli())

	return b.String()
}
