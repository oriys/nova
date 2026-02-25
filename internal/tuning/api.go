package tuning

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TuningResponse is the API response for GET /functions/{name}/tuning.
type TuningResponse struct {
	FunctionID      string           `json:"function_id"`
	Signals         *FunctionSignals `json:"signals"`
	Recommendations []Recommendation `json:"recommendations"`
	Experiments     []*Experiment    `json:"active_experiments,omitempty"`
	GeneratedAt     time.Time        `json:"generated_at"`
}

// TuningHistory records applied tuning changes.
type TuningHistory struct {
	ID            string          `json:"id"`
	FunctionID    string          `json:"function_id"`
	Parameter     TuningParameter `json:"parameter"`
	OldValue      string          `json:"old_value"`
	NewValue      string          `json:"new_value"`
	Source        string          `json:"source"` // "manual", "auto-promote", "advisor"
	AppliedAt     time.Time       `json:"applied_at"`
	MetricsBefore json.RawMessage `json:"metrics_before,omitempty"`
	MetricsAfter  json.RawMessage `json:"metrics_after,omitempty"`
}

// TuningService orchestrates the full tuning workflow.
type TuningService struct {
	aggregator *SignalAggregator
	advisor    *Advisor
	canary     *CanaryEngine
	history    []TuningHistory
}

// NewTuningService creates a new tuning service.
func NewTuningService(aggregator *SignalAggregator, advisor *Advisor, canary *CanaryEngine) *TuningService {
	return &TuningService{
		aggregator: aggregator,
		advisor:    advisor,
		canary:     canary,
	}
}

// GetTuning returns the full tuning report for a function.
func (ts *TuningService) GetTuning(funcID string) *TuningResponse {
	return &TuningResponse{
		FunctionID:      funcID,
		Signals:         ts.aggregator.GetSignals(funcID),
		Recommendations: ts.advisor.Analyze(funcID),
		Experiments:     ts.canary.ListExperiments(funcID),
		GeneratedAt:     time.Now(),
	}
}

// ApplyRecommendation creates a canary experiment from a recommendation.
func (ts *TuningService) ApplyRecommendation(funcID string, recIndex int) (*Experiment, error) {
	recs := ts.advisor.Analyze(funcID)
	if recIndex < 0 || recIndex >= len(recs) {
		return nil, fmt.Errorf("recommendation index %d out of range", recIndex)
	}

	exp, err := ts.canary.CreateExperiment(recs[recIndex], funcID)
	if err != nil {
		return nil, err
	}

	return exp, ts.canary.StartExperiment(exp.ID)
}

// GetHistory returns tuning history for a function.
func (ts *TuningService) GetHistory(funcID string) []TuningHistory {
	var result []TuningHistory
	for _, h := range ts.history {
		if funcID == "" || h.FunctionID == funcID {
			result = append(result, h)
		}
	}
	return result
}

// ExportPrometheus returns tuning metrics in Prometheus format.
func (ts *TuningService) ExportPrometheus() string {
	var b strings.Builder

	b.WriteString("# HELP nova_tuning_recommendations_total Tuning recommendations generated\n")
	b.WriteString("# TYPE nova_tuning_recommendations_total gauge\n")

	allRecs := ts.advisor.AnalyzeAll()
	for funcID, recs := range allRecs {
		for _, rec := range recs {
			fmt.Fprintf(&b, "nova_tuning_recommendation{function=%q,parameter=%q,confidence=%q} 1\n",
				funcID, rec.Parameter, rec.Confidence)
		}
	}

	b.WriteString("# HELP nova_tuning_experiments_active Active canary experiments\n")
	b.WriteString("# TYPE nova_tuning_experiments_active gauge\n")
	active := ts.canary.ActiveExperiments()
	fmt.Fprintf(&b, "nova_tuning_experiments_active %d\n", len(active))

	b.WriteString("# HELP nova_tuning_history_total Total tuning changes applied\n")
	b.WriteString("# TYPE nova_tuning_history_total counter\n")
	fmt.Fprintf(&b, "nova_tuning_history_total %d\n", len(ts.history))

	return b.String()
}
