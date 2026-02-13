package controlplane

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/oriys/nova/internal/secrets"
)

// SecretHandler handles secret management endpoints.
type SecretHandler struct {
	Store *secrets.Store
}

func (h *SecretHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /secrets", h.CreateSecret)
	mux.HandleFunc("GET /secrets", h.ListSecrets)
	mux.HandleFunc("DELETE /secrets/{name}", h.DeleteSecret)
}

func (h *SecretHandler) CreateSecret(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Value == "" {
		http.Error(w, "value is required", http.StatusBadRequest)
		return
	}

	if err := h.Store.Set(r.Context(), req.Name, []byte(req.Value)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"name":   req.Name,
		"status": "created",
	})
}

func (h *SecretHandler) ListSecrets(w http.ResponseWriter, r *http.Request) {
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	secretsMap, err := h.Store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type secretEntry struct {
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
	}

	result := make([]secretEntry, 0, len(secretsMap))
	for name, createdAt := range secretsMap {
		result = append(result, secretEntry{
			Name:      name,
			CreatedAt: createdAt,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	pagedResult, total := paginateSliceWindow(result, limit, offset)
	writePaginatedList(w, limit, offset, len(pagedResult), int64(total), pagedResult)
}

func (h *SecretHandler) DeleteSecret(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	if err := h.Store.Delete(r.Context(), name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "name": name})
}
