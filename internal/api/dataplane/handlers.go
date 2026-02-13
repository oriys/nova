package dataplane

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oriys/nova/internal/advisor"
	"github.com/oriys/nova/internal/ai"
	"github.com/oriys/nova/internal/cost"
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
	Store     *store.Store
	Exec      *executor.Executor
	Pool      *pool.Pool
	AIService *ai.Service // Optional: for AI-powered diagnostics analysis
	Advisor   interface{} // Optional: for performance recommendations (type advisor.PerformanceAdvisor)
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

func writeTenantQuotaExceeded(w http.ResponseWriter, decision *store.TenantQuotaDecision) {
	if decision == nil {
		http.Error(w, "tenant quota exceeded", http.StatusTooManyRequests)
		return
	}
	if decision.RetryAfterS > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(decision.RetryAfterS))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":         "tenant quota exceeded",
		"tenant_id":     decision.TenantID,
		"dimension":     decision.Dimension,
		"used":          decision.Used,
		"limit":         decision.Limit,
		"window_s":      decision.WindowS,
		"retry_after_s": decision.RetryAfterS,
	})
}

// RegisterRoutes registers all data plane routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Function invocation
	mux.HandleFunc("POST /functions/{name}/invoke", h.InvokeFunction)
	mux.HandleFunc("POST /functions/{name}/invoke-stream", h.InvokeFunctionStream)
	mux.HandleFunc("POST /functions/{name}/invoke-async", h.EnqueueAsyncFunction)
	mux.HandleFunc("GET /functions/{name}/async-invocations", h.ListFunctionAsyncInvocations)
	mux.HandleFunc("GET /async-invocations/{id}", h.GetAsyncInvocation)
	mux.HandleFunc("GET /async-invocations", h.ListAsyncInvocations)
	mux.HandleFunc("POST /async-invocations/{id}/retry", h.RetryAsyncInvocation)
	mux.HandleFunc("POST /async-invocations/{id}/pause", h.PauseAsyncInvocation)
	mux.HandleFunc("POST /async-invocations/{id}/resume", h.ResumeAsyncInvocation)
	mux.HandleFunc("DELETE /async-invocations/{id}", h.DeleteAsyncInvocation)

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

// InvokeFunction handles POST /functions/{name}/invoke
func (h *Handler) InvokeFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := h.enforceIngressPolicy(r.Context(), r, fn); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	var payload json.RawMessage
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	} else {
		payload = json.RawMessage("{}")
	}

	scope := store.TenantScopeFromContext(r.Context())
	invQuotaDecision, err := h.Store.CheckAndConsumeTenantQuota(r.Context(), scope.TenantID, store.TenantDimensionInvocations, 1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if invQuotaDecision != nil && !invQuotaDecision.Allowed {
		reason := "tenant_quota_" + invQuotaDecision.Dimension
		metrics.RecordAdmissionResult(fn.Name, "rejected", reason)
		metrics.RecordShed(fn.Name, reason)
		writeTenantQuotaExceeded(w, invQuotaDecision)
		return
	}

	metrics.SetQueueDepth(fn.Name, h.Pool.QueueDepth(fn.ID))
	metrics.SetQueueWaitMs(fn.Name, h.Pool.FunctionQueueWaitMs(fn.ID))
	defer func() {
		metrics.SetQueueDepth(fn.Name, h.Pool.QueueDepth(fn.ID))
		metrics.SetQueueWaitMs(fn.Name, h.Pool.FunctionQueueWaitMs(fn.ID))
	}()

	resp, err := h.Exec.Invoke(r.Context(), name, payload)
	if err != nil {
		status := http.StatusInternalServerError
		reason := "internal_error"

		switch {
		case errors.Is(err, pool.ErrQueueFull):
			status = capacityShedStatus(fn)
			reason = "queue_full"
		case errors.Is(err, pool.ErrInflightLimit):
			status = capacityShedStatus(fn)
			reason = "inflight_limit"
		case errors.Is(err, pool.ErrQueueWaitTimeout):
			status = capacityShedStatus(fn)
			reason = "queue_wait_timeout"
		case errors.Is(err, pool.ErrConcurrencyLimit):
			status = http.StatusServiceUnavailable
			reason = "concurrency_limit"
		case errors.Is(err, executor.ErrCircuitOpen):
			status = http.StatusServiceUnavailable
			reason = "circuit_breaker_open"
		case errors.Is(err, context.DeadlineExceeded):
			status = http.StatusGatewayTimeout
			reason = "timeout"
		}

		metrics.RecordAdmissionResult(fn.Name, "rejected", reason)
		if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
			metrics.RecordShed(fn.Name, reason)
			if retryAfter := capacityRetryAfter(fn); retryAfter > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			}
		}

		http.Error(w, err.Error(), status)
		return
	}
	metrics.RecordAdmissionResult(fn.Name, "accepted", "ok")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// InvokeFunctionStream handles POST /functions/{name}/invoke-stream with streaming response
