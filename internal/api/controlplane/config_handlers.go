package controlplane

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// GetConfig handles GET /config
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	config, err := h.Store.GetConfig(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if config == nil {
		config = make(map[string]string)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// UpdateConfig handles PUT /config
func (h *Handler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var updates map[string]string
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	for key, value := range updates {
		if err := h.Store.SetConfig(r.Context(), key, value); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Apply max_global_vms to the pool if present
	if v, ok := updates["max_global_vms"]; ok && h.Pool != nil {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			h.Pool.SetMaxGlobalVMs(n)
		}
	}

	config, err := h.Store.GetConfig(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}
