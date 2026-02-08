package dataplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
)

const (
	defaultRangeSeconds = 3600 // 1 hour
	maxDataPoints       = 30
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

// Handler handles data plane HTTP requests (invocations and observability).
type Handler struct {
	Store *store.Store
	Exec  *executor.Executor
	Pool  *pool.Pool
}

type enqueueAsyncInvokeRequest struct {
	Payload       json.RawMessage `json:"payload"`
	MaxAttempts   int             `json:"max_attempts"`
	BackoffBaseMS int             `json:"backoff_base_ms"`
	BackoffMaxMS  int             `json:"backoff_max_ms"`
}

type retryAsyncInvokeRequest struct {
	MaxAttempts int `json:"max_attempts"`
}

// RegisterRoutes registers all data plane routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Function invocation
	mux.HandleFunc("POST /functions/{name}/invoke", h.InvokeFunction)
	mux.HandleFunc("POST /functions/{name}/invoke-async", h.EnqueueAsyncFunction)
	mux.HandleFunc("GET /functions/{name}/async-invocations", h.ListFunctionAsyncInvocations)
	mux.HandleFunc("GET /async-invocations/{id}", h.GetAsyncInvocation)
	mux.HandleFunc("GET /async-invocations", h.ListAsyncInvocations)
	mux.HandleFunc("POST /async-invocations/{id}/retry", h.RetryAsyncInvocation)

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
	mux.HandleFunc("GET /functions/{name}/heatmap", h.FunctionHeatmap)
	mux.HandleFunc("GET /metrics/heatmap", h.GlobalHeatmap)
}

// InvokeFunction handles POST /functions/{name}/invoke
func (h *Handler) InvokeFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
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

// EnqueueAsyncFunction handles POST /functions/{name}/invoke-async
func (h *Handler) EnqueueAsyncFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
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
	statuses, err := parseAsyncStatuses(r.URL.Query().Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	invs, err := h.Store.ListAsyncInvocations(r.Context(), limit, statuses)
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
	statuses, err := parseAsyncStatuses(r.URL.Query().Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	invs, err := h.Store.ListFunctionAsyncInvocations(r.Context(), fn.ID, limit, statuses)
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
			store.AsyncInvocationStatusDLQ:
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

	entries, err := h.Store.ListInvocationLogs(r.Context(), fn.ID, tail)
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

	entries, err := h.Store.ListAllInvocationLogs(r.Context(), limit)
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
