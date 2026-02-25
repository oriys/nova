package tuning

import (
	"fmt"
	"time"
)

// TuningParameter identifies a tunable configuration parameter.
type TuningParameter string

const (
	ParamMinReplicas     TuningParameter = "min_replicas"
	ParamMaxReplicas     TuningParameter = "max_replicas"
	ParamConcurrency     TuningParameter = "concurrency"
	ParamIdleTTL         TuningParameter = "idle_ttl"
	ParamPreWarmWindow   TuningParameter = "prewarm_window"
	ParamCompileOptLevel TuningParameter = "compile_opt_level"
	ParamSnapshotEnabled TuningParameter = "snapshot_enabled"
	ParamTimeout         TuningParameter = "timeout"
)

// Confidence represents how confident the advisor is in a recommendation.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// Recommendation is a single tuning suggestion.
type Recommendation struct {
	Parameter  TuningParameter `json:"parameter"`
	Current    string          `json:"current"`
	Suggested  string          `json:"suggested"`
	Reason     string          `json:"reason"`
	Confidence Confidence      `json:"confidence"`
	Impact     string          `json:"impact"` // Expected improvement description
}

// AdvisorConfig configures the recommendation engine thresholds.
type AdvisorConfig struct {
	HighColdRatio      float64       // Cold ratio above this triggers recommendation (default: 0.3)
	HighP99Threshold   time.Duration // P99 above this triggers latency advice (default: 1s)
	LowUtilization     float64       // VM busy ratio below this suggests scaling down (default: 0.1)
	HighErrorRate      float64       // Error rate above this triggers investigation (default: 0.05)
	SlowCompileTime    time.Duration // Compile time above this suggests optimization (default: 30s)
	LowSnapshotHitRate float64       // Hit rate below this suggests enabling snapshots (default: 0.5)
}

// DefaultAdvisorConfig returns default thresholds.
func DefaultAdvisorConfig() AdvisorConfig {
	return AdvisorConfig{
		HighColdRatio:      0.3,
		HighP99Threshold:   time.Second,
		LowUtilization:     0.1,
		HighErrorRate:      0.05,
		SlowCompileTime:    30 * time.Second,
		LowSnapshotHitRate: 0.5,
	}
}

// Advisor analyzes function signals and produces tuning recommendations.
type Advisor struct {
	cfg        AdvisorConfig
	aggregator *SignalAggregator
}

// NewAdvisor creates a new recommendation advisor.
func NewAdvisor(cfg AdvisorConfig, aggregator *SignalAggregator) *Advisor {
	return &Advisor{cfg: cfg, aggregator: aggregator}
}

