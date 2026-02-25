package snapshot

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// SnapshotMetrics tracks snapshot operational metrics.
type SnapshotMetrics struct {
	mu sync.RWMutex

	// Hit/miss by layer
	hits   map[Layer]int64
	misses int64

	// Restore latency by layer
	restoreLatencies map[Layer][]time.Duration

	// Storage stats
	totalSnapshots int64
	totalBytes     int64

	// GC stats
	gcRuns      int64
	gcEvictions int64
	gcDuration  time.Duration
}

// NewSnapshotMetrics creates a new metrics tracker.
func NewSnapshotMetrics() *SnapshotMetrics {
	return &SnapshotMetrics{
		hits:             make(map[Layer]int64),
		restoreLatencies: make(map[Layer][]time.Duration),
	}
}

// RecordHit records a snapshot cache hit at the given layer.
func (sm *SnapshotMetrics) RecordHit(layer Layer) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.hits[layer]++
}

// RecordMiss records a snapshot cache miss (full boot required).
func (sm *SnapshotMetrics) RecordMiss() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.misses++
}

// RecordRestoreLatency records a restore duration at the given layer.
func (sm *SnapshotMetrics) RecordRestoreLatency(layer Layer, d time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	latencies := sm.restoreLatencies[layer]
	// Keep last 1000 samples per layer
	if len(latencies) >= 1000 {
		latencies = latencies[1:]
	}
	sm.restoreLatencies[layer] = append(latencies, d)
}

// RecordGC records a GC run.
func (sm *SnapshotMetrics) RecordGC(evictions int, duration time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.gcRuns++
	sm.gcEvictions += int64(evictions)
	sm.gcDuration += duration
}

// UpdateStorage updates storage stats.
func (sm *SnapshotMetrics) UpdateStorage(count, bytes int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.totalSnapshots = count
	sm.totalBytes = bytes
}

// ExportPrometheus returns metrics in Prometheus text format.
func (sm *SnapshotMetrics) ExportPrometheus() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var b strings.Builder

	// Hit rate by layer
	b.WriteString("# HELP nova_snapshot_hits_total Snapshot cache hits by layer\n")
	b.WriteString("# TYPE nova_snapshot_hits_total counter\n")
	for layer, count := range sm.hits {
		fmt.Fprintf(&b, "nova_snapshot_hits_total{layer=%q} %d\n", layer.String(), count)
	}

	b.WriteString("# HELP nova_snapshot_misses_total Snapshot cache misses (full boot)\n")
	b.WriteString("# TYPE nova_snapshot_misses_total counter\n")
	fmt.Fprintf(&b, "nova_snapshot_misses_total %d\n", sm.misses)

	// Restore latency by layer
	b.WriteString("# HELP nova_snapshot_restore_seconds Snapshot restore latency\n")
	b.WriteString("# TYPE nova_snapshot_restore_seconds summary\n")
	for layer, latencies := range sm.restoreLatencies {
		if len(latencies) == 0 {
			continue
		}
		var total time.Duration
		var maxL time.Duration
		for _, l := range latencies {
			total += l
			if l > maxL {
				maxL = l
			}
		}
		avg := float64(total.Nanoseconds()) / float64(len(latencies)) / 1e9
		fmt.Fprintf(&b, "nova_snapshot_restore_seconds{layer=%q,quantile=\"avg\"} %.9f\n", layer.String(), avg)
		fmt.Fprintf(&b, "nova_snapshot_restore_seconds{layer=%q,quantile=\"max\"} %.9f\n", layer.String(), maxL.Seconds())
		fmt.Fprintf(&b, "nova_snapshot_restore_seconds_count{layer=%q} %d\n", layer.String(), len(latencies))
	}

	// Storage
	b.WriteString("# HELP nova_snapshot_storage_total Total snapshots stored\n")
	b.WriteString("# TYPE nova_snapshot_storage_total gauge\n")
	fmt.Fprintf(&b, "nova_snapshot_storage_total %d\n", sm.totalSnapshots)

	b.WriteString("# HELP nova_snapshot_storage_bytes Total snapshot storage bytes\n")
	b.WriteString("# TYPE nova_snapshot_storage_bytes gauge\n")
	fmt.Fprintf(&b, "nova_snapshot_storage_bytes %d\n", sm.totalBytes)

	// GC
	b.WriteString("# HELP nova_snapshot_gc_runs_total Total GC runs\n")
	b.WriteString("# TYPE nova_snapshot_gc_runs_total counter\n")
	fmt.Fprintf(&b, "nova_snapshot_gc_runs_total %d\n", sm.gcRuns)

	b.WriteString("# HELP nova_snapshot_gc_evictions_total Total snapshots evicted by GC\n")
	b.WriteString("# TYPE nova_snapshot_gc_evictions_total counter\n")
	fmt.Fprintf(&b, "nova_snapshot_gc_evictions_total %d\n", sm.gcEvictions)

	return b.String()
}
