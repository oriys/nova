package controlplane

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/triggers"
)

// CreateTrigger handles POST /triggers
func (h *Handler) CreateTrigger(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string                 `json:"name"`
		Type         string                 `json:"type"`
		FunctionName string                 `json:"function_name"`
		Enabled      bool                   `json:"enabled"`
		Config       map[string]interface{} `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	if req.Type == "" {
		http.Error(w, "type is required", http.StatusBadRequest)
		return
	}

	if req.FunctionName == "" {
		http.Error(w, "function_name is required", http.StatusBadRequest)
		return
	}

	fn, err := h.Store.GetFunctionByName(r.Context(), req.FunctionName)
	if err != nil {
		http.Error(w, "function not found: "+req.FunctionName, http.StatusNotFound)
		return
	}

	trigger := &store.TriggerRecord{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Type:         req.Type,
		FunctionID:   fn.ID,
		FunctionName: req.FunctionName,
		Enabled:      req.Enabled,
		Config:       req.Config,
	}

	if err := h.Store.CreateTrigger(r.Context(), trigger); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Hot-load: register with trigger manager so it starts immediately
	if h.TriggerManager != nil {
		t := &triggers.Trigger{
			ID:           trigger.ID,
			TenantID:     trigger.TenantID,
			Namespace:    trigger.Namespace,
			Name:         trigger.Name,
			Type:         triggers.TriggerType(trigger.Type),
			FunctionID:   trigger.FunctionID,
			FunctionName: trigger.FunctionName,
			Enabled:      trigger.Enabled,
			Config:       trigger.Config,
		}
		_ = h.TriggerManager.RegisterTrigger(t)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(trigger)
}

// ListTriggers handles GET /triggers
func (h *Handler) ListTriggers(w http.ResponseWriter, r *http.Request) {
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	triggers, err := h.Store.ListTriggers(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if triggers == nil {
		triggers = []*store.TriggerRecord{}
	}

	total := estimatePaginatedTotal(limit, offset, len(triggers))
	writePaginatedList(w, limit, offset, len(triggers), total, triggers)
}

// GetTrigger handles GET /triggers/{id}
func (h *Handler) GetTrigger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	trigger, err := h.Store.GetTrigger(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trigger)
}

// UpdateTrigger handles PATCH /triggers/{id}
func (h *Handler) UpdateTrigger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var update store.TriggerUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	trigger, err := h.Store.UpdateTrigger(r.Context(), id, &update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trigger)
}

// DeleteTrigger handles DELETE /triggers/{id}
func (h *Handler) DeleteTrigger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.Store.DeleteTrigger(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Hot-unload: stop the running connector
	if h.TriggerManager != nil {
		_ = h.TriggerManager.UnregisterTrigger(id)
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListTriggerStatuses handles GET /triggers:statuses
func (h *Handler) ListTriggerStatuses(w http.ResponseWriter, r *http.Request) {
	if h.TriggerManager == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}
	statuses := h.TriggerManager.ListTriggerStatuses()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statuses)
}

// GetTriggerStatus handles GET /triggers/{id}/status
func (h *Handler) GetTriggerStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if h.TriggerManager == nil {
		http.Error(w, "trigger manager not available", http.StatusServiceUnavailable)
		return
	}

	status, err := h.TriggerManager.GetTriggerStatus(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
