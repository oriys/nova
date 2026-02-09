package controlplane

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/oriys/nova/internal/domain"
)

// CreateLayer creates a new shared dependency layer from uploaded files
func (h *Handler) CreateLayer(w http.ResponseWriter, r *http.Request) {
	if h.LayerManager == nil {
		http.Error(w, "layers not enabled", http.StatusNotImplemented)
		return
	}

	var req struct {
		Name    string            `json:"name"`
		Runtime string            `json:"runtime"`
		Files   map[string]string `json:"files"` // path -> base64-encoded content
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if len(req.Files) == 0 {
		http.Error(w, "files are required", http.StatusBadRequest)
		return
	}

	// Decode base64 files
	files := make(map[string][]byte, len(req.Files))
	for path, b64 := range req.Files {
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			http.Error(w, "invalid base64 for file "+path+": "+err.Error(), http.StatusBadRequest)
			return
		}
		files[path] = data
	}

	layer, err := h.LayerManager.BuildLayer(r.Context(), req.Name, domain.Runtime(req.Runtime), files)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(layer)
}

// ListLayers returns all shared dependency layers
func (h *Handler) ListLayers(w http.ResponseWriter, r *http.Request) {
	if h.LayerManager == nil {
		http.Error(w, "layers not enabled", http.StatusNotImplemented)
		return
	}

	layers, err := h.Store.ListLayers(r.Context(), 0, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if layers == nil {
		layers = []*domain.Layer{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(layers)
}

// GetLayer returns a single layer by name
func (h *Handler) GetLayer(w http.ResponseWriter, r *http.Request) {
	if h.LayerManager == nil {
		http.Error(w, "layers not enabled", http.StatusNotImplemented)
		return
	}

	name := r.PathValue("name")
	layer, err := h.Store.GetLayerByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(layer)
}

// DeleteLayer removes a layer if no functions reference it
func (h *Handler) DeleteLayer(w http.ResponseWriter, r *http.Request) {
	if h.LayerManager == nil {
		http.Error(w, "layers not enabled", http.StatusNotImplemented)
		return
	}

	name := r.PathValue("name")
	layer, err := h.Store.GetLayerByName(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if err := h.LayerManager.DeleteLayer(r.Context(), layer.ID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "name": name})
}

// SetFunctionLayers associates layers with a function
func (h *Handler) SetFunctionLayers(w http.ResponseWriter, r *http.Request) {
	if h.LayerManager == nil {
		http.Error(w, "layers not enabled", http.StatusNotImplemented)
		return
	}

	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found: "+name, http.StatusNotFound)
		return
	}

	var req struct {
		LayerIDs []string `json:"layer_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate layers
	if err := h.LayerManager.ValidateFunctionLayers(r.Context(), fn.ID, req.LayerIDs, fn.Runtime); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.Store.SetFunctionLayers(r.Context(), fn.ID, req.LayerIDs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch the resolved layers to show the user the ordering
	resolvedLayers, _ := h.Store.GetFunctionLayers(r.Context(), fn.ID)
	type layerInfo struct {
		Position int    `json:"position"`
		ID       string `json:"id"`
		Name     string `json:"name"`
		SizeMB   int    `json:"size_mb"`
	}
	var layerInfos []layerInfo
	for i, l := range resolvedLayers {
		layerInfos = append(layerInfos, layerInfo{
			Position: i,
			ID:       l.ID,
			Name:     l.Name,
			SizeMB:   l.SizeMB,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"function": name,
		"layers":   layerInfos,
		"note":     "Layers are mounted in position order. Position 0 has highest precedence in environment paths.",
	})
}

// GetFunctionLayers returns layers associated with a function
func (h *Handler) GetFunctionLayers(w http.ResponseWriter, r *http.Request) {
	if h.LayerManager == nil {
		http.Error(w, "layers not enabled", http.StatusNotImplemented)
		return
	}

	name := r.PathValue("name")
	fn, err := h.Store.GetFunctionByName(r.Context(), name)
	if err != nil {
		http.Error(w, "function not found: "+name, http.StatusNotFound)
		return
	}

	layers, err := h.Store.GetFunctionLayers(r.Context(), fn.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if layers == nil {
		layers = []*domain.Layer{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(layers)
}
