package controlplane

import (
	"encoding/json"
	"net/http"

	"github.com/oriys/nova/internal/store"
)

// ListRuntimes handles GET /runtimes
func (h *Handler) ListRuntimes(w http.ResponseWriter, r *http.Request) {
	runtimes, err := h.Store.ListRuntimes(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if runtimes == nil {
		runtimes = []*store.RuntimeRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runtimes)
}

// CreateRuntime handles POST /runtimes
func (h *Handler) CreateRuntime(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID            string            `json:"id"`
		Name          string            `json:"name"`
		Version       string            `json:"version"`
		Status        string            `json:"status"`
		ImageName     string            `json:"image_name"`
		Entrypoint    []string          `json:"entrypoint"`
		FileExtension string            `json:"file_extension"`
		EnvVars       map[string]string `json:"env_vars"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Version == "" {
		req.Version = "dynamic"
	}
	if req.Status == "" {
		req.Status = "available"
	}
	if req.ImageName == "" {
		http.Error(w, "image_name is required", http.StatusBadRequest)
		return
	}
	if len(req.Entrypoint) == 0 {
		http.Error(w, "entrypoint is required", http.StatusBadRequest)
		return
	}
	if req.FileExtension == "" {
		http.Error(w, "file_extension is required", http.StatusBadRequest)
		return
	}

	rt := &store.RuntimeRecord{
		ID:            req.ID,
		Name:          req.Name,
		Version:       req.Version,
		Status:        req.Status,
		ImageName:     req.ImageName,
		Entrypoint:    req.Entrypoint,
		FileExtension: req.FileExtension,
		EnvVars:       req.EnvVars,
	}

	if err := h.Store.SaveRuntime(r.Context(), rt); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rt)
}

// DeleteRuntime handles DELETE /runtimes/{id}
func (h *Handler) DeleteRuntime(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "runtime id is required", http.StatusBadRequest)
		return
	}

	if err := h.Store.DeleteRuntime(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "deleted",
		"id":     id,
	})
}
