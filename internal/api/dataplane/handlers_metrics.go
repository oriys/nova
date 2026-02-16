package dataplane

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/oriys/nova/internal/ai"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/store"
)

// FunctionMetrics handles GET /functions/{name}/metrics
func (h *Handler) FunctionMetrics(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Get function-specific metrics
	allStats := metrics.Global().FunctionStats()
	funcStats, ok := allStats[fn.ID]
	if !ok {
		// Return zero metrics if no invocations yet
		funcStats = map[string]interface{}{
			"invocations": int64(0),
			"successes":   int64(0),
			"failures":    int64(0),
			"cold_starts": int64(0),
			"warm_starts": int64(0),
			"avg_ms":      float64(0),
			"min_ms":      int64(0),
			"max_ms":      int64(0),
		}
	}

	// Get pool stats for this function
	poolStats := h.Pool.FunctionStats(fn.ID)

	// Get minute-level time series data for recent window.
	rangeSec, bucketSec := parseRangeParam(r.URL.Query().Get("range"))
	timeSeries, err := h.Store.GetFunctionTimeSeries(r.Context(), fn.ID, rangeSec, bucketSec)
	if err != nil {
		timeSeries = []store.TimeSeriesBucket{}
	}

	result := map[string]interface{}{
		"function_id":   fn.ID,
		"function_name": fn.Name,
		"invocations":   funcStats,
		"pool":          poolStats,
		"timeseries":    timeSeries,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// FunctionSLOStatus handles GET /functions/{name}/slo/status
func (h *Handler) FunctionSLOStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	resp := functionSLOStatusResponse{
		FunctionID:   fn.ID,
		FunctionName: fn.Name,
		Enabled:      fn.SLOPolicy != nil && fn.SLOPolicy.Enabled,
		Breaches:     []string{},
	}
	if fn.SLOPolicy == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	policy := *fn.SLOPolicy
	if policy.WindowS <= 0 {
		policy.WindowS = 900
	}
	if policy.MinSamples <= 0 {
		policy.MinSamples = 20
	}
	resp.Policy = &policy

	snapshot, err := h.Store.GetFunctionSLOSnapshot(r.Context(), fn.ID, policy.WindowS)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp.Snapshot = snapshot

	if snapshot.TotalInvocations >= int64(policy.MinSamples) {
		if policy.Objectives.SuccessRatePct > 0 && snapshot.SuccessRatePct < policy.Objectives.SuccessRatePct {
			resp.Breaches = append(resp.Breaches, "success_rate")
		}
		if policy.Objectives.P95DurationMs > 0 && snapshot.P95DurationMs > policy.Objectives.P95DurationMs {
			resp.Breaches = append(resp.Breaches, "p95_latency")
		}
		if policy.Objectives.ColdStartRatePct > 0 && snapshot.ColdStartRatePct > policy.Objectives.ColdStartRatePct {
			resp.Breaches = append(resp.Breaches, "cold_start_rate")
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// FunctionDiagnostics handles GET /functions/{name}/diagnostics
func (h *Handler) FunctionDiagnostics(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	windowSeconds := parseWindowParam(r.URL.Query().Get("window"), defaultDiagnosticsWindowSeconds)
	sampleSize := parseLimitQuery(r.URL.Query().Get("sample"), defaultDiagnosticsSampleSize, maxDiagnosticsSampleSize)
	cutoff := time.Now().Add(-time.Duration(windowSeconds) * time.Second)

	entries, err := h.Store.ListInvocationLogs(r.Context(), fn.ID, sampleSize, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	filtered := make([]*store.InvocationLog, 0, len(entries))
	for _, entry := range entries {
		if entry.CreatedAt.Before(cutoff) {
			continue
		}
		filtered = append(filtered, entry)
	}

	result := functionDiagnosticsResponse{
		FunctionID:      fn.ID,
		FunctionName:    fn.Name,
		WindowSeconds:   windowSeconds,
		SampleSize:      sampleSize,
		SlowThresholdMs: 500,
		SlowInvocations: []slowInvocationSummary{},
	}

	if len(filtered) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	durations := make([]int64, 0, len(filtered))
	var totalDuration int64
	var maxDuration int64
	errorCount := 0
	coldStartCount := 0

	for _, entry := range filtered {
		duration := entry.DurationMs
		if duration < 0 {
			duration = 0
		}
		durations = append(durations, duration)
		totalDuration += duration
		if duration > maxDuration {
			maxDuration = duration
		}
		if !entry.Success {
			errorCount++
		}
		if entry.ColdStart {
			coldStartCount++
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	p50 := percentile(durations, 0.50)
	p95 := percentile(durations, 0.95)
	p99 := percentile(durations, 0.99)
	slowThreshold := int64(math.Max(float64(p95)*1.5, 500))

	slowCount := 0
	slowCandidates := make([]*store.InvocationLog, 0, len(filtered))
	for _, entry := range filtered {
		if entry.DurationMs >= slowThreshold {
			slowCount++
			slowCandidates = append(slowCandidates, entry)
		}
	}
	sort.Slice(slowCandidates, func(i, j int) bool {
		if slowCandidates[i].DurationMs == slowCandidates[j].DurationMs {
			return slowCandidates[i].CreatedAt.After(slowCandidates[j].CreatedAt)
		}
		return slowCandidates[i].DurationMs > slowCandidates[j].DurationMs
	})

	if len(slowCandidates) > 10 {
		slowCandidates = slowCandidates[:10]
	}
	slowInvocations := make([]slowInvocationSummary, 0, len(slowCandidates))
	for _, entry := range slowCandidates {
		slowInvocations = append(slowInvocations, slowInvocationSummary{
			ID:           entry.ID,
			CreatedAt:    entry.CreatedAt,
			DurationMs:   entry.DurationMs,
			ColdStart:    entry.ColdStart,
			Success:      entry.Success,
			ErrorMessage: entry.ErrorMessage,
		})
	}

	total := float64(len(filtered))
	result = functionDiagnosticsResponse{
		FunctionID:       fn.ID,
		FunctionName:     fn.Name,
		WindowSeconds:    windowSeconds,
		SampleSize:       sampleSize,
		TotalInvocations: len(filtered),
		AvgDurationMs:    float64(totalDuration) / total,
		P50DurationMs:    p50,
		P95DurationMs:    p95,
		P99DurationMs:    p99,
		MaxDurationMs:    maxDuration,
		ErrorRatePct:     float64(errorCount) * 100 / total,
		ColdStartRatePct: float64(coldStartCount) * 100 / total,
		SlowThresholdMs:  slowThreshold,
		SlowCount:        slowCount,
		SlowInvocations:  slowInvocations,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// AnalyzeFunctionDiagnostics handles POST /functions/{name}/diagnostics/analyze
func (h *Handler) AnalyzeFunctionDiagnostics(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// Check if AI service is available
	if h.AIService == nil || !h.AIService.Enabled() {
		http.Error(w, "AI service is not enabled", http.StatusServiceUnavailable)
		return
	}

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	windowSeconds := parseWindowParam(r.URL.Query().Get("window"), defaultDiagnosticsWindowSeconds)
	sampleSize := parseLimitQuery(r.URL.Query().Get("sample"), defaultDiagnosticsSampleSize, maxDiagnosticsSampleSize)
	cutoff := time.Now().Add(-time.Duration(windowSeconds) * time.Second)

	entries, err := h.Store.ListInvocationLogs(r.Context(), fn.ID, sampleSize, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	filtered := make([]*store.InvocationLog, 0, len(entries))
	for _, entry := range entries {
		if entry.CreatedAt.Before(cutoff) {
			continue
		}
		filtered = append(filtered, entry)
	}

	if len(filtered) == 0 {
		http.Error(w, "no invocation data available for analysis", http.StatusBadRequest)
		return
	}

	// Calculate metrics
	durations := make([]int64, 0, len(filtered))
	var totalDuration int64
	var maxDuration int64
	errorCount := 0
	coldStartCount := 0
	errorSamples := []ai.DiagnosticsErrorSample{}
	slowSamples := []ai.DiagnosticsSlowSample{}

	for _, entry := range filtered {
		duration := entry.DurationMs
		if duration < 0 {
			duration = 0
		}
		durations = append(durations, duration)
		totalDuration += duration
		if duration > maxDuration {
			maxDuration = duration
		}
		if !entry.Success {
			errorCount++
			if len(errorSamples) < 10 && entry.ErrorMessage != "" {
				errorSamples = append(errorSamples, ai.DiagnosticsErrorSample{
					Timestamp:    entry.CreatedAt.Format(time.RFC3339),
					ErrorMessage: entry.ErrorMessage,
					DurationMs:   entry.DurationMs,
					ColdStart:    entry.ColdStart,
				})
			}
		}
		if entry.ColdStart {
			coldStartCount++
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	p50 := percentile(durations, 0.50)
	p95 := percentile(durations, 0.95)
	p99 := percentile(durations, 0.99)
	slowThreshold := int64(math.Max(float64(p95)*1.5, 500))

	// Collect slow samples
	slowCount := 0
	for _, entry := range filtered {
		if entry.DurationMs >= slowThreshold {
			slowCount++
			if len(slowSamples) < 10 {
				slowSamples = append(slowSamples, ai.DiagnosticsSlowSample{
					Timestamp:  entry.CreatedAt.Format(time.RFC3339),
					DurationMs: entry.DurationMs,
					ColdStart:  entry.ColdStart,
				})
			}
		}
	}

	total := float64(len(filtered))

	// Prepare AI analysis request
	analysisReq := ai.DiagnosticsAnalysisRequest{
		FunctionName:     fn.Name,
		TotalInvocations: len(filtered),
		AvgDurationMs:    float64(totalDuration) / total,
		P50DurationMs:    p50,
		P95DurationMs:    p95,
		P99DurationMs:    p99,
		MaxDurationMs:    maxDuration,
		ErrorRatePct:     float64(errorCount) * 100 / total,
		ColdStartRatePct: float64(coldStartCount) * 100 / total,
		SlowCount:        slowCount,
		ErrorSamples:     errorSamples,
		SlowSamples:      slowSamples,
		MemoryMB:         fn.MemoryMB,
		TimeoutS:         fn.TimeoutS,
	}

	// Call AI service for analysis
	analysis, err := h.AIService.AnalyzeDiagnostics(r.Context(), analysisReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analysis)
}

// FunctionHeatmap handles GET /functions/{name}/heatmap?weeks=52
func (h *Handler) FunctionHeatmap(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	weeks := 52
	if ws := r.URL.Query().Get("weeks"); ws != "" {
		if n, err := strconv.Atoi(ws); err == nil && n > 0 && n <= 104 {
			weeks = n
		}
	}

	data, err := h.Store.GetFunctionDailyHeatmap(r.Context(), fn.ID, weeks)
	if err != nil {
		data = []store.DailyCount{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// GlobalHeatmap handles GET /metrics/heatmap?weeks=52
func (h *Handler) GlobalHeatmap(w http.ResponseWriter, r *http.Request) {
	weeks := 52
	if ws := r.URL.Query().Get("weeks"); ws != "" {
		if n, err := strconv.Atoi(ws); err == nil && n > 0 && n <= 104 {
			weeks = n
		}
	}

	data, err := h.Store.GetGlobalDailyHeatmap(r.Context(), weeks)
	if err != nil {
		data = []store.DailyCount{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// GlobalTimeSeries handles GET /metrics/timeseries?range=1h
func (h *Handler) GlobalTimeSeries(w http.ResponseWriter, r *http.Request) {
	rangeSec, bucketSec := parseRangeParam(r.URL.Query().Get("range"))
	timeSeries, err := h.Store.GetGlobalTimeSeries(r.Context(), rangeSec, bucketSec)
	if err != nil {
		timeSeries = []store.TimeSeriesBucket{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(timeSeries)
}

