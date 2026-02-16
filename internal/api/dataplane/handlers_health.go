package dataplane

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

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
