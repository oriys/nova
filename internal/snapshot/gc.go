package snapshot

import (
	"context"
	"sync"
	"time"

	"github.com/oriys/nova/internal/logging"
)

// GCConfig configures snapshot garbage collection.
type GCConfig struct {
	Interval        time.Duration // How often to run GC (default: 5 min)
	MaxStorageBytes int64         // Maximum total snapshot storage (default: 10 GB)
	MaxAge          time.Duration // Maximum snapshot age (default: 24h)
	MinKeepPerFunc  int           // Minimum snapshots to keep per function (default: 1)
}

// DefaultGCConfig returns sensible GC defaults.
func DefaultGCConfig() GCConfig {
	return GCConfig{
		Interval:        5 * time.Minute,
		MaxStorageBytes: 10 * 1024 * 1024 * 1024, // 10 GB
		MaxAge:          24 * time.Hour,
		MinKeepPerFunc:  1,
	}
}

// GarbageCollector periodically cleans up stale and excess snapshots.
type GarbageCollector struct {
	mu      sync.Mutex
	manager *Manager
	cfg     GCConfig
	ctx     context.Context
	cancel  context.CancelFunc

	// Callbacks for actual file deletion (injected for testability)
	deleteFunc func(snapshotPath, memoryPath string) error

	// Stats
	totalCollected  int64
	totalBytesFreed int64
	lastRunAt       time.Time
}

// NewGarbageCollector creates a new snapshot GC.
func NewGarbageCollector(manager *Manager, cfg GCConfig) *GarbageCollector {
	ctx, cancel := context.WithCancel(context.Background())
	return &GarbageCollector{
		manager:    manager,
		cfg:        cfg,
		ctx:        ctx,
		cancel:     cancel,
		deleteFunc: func(string, string) error { return nil }, // No-op default
	}
}

// SetDeleteFunc sets the function used to delete snapshot files.
func (gc *GarbageCollector) SetDeleteFunc(fn func(snapshotPath, memoryPath string) error) {
	gc.deleteFunc = fn
}

// Start begins the periodic GC loop.
func (gc *GarbageCollector) Start() {
	go gc.loop()
}

// Stop halts the GC loop.
func (gc *GarbageCollector) Stop() {
	gc.cancel()
}

// RunOnce performs a single GC pass.
func (gc *GarbageCollector) RunOnce() GCStats {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	stats := GCStats{StartedAt: time.Now()}

	// Phase 1: Remove expired snapshots (beyond MaxAge)
	expired := gc.collectExpired()
	stats.ExpiredRemoved = len(expired)

	// Phase 2: Remove snapshots for deleted functions (stale codeHash)
	stale := gc.collectStale()
	stats.StaleRemoved = len(stale)

	// Phase 3: LRU eviction if storage exceeds threshold
	evicted := gc.evictLRU()
	stats.LRUEvicted = len(evicted)

	stats.TotalRemoved = stats.ExpiredRemoved + stats.StaleRemoved + stats.LRUEvicted
	stats.Duration = time.Since(stats.StartedAt)
	gc.lastRunAt = stats.StartedAt
	gc.totalCollected += int64(stats.TotalRemoved)

	if stats.TotalRemoved > 0 {
		logging.Op().Info("snapshot GC completed",
			"expired", stats.ExpiredRemoved,
			"stale", stats.StaleRemoved,
			"lru_evicted", stats.LRUEvicted,
			"duration", stats.Duration)
	}

	return stats
}

// GCStats reports the results of a GC pass.
type GCStats struct {
	StartedAt      time.Time     `json:"started_at"`
	Duration       time.Duration `json:"duration"`
	ExpiredRemoved int           `json:"expired_removed"`
	StaleRemoved   int           `json:"stale_removed"`
	LRUEvicted     int           `json:"lru_evicted"`
	TotalRemoved   int           `json:"total_removed"`
}

func (gc *GarbageCollector) collectExpired() []string {
	entries := gc.manager.ListAll()
	cutoff := time.Now().Add(-gc.cfg.MaxAge)
	var removed []string

	for _, entry := range entries {
		if entry.CreatedAt.Before(cutoff) {
			if err := gc.deleteFunc(entry.SnapPath, entry.MemPath); err != nil {
				logging.Op().Error("failed to delete expired snapshot", "id", entry.Key.String(), "error", err)
				continue
			}
			gc.manager.Remove(entry.Key.String())
			removed = append(removed, entry.Key.String())
		}
	}
	return removed
}

func (gc *GarbageCollector) collectStale() []string {
	// Get all L2 (function-level) snapshots and check if their codeHash is still current
	entries := gc.manager.ListAll()
	var removed []string

	funcLatest := make(map[string]string) // funcID -> latest codeHash
	for _, entry := range entries {
		if entry.Key.Layer == LayerFunction {
			if latest, ok := funcLatest[entry.Key.FuncID]; ok {
				if entry.Key.CodeHash != latest {
					// Stale version
					if err := gc.deleteFunc(entry.SnapPath, entry.MemPath); err != nil {
						continue
					}
					gc.manager.Remove(entry.Key.String())
					removed = append(removed, entry.Key.String())
				}
			} else {
				funcLatest[entry.Key.FuncID] = entry.Key.CodeHash
			}
		}
	}
	return removed
}

func (gc *GarbageCollector) evictLRU() []string {
	// Use the manager's built-in LRU eviction
	evicted := gc.manager.EvictLRU(gc.cfg.MaxStorageBytes)
	for _, id := range evicted {
		entry := gc.manager.Get(id)
		if entry != nil {
			gc.deleteFunc(entry.SnapPath, entry.MemPath)
		}
	}
	return evicted
}

func (gc *GarbageCollector) loop() {
	ticker := time.NewTicker(gc.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-gc.ctx.Done():
			return
		case <-ticker.C:
			gc.RunOnce()
		}
	}
}