// Analyze produces recommendations for a specific function.
func (a *Advisor) Analyze(funcID string) []Recommendation {
	signals := a.aggregator.GetSignals(funcID)
	if signals == nil {
		return nil
	}

	var recs []Recommendation

	// Rule 1: High cold-start ratio → increase min_replicas or prewarm
	if signals.ColdRatio > a.cfg.HighColdRatio && (signals.ColdStarts+signals.WarmStarts) > 10 {
		recs = append(recs, Recommendation{
			Parameter:  ParamMinReplicas,
			Current:    "auto",
			Suggested:  fmt.Sprintf("%d", max(1, int(signals.RequestRate)+1)),
			Reason:     fmt.Sprintf("Cold-start ratio %.1f%% exceeds threshold %.1f%%", signals.ColdRatio*100, a.cfg.HighColdRatio*100),
			Confidence: ConfidenceHigh,
			Impact:     "Reduce cold starts, improve P99 latency",
		})
		recs = append(recs, Recommendation{
			Parameter:  ParamPreWarmWindow,
			Current:    "disabled",
			Suggested:  "5m",
			Reason:     "High cold-start ratio suggests pre-warming would help",
			Confidence: ConfidenceMedium,
			Impact:     "Pre-restore snapshots before predicted demand spikes",
		})
	}

	// Rule 2: High P99 but low P50 → add concurrency
	if signals.P99Latency > a.cfg.HighP99Threshold && signals.P50Latency < signals.P99Latency/5 {
		recs = append(recs, Recommendation{
			Parameter:  ParamConcurrency,
			Current:    "auto",
			Suggested:  "increase by 2x",
			Reason:     fmt.Sprintf("P99 (%s) is >5x P50 (%s), indicating queuing delays", signals.P99Latency, signals.P50Latency),
			Confidence: ConfidenceHigh,
			Impact:     "Reduce tail latency by parallelizing execution",
		})
	}

	// Rule 3: Low utilization → decrease max_replicas or increase idle TTL
	if signals.VMBusyRatio < a.cfg.LowUtilization && signals.VMCount > 1 {
		recs = append(recs, Recommendation{
			Parameter:  ParamMaxReplicas,
			Current:    fmt.Sprintf("%d", signals.VMCount),
			Suggested:  fmt.Sprintf("%d", max(1, signals.VMCount/2)),
			Reason:     fmt.Sprintf("VM utilization %.1f%% is very low", signals.VMBusyRatio*100),
			Confidence: ConfidenceMedium,
			Impact:     "Reduce resource waste while maintaining capacity",
		})
		recs = append(recs, Recommendation{
			Parameter:  ParamIdleTTL,
			Current:    "60s",
			Suggested:  "120s",
			Reason:     "Low utilization with multiple VMs; longer TTL may reduce cold starts",
			Confidence: ConfidenceLow,
			Impact:     "Trade memory for fewer cold starts during bursty traffic",
		})
	}

	// Rule 4: High error rate → investigate timeout
	if signals.ErrorRate > a.cfg.HighErrorRate && (signals.ColdStarts+signals.WarmStarts) > 10 {
		recs = append(recs, Recommendation{
			Parameter:  ParamTimeout,
			Current:    "auto",
			Suggested:  "increase by 50%",
			Reason:     fmt.Sprintf("Error rate %.1f%% exceeds threshold %.1f%%", signals.ErrorRate*100, a.cfg.HighErrorRate*100),
			Confidence: ConfidenceLow,
			Impact:     "May reduce timeout-related errors if functions need more time",
		})
	}

	// Rule 5: Slow compilation → suggest optimization level change
	if signals.CompileTime > a.cfg.SlowCompileTime && signals.CompileCount > 0 {
		recs = append(recs, Recommendation{
			Parameter:  ParamCompileOptLevel,
			Current:    "default",
			Suggested:  "O1 (reduced optimization)",
			Reason:     fmt.Sprintf("Average compile time %s exceeds threshold", signals.CompileTime),
			Confidence: ConfidenceMedium,
			Impact:     "Faster compilation at slight runtime performance cost",
		})
	}

	// Rule 6: Low snapshot hit rate → enable/fix snapshots
	if signals.SnapshotHitRate < a.cfg.LowSnapshotHitRate && (signals.SnapshotHits+signals.SnapshotMisses) > 5 {
		recs = append(recs, Recommendation{
			Parameter:  ParamSnapshotEnabled,
			Current:    "partial",
			Suggested:  "full (L0+L1+L2)",
			Reason:     fmt.Sprintf("Snapshot hit rate %.1f%% is below threshold", signals.SnapshotHitRate*100),
			Confidence: ConfidenceHigh,
			Impact:     "Enable all snapshot layers for sub-10ms cold starts",
		})
	}

	return recs
}

// AnalyzeAll produces recommendations for all tracked functions.
func (a *Advisor) AnalyzeAll() map[string][]Recommendation {
	funcIDs := a.aggregator.ListFunctions()
	results := make(map[string][]Recommendation)

	for _, funcID := range funcIDs {
		recs := a.Analyze(funcID)
		if len(recs) > 0 {
			results[funcID] = recs
		}
	}
	return results
}
