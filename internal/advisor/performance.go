package advisor

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/oriys/nova/internal/ai"
	"github.com/oriys/nova/internal/store"
)

// PerformanceAdvisor provides AI-powered performance optimization recommendations.
type PerformanceAdvisor struct {
	Store     *store.Store
	AIService *ai.Service
}

// RecommendationRequest contains data for generating performance recommendations.
type RecommendationRequest struct {
	FunctionID      string
	FunctionName    string
	CurrentMemoryMB int
	CurrentTimeoutS int
	MinReplicas     int
	MaxReplicas     int
	LookbackDays    int // How many days to analyze (default: 7)
}

// RecommendationResponse contains performance optimization recommendations.
type RecommendationResponse struct {
	Recommendations  []Recommendation `json:"recommendations"`
	Confidence       float64          `json:"confidence"` // 0-1
	EstimatedSavings string           `json:"estimated_savings,omitempty"`
	AnalysisSummary  string           `json:"analysis_summary"`
}

// Recommendation represents a single actionable recommendation.
type Recommendation struct {
	Category         string            `json:"category"`
	Priority         string            `json:"priority"`
	CurrentValue     interface{}       `json:"current_value"`
	RecommendedValue interface{}       `json:"recommended_value"`
	Reasoning        string            `json:"reasoning"`
	ExpectedImpact   string            `json:"expected_impact"`
	Metrics          map[string]string `json:"metrics,omitempty"`
}

// TrafficPrediction represents advisor-estimated near-term traffic for a function.
type TrafficPrediction struct {
	FunctionID          string    `json:"function_id"`
	CurrentRatePerSec   float64   `json:"current_rate_per_sec"`
	PredictedRatePerSec float64   `json:"predicted_rate_per_sec"`
	Confidence          float64   `json:"confidence"`
	LookbackDays        int       `json:"lookback_days"`
	PredictedAt         time.Time `json:"predicted_at"`
}

// AnalyzePerformance analyzes function performance and provides recommendations.
func (p *PerformanceAdvisor) AnalyzePerformance(ctx context.Context, req RecommendationRequest) (*RecommendationResponse, error) {
	return p.basicAnalysis(ctx, req)
}

// PredictTraffic estimates near-term request rate for proactive autoscaling.
func (p *PerformanceAdvisor) PredictTraffic(ctx context.Context, functionID string, lookbackDays int) (*TrafficPrediction, error) {
	if p.Store == nil {
		return nil, fmt.Errorf("advisor store is not configured")
	}
	if functionID == "" {
		return nil, fmt.Errorf("function id is required")
	}

	if lookbackDays <= 0 {
		lookbackDays = 7
	}

	logs, err := p.Store.ListInvocationLogs(ctx, functionID, 10000, 0)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	cutoff := now.Add(-time.Duration(lookbackDays) * 24 * time.Hour)
	currentWindow := now.Add(-1 * time.Minute)
	recentWindow := now.Add(-5 * time.Minute)

	// Seasonality window: +/-2 minutes around current minute for each day.
	seasonalStartMinute := now.Minute() - 2
	seasonalEndMinute := now.Minute() + 2

	var (
		totalSamples  int
		currentCount  int
		recentCount   int
		seasonalCount int
	)

	for _, log := range logs {
		if log.CreatedAt.Before(cutoff) {
			continue
		}

		totalSamples++
		if log.CreatedAt.After(currentWindow) {
			currentCount++
		}
		if log.CreatedAt.After(recentWindow) {
			recentCount++
		}

		if log.CreatedAt.Hour() != now.Hour() {
			continue
		}
		minute := log.CreatedAt.Minute()
		if minute >= seasonalStartMinute && minute <= seasonalEndMinute {
			seasonalCount++
		}
	}

	currentRate := float64(currentCount) / 60.0
	recentRate := float64(recentCount) / (5.0 * 60.0)
	seasonalRate := float64(seasonalCount) / float64(maxInt(lookbackDays, 1)*5*60)

	predictedRate := recentRate
	if seasonalRate > predictedRate {
		predictedRate = seasonalRate
	}
	if trend := recentRate - currentRate; trend > 0 {
		predictedRate = math.Max(predictedRate, recentRate+trend*0.5)
	}
	if currentRate > predictedRate {
		predictedRate = currentRate
	}
	if predictedRate < 0 {
		predictedRate = 0
	}

	sampleConfidence := math.Min(1, float64(totalSamples)/500.0)
	burstSignal := 0.0
	if currentRate > 0 {
		burstSignal = math.Min(1, math.Abs(recentRate-currentRate)/currentRate)
	}
	confidence := 0.35 + sampleConfidence*0.45 + burstSignal*0.2
	if confidence > 0.95 {
		confidence = 0.95
	}
	if totalSamples == 0 {
		confidence = 0.2
	}

	return &TrafficPrediction{
		FunctionID:          functionID,
		CurrentRatePerSec:   currentRate,
		PredictedRatePerSec: predictedRate,
		Confidence:          confidence,
		LookbackDays:        lookbackDays,
		PredictedAt:         now,
	}, nil
}

