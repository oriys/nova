package fairqueue

import (
	"sync"
	"sync/atomic"
	"time"
)

// QoSMetrics tracks queue performance metrics.
type QoSMetrics struct {
	mu sync.Mutex

	// Per-tenant metrics
	tenantWaitTimes       map[string][]time.Duration // Sliding window of wait times
	tenantBudgetExhausted map[string]*atomic.Int64

	// Global metrics
	TotalEnqueued        atomic.Int64
	TotalDequeued        atomic.Int64
	TotalRejected        atomic.Int64
	TotalExpired         atomic.Int64
	TotalBudgetExhausted atomic.Int64

	// Per-priority metrics
	PriorityEnqueued [3]atomic.Int64 // background, standard, critical
	PriorityDequeued [3]atomic.Int64
}

// NewQoSMetrics creates a new QoS metrics tracker.
func NewQoSMetrics() *QoSMetrics {
	return &QoSMetrics{
		tenantWaitTimes:       make(map[string][]time.Duration),
		tenantBudgetExhausted: make(map[string]*atomic.Int64),
	}
}

// RecordEnqueue records an item being enqueued.
func (m *QoSMetrics) RecordEnqueue(tenantID string, priority PriorityClass) {
	m.TotalEnqueued.Add(1)
	if int(priority) < len(m.PriorityEnqueued) {
		m.PriorityEnqueued[priority].Add(1)
	}
}

// RecordDequeue records an item being dequeued with wait time.
func (m *QoSMetrics) RecordDequeue(tenantID string, priority PriorityClass, waitTime time.Duration) {
	m.TotalDequeued.Add(1)
	if int(priority) < len(m.PriorityDequeued) {
		m.PriorityDequeued[priority].Add(1)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	waits := m.tenantWaitTimes[tenantID]
	if len(waits) >= 100 {
		waits = waits[1:]
	}
	m.tenantWaitTimes[tenantID] = append(waits, waitTime)
}

// RecordRejection records a rejected request.
func (m *QoSMetrics) RecordRejection(tenantID string) {
	m.TotalRejected.Add(1)
}

// RecordBudgetExhausted records a timeout budget exhaustion at a pipeline stage.
func (m *QoSMetrics) RecordBudgetExhausted(tenantID string) {
	m.TotalBudgetExhausted.Add(1)
	m.mu.Lock()
	counter, ok := m.tenantBudgetExhausted[tenantID]
	if !ok {
		counter = &atomic.Int64{}
		m.tenantBudgetExhausted[tenantID] = counter
	}
	m.mu.Unlock()
	counter.Add(1)
}

// TenantAvgWaitMs returns the average queue wait time for a tenant in milliseconds.
func (m *QoSMetrics) TenantAvgWaitMs(tenantID string) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tenantAvgWaitMsLocked(tenantID)
}

// tenantAvgWaitMsLocked computes average wait time while mu is already held.
func (m *QoSMetrics) tenantAvgWaitMsLocked(tenantID string) float64 {
	waits := m.tenantWaitTimes[tenantID]
	if len(waits) == 0 {
		return 0
	}
	var total time.Duration
	for _, w := range waits {
		total += w
	}
	return float64(total.Milliseconds()) / float64(len(waits))
}

// MetricsSnapshot is a point-in-time snapshot of all metrics.
type MetricsSnapshot struct {
	TotalEnqueued        int64              `json:"total_enqueued"`
	TotalDequeued        int64              `json:"total_dequeued"`
	TotalRejected        int64              `json:"total_rejected"`
	TotalExpired         int64              `json:"total_expired"`
	TotalBudgetExhausted int64              `json:"total_budget_exhausted"`
	PriorityEnqueued     map[string]int64   `json:"priority_enqueued"`
	PriorityDequeued     map[string]int64   `json:"priority_dequeued"`
	TenantAvgWaitMs      map[string]float64 `json:"tenant_avg_wait_ms"`
}

// Snapshot returns a point-in-time snapshot of all metrics.
func (m *QoSMetrics) Snapshot() MetricsSnapshot {
	s := MetricsSnapshot{
		TotalEnqueued:        m.TotalEnqueued.Load(),
		TotalDequeued:        m.TotalDequeued.Load(),
		TotalRejected:        m.TotalRejected.Load(),
		TotalExpired:         m.TotalExpired.Load(),
		TotalBudgetExhausted: m.TotalBudgetExhausted.Load(),
		PriorityEnqueued:     make(map[string]int64),
		PriorityDequeued:     make(map[string]int64),
		TenantAvgWaitMs:      make(map[string]float64),
	}
	for i, name := range []string{"background", "standard", "critical"} {
		s.PriorityEnqueued[name] = m.PriorityEnqueued[i].Load()
		s.PriorityDequeued[name] = m.PriorityDequeued[i].Load()
	}
	m.mu.Lock()
	for tenantID := range m.tenantWaitTimes {
		s.TenantAvgWaitMs[tenantID] = m.tenantAvgWaitMsLocked(tenantID)
	}
	m.mu.Unlock()
	return s
}
