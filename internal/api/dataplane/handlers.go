package dataplane

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oriys/nova/internal/ai"
	"github.com/oriys/nova/internal/cluster"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
)

const (
	defaultRangeSeconds             = 3600 // 1 hour
	maxDataPoints                   = 30
	defaultDiagnosticsWindowSeconds = 24 * 3600
	defaultDiagnosticsSampleSize    = 1000
	maxDiagnosticsSampleSize        = 5000
)

// parseRangeParam parses a range string like "1m", "5m", "1h", "24h" into
// (rangeSeconds, bucketSeconds). bucketSeconds = rangeSeconds / maxDataPoints
// rounded up to at least 1 second.
func parseRangeParam(raw string) (int, int) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultRangeSeconds, defaultRangeSeconds / maxDataPoints
	}

	unit := raw[len(raw)-1]
	numStr := raw[:len(raw)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil || num <= 0 {
		return defaultRangeSeconds, defaultRangeSeconds / maxDataPoints
	}

	var rangeSeconds int
	switch unit {
	case 'm':
		rangeSeconds = num * 60
	case 'h':
		rangeSeconds = num * 3600
	case 'd':
		rangeSeconds = num * 86400
	default:
		return defaultRangeSeconds, defaultRangeSeconds / maxDataPoints
	}

	bucketSeconds := rangeSeconds / maxDataPoints
	if bucketSeconds < 1 {
		bucketSeconds = 1
	}
	return rangeSeconds, bucketSeconds
}

func parseWindowParam(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		return seconds
	}
	if len(raw) < 2 {
		return fallback
	}
	unit := raw[len(raw)-1]
	numStr := raw[:len(raw)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil || num <= 0 {
		return fallback
	}
	switch unit {
	case 'm':
		return num * 60
	case 'h':
		return num * 3600
	case 'd':
		return num * 86400
	default:
		return fallback
	}
}

