package controlplane

import (
	"encoding/json"
	"net/http"

	"github.com/oriys/nova/internal/scheduler"
	"github.com/oriys/nova/internal/store"
)

// ScheduleHandler handles schedule management endpoints.
type ScheduleHandler struct {
	Store     *store.Store
	Scheduler *scheduler.Scheduler
}

func (h *ScheduleHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /functions/{name}/schedules", h.CreateSchedule)
	mux.HandleFunc("GET /functions/{name}/schedules", h.ListSchedules)
	mux.HandleFunc("DELETE /functions/{name}/schedules/{id}", h.DeleteSchedule)
	mux.HandleFunc("PATCH /functions/{name}/schedules/{id}", h.ToggleSchedule)
}

func (h *ScheduleHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	fnName := r.PathValue("name")
	if fnName == "" {
		http.Error(w, "function name is required", http.StatusBadRequest)
		return
	}

	// Verify function exists
	_, err := h.Store.GetFunctionByName(r.Context(), fnName)
	if err != nil {
		http.Error(w, "function not found: "+fnName, http.StatusNotFound)
		return
	}

	var req struct {
		CronExpression string          `json:"cron_expression"`
		Input          json.RawMessage `json:"input,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.CronExpression == "" {
		http.Error(w, "cron_expression is required", http.StatusBadRequest)
		return
	}

	sched := store.NewSchedule(fnName, req.CronExpression, req.Input)

	if err := h.Store.SaveSchedule(r.Context(), sched); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if h.Scheduler != nil {
		if err := h.Scheduler.Add(sched); err != nil {
			http.Error(w, "schedule saved but failed to register cron: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sched)
}

func (h *ScheduleHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	fnName := r.PathValue("name")
	if fnName == "" {
		http.Error(w, "function name is required", http.StatusBadRequest)
		return
	}

	schedules, err := h.Store.ListSchedulesByFunction(r.Context(), fnName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(schedules)
}

func (h *ScheduleHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "schedule id is required", http.StatusBadRequest)
		return
	}

	if h.Scheduler != nil {
		h.Scheduler.Remove(id)
	}

	if err := h.Store.DeleteSchedule(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "id": id})
}

func (h *ScheduleHandler) ToggleSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "schedule id is required", http.StatusBadRequest)
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.Store.UpdateScheduleEnabled(r.Context(), id, req.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if h.Scheduler != nil {
		if req.Enabled {
			sched, err := h.Store.GetSchedule(r.Context(), id)
			if err == nil {
				h.Scheduler.Add(sched)
			}
		} else {
			h.Scheduler.Remove(id)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":      id,
		"enabled": req.Enabled,
	})
}
