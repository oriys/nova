package snapshot

import (
	"context"
	"sync"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
)

// PreWarmConfig configures traffic-prediction pre-warming.
type PreWarmConfig struct {
	Enabled       bool          // Enable pre-warming
	PredictWindow time.Duration // How far ahead to predict (default: 5min)
	CheckInterval time.Duration // How often to check predictions (default: 30s)
	MaxPreWarm    int           // Maximum snapshots to pre-warm per cycle
}

// DefaultPreWarmConfig returns sensible defaults.
func DefaultPreWarmConfig() PreWarmConfig {
	return PreWarmConfig{
		Enabled:       true,
		PredictWindow: 5 * time.Minute,
		CheckInterval: 30 * time.Second,
		MaxPreWarm:    10,
	}
}

// SeasonalitySlot tracks hourly demand for a function.
type SeasonalitySlot struct {
	Hour        int     `json:"hour"`
	AvgDemand   float64 `json:"avg_demand"`
	PeakDemand  float64 `json:"peak_demand"`
	SampleCount int     `json:"sample_count"`
}

// PreWarmer uses traffic predictions to pre-restore function snapshots.
type PreWarmer struct {
	mu          sync.Mutex
	manager     *Manager
	cfg         PreWarmConfig
	seasonality map[string][24]SeasonalitySlot // funcID -> 24-hour slots
	currentWarm map[string]int                 // funcID -> warm count
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewPreWarmer creates a new pre-warming engine.
func NewPreWarmer(manager *Manager, cfg PreWarmConfig) *PreWarmer {
	ctx, cancel := context.WithCancel(context.Background())
	return &PreWarmer{
		manager:     manager,
		cfg:         cfg,
		seasonality: make(map[string][24]SeasonalitySlot),
		currentWarm: make(map[string]int),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start begins the pre-warming loop.
func (pw *PreWarmer) Start() {
	if !pw.cfg.Enabled {
		return
	}
	go pw.loop()
}

// Stop halts the pre-warming loop.
func (pw *PreWarmer) Stop() {
	pw.cancel()
}

// RecordDemand updates the seasonality model for a function.
func (pw *PreWarmer) RecordDemand(funcID string, concurrent int) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	hour := time.Now().Hour()
	slots := pw.seasonality[funcID]
	slot := &slots[hour]
	slot.Hour = hour
	slot.SampleCount++
	// Exponential moving average
	alpha := 0.1
	slot.AvgDemand = slot.AvgDemand*(1-alpha) + float64(concurrent)*alpha
	if float64(concurrent) > slot.PeakDemand {
		slot.PeakDemand = float64(concurrent)
	}
	pw.seasonality[funcID] = slots
}

// SetCurrentWarm updates the current warm VM count for a function.
func (pw *PreWarmer) SetCurrentWarm(funcID string, count int) {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	pw.currentWarm[funcID] = count
}

// PredictDemand returns the predicted demand for a function in the next window.
func (pw *PreWarmer) PredictDemand(funcID string) int {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	return pw.predictDemandLocked(funcID)
}

// predictDemandLocked is the lock-free implementation of PredictDemand.
// Caller must hold pw.mu.
func (pw *PreWarmer) predictDemandLocked(funcID string) int {
	slots, ok := pw.seasonality[funcID]
	if !ok {
		return 0
	}

	predictedHour := time.Now().Add(pw.cfg.PredictWindow).Hour()
	slot := slots[predictedHour]

	if slot.SampleCount < 3 {
		return 0 // Not enough data
	}

	// Use average + 50% of peak-average gap for safety
	predicted := slot.AvgDemand + (slot.PeakDemand-slot.AvgDemand)*0.5
	return int(predicted) + 1 // Round up
}

// PreWarmTargets returns functions that need pre-warming with their target counts.
func (pw *PreWarmer) PreWarmTargets() map[string]int {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	targets := make(map[string]int)
	for funcID := range pw.seasonality {
		predicted := pw.predictDemandLocked(funcID)
		current := pw.currentWarm[funcID]
		delta := predicted - current
		if delta > 0 {
			if delta > pw.cfg.MaxPreWarm {
				delta = pw.cfg.MaxPreWarm
			}
			targets[funcID] = delta
		}
	}
	return targets
}

func (pw *PreWarmer) loop() {
	ticker := time.NewTicker(pw.cfg.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-pw.ctx.Done():
			return
		case <-ticker.C:
			targets := pw.PreWarmTargets()
			for funcID, count := range targets {
				logging.Op().Info("pre-warming snapshots",
					"function", funcID,
					"count", count)
				_ = funcID
				_ = count
				// In production: trigger snapshot restores via pool
			}
		}
	}
}

// SnapshotLayerResolver helps the pool decide whether to restore from
// a function snapshot (L2) or fall back to runtime snapshot (L1).
type SnapshotLayerResolver struct {
	manager *Manager
}

// NewSnapshotLayerResolver creates a resolver.
func NewSnapshotLayerResolver(manager *Manager) *SnapshotLayerResolver {
	return &SnapshotLayerResolver{manager: manager}
}

// ResolveForFunction returns the best snapshot to restore from.
func (r *SnapshotLayerResolver) ResolveForFunction(fn *domain.Function) (*SnapshotEntry, Layer, bool) {
	return r.manager.Resolve(fn)
}
