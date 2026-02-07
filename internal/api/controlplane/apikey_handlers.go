package controlplane

import (
	"encoding/json"
	"net/http"

	"github.com/oriys/nova/internal/auth"
)

// APIKeyHandler handles API key management endpoints.
type APIKeyHandler struct {
	Manager *auth.APIKeyManager
}

func (h *APIKeyHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /apikeys", h.CreateAPIKey)
	mux.HandleFunc("GET /apikeys", h.ListAPIKeys)
	mux.HandleFunc("DELETE /apikeys/{name}", h.DeleteAPIKey)
	mux.HandleFunc("PATCH /apikeys/{name}", h.ToggleAPIKey)
}

func (h *APIKeyHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Tier string `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	key, err := h.Manager.Create(r.Context(), req.Name, req.Tier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	tier := req.Tier
	if tier == "" {
		tier = "default"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"name": req.Name,
		"key":  key,
		"tier": tier,
	})
}

func (h *APIKeyHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.Manager.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type apiKeyResponse struct {
		Name      string `json:"name"`
		Tier      string `json:"tier"`
		Enabled   bool   `json:"enabled"`
		CreatedAt string `json:"created_at"`
	}

	result := make([]apiKeyResponse, len(keys))
	for i, k := range keys {
		result[i] = apiKeyResponse{
			Name:      k.Name,
			Tier:      k.Tier,
			Enabled:   k.Enabled,
			CreatedAt: k.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *APIKeyHandler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	if err := h.Manager.Delete(r.Context(), name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "name": name})
}

func (h *APIKeyHandler) ToggleAPIKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Enabled {
		if err := h.Manager.Enable(r.Context(), name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		if err := h.Manager.Revoke(r.Context(), name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"name":    name,
		"enabled": req.Enabled,
	})
}
