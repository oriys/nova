// Package snapshot implements a three-layer snapshot hierarchy for fast cold starts.
//
// Layer 0 (Base):     Kernel + rootfs + agent init per runtime
// Layer 1 (Runtime):  Layer 0 + runtime warm-up (stdlib imports, JIT baseline)
// Layer 2 (Function): Layer 1 + function code loaded + handler resolved
//
// Restore priority: L2 → L1 → L0 → full boot fallback.
package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
)

// Layer represents the snapshot hierarchy level.
type Layer int

const (
	LayerBase     Layer = 0 // Kernel + rootfs + agent
	LayerRuntime  Layer = 1 // Runtime warm-up
	LayerFunction Layer = 2 // Function-specific
)

func (l Layer) String() string {
	switch l {
	case LayerBase:
		return "base"
	case LayerRuntime:
		return "runtime"
	case LayerFunction:
		return "function"
	default:
		return "unknown"
	}
}

// SnapshotKey uniquely identifies a snapshot in the hierarchy.
type SnapshotKey struct {
	Layer    Layer          `json:"layer"`
	Runtime  domain.Runtime `json:"runtime"`
	Arch     domain.Arch    `json:"arch"`
	FuncID   string         `json:"func_id,omitempty"`   // Only for Layer 2
	CodeHash string         `json:"code_hash,omitempty"` // Only for Layer 2
}

func (k SnapshotKey) String() string {
	switch k.Layer {
	case LayerBase:
		return fmt.Sprintf("L0/%s-%s", k.Runtime, k.Arch)
	case LayerRuntime:
		return fmt.Sprintf("L1/%s-%s", k.Runtime, k.Arch)
	case LayerFunction:
		return fmt.Sprintf("L2/%s-%s/%s", k.Runtime, k.Arch, k.CodeHash[:min(12, len(k.CodeHash))])
	default:
		return "unknown"
	}
}

// SnapshotEntry holds metadata for a stored snapshot.
type SnapshotEntry struct {
	Key          SnapshotKey `json:"key"`
	SnapPath     string      `json:"snap_path"` // Path to .snap file
	MemPath      string      `json:"mem_path"`  // Path to .mem file
	MetaPath     string      `json:"meta_path"` // Path to .meta file
	SizeBytes    int64       `json:"size_bytes"`
	CreatedAt    time.Time   `json:"created_at"`
	LastUsed     time.Time   `json:"last_used"`
	RestoreCount int64       `json:"restore_count"`
	DiffSnapshot bool        `json:"diff_snapshot"` // Uses diff snapshots for smaller memory files
}

// Metrics tracks snapshot performance.
type Metrics struct {
	mu             sync.Mutex
	HitsL0         int64                    `json:"hits_l0"`
	HitsL1         int64                    `json:"hits_l1"`
	HitsL2         int64                    `json:"hits_l2"`
	Misses         int64                    `json:"misses"`
	RestoreLatency map[Layer][]time.Duration `json:"-"` // Per-layer restore latencies
	StorageBytes   int64                    `json:"storage_bytes"`
}

// RecordHit records a snapshot cache hit at the given layer.
func (m *Metrics) RecordHit(layer Layer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch layer {
	case LayerBase:
		m.HitsL0++
	case LayerRuntime:
		m.HitsL1++
	case LayerFunction:
		m.HitsL2++
	}
}

// RecordMiss records a snapshot cache miss.
func (m *Metrics) RecordMiss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Misses++
}

// RecordRestoreLatency records the restore duration for a given layer.
func (m *Metrics) RecordRestoreLatency(layer Layer, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.RestoreLatency == nil {
		m.RestoreLatency = make(map[Layer][]time.Duration)
	}
	latencies := m.RestoreLatency[layer]
	if len(latencies) >= 1000 {
		latencies = latencies[1:]
	}
	m.RestoreLatency[layer] = append(latencies, d)
}

// HitRate returns the overall snapshot hit rate.
func (m *Metrics) HitRate() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := m.HitsL0 + m.HitsL1 + m.HitsL2 + m.Misses
	if total == 0 {
		return 0
	}
	return float64(m.HitsL0+m.HitsL1+m.HitsL2) / float64(total)
}

