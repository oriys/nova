package tuning

import (
	"math"
	"sync"
	"time"
)

// Signal represents a named telemetry signal for a function.
type Signal struct {
	Name      string    `json:"name"`
	Value     float64   `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

// FunctionSignals aggregates all tuning-relevant signals for a function.
type FunctionSignals struct {
	FunctionID string `json:"function_id"`

	// Cold/warm start ratio
	ColdStarts  int64   `json:"cold_starts"`
	WarmStarts  int64   `json:"warm_starts"`
	ColdRatio   float64 `json:"cold_ratio"`

	// Latency breakdown
	P50Latency    time.Duration `json:"p50_latency"`
	P95Latency    time.Duration `json:"p95_latency"`
	P99Latency    time.Duration `json:"p99_latency"`
	QueueWait     time.Duration `json:"avg_queue_wait"`
	BootTime      time.Duration `json:"avg_boot_time"`
	ExecTime      time.Duration `json:"avg_exec_time"`

	// Snapshot metrics
	SnapshotHitRate float64 `json:"snapshot_hit_rate"`
	SnapshotHits    int64   `json:"snapshot_hits"`
	SnapshotMisses  int64   `json:"snapshot_misses"`

	// VM utilization
	VMBusyRatio  float64 `json:"vm_busy_ratio"`  // busy_time / alive_time
	VMCount      int     `json:"vm_count"`        // Current pool size
	VMIdleCount  int     `json:"vm_idle_count"`   // Currently idle VMs

	// Compilation
	CompileTime  time.Duration `json:"avg_compile_time"`
	CompileCount int64         `json:"compile_count"`

	// Request rate
	RequestRate float64 `json:"request_rate_per_sec"`
	ErrorRate   float64 `json:"error_rate"`

	// Collected window
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

// LatencyBucket stores latency samples for percentile calculation.
type LatencyBucket struct {
	samples  []time.Duration
	maxSize  int
}

// NewLatencyBucket creates a bucket with max sample size.
func NewLatencyBucket(maxSize int) *LatencyBucket {
	return &LatencyBucket{
		samples: make([]time.Duration, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add adds a latency sample.
func (lb *LatencyBucket) Add(d time.Duration) {
	if len(lb.samples) >= lb.maxSize {
		// Evict oldest (FIFO)
		lb.samples = lb.samples[1:]
	}
	lb.samples = append(lb.samples, d)
}

// Percentile returns the p-th percentile (0-100).
func (lb *LatencyBucket) Percentile(p float64) time.Duration {
	n := len(lb.samples)
	if n == 0 {
		return 0
	}

	// Sort a copy
	sorted := make([]time.Duration, n)
	copy(sorted, lb.samples)
	sortDurations(sorted)

	idx := int(math.Ceil(p/100*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}

func sortDurations(d []time.Duration) {
	// Simple insertion sort (good for small-medium arrays)
	for i := 1; i < len(d); i++ {
		key := d[i]
		j := i - 1
		for j >= 0 && d[j] > key {
			d[j+1] = d[j]
			j--
		}
		d[j+1] = key
	}
}

// SignalAggregator collects and aggregates telemetry signals per function.
type SignalAggregator struct {
	mu sync.RWMutex

	// Per-function state
	coldStarts   map[string]int64
	warmStarts   map[string]int64
	latencies    map[string]*LatencyBucket
	queueWaits   map[string]*LatencyBucket
	bootTimes    map[string]*LatencyBucket
	execTimes    map[string]*LatencyBucket
	compileTimes map[string]*LatencyBucket
	snapHits     map[string]int64
	snapMisses   map[string]int64
	requestCount map[string]int64
	errorCount   map[string]int64

	// Time tracking
	windowStart time.Time
	bucketSize  int
}

// NewSignalAggregator creates a new aggregator.
func NewSignalAggregator() *SignalAggregator {
	return &SignalAggregator{
		coldStarts:   make(map[string]int64),
		warmStarts:   make(map[string]int64),
		latencies:    make(map[string]*LatencyBucket),
		queueWaits:   make(map[string]*LatencyBucket),
		bootTimes:    make(map[string]*LatencyBucket),
		execTimes:    make(map[string]*LatencyBucket),
		compileTimes: make(map[string]*LatencyBucket),
		snapHits:     make(map[string]int64),
		snapMisses:   make(map[string]int64),
		requestCount: make(map[string]int64),
		errorCount:   make(map[string]int64),
		windowStart:  time.Now(),
		bucketSize:   10000,
	}
}

// RecordInvocation records a single invocation's telemetry.
func (sa *SignalAggregator) RecordInvocation(funcID string, cold bool, totalLatency, queueWait, bootTime, execTime time.Duration, err bool) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if cold {
		sa.coldStarts[funcID]++
	} else {
		sa.warmStarts[funcID]++
	}

	sa.ensureBucket(funcID)
	sa.latencies[funcID].Add(totalLatency)
	sa.queueWaits[funcID].Add(queueWait)
	sa.bootTimes[funcID].Add(bootTime)
	sa.execTimes[funcID].Add(execTime)

	sa.requestCount[funcID]++
	if err {
		sa.errorCount[funcID]++
	}
}

// RecordSnapshotHit records a snapshot cache hit.
func (sa *SignalAggregator) RecordSnapshotHit(funcID string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.snapHits[funcID]++
}

// RecordSnapshotMiss records a snapshot cache miss.
func (sa *SignalAggregator) RecordSnapshotMiss(funcID string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.snapMisses[funcID]++
}

// RecordCompilation records a compilation event.
func (sa *SignalAggregator) RecordCompilation(funcID string, duration time.Duration) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.ensureBucket(funcID)
	sa.compileTimes[funcID].Add(duration)
}

// GetSignals returns aggregated signals for a function.
func (sa *SignalAggregator) GetSignals(funcID string) *FunctionSignals {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	now := time.Now()
	signals := &FunctionSignals{
		FunctionID:  funcID,
		ColdStarts:  sa.coldStarts[funcID],
		WarmStarts:  sa.warmStarts[funcID],
		SnapshotHits:   sa.snapHits[funcID],
		SnapshotMisses: sa.snapMisses[funcID],
		WindowStart: sa.windowStart,
		WindowEnd:   now,
	}

	total := signals.ColdStarts + signals.WarmStarts
	if total > 0 {
		signals.ColdRatio = float64(signals.ColdStarts) / float64(total)
	}

	snapTotal := signals.SnapshotHits + signals.SnapshotMisses
	if snapTotal > 0 {
		signals.SnapshotHitRate = float64(signals.SnapshotHits) / float64(snapTotal)
	}

	if lb, ok := sa.latencies[funcID]; ok {
		signals.P50Latency = lb.Percentile(50)
		signals.P95Latency = lb.Percentile(95)
		signals.P99Latency = lb.Percentile(99)
	}
	if lb, ok := sa.queueWaits[funcID]; ok {
		signals.QueueWait = lb.Percentile(50)
	}
	if lb, ok := sa.bootTimes[funcID]; ok {
		signals.BootTime = lb.Percentile(50)
	}
	if lb, ok := sa.execTimes[funcID]; ok {
		signals.ExecTime = lb.Percentile(50)
	}
	if lb, ok := sa.compileTimes[funcID]; ok {
		signals.CompileTime = lb.Percentile(50)
	}

	elapsed := now.Sub(sa.windowStart).Seconds()
	if elapsed > 0 {
		signals.RequestRate = float64(sa.requestCount[funcID]) / elapsed
		if sa.requestCount[funcID] > 0 {
			signals.ErrorRate = float64(sa.errorCount[funcID]) / float64(sa.requestCount[funcID])
		}
	}

	return signals
}

// ListFunctions returns all function IDs with recorded signals.
func (sa *SignalAggregator) ListFunctions() []string {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	seen := make(map[string]bool)
	for id := range sa.requestCount {
		seen[id] = true
	}
	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	return result
}

// Reset clears all aggregated data and starts a new window.
func (sa *SignalAggregator) Reset() {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	sa.coldStarts = make(map[string]int64)
	sa.warmStarts = make(map[string]int64)
	sa.latencies = make(map[string]*LatencyBucket)
	sa.queueWaits = make(map[string]*LatencyBucket)
	sa.bootTimes = make(map[string]*LatencyBucket)
	sa.execTimes = make(map[string]*LatencyBucket)
	sa.compileTimes = make(map[string]*LatencyBucket)
	sa.snapHits = make(map[string]int64)
	sa.snapMisses = make(map[string]int64)
	sa.requestCount = make(map[string]int64)
	sa.errorCount = make(map[string]int64)
	sa.windowStart = time.Now()
}

func (sa *SignalAggregator) ensureBucket(funcID string) {
	if _, ok := sa.latencies[funcID]; !ok {
		sa.latencies[funcID] = NewLatencyBucket(sa.bucketSize)
		sa.queueWaits[funcID] = NewLatencyBucket(sa.bucketSize)
		sa.bootTimes[funcID] = NewLatencyBucket(sa.bucketSize)
		sa.execTimes[funcID] = NewLatencyBucket(sa.bucketSize)
		sa.compileTimes[funcID] = NewLatencyBucket(sa.bucketSize)
	}
}
