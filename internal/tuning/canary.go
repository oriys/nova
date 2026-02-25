package tuning

import (
	"fmt"
	"sync"
	"time"
)

// ExperimentStatus represents the lifecycle of a canary experiment.
type ExperimentStatus string

const (
	ExperimentPending    ExperimentStatus = "pending"
	ExperimentRunning    ExperimentStatus = "running"
	ExperimentPromoted   ExperimentStatus = "promoted"
	ExperimentRolledBack ExperimentStatus = "rolled_back"
	ExperimentFailed     ExperimentStatus = "failed"
)

// Experiment represents a canary tuning experiment.
type Experiment struct {
	ID              string           `json:"id"`
	FunctionID      string           `json:"function_id"`
	Parameter       TuningParameter  `json:"parameter"`
	ControlValue    string           `json:"control_value"`
	ExperimentValue string           `json:"experiment_value"`
	TrafficPercent  float64          `json:"traffic_percent"` // 0.0-1.0
	Status          ExperimentStatus `json:"status"`
	StartedAt       time.Time        `json:"started_at"`
	CompletedAt     time.Time        `json:"completed_at,omitempty"`
	MinDuration     time.Duration    `json:"min_duration"`
	MaxDuration     time.Duration    `json:"max_duration"`

	// Results
	ControlMetrics    *FunctionSignals `json:"control_metrics,omitempty"`
	ExperimentMetrics *FunctionSignals `json:"experiment_metrics,omitempty"`
	Verdict           string           `json:"verdict,omitempty"`
}

// CanaryConfig configures the canary experiment engine.
type CanaryConfig struct {
	DefaultTrafficPercent float64       // Default % of traffic for experiments (default: 0.1)
	MinExperimentDuration time.Duration // Minimum time before auto-promote (default: 10m)
	MaxExperimentDuration time.Duration // Maximum experiment time (default: 1h)
	ImprovementThreshold  float64       // Required improvement ratio to promote (default: 0.1 = 10%)
	RegressionThreshold   float64       // Regression ratio that triggers rollback (default: 0.05 = 5%)
	AutoPromote           bool          // Automatically promote successful experiments
	AutoRollback          bool          // Automatically rollback regressions
}

// DefaultCanaryConfig returns sensible defaults.
func DefaultCanaryConfig() CanaryConfig {
	return CanaryConfig{
		DefaultTrafficPercent: 0.1,
		MinExperimentDuration: 10 * time.Minute,
		MaxExperimentDuration: time.Hour,
		ImprovementThreshold:  0.1,
		RegressionThreshold:   0.05,
		AutoPromote:           false,
		AutoRollback:          true,
	}
}

// CanaryEngine manages canary tuning experiments.
type CanaryEngine struct {
	mu          sync.RWMutex
	experiments map[string]*Experiment
	cfg         CanaryConfig
	aggregator  *SignalAggregator
}

// NewCanaryEngine creates a new canary experiment engine.
func NewCanaryEngine(cfg CanaryConfig, aggregator *SignalAggregator) *CanaryEngine {
	return &CanaryEngine{
		experiments: make(map[string]*Experiment),
		cfg:         cfg,
		aggregator:  aggregator,
	}
}

// CreateExperiment creates a new canary experiment from a recommendation.
func (ce *CanaryEngine) CreateExperiment(rec Recommendation, funcID string) (*Experiment, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	// Check for conflicting experiments
	for _, exp := range ce.experiments {
		if exp.FunctionID == funcID && exp.Parameter == rec.Parameter && exp.Status == ExperimentRunning {
			return nil, fmt.Errorf("experiment already running for %s/%s", funcID, rec.Parameter)
		}
	}

	id := fmt.Sprintf("exp-%s-%s-%d", funcID, rec.Parameter, time.Now().UnixMilli())
	exp := &Experiment{
		ID:              id,
		FunctionID:      funcID,
		Parameter:       rec.Parameter,
		ControlValue:    rec.Current,
		ExperimentValue: rec.Suggested,
		TrafficPercent:  ce.cfg.DefaultTrafficPercent,
		Status:          ExperimentPending,
		MinDuration:     ce.cfg.MinExperimentDuration,
		MaxDuration:     ce.cfg.MaxExperimentDuration,
	}
	ce.experiments[id] = exp
	return exp, nil
}

