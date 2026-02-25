package tuning

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oriys/nova/internal/logging"
)

// RollbackConfig configures auto-rollback behavior.
type RollbackConfig struct {
	CheckInterval        time.Duration // How often to check experiments (default: 30s)
	RegressionWindow     time.Duration // Window to detect regression (default: 5m)
	RegressionP99Delta   float64       // P99 increase ratio that triggers rollback (default: 0.10)
	RegressionErrorDelta float64       // Error rate increase that triggers rollback (default: 0.02)
	MaxRollbacksPerHour  int           // Rate limit on rollbacks (default: 5)
}

// DefaultRollbackConfig returns sensible defaults.
func DefaultRollbackConfig() RollbackConfig {
	return RollbackConfig{
		CheckInterval:        30 * time.Second,
		RegressionWindow:     5 * time.Minute,
		RegressionP99Delta:   0.10,
		RegressionErrorDelta: 0.02,
		MaxRollbacksPerHour:  5,
	}
}

// RollbackEvent records a rollback action.
type RollbackEvent struct {
	ExperimentID string    `json:"experiment_id"`
	FunctionID   string    `json:"function_id"`
	Reason       string    `json:"reason"`
	RolledBackAt time.Time `json:"rolled_back_at"`
}

// RollbackMonitor watches experiments and triggers auto-rollback on regression.
type RollbackMonitor struct {
	mu            sync.Mutex
	canary        *CanaryEngine
	aggregator    *SignalAggregator
	cfg           RollbackConfig
	events        []RollbackEvent
	rollbackCount int
	lastResetHour time.Time
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewRollbackMonitor creates a new rollback monitor.
func NewRollbackMonitor(canary *CanaryEngine, aggregator *SignalAggregator, cfg RollbackConfig) *RollbackMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &RollbackMonitor{
		canary:        canary,
		aggregator:    aggregator,
		cfg:           cfg,
		lastResetHour: time.Now(),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start begins monitoring experiments.
func (rm *RollbackMonitor) Start() {
	go rm.monitorLoop()
}

// Stop halts the monitor.
func (rm *RollbackMonitor) Stop() {
	rm.cancel()
}

// Events returns all rollback events.
func (rm *RollbackMonitor) Events() []RollbackEvent {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	result := make([]RollbackEvent, len(rm.events))
	copy(result, rm.events)
	return result
}

func (rm *RollbackMonitor) monitorLoop() {
	ticker := time.NewTicker(rm.cfg.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-ticker.C:
			rm.checkExperiments()
		}
	}
}

func (rm *RollbackMonitor) checkExperiments() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Reset hourly rate limit
	if time.Since(rm.lastResetHour) > time.Hour {
		rm.rollbackCount = 0
		rm.lastResetHour = time.Now()
	}

	active := rm.canary.ActiveExperiments()
	for _, exp := range active {
		if rm.rollbackCount >= rm.cfg.MaxRollbacksPerHour {
			logging.Op().Warn("rollback rate limit reached", "limit", rm.cfg.MaxRollbacksPerHour)
			return
		}

		signals := rm.aggregator.GetSignals(exp.FunctionID)
		if signals == nil {
			continue
		}

		reason := rm.detectRegression(exp, signals)
		if reason != "" {
			rm.canary.Rollback(exp.ID)
			event := RollbackEvent{
				ExperimentID: exp.ID,
				FunctionID:   exp.FunctionID,
				Reason:       reason,
				RolledBackAt: time.Now(),
			}
			rm.events = append(rm.events, event)
			rm.rollbackCount++

			logging.Op().Warn("auto-rollback triggered",
				"experiment", exp.ID,
				"function", exp.FunctionID,
				"reason", reason)
		}
	}
}

func (rm *RollbackMonitor) detectRegression(exp *Experiment, signals *FunctionSignals) string {
	// Check P99 regression
	if exp.ControlMetrics != nil && exp.ControlMetrics.P99Latency > 0 {
		if signals.P99Latency > 0 {
			delta := float64(signals.P99Latency-exp.ControlMetrics.P99Latency) / float64(exp.ControlMetrics.P99Latency)
			if delta > rm.cfg.RegressionP99Delta {
				return "P99 latency regression: " + signals.P99Latency.String() + " vs baseline " + exp.ControlMetrics.P99Latency.String()
			}
		}
	}

	// Check error rate regression
	if signals.ErrorRate > rm.cfg.RegressionErrorDelta {
		return "error rate regression: " + formatPercent(signals.ErrorRate)
	}

	return ""
}

func formatPercent(f float64) string {
	return fmt.Sprintf("%.1f%%", f*100)
}