// Config holds snapshot manager configuration.
type Config struct {
	BaseDir         string        // Root directory for all snapshots
	MaxStorageBytes int64         // Max total storage (0 = unlimited)
	GCInterval      time.Duration // How often to run GC
	MaxAge          time.Duration // Max age before eviction
	DiffSnapshots   bool          // Use diff snapshots for smaller memory files
	HugePages       bool          // Use huge pages for memory backend
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(baseDir string) Config {
	return Config{
		BaseDir:         baseDir,
		MaxStorageBytes: 10 * 1024 * 1024 * 1024, // 10 GB
		GCInterval:      5 * time.Minute,
		MaxAge:          24 * time.Hour,
		DiffSnapshots:   true,
		HugePages:       false,
	}
}

// Manager manages the three-layer snapshot hierarchy.
type Manager struct {
	cfg     Config
	entries sync.Map // SnapshotKey.String() -> *SnapshotEntry
	metrics Metrics
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewManager creates a new snapshot manager.
func NewManager(cfg Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}
	// Create layer directories
	for _, layer := range []string{"L0", "L1", "L2"} {
		os.MkdirAll(filepath.Join(cfg.BaseDir, layer), 0755)
	}
	// Load existing entries from disk
	m.loadFromDisk()
	// Start GC loop
	go m.gcLoop()
	return m
}

// Resolve finds the best available snapshot for the given function, trying L2 → L1 → L0.
func (m *Manager) Resolve(fn *domain.Function) (*SnapshotEntry, Layer, bool) {
	arch := fn.Arch
	if arch == "" {
		arch = domain.ArchAMD64
	}

	// Try L2 (function-specific)
	l2Key := SnapshotKey{Layer: LayerFunction, Runtime: fn.Runtime, Arch: arch, FuncID: fn.ID, CodeHash: fn.CodeHash}
	if entry, ok := m.get(l2Key); ok {
		m.metrics.RecordHit(LayerFunction)
		return entry, LayerFunction, true
	}

	// Try L1 (runtime)
	l1Key := SnapshotKey{Layer: LayerRuntime, Runtime: fn.Runtime, Arch: arch}
	if entry, ok := m.get(l1Key); ok {
		m.metrics.RecordHit(LayerRuntime)
		return entry, LayerRuntime, true
	}

	// Try L0 (base)
	l0Key := SnapshotKey{Layer: LayerBase, Runtime: fn.Runtime, Arch: arch}
	if entry, ok := m.get(l0Key); ok {
		m.metrics.RecordHit(LayerBase)
		return entry, LayerBase, true
	}

	m.metrics.RecordMiss()
	return nil, -1, false
}

// Store registers a new snapshot entry.
func (m *Manager) Store(key SnapshotKey, snapPath, memPath string) (*SnapshotEntry, error) {
	snapInfo, err := os.Stat(snapPath)
	if err != nil {
		return nil, fmt.Errorf("stat snapshot: %w", err)
	}
	memInfo, err := os.Stat(memPath)
	if err != nil {
		return nil, fmt.Errorf("stat memory: %w", err)
	}

	entry := &SnapshotEntry{
		Key:          key,
		SnapPath:     snapPath,
		MemPath:      memPath,
		MetaPath:     snapPath + ".meta",
		SizeBytes:    snapInfo.Size() + memInfo.Size(),
		CreatedAt:    time.Now(),
		LastUsed:     time.Now(),
		DiffSnapshot: m.cfg.DiffSnapshots,
	}

	m.entries.Store(key.String(), entry)

	// Persist metadata
	m.persistEntry(entry)

	logging.Op().Info("snapshot stored", "key", key.String(), "size_mb", entry.SizeBytes/(1024*1024))
	return entry, nil
}

// Invalidate removes snapshots for a function (e.g., on code update).
func (m *Manager) Invalidate(funcID string) {
	m.entries.Range(func(k, v interface{}) bool {
		entry := v.(*SnapshotEntry)
		if entry.Key.FuncID == funcID {
			m.removeEntry(k.(string), entry)
		}
		return true
	})
}

// InvalidateByCodeHash removes L2 snapshots with a mismatched code hash.
func (m *Manager) InvalidateByCodeHash(funcID, newCodeHash string) {
	m.entries.Range(func(k, v interface{}) bool {
		entry := v.(*SnapshotEntry)
		if entry.Key.FuncID == funcID && entry.Key.Layer == LayerFunction && entry.Key.CodeHash != newCodeHash {
			m.removeEntry(k.(string), entry)
		}
		return true
	})
}

// GetMetrics returns current snapshot metrics.
func (m *Manager) GetMetrics() Metrics {
	m.metrics.mu.Lock()
	defer m.metrics.mu.Unlock()
	return Metrics{
		HitsL0:       m.metrics.HitsL0,
		HitsL1:       m.metrics.HitsL1,
		HitsL2:       m.metrics.HitsL2,
		Misses:       m.metrics.Misses,
		StorageBytes: m.metrics.StorageBytes,
	}
}

// Stop shuts down the snapshot manager.
func (m *Manager) Stop() {
	m.cancel()
}

// Get returns a snapshot entry by its key string, or nil if not found.
func (m *Manager) Get(id string) *SnapshotEntry {
	v, ok := m.entries.Load(id)
	if !ok {
		return nil
	}
	return v.(*SnapshotEntry)
}

// ListAll returns all snapshot entries.
func (m *Manager) ListAll() []*SnapshotEntry {
	var result []*SnapshotEntry
	m.entries.Range(func(_, v interface{}) bool {
		result = append(result, v.(*SnapshotEntry))
		return true
	})
	return result
}

// Remove deletes a snapshot entry by its key string.
func (m *Manager) Remove(id string) {
	if v, ok := m.entries.Load(id); ok {
		m.removeEntry(id, v.(*SnapshotEntry))
	}
}

// RecordRestore records a successful restore for metrics tracking.
func (m *Manager) RecordRestore(id string, layer Layer) {
	m.metrics.RecordHit(layer)
}

// EvictLRU evicts least-recently-used snapshots until total storage is within maxBytes.
// Returns the key strings of evicted entries.
func (m *Manager) EvictLRU(maxBytes int64) []string {
	if maxBytes <= 0 {
		return nil
	}

	type entryWithKey struct {
		key   string
		entry *SnapshotEntry
	}

	var all []entryWithKey
	var totalSize int64
	m.entries.Range(func(k, v interface{}) bool {
		e := v.(*SnapshotEntry)
		all = append(all, entryWithKey{k.(string), e})
		totalSize += e.SizeBytes
		return true
	})

	if totalSize <= maxBytes {
		return nil
	}

	// Sort by last used (oldest first)
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[i].entry.LastUsed.After(all[j].entry.LastUsed) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	var evicted []string
	for _, e := range all {
		if totalSize <= maxBytes {
			break
		}
		totalSize -= e.entry.SizeBytes
		m.entries.Delete(e.key)
		evicted = append(evicted, e.key)
	}
	return evicted
}

func (m *Manager) get(key SnapshotKey) (*SnapshotEntry, bool) {
	v, ok := m.entries.Load(key.String())
	if !ok {
		return nil, false
	}
	entry := v.(*SnapshotEntry)
	// Verify files still exist
	if _, err := os.Stat(entry.SnapPath); err != nil {
		m.entries.Delete(key.String())
		return nil, false
	}
	entry.LastUsed = time.Now()
	entry.RestoreCount++
	return entry, true
}

func (m *Manager) removeEntry(key string, entry *SnapshotEntry) {
	m.entries.Delete(key)
	os.Remove(entry.SnapPath)
	os.Remove(entry.MemPath)
	os.Remove(entry.MetaPath)
	logging.Op().Info("snapshot removed", "key", key)
}

func (m *Manager) persistEntry(entry *SnapshotEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	os.WriteFile(entry.MetaPath, data, 0644)
}

func (m *Manager) loadFromDisk() {
	for _, layer := range []string{"L0", "L1", "L2"} {
		dir := filepath.Join(m.cfg.BaseDir, layer)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if filepath.Ext(e.Name()) != ".meta" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			var entry SnapshotEntry
			if err := json.Unmarshal(data, &entry); err != nil {
				continue
			}
			// Verify snapshot files exist
			if _, err := os.Stat(entry.SnapPath); err != nil {
				continue
			}
			m.entries.Store(entry.Key.String(), &entry)
		}
	}
}