func (h *Handler) InvokeFunctionStream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := h.enforceIngressPolicy(r.Context(), r, fn); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	var payload json.RawMessage
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	} else {
		payload = json.RawMessage("{}")
	}

	// Check tenant quota
	scope := store.TenantScopeFromContext(r.Context())
	invQuotaDecision, err := h.Store.CheckAndConsumeTenantQuota(r.Context(), scope.TenantID, store.TenantDimensionInvocations, 1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if invQuotaDecision != nil && !invQuotaDecision.Allowed {
		reason := "tenant_quota_" + invQuotaDecision.Dimension
		metrics.RecordAdmissionResult(fn.Name, "rejected", reason)
		metrics.RecordShed(fn.Name, reason)
		writeTenantQuotaExceeded(w, invQuotaDecision)
		return
	}

	// Track queue metrics
	metrics.SetQueueDepth(fn.Name, h.Pool.QueueDepth(fn.ID))
	metrics.SetQueueWaitMs(fn.Name, h.Pool.FunctionQueueWaitMs(fn.ID))
	defer func() {
		metrics.SetQueueDepth(fn.Name, h.Pool.QueueDepth(fn.ID))
		metrics.SetQueueWaitMs(fn.Name, h.Pool.FunctionQueueWaitMs(fn.ID))
	}()

	// Set up streaming response headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Write headers immediately
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Execute in streaming mode
	execErr := h.Exec.InvokeStream(r.Context(), name, payload, func(chunk []byte, isLast bool, err error) error {
		if err != nil {
			// Send error as SSE event
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
			flusher.Flush()
			return err
		}

		if len(chunk) > 0 {
			// Send data chunk as SSE event
			// Base64 encode binary data for safe transport
			encoded := base64.StdEncoding.EncodeToString(chunk)
			fmt.Fprintf(w, "data: %s\n\n", encoded)
			flusher.Flush()
		}

		if isLast {
			// Send completion event
			fmt.Fprintf(w, "event: done\ndata: {}\n\n")
			flusher.Flush()
		}

		return nil
	})

	if execErr != nil {
		// Handle errors that occur before streaming starts
		status := http.StatusInternalServerError
		reason := "internal_error"

		switch {
		case errors.Is(execErr, pool.ErrQueueFull):
			status = capacityShedStatus(fn)
			reason = "queue_full"
		case errors.Is(execErr, pool.ErrInflightLimit):
			status = capacityShedStatus(fn)
			reason = "inflight_limit"
		case errors.Is(execErr, pool.ErrQueueWaitTimeout):
			status = capacityShedStatus(fn)
			reason = "queue_wait_timeout"
		case errors.Is(execErr, pool.ErrConcurrencyLimit):
			status = http.StatusServiceUnavailable
			reason = "concurrency_limit"
		case errors.Is(execErr, executor.ErrCircuitOpen):
			status = http.StatusServiceUnavailable
			reason = "circuit_breaker_open"
		case errors.Is(execErr, context.DeadlineExceeded):
			status = http.StatusGatewayTimeout
			reason = "timeout"
		}

		metrics.RecordAdmissionResult(fn.Name, "rejected", reason)
		if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
			metrics.RecordShed(fn.Name, reason)
		}

		// Send error event
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", execErr.Error())
		flusher.Flush()
		return
	}
	metrics.RecordAdmissionResult(fn.Name, "accepted", "ok")
}

