package dataplane

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/oriys/nova/internal/domain"
)

// RegisterDurableRoutes registers durable execution endpoints.
func (h *Handler) RegisterDurableRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /functions/{name}/invoke-durable", h.InvokeDurable)
	mux.HandleFunc("GET /functions/{name}/durable-executions", h.ListDurableExecutions)
	mux.HandleFunc("GET /durable-executions/{id}", h.GetDurableExecution)
}

// InvokeDurable executes a function with durable execution tracking.
func (h *Handler) InvokeDurable(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var payload json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		payload = json.RawMessage(`null`)
	}

	exec, err := h.Exec.InvokeDurable(r.Context(), name, payload)
	if err != nil && exec == nil {
		safeError(w, "invoke failed", http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if exec.Error != "" {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(exec)
}

// ListDurableExecutions returns durable executions for a function.
func (h *Handler) ListDurableExecutions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		safeError(w, "not found", http.StatusNotFound, err)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 20
	}

	executions, err := h.Store.ListDurableExecutions(r.Context(), fn.ID, limit, offset)
	if err != nil {
		safeError(w, "internal error", http.StatusInternalServerError, err)
		return
	}
	if executions == nil {
		executions = make([]*domain.DurableExecution, 0)
	}

	total, err := h.Store.CountDurableExecutions(r.Context(), fn.ID)
	if err != nil {
		total = int64(len(executions))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items":  executions,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetDurableExecution returns a single durable execution with steps.
func (h *Handler) GetDurableExecution(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	exec, err := h.Store.GetDurableExecution(r.Context(), id)
	if err != nil {
		safeError(w, "not found", http.StatusNotFound, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(exec)
}
