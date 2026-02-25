package snapshot

import (
	"context"
	"fmt"
	"time"

	"github.com/oriys/nova/internal/domain"
)

// RestoreResult describes the outcome of a snapshot restore attempt.
type RestoreResult struct {
	Layer       Layer         `json:"layer"`
	SnapshotID  string        `json:"snapshot_id"`
	RestoreTime time.Duration `json:"restore_time"`
	FullBoot    bool          `json:"full_boot"` // True if no snapshot was available
}

// RestorePipeline orchestrates layered snapshot restore with fallback.
type RestorePipeline struct {
	manager *Manager
}

// NewRestorePipeline creates a new restore pipeline.
func NewRestorePipeline(manager *Manager) *RestorePipeline {
	return &RestorePipeline{manager: manager}
}

// Restore attempts to restore a VM from the best available snapshot.
// It tries layers in order: L2 (function) → L1 (runtime warm) → L0 (base) → full boot.
func (rp *RestorePipeline) Restore(ctx context.Context, fn *domain.Function) (*RestoreResult, error) {
	start := time.Now()

	// Try resolving from snapshot manager (already does L2 → L1 → L0)
	entry, layer, ok := rp.manager.Resolve(fn)
	if ok {
		id := entry.Key.String()
		rp.manager.RecordRestore(id, layer)
		return &RestoreResult{
			Layer:       layer,
			SnapshotID:  id,
			RestoreTime: time.Since(start),
		}, nil
	}

	// No snapshot available — full boot required
	return &RestoreResult{
		FullBoot:    true,
		RestoreTime: time.Since(start),
	}, nil
}

// RestoreOrBoot attempts snapshot restore; if unavailable, signals full boot.
// Returns the snapshot path and memory path if available, empty strings for full boot.
func (rp *RestorePipeline) RestoreOrBoot(ctx context.Context, fn *domain.Function) (snapPath, memPath string, result *RestoreResult, err error) {
	result, err = rp.Restore(ctx, fn)
	if err != nil {
		return "", "", nil, fmt.Errorf("restore pipeline: %w", err)
	}

	if result.FullBoot {
		return "", "", result, nil
	}

	entry := rp.manager.Get(result.SnapshotID)
	if entry == nil {
		// Snapshot was evicted between resolve and get — fall back to full boot
		result.FullBoot = true
		return "", "", result, nil
	}

	return entry.SnapPath, entry.MemPath, result, nil
}

// RecordRestore records a successful restore for metrics.
func (rp *RestorePipeline) RecordRestore(snapshotID string, layer Layer) {
	rp.manager.RecordRestore(snapshotID, layer)
}