// EnqueueAsyncFunction handles POST /functions/{name}/invoke-async
func (h *Handler) EnqueueAsyncFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := h.enforceIngressPolicy(r.Context(), r, fn); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	req := enqueueAsyncInvokeRequest{}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	payload := req.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}

	scope := store.TenantScopeFromContext(r.Context())
	queueDepth, err := h.Store.GetTenantAsyncQueueDepth(r.Context(), scope.TenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	queueQuotaDecision, err := h.Store.CheckTenantAbsoluteQuota(r.Context(), scope.TenantID, store.TenantDimensionAsyncQueueDepth, queueDepth+1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if queueQuotaDecision != nil && !queueQuotaDecision.Allowed {
		writeTenantQuotaExceeded(w, queueQuotaDecision)
		return
	}

	inv := store.NewAsyncInvocation(fn.ID, fn.Name, payload)
	if req.MaxAttempts > 0 {
		inv.MaxAttempts = req.MaxAttempts
	}
	if req.BackoffBaseMS > 0 {
		inv.BackoffBaseMS = req.BackoffBaseMS
	}
	if req.BackoffMaxMS > 0 {
		inv.BackoffMaxMS = req.BackoffMaxMS
	}
	if inv.BackoffMaxMS < inv.BackoffBaseMS {
		inv.BackoffMaxMS = inv.BackoffBaseMS
	}

	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey != "" {
		ttl := time.Duration(req.IdempotencyTTLS) * time.Second
		enqueued, deduplicated, err := h.Store.EnqueueAsyncInvocationWithIdempotency(r.Context(), inv, idempotencyKey, ttl)
		if err != nil {
			if errors.Is(err, store.ErrInvalidIdempotencyKey) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if deduplicated {
			w.Header().Set("X-Idempotency-Status", "replay")
			w.WriteHeader(http.StatusOK)
		} else {
			w.Header().Set("Location", "/async-invocations/"+enqueued.ID)
			w.WriteHeader(http.StatusAccepted)
		}
		json.NewEncoder(w).Encode(enqueued)
		return
	}

	if err := h.Store.EnqueueAsyncInvocation(r.Context(), inv); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/async-invocations/"+inv.ID)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(inv)
}

// GetAsyncInvocation handles GET /async-invocations/{id}
func (h *Handler) GetAsyncInvocation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inv, err := h.Store.GetAsyncInvocation(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrAsyncInvocationNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inv)
}

// ListAsyncInvocations handles GET /async-invocations
func (h *Handler) ListAsyncInvocations(w http.ResponseWriter, r *http.Request) {
	limit := parseLimitQuery(r.URL.Query().Get("limit"), store.DefaultAsyncListLimit, store.MaxAsyncListLimit)
	offset := parseLimitQuery(r.URL.Query().Get("offset"), 0, 0)
	statuses, err := parseAsyncStatuses(r.URL.Query().Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	invs, err := h.Store.ListAsyncInvocations(r.Context(), limit, offset, statuses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if invs == nil {
		invs = []*store.AsyncInvocation{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invs)
}

// ListFunctionAsyncInvocations handles GET /functions/{name}/async-invocations
func (h *Handler) ListFunctionAsyncInvocations(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	limit := parseLimitQuery(r.URL.Query().Get("limit"), store.DefaultAsyncListLimit, store.MaxAsyncListLimit)
	offset := parseLimitQuery(r.URL.Query().Get("offset"), 0, 0)
	statuses, err := parseAsyncStatuses(r.URL.Query().Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	invs, err := h.Store.ListFunctionAsyncInvocations(r.Context(), fn.ID, limit, offset, statuses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if invs == nil {
		invs = []*store.AsyncInvocation{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invs)
}

// RetryAsyncInvocation handles POST /async-invocations/{id}/retry
func (h *Handler) RetryAsyncInvocation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req := retryAsyncInvokeRequest{}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	}

	inv, err := h.Store.RequeueAsyncInvocation(r.Context(), id, req.MaxAttempts)
	if err != nil {
		if errors.Is(err, store.ErrAsyncInvocationNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrAsyncInvocationNotDLQ) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inv)
}

// PauseAsyncInvocation handles POST /async-invocations/{id}/pause
func (h *Handler) PauseAsyncInvocation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inv, err := h.Store.PauseAsyncInvocation(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrAsyncInvocationNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrAsyncInvocationNotQueued) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inv)
}

// ResumeAsyncInvocation handles POST /async-invocations/{id}/resume
func (h *Handler) ResumeAsyncInvocation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inv, err := h.Store.ResumeAsyncInvocation(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrAsyncInvocationNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrAsyncInvocationNotPaused) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inv)
}

// DeleteAsyncInvocation handles DELETE /async-invocations/{id}
func (h *Handler) DeleteAsyncInvocation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := h.Store.DeleteAsyncInvocation(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrAsyncInvocationNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrAsyncInvocationNotDeletable) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func capacityShedStatus(fn *domain.Function) int {
	if fn != nil && fn.CapacityPolicy != nil && fn.CapacityPolicy.ShedStatusCode != 0 {
		return fn.CapacityPolicy.ShedStatusCode
	}
	return http.StatusServiceUnavailable
}

func capacityRetryAfter(fn *domain.Function) int {
	if fn != nil && fn.CapacityPolicy != nil && fn.CapacityPolicy.RetryAfterS > 0 {
		return fn.CapacityPolicy.RetryAfterS
	}
	return 1
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

func parseAsyncStatuses(raw string) ([]store.AsyncInvocationStatus, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	statuses := make([]store.AsyncInvocationStatus, 0, len(parts))
	for _, part := range parts {
		status := store.AsyncInvocationStatus(strings.TrimSpace(part))
		if status == "" {
			continue
		}
		switch status {
		case store.AsyncInvocationStatusQueued,
			store.AsyncInvocationStatusRunning,
			store.AsyncInvocationStatusSucceeded,
			store.AsyncInvocationStatusDLQ,
			store.AsyncInvocationStatusPaused:
			statuses = append(statuses, status)
		default:
			return nil, fmt.Errorf("invalid status: %s", status)
		}
	}
	return statuses, nil
}

// Health handles GET /health - detailed status
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	pgOK := h.Store.PingPostgres(ctx) == nil
	stats := h.Pool.Stats()

	status := "ok"
	if !pgOK {
		status = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": status,
		"components": map[string]interface{}{
			"postgres": pgOK,
			"pool": map[string]interface{}{
				"active_vms":  stats["active_vms"],
				"total_pools": stats["total_pools"],
			},
		},
		"uptime_seconds": int64(time.Since(time.Now()).Seconds()),
	})
}