// StartExperiment begins routing experiment traffic.
func (ce *CanaryEngine) StartExperiment(id string) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	exp, ok := ce.experiments[id]
	if !ok {
		return fmt.Errorf("experiment %s not found", id)
	}
	if exp.Status != ExperimentPending {
		return fmt.Errorf("experiment %s is %s, not pending", id, exp.Status)
	}

	exp.Status = ExperimentRunning
	exp.StartedAt = time.Now()
	return nil
}

// Evaluate checks experiment results and returns verdict.
func (ce *CanaryEngine) Evaluate(id string) (*Experiment, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	exp, ok := ce.experiments[id]
	if !ok {
		return nil, fmt.Errorf("experiment %s not found", id)
	}
	if exp.Status != ExperimentRunning {
		return exp, nil
	}

	elapsed := time.Since(exp.StartedAt)

	// Collect metrics for both groups
	controlSignals := ce.aggregator.GetSignals(exp.FunctionID)
	exp.ControlMetrics = controlSignals
	exp.ExperimentMetrics = controlSignals // In production, separate metric collection per group

	// Check if experiment has run long enough
	if elapsed < exp.MinDuration {
		exp.Verdict = fmt.Sprintf("waiting: %s remaining", (exp.MinDuration - elapsed).Round(time.Second))
		return exp, nil
	}

	// Compare P99 latency
	if controlSignals != nil && controlSignals.P99Latency > 0 {
		improvement := 1.0 - float64(controlSignals.P99Latency)/float64(controlSignals.P99Latency)

		if improvement > ce.cfg.ImprovementThreshold {
			exp.Verdict = fmt.Sprintf("improvement: %.1f%% P99 reduction", improvement*100)
			if ce.cfg.AutoPromote {
				exp.Status = ExperimentPromoted
				exp.CompletedAt = time.Now()
			}
		} else if improvement < -ce.cfg.RegressionThreshold {
			exp.Verdict = fmt.Sprintf("regression: %.1f%% P99 increase", -improvement*100)
			if ce.cfg.AutoRollback {
				exp.Status = ExperimentRolledBack
				exp.CompletedAt = time.Now()
			}
		} else {
			exp.Verdict = "neutral: no significant difference"
		}
	}

	// Auto-expire
	if elapsed > exp.MaxDuration && exp.Status == ExperimentRunning {
		exp.Status = ExperimentRolledBack
		exp.CompletedAt = time.Now()
		exp.Verdict = "expired: max duration reached without clear improvement"
	}

	return exp, nil
}

// Promote applies the experiment value as the new default.
func (ce *CanaryEngine) Promote(id string) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	exp, ok := ce.experiments[id]
	if !ok {
		return fmt.Errorf("experiment %s not found", id)
	}
	exp.Status = ExperimentPromoted
	exp.CompletedAt = time.Now()
	return nil
}

// Rollback reverts to the control value.
func (ce *CanaryEngine) Rollback(id string) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	exp, ok := ce.experiments[id]
	if !ok {
		return fmt.Errorf("experiment %s not found", id)
	}
	exp.Status = ExperimentRolledBack
	exp.CompletedAt = time.Now()
	return nil
}

// GetExperiment returns an experiment by ID.
func (ce *CanaryEngine) GetExperiment(id string) (*Experiment, bool) {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	exp, ok := ce.experiments[id]
	return exp, ok
}

// ListExperiments returns all experiments, optionally filtered by function.
func (ce *CanaryEngine) ListExperiments(funcID string) []*Experiment {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	var result []*Experiment
	for _, exp := range ce.experiments {
		if funcID == "" || exp.FunctionID == funcID {
			result = append(result, exp)
		}
	}
	return result
}

// ActiveExperiments returns only running experiments.
func (ce *CanaryEngine) ActiveExperiments() []*Experiment {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	var result []*Experiment
	for _, exp := range ce.experiments {
		if exp.Status == ExperimentRunning {
			result = append(result, exp)
		}
	}
	return result
}

// ShouldRouteToExperiment decides if a request should use experimental config.
// Uses a deterministic hash of request_id for consistent routing.
func (ce *CanaryEngine) ShouldRouteToExperiment(funcID, requestID string) (bool, *Experiment) {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	for _, exp := range ce.experiments {
		if exp.FunctionID == funcID && exp.Status == ExperimentRunning {
			// Simple hash-based routing
			hash := fnvHash(requestID)
			if float64(hash%100)/100.0 < exp.TrafficPercent {
				return true, exp
			}
		}
	}
	return false, nil
}

func fnvHash(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}
