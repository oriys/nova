package dataplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
)

// Handler handles data plane HTTP requests (invocations and observability).
type Handler struct {
	Store *store.Store
	Exec  *executor.Executor
	Pool  *pool.Pool
}

// RegisterRoutes registers all data plane routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Function invocation
	mux.HandleFunc("POST /functions/{name}/invoke", h.InvokeFunction)

	// Health probes
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /health/live", h.HealthLive)
	mux.HandleFunc("GET /health/ready", h.HealthReady)
	mux.HandleFunc("GET /health/startup", h.HealthStartup)

	// Observability
	mux.HandleFunc("GET /stats", h.Stats)
	mux.Handle("GET /metrics", metrics.Global().JSONHandler())
	mux.Handle("GET /metrics/prometheus", metrics.PrometheusHandler())
	mux.HandleFunc("GET /functions/{name}/logs", h.Logs)
	mux.HandleFunc("GET /functions/{name}/metrics", h.FunctionMetrics)
}

// InvokeFunction handles POST /functions/{name}/invoke
func (h *Handler) InvokeFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var payload json.RawMessage
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
	} else {
		payload = json.RawMessage("{}")
	}

	resp, err := h.Exec.Invoke(r.Context(), name, payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Health handles GET /health - detailed status
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	pgOK := h.Store.PingPostgres(ctx) == nil
	redisOK := h.Store.PingRedis(ctx) == nil
	stats := h.Pool.Stats()

	status := "ok"
	if !pgOK || !redisOK {
		status = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": status,
		"components": map[string]interface{}{
			"postgres": pgOK,
			"redis":    redisOK,
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

	if err := h.Store.PingRedis(ctx); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "not_ready",
			"error":  "redis unavailable: " + err.Error(),
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

	// Check Redis is reachable
	if err := h.Store.PingRedis(ctx); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "starting",
			"error":  "waiting for redis: " + err.Error(),
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

	store := logging.GetOutputStore()
	if store == nil {
		http.Error(w, "output capture not enabled", http.StatusServiceUnavailable)
		return
	}

	// Get request_id from query params if specified
	requestID := r.URL.Query().Get("request_id")
	if requestID != "" {
		entry, found := store.Get(requestID)
		if !found {
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

	entries := store.GetByFunction(fn.ID, tail)
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

	result := map[string]interface{}{
		"function_id":   fn.ID,
		"function_name": fn.Name,
		"invocations":   funcStats,
		"pool":          poolStats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
