package dataplane

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/oriys/nova/internal/advisor"
	"github.com/oriys/nova/internal/cost"
	"github.com/oriys/nova/internal/store"
)

// GetPerformanceRecommendations handles GET /functions/{name}/recommendations
func (h *Handler) GetPerformanceRecommendations(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Parse lookback days parameter
	lookbackDays := 7
	if days := r.URL.Query().Get("days"); days != "" {
		if d, err := strconv.Atoi(days); err == nil && d > 0 && d <= 90 {
			lookbackDays = d
		}
	}

	// Create advisor if needed
	if h.Advisor == nil {
		adv := &advisor.PerformanceAdvisor{
			Store:     h.Store,
			AIService: h.AIService,
		}
		h.Advisor = adv
	}

	// Type assert to get the advisor
	adv, ok := h.Advisor.(*advisor.PerformanceAdvisor)
	if !ok {
		http.Error(w, "performance advisor not available", http.StatusServiceUnavailable)
		return
	}

	// Prepare request
	req := advisor.RecommendationRequest{
		FunctionID:      fn.ID,
		FunctionName:    fn.Name,
		CurrentMemoryMB: fn.MemoryMB,
		CurrentTimeoutS: fn.TimeoutS,
		MinReplicas:     fn.MinReplicas,
		MaxReplicas:     fn.MaxReplicas,
		LookbackDays:    lookbackDays,
	}

	// Get recommendations
	resp, err := adv.AnalyzePerformance(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// FunctionCost handles GET /functions/{name}/cost
func (h *Handler) FunctionCost(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	windowStr := r.URL.Query().Get("window")
	windowSeconds := defaultDiagnosticsWindowSeconds
	if windowStr != "" {
		if v, err := strconv.Atoi(windowStr); err == nil && v > 0 {
			windowSeconds = v
		}
	}

	logs, err := h.Store.ListInvocationLogs(r.Context(), fn.ID, 10000, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cutoff := time.Now().Add(-time.Duration(windowSeconds) * time.Second)
	var totalDurationMs int64
	var invocations int64
	var coldStarts int64
	for _, log := range logs {
		if log.CreatedAt.Before(cutoff) {
			continue
		}
		invocations++
		totalDurationMs += log.DurationMs
		if log.ColdStart {
			coldStarts++
		}
	}

	calc := cost.NewDefaultCalculator()
	summary := cost.AggregateFunctionCost(
		fn.ID, fn.Name,
		invocations, totalDurationMs, coldStarts,
		fn.MemoryMB, calc.GetPricing(),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// CostSummary handles GET /cost/summary
func (h *Handler) CostSummary(w http.ResponseWriter, r *http.Request) {
	windowStr := r.URL.Query().Get("window")
	windowSeconds := defaultDiagnosticsWindowSeconds
	if windowStr != "" {
		if v, err := strconv.Atoi(windowStr); err == nil && v > 0 {
			windowSeconds = v
		}
	}

	functions, err := h.Store.ListFunctions(r.Context(), 1000, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	calc := cost.NewDefaultCalculator()
	cutoff := time.Now().Add(-time.Duration(windowSeconds) * time.Second)
	var summaries []*cost.FunctionCostSummary
	var totalCost float64

	for _, fn := range functions {
		logs, err := h.Store.ListInvocationLogs(r.Context(), fn.ID, 10000, 0)
		if err != nil {
			continue
		}

		var totalDurationMs int64
		var invocations int64
		var coldStarts int64
		for _, log := range logs {
			if log.CreatedAt.Before(cutoff) {
				continue
			}
			invocations++
			totalDurationMs += log.DurationMs
			if log.ColdStart {
				coldStarts++
			}
		}

		if invocations == 0 {
			continue
		}

		summary := cost.AggregateFunctionCost(
			fn.ID, fn.Name,
			invocations, totalDurationMs, coldStarts,
			fn.MemoryMB, calc.GetPricing(),
		)
		summaries = append(summaries, summary)
		totalCost += summary.TotalCost
	}

	scope := store.TenantScopeFromContext(r.Context())
	costResp := cost.TenantCostSummary{
		TenantID:   scope.TenantID,
		TotalCost:  totalCost,
		Functions:  summaries,
		PeriodFrom: cutoff,
		PeriodTo:   time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(costResp)
}