// basicAnalysis provides rule-based recommendations.
func (p *PerformanceAdvisor) basicAnalysis(ctx context.Context, req RecommendationRequest) (*RecommendationResponse, error) {
	recommendations := []Recommendation{}

	// Gather historical data for analysis
	lookback := req.LookbackDays
	if lookback <= 0 {
		lookback = 7
	}
	cutoff := time.Now().Add(-time.Duration(lookback) * 24 * time.Hour)

	logs, err := p.Store.ListInvocationLogs(ctx, req.FunctionID, 5000, 0)
	if err == nil && len(logs) > 0 {
		// Filter to lookback window
		filtered := make([]*store.InvocationLog, 0)
		for _, log := range logs {
			if log.CreatedAt.After(cutoff) {
				filtered = append(filtered, log)
			}
		}

		if len(filtered) >= 10 {
			// Calculate metrics
			var totalDuration int64
			coldStarts := 0
			errors := 0
			timeouts := 0

			for _, log := range filtered {
				totalDuration += log.DurationMs
				if log.ColdStart {
					coldStarts++
				}
				if !log.Success {
					errors++
					// Check if it's a timeout
					timeoutThreshold := int64(req.CurrentTimeoutS * 1000 * 95 / 100)
					if log.DurationMs >= timeoutThreshold {
						timeouts++
					}
				}
			}

			avgDuration := float64(totalDuration) / float64(len(filtered))
			coldStartRate := float64(coldStarts) / float64(len(filtered)) * 100
			errorRate := float64(errors) / float64(len(filtered)) * 100

			// Rule: High cold start rate
			if coldStartRate > 20 && req.MinReplicas == 0 {
				recommendations = append(recommendations, Recommendation{
					Category:         "scaling",
					Priority:         "high",
					CurrentValue:     0,
					RecommendedValue: 1,
					Reasoning:        fmt.Sprintf("Cold start rate is %.1f%%. Setting min_replicas=1 will keep at least one instance warm", coldStartRate),
					ExpectedImpact:   "Reduce cold start rate to near 0% for consistent traffic",
					Metrics: map[string]string{
						"current_cold_start_rate": fmt.Sprintf("%.1f%%", coldStartRate),
					},
				})
			}

			// Rule: High average latency
			if avgDuration > 1000 && req.CurrentMemoryMB <= 256 {
				recommendations = append(recommendations, Recommendation{
					Category:         "memory",
					Priority:         "medium",
					CurrentValue:     req.CurrentMemoryMB,
					RecommendedValue: req.CurrentMemoryMB * 2,
					Reasoning:        fmt.Sprintf("Average latency is %.1fms. Increasing memory may improve performance", avgDuration),
					ExpectedImpact:   "Potentially reduce latency by 20-40%",
					Metrics: map[string]string{
						"current_avg_latency": fmt.Sprintf("%.1fms", avgDuration),
					},
				})
			}

			// Rule: Timeout issues
			if timeouts > 0 && errors > 0 {
				recommendations = append(recommendations, Recommendation{
					Category:         "timeout",
					Priority:         "critical",
					CurrentValue:     req.CurrentTimeoutS,
					RecommendedValue: req.CurrentTimeoutS * 2,
					Reasoning:        fmt.Sprintf("Detected %d timeout errors (%.1f%% of errors)", timeouts, float64(timeouts)/float64(errors)*100),
					ExpectedImpact:   "Eliminate timeout-related failures",
					Metrics: map[string]string{
						"timeout_count": fmt.Sprintf("%d", timeouts),
						"error_rate":    fmt.Sprintf("%.1f%%", errorRate),
					},
				})
			}
		}
	}

	// Fallback rules if no data-driven recommendations
	if len(recommendations) == 0 {
		if req.CurrentTimeoutS < 5 {
			recommendations = append(recommendations, Recommendation{
				Category:         "timeout",
				Priority:         "medium",
				CurrentValue:     req.CurrentTimeoutS,
				RecommendedValue: 30,
				Reasoning:        "Timeout is very low and may cause premature failures",
				ExpectedImpact:   "Reduce timeout-related errors",
			})
		}

		if req.CurrentMemoryMB <= 128 {
			recommendations = append(recommendations, Recommendation{
				Category:         "memory",
				Priority:         "low",
				CurrentValue:     req.CurrentMemoryMB,
				RecommendedValue: 256,
				Reasoning:        "Consider increasing memory if you observe OOM errors or high latency",
				ExpectedImpact:   "Potentially reduce latency by 20-40% if memory-bound",
			})
		}
	}

	summary := fmt.Sprintf("Analyzed function '%s' with rule-based advisor. ", req.FunctionName)
	if len(recommendations) > 0 {
		summary += fmt.Sprintf("Found %d recommendations.", len(recommendations))
	} else {
		summary += "No immediate optimizations needed."
	}

	return &RecommendationResponse{
		Recommendations:  recommendations,
		Confidence:       0.7,
		AnalysisSummary:  summary,
		EstimatedSavings: "Enable AI service for advanced cost estimation",
	}, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
