// Package fairqueue implements a Weighted Fair Queue (WFQ) for multi-tenant
// scheduling. Each tenant has a configurable weight; tenants with higher
// weights receive proportionally more scheduling slots.
//
// The implementation uses virtual finish-time ordering: each enqueued item
// is stamped with a virtual finish time computed as (virtual_start + 1/weight).
// A min-heap orders items by virtual finish time so the "most deserving"
// tenant is always dequeued first.
package fairqueue

import (
	"container/heap"
	"context"
	"sync"
	"time"
)

// PriorityClass defines scheduling tiers.
type PriorityClass int

const (
	PriorityBackground PriorityClass = 0
	PriorityStandard   PriorityClass = 1
	PriorityCritical   PriorityClass = 2
)

func (p PriorityClass) String() string {
	switch p {
	case PriorityBackground:
		return "background"
	case PriorityStandard:
		return "standard"
	case PriorityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// ParsePriorityClass parses a string into a PriorityClass.
func ParsePriorityClass(s string) PriorityClass {
	switch s {
	case "critical":
		return PriorityCritical
	case "background":
		return PriorityBackground
	default:
		return PriorityStandard
	}
}

// Item represents a unit of work in the fair queue.
type Item struct {
	TenantID      string
	FunctionID    string
	Priority      PriorityClass
	EnqueuedAt    time.Time
	Deadline      time.Time   // Timeout budget deadline
	VirtualFinish float64     // Computed by the queue
	Payload       interface{}
	index         int // Heap index
}

// TenantConfig holds per-tenant scheduling configuration.
type TenantConfig struct {
	Weight        float64 // Higher = more scheduling slots (default: 1.0)
	MaxInflight   int     // Hard limit on concurrent executions (0 = unlimited)
	SoftInflight  int     // Soft limit; degrades priority when exceeded
	MaxQueueDepth int     // Maximum pending items (0 = unlimited)
}

// DefaultTenantConfig returns a TenantConfig with sensible defaults.
func DefaultTenantConfig() TenantConfig {
	return TenantConfig{
		Weight:        1.0,
		MaxInflight:   0,
		SoftInflight:  0,
		MaxQueueDepth: 0,
	}
}

// tenantState tracks per-tenant scheduling state.
type tenantState struct {
	config      TenantConfig
	virtualTime float64
	inflight    int
	queueDepth  int
}

// Queue is the main weighted fair queue.
type Queue struct {
	mu       sync.Mutex
	cond     *sync.Cond
	items    itemHeap
	tenants  map[string]*tenantState
	defaults TenantConfig
	closed   bool

	// Metrics
	totalEnqueued int64
	totalDequeued int64
	totalRejected int64
	totalExpired  int64
}

// NewQueue creates a new weighted fair queue.
func NewQueue(defaults TenantConfig) *Queue {
	q := &Queue{
		tenants:  make(map[string]*tenantState),
		defaults: defaults,
	}
	q.cond = sync.NewCond(&q.mu)
	heap.Init(&q.items)
	return q
}

// SetTenantConfig sets the scheduling configuration for a tenant.
func (q *Queue) SetTenantConfig(tenantID string, cfg TenantConfig) {
	q.mu.Lock()
	defer q.mu.Unlock()
	ts := q.getOrCreateTenant(tenantID)
	ts.config = cfg
}

// Enqueue adds an item to the queue. Returns an error if the tenant's
// queue depth limit is exceeded or the deadline has already passed.
func (q *Queue) Enqueue(item *Item) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ErrQueueClosed
	}

	// Check deadline
	if !item.Deadline.IsZero() && time.Now().After(item.Deadline) {
		q.totalExpired++
		return ErrDeadlineExceeded
	}

	ts := q.getOrCreateTenant(item.TenantID)

	// Check queue depth limit
	if ts.config.MaxQueueDepth > 0 && ts.queueDepth >= ts.config.MaxQueueDepth {
		q.totalRejected++
		return ErrQueueFull
	}

	// Check inflight hard limit
	if ts.config.MaxInflight > 0 && ts.inflight >= ts.config.MaxInflight {
		q.totalRejected++
		return ErrInflightLimit
	}

	// Compute virtual finish time
	weight := ts.config.Weight
	if weight <= 0 {
		weight = 1.0
	}
	// Degrade priority if over soft limit
	if ts.config.SoftInflight > 0 && ts.inflight >= ts.config.SoftInflight {
		weight *= 0.5
	}
	// Priority boost: critical items get 3x weight, background gets 0.3x
	switch item.Priority {
	case PriorityCritical:
		weight *= 3.0
	case PriorityBackground:
		weight *= 0.3
	}

	item.VirtualFinish = ts.virtualTime + (1.0 / weight)
	ts.virtualTime = item.VirtualFinish
	ts.queueDepth++

	heap.Push(&q.items, item)
	q.totalEnqueued++
	q.cond.Signal()
	return nil
}