// HealthLive handles GET /health/live - Kubernetes liveness probe
func (h *Handler) HealthLive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HealthReady handles GET /health/ready - Kubernetes readiness probe
func (h *Handler) HealthReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.Store.PingPostgres(ctx); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "not_ready",
			"error":  "postgres unavailable: " + err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

// HealthStartup handles GET /health/startup - Kubernetes startup probe
func (h *Handler) HealthStartup(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Check Postgres is reachable
	if err := h.Store.PingPostgres(ctx); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "starting",
			"error":  "waiting for postgres: " + err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// Stats handles GET /stats
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.Pool.Stats())
}

// Logs handles GET /functions/{name}/logs
func (h *Handler) Logs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Get request_id from query params if specified
	requestID := r.URL.Query().Get("request_id")
	if requestID != "" {
		entry, err := h.Store.GetInvocationLog(r.Context(), requestID)
		if err != nil {
			http.Error(w, "logs not found for request_id", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
		return
	}

	// Otherwise return recent logs for function
	tailStr := r.URL.Query().Get("tail")
	tail := 10
	if tailStr != "" {
		if n, err := fmt.Sscanf(tailStr, "%d", &tail); err != nil || n != 1 {
			tail = 10
		}
	}

	offset := parseLimitQuery(r.URL.Query().Get("offset"), 0, 0)
	entries, err := h.Store.ListInvocationLogs(r.Context(), fn.ID, tail, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure we return empty array instead of null
	if entries == nil {
		entries = []*store.InvocationLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// ListAllInvocations handles GET /invocations
func (h *Handler) ListAllInvocations(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if n, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || n != 1 {
			limit = 100
		}
	}
	if limit > 500 {
		limit = 500
	}
	offset := parseLimitQuery(r.URL.Query().Get("offset"), 0, 0)

	entries, err := h.Store.ListAllInvocationLogs(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure we return empty array instead of null
	if entries == nil {
		entries = []*store.InvocationLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

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
