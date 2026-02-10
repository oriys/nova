package controlplane

import (
"encoding/json"
"net/http"

"github.com/oriys/nova/internal/domain"
)

// CreateVolume handles POST /volumes
func (h *Handler) CreateVolume(w http.ResponseWriter, r *http.Request) {
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
volumes, err := h.Store.ListVolumes(r.Context())
if err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]interface{}{
"volumes": volumes,
"count":   len(volumes),
})
}

// DeleteVolume handles DELETE /volumes/{name}
func (h *Handler) DeleteVolume(w http.ResponseWriter, r *http.Request) {
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