// Dequeue removes and returns the next item from the queue.
// Blocks until an item is available or the context is cancelled.
func (q *Queue) Dequeue(ctx context.Context) (*Item, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for q.items.Len() == 0 && !q.closed {
		// Wait with context awareness
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				q.cond.Broadcast()
			case <-done:
			}
		}()
		q.cond.Wait()
		close(done)

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	if q.closed && q.items.Len() == 0 {
		return nil, ErrQueueClosed
	}

	// Skip expired items
	for q.items.Len() > 0 {
		item := heap.Pop(&q.items).(*Item)
		ts := q.getOrCreateTenant(item.TenantID)
		ts.queueDepth--

		if !item.Deadline.IsZero() && time.Now().After(item.Deadline) {
			q.totalExpired++
			continue
		}

		ts.inflight++
		q.totalDequeued++
		return item, nil
	}

	return nil, ErrQueueEmpty
}

// Release signals that an item has completed processing,
// decrementing the tenant's inflight count.
func (q *Queue) Release(tenantID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if ts, ok := q.tenants[tenantID]; ok {
		if ts.inflight > 0 {
			ts.inflight--
		}
	}
}

// Close shuts down the queue, unblocking all waiters.
func (q *Queue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.cond.Broadcast()
}

// Stats returns current queue statistics.
type Stats struct {
	QueueDepth    int                    `json:"queue_depth"`
	TenantStats   map[string]TenantStats `json:"tenant_stats"`
	TotalEnqueued int64                  `json:"total_enqueued"`
	TotalDequeued int64                  `json:"total_dequeued"`
	TotalRejected int64                  `json:"total_rejected"`
	TotalExpired  int64                  `json:"total_expired"`
}

// TenantStats holds per-tenant queue statistics.
type TenantStats struct {
	Inflight   int     `json:"inflight"`
	QueueDepth int     `json:"queue_depth"`
	Weight     float64 `json:"weight"`
}

func (q *Queue) Stats() Stats {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := Stats{
		QueueDepth:    q.items.Len(),
		TenantStats:   make(map[string]TenantStats),
		TotalEnqueued: q.totalEnqueued,
		TotalDequeued: q.totalDequeued,
		TotalRejected: q.totalRejected,
		TotalExpired:  q.totalExpired,
	}
	for id, ts := range q.tenants {
		s.TenantStats[id] = TenantStats{
			Inflight:   ts.inflight,
			QueueDepth: ts.queueDepth,
			Weight:     ts.config.Weight,
		}
	}
	return s
}

func (q *Queue) getOrCreateTenant(tenantID string) *tenantState {
	ts, ok := q.tenants[tenantID]
	if !ok {
		ts = &tenantState{config: q.defaults}
		q.tenants[tenantID] = ts
	}
	return ts
}

// itemHeap implements heap.Interface for *Item sorted by VirtualFinish.
type itemHeap []*Item

func (h itemHeap) Len() int          { return len(h) }
func (h itemHeap) Less(i, j int) bool { return h[i].VirtualFinish < h[j].VirtualFinish }
func (h itemHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *itemHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*Item)
	item.index = n
	*h = append(*h, item)
}
func (h *itemHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[:n-1]
	return item
}