func percentile(sortedValues []int64, p float64) int64 {
	if len(sortedValues) == 0 {
		return 0
	}
	if p <= 0 {
		return sortedValues[0]
	}
	if p >= 1 {
		return sortedValues[len(sortedValues)-1]
	}
	index := int(math.Ceil(p*float64(len(sortedValues)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sortedValues) {
		index = len(sortedValues) - 1
	}
	return sortedValues[index]
}

type slowInvocationSummary struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	DurationMs   int64     `json:"duration_ms"`
	ColdStart    bool      `json:"cold_start"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

type functionDiagnosticsResponse struct {
	FunctionID       string                  `json:"function_id"`
	FunctionName     string                  `json:"function_name"`
	WindowSeconds    int                     `json:"window_seconds"`
	SampleSize       int                     `json:"sample_size"`
	TotalInvocations int                     `json:"total_invocations"`
	AvgDurationMs    float64                 `json:"avg_duration_ms"`
	P50DurationMs    int64                   `json:"p50_duration_ms"`
	P95DurationMs    int64                   `json:"p95_duration_ms"`
	P99DurationMs    int64                   `json:"p99_duration_ms"`
	MaxDurationMs    int64                   `json:"max_duration_ms"`
	ErrorRatePct     float64                 `json:"error_rate_pct"`
	ColdStartRatePct float64                 `json:"cold_start_rate_pct"`
	SlowThresholdMs  int64                   `json:"slow_threshold_ms"`
	SlowCount        int                     `json:"slow_count"`
	SlowInvocations  []slowInvocationSummary `json:"slow_invocations"`
}

type functionSLOStatusResponse struct {
	FunctionID   string                     `json:"function_id"`
	FunctionName string                     `json:"function_name"`
	Enabled      bool                       `json:"enabled"`
	Policy       *domain.SLOPolicy          `json:"policy,omitempty"`
	Snapshot     *store.FunctionSLOSnapshot `json:"snapshot,omitempty"`
	Breaches     []string                   `json:"breaches"`
}

// Handler handles data plane HTTP requests (invocations and observability).
type Handler struct {
	Store         *store.Store
	Exec          *executor.Executor
	Pool          *pool.Pool
	AIService     *ai.Service     // Optional: for AI-powered diagnostics analysis
	Advisor       interface{}     // Optional: for performance recommendations (type advisor.PerformanceAdvisor)
	ClusterRouter *cluster.Router // Optional: for cross-node invoke/prewarm routing
}

type invocationPaginationStore interface {
	CountInvocationLogs(ctx context.Context, functionID string) (int64, error)
	CountAllInvocationLogs(ctx context.Context) (int64, error)
	ListAllInvocationLogsFiltered(ctx context.Context, limit, offset int, search, functionName string, success *bool) ([]*store.InvocationLog, error)
	CountAllInvocationLogsFiltered(ctx context.Context, search, functionName string, success *bool) (int64, error)
	GetAllInvocationLogsSummary(ctx context.Context) (*store.InvocationLogSummary, error)
	GetAllInvocationLogsSummaryFiltered(ctx context.Context, search, functionName string, success *bool) (*store.InvocationLogSummary, error)
}

type asyncInvocationPaginationStore interface {
	CountAsyncInvocations(ctx context.Context, statuses []store.AsyncInvocationStatus) (int64, error)
	CountFunctionAsyncInvocations(ctx context.Context, functionID string, statuses []store.AsyncInvocationStatus) (int64, error)
	GetAsyncInvocationSummary(ctx context.Context) (*store.AsyncInvocationSummary, error)
}

type enqueueAsyncInvokeRequest struct {
	Payload         json.RawMessage `json:"payload"`
	MaxAttempts     int             `json:"max_attempts"`
	BackoffBaseMS   int             `json:"backoff_base_ms"`
	BackoffMaxMS    int             `json:"backoff_max_ms"`
	IdempotencyKey  string          `json:"idempotency_key"`
	IdempotencyTTLS int             `json:"idempotency_ttl_s"`
}

type retryAsyncInvokeRequest struct {
	MaxAttempts int `json:"max_attempts"`
}

// RegisterRoutes registers all data plane routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Function invocation
	mux.HandleFunc("POST /functions/{name}/invoke", h.InvokeFunction)
	mux.HandleFunc("POST /functions/{name}/invoke-stream", h.InvokeFunctionStream)
	mux.HandleFunc("POST /functions/{name}/prewarm", h.PrewarmFunction)
	mux.HandleFunc("POST /functions/{name}/invoke-async", h.EnqueueAsyncFunction)
	mux.HandleFunc("GET /functions/{name}/state", h.GetFunctionState)
	mux.HandleFunc("PUT /functions/{name}/state", h.PutFunctionState)
	mux.HandleFunc("DELETE /functions/{name}/state", h.DeleteFunctionState)
	mux.HandleFunc("GET /functions/{name}/async-invocations", h.ListFunctionAsyncInvocations)
	mux.HandleFunc("GET /async-invocations/summary", h.AsyncInvocationsSummary)
	mux.HandleFunc("GET /async-invocations/{id}", h.GetAsyncInvocation)
	mux.HandleFunc("GET /async-invocations", h.ListAsyncInvocations)
	mux.HandleFunc("POST /async-invocations/{id}/retry", h.RetryAsyncInvocation)
	mux.HandleFunc("POST /async-invocations/{id}/pause", h.PauseAsyncInvocation)
	mux.HandleFunc("POST /async-invocations/{id}/resume", h.ResumeAsyncInvocation)
	mux.HandleFunc("DELETE /async-invocations/{id}", h.DeleteAsyncInvocation)
	mux.HandleFunc("POST /async-invocations/functions/{id}/pause", h.PauseAsyncInvocationsByFunction)
	mux.HandleFunc("POST /async-invocations/functions/{id}/resume", h.ResumeAsyncInvocationsByFunction)
	mux.HandleFunc("POST /async-invocations/workflows/{id}/pause", h.PauseAsyncInvocationsByWorkflow)
	mux.HandleFunc("POST /async-invocations/workflows/{id}/resume", h.ResumeAsyncInvocationsByWorkflow)
	mux.HandleFunc("GET /async-invocations/global-pause", h.GetGlobalAsyncPause)
	mux.HandleFunc("POST /async-invocations/global-pause", h.SetGlobalAsyncPause)
	mux.HandleFunc("GET /workflows/{name}/async-invocations", h.ListWorkflowAsyncInvocations)
	mux.HandleFunc("GET /async-invocations/dlq", h.ListDLQInvocations)
	mux.HandleFunc("POST /async-invocations/dlq/retry-all", h.RetryAllDLQ)

	// Health probes
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /health/live", h.HealthLive)
	mux.HandleFunc("GET /health/ready", h.HealthReady)
	mux.HandleFunc("GET /health/startup", h.HealthStartup)

	// Observability
	mux.HandleFunc("GET /stats", h.Stats)
	mux.Handle("GET /metrics", metrics.Global().JSONHandler())
	mux.HandleFunc("GET /metrics/timeseries", h.GlobalTimeSeries)
	mux.Handle("GET /metrics/prometheus", metrics.PrometheusHandler())
	mux.HandleFunc("GET /invocations", h.ListAllInvocations)
	mux.HandleFunc("GET /functions/{name}/logs", h.Logs)
	mux.HandleFunc("GET /functions/{name}/logs/stream", h.StreamLogs)
	mux.HandleFunc("GET /functions/{name}/metrics", h.FunctionMetrics)
	mux.HandleFunc("GET /functions/{name}/slo/status", h.FunctionSLOStatus)
	mux.HandleFunc("GET /functions/{name}/diagnostics", h.FunctionDiagnostics)
	mux.HandleFunc("POST /functions/{name}/diagnostics/analyze", h.AnalyzeFunctionDiagnostics)
	mux.HandleFunc("GET /functions/{name}/recommendations", h.GetPerformanceRecommendations)
	mux.HandleFunc("GET /functions/{name}/heatmap", h.FunctionHeatmap)
	mux.HandleFunc("GET /metrics/heatmap", h.GlobalHeatmap)

	// Cost intelligence
	mux.HandleFunc("GET /functions/{name}/cost", h.FunctionCost)
	mux.HandleFunc("GET /cost/summary", h.CostSummary)
}

func parseLimitQuery(raw string, fallback, max int) int {
	limit := fallback
	if raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	if limit <= 0 {
		limit = fallback
	}
	if max > 0 && limit > max {
		limit = max
	}
	return limit
}
