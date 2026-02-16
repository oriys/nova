package controlplane

import (
	"encoding/json"
	"net/http"

	"github.com/oriys/nova/internal/domain"
)

// CreateVolume handles POST /volumes
func (h *Handler) CreateVolume(w http.ResponseWriter, r *http.Request) {
	if h.VolumeManager == nil {
		http.Error(w, "volumes not enabled", http.StatusNotImplemented)
		return
	}

	var req struct {
		Name        string `json:"name"`
		SizeMB      int    `json:"size_mb"`
		Shared      bool   `json:"shared"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	if req.SizeMB <= 0 {
		http.Error(w, "size_mb must be positive", http.StatusBadRequest)
		return
	}

	vol := &domain.Volume{
		Name:        req.Name,
		SizeMB:      req.SizeMB,
		Shared:      req.Shared,
		Description: req.Description,
	}

	if err := h.VolumeManager.CreateVolume(r.Context(), vol); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(vol)
}

// GetVolume handles GET /volumes/{name}
func (h *Handler) GetVolume(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	vol, err := h.Store.GetVolumeByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vol)
}

// ListVolumes handles GET /volumes
func (h *Handler) ListVolumes(w http.ResponseWriter, r *http.Request) {
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	volumes, err := h.Store.ListVolumes(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pagedVolumes, total := paginateSliceWindow(volumes, limit, offset)
	writePaginatedList(w, limit, offset, len(pagedVolumes), int64(total), pagedVolumes)
}

// DeleteVolume handles DELETE /volumes/{name}
func (h *Handler) DeleteVolume(w http.ResponseWriter, r *http.Request) {
	if h.VolumeManager == nil {
		http.Error(w, "volumes not enabled", http.StatusNotImplemented)
		return
	}

	name := r.PathValue("name")

	vol, err := h.Store.GetVolumeByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if err := h.VolumeManager.DeleteVolume(r.Context(), vol.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SetFunctionMounts handles PUT /functions/{name}/mounts
func (h *Handler) SetFunctionMounts(w http.ResponseWriter, r *http.Request) {
	if h.VolumeManager == nil {
		http.Error(w, "volumes not enabled", http.StatusNotImplemented)
		return
	}

	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found: "+name, http.StatusNotFound)
		return
	}

	var req struct {
		Mounts []domain.VolumeMount `json:"mounts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate each mount: volume must exist and mount_path must be set
	for _, m := range req.Mounts {
		if m.VolumeID == "" {
			http.Error(w, "volume_id is required for each mount", http.StatusBadRequest)
			return
		}
		if m.MountPath == "" {
			http.Error(w, "mount_path is required for each mount", http.StatusBadRequest)
			return
		}
		vol, err := h.Store.GetVolume(r.Context(), m.VolumeID)
		if err != nil {
			http.Error(w, "volume not found: "+m.VolumeID, http.StatusBadRequest)
			return
		}
		if vol.ImagePath == "" {
			http.Error(w, "volume has no image: "+m.VolumeID, http.StatusBadRequest)
			return
		}
	}

	fn.Mounts = req.Mounts
	fn.UpdatedAt = fn.UpdatedAt // will be updated by SaveFunction
	if err := h.Store.SaveFunction(r.Context(), fn); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"function": name,
		"mounts":   fn.Mounts,
	})
}

// GetFunctionMounts handles GET /functions/{name}/mounts
func (h *Handler) GetFunctionMounts(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found: "+name, http.StatusNotFound)
		return
	}

	mounts := fn.Mounts
	if mounts == nil {
		mounts = []domain.VolumeMount{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mounts)
}