func (m *Manager) gcLoop() {
	ticker := time.NewTicker(m.cfg.GCInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.gc()
		}
	}
}

func (m *Manager) gc() {
	now := time.Now()
	var totalSize int64
	var staleKeys []string

	// Pass 1: identify stale entries and compute total size
	m.entries.Range(func(k, v interface{}) bool {
		entry := v.(*SnapshotEntry)
		totalSize += entry.SizeBytes
		if now.Sub(entry.LastUsed) > m.cfg.MaxAge {
			staleKeys = append(staleKeys, k.(string))
		}
		return true
	})

	// Remove stale entries
	for _, key := range staleKeys {
		if v, ok := m.entries.Load(key); ok {
			m.removeEntry(key, v.(*SnapshotEntry))
		}
	}

	// If still over budget, evict LRU
	if m.cfg.MaxStorageBytes > 0 && totalSize > m.cfg.MaxStorageBytes {
		type entryWithKey struct {
			key   string
			entry *SnapshotEntry
		}
		var all []entryWithKey
		m.entries.Range(func(k, v interface{}) bool {
			all = append(all, entryWithKey{k.(string), v.(*SnapshotEntry)})
			return true
		})
		// Sort by last used (oldest first)
		for i := 0; i < len(all); i++ {
			for j := i + 1; j < len(all); j++ {
				if all[i].entry.LastUsed.After(all[j].entry.LastUsed) {
					all[i], all[j] = all[j], all[i]
				}
			}
		}
		for _, e := range all {
			if totalSize <= m.cfg.MaxStorageBytes {
				break
			}
			totalSize -= e.entry.SizeBytes
			m.removeEntry(e.key, e.entry)
		}
	}

	m.metrics.mu.Lock()
	m.metrics.StorageBytes = totalSize
	m.metrics.mu.Unlock()
}
