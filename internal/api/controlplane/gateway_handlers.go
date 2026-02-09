package controlplane

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// GatewayHandler handles gateway route management endpoints
type GatewayHandler struct {
	Store *store.Store
}

func (h *GatewayHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /gateway/routes", h.CreateRoute)
	mux.HandleFunc("GET /gateway/routes", h.ListRoutes)
	mux.HandleFunc("GET /gateway/routes/{id}", h.GetRoute)
	mux.HandleFunc("PATCH /gateway/routes/{id}", h.UpdateRoute)
	mux.HandleFunc("DELETE /gateway/routes/{id}", h.DeleteRoute)
}

func (h *GatewayHandler) CreateRoute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain        string            `json:"domain"`
		Path          string            `json:"path"`
		Methods       []string          `json:"methods,omitempty"`
		FunctionName  string            `json:"function_name"`
		AuthStrategy  string            `json:"auth_strategy"`
		AuthConfig    map[string]string `json:"auth_config,omitempty"`
		RequestSchema json.RawMessage   `json:"request_schema,omitempty"`
		RateLimit     *domain.RouteRateLimit `json:"rate_limit,omitempty"`
		Enabled       *bool             `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	if req.FunctionName == "" {
		http.Error(w, "function_name is required", http.StatusBadRequest)
		return
	}

	// Verify function exists
	_, err := h.Store.GetFunctionByName(r.Context(), req.FunctionName)
	if err != nil {
		http.Error(w, "function not found: "+req.FunctionName, http.StatusBadRequest)
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if req.AuthStrategy == "" {
		req.AuthStrategy = "none"
	}

	now := time.Now()
	route := &domain.GatewayRoute{
		ID:            uuid.New().String()[:8],
		Domain:        req.Domain,
		Path:          req.Path,
		Methods:       req.Methods,
		FunctionName:  req.FunctionName,
		AuthStrategy:  req.AuthStrategy,
		AuthConfig:    req.AuthConfig,
		RequestSchema: req.RequestSchema,
		RateLimit:     req.RateLimit,
		Enabled:       enabled,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := h.Store.SaveGatewayRoute(r.Context(), route); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(route)
}

func (h *GatewayHandler) ListRoutes(w http.ResponseWriter, r *http.Request) {
	domainFilter := r.URL.Query().Get("domain")
	limit := parsePaginationParam(r.URL.Query().Get("limit"), 100, 500)
	offset := parsePaginationParam(r.URL.Query().Get("offset"), 0, 0)

	var routes []*domain.GatewayRoute
	var err error
	if domainFilter != "" {
		routes, err = h.Store.ListRoutesByDomain(r.Context(), domainFilter, limit, offset)
	} else {
		routes, err = h.Store.ListGatewayRoutes(r.Context(), limit, offset)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if routes == nil {
		routes = []*domain.GatewayRoute{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(routes)
}

func (h *GatewayHandler) GetRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	route, err := h.Store.GetGatewayRoute(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(route)
}

func (h *GatewayHandler) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	existing, err := h.Store.GetGatewayRoute(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var req struct {
		Domain        *string            `json:"domain"`
		Path          *string            `json:"path"`
		Methods       []string           `json:"methods"`
		FunctionName  *string            `json:"function_name"`
		AuthStrategy  *string            `json:"auth_strategy"`
		AuthConfig    map[string]string  `json:"auth_config"`
		RequestSchema json.RawMessage    `json:"request_schema"`
		RateLimit     *domain.RouteRateLimit `json:"rate_limit"`
		Enabled       *bool              `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Domain != nil {
		existing.Domain = *req.Domain
	}
	if req.Path != nil {
		existing.Path = *req.Path
	}
	if req.Methods != nil {
		existing.Methods = req.Methods
	}
	if req.FunctionName != nil {
		// Verify function exists
		if _, err := h.Store.GetFunctionByName(r.Context(), *req.FunctionName); err != nil {
			http.Error(w, "function not found: "+*req.FunctionName, http.StatusBadRequest)
			return
		}
		existing.FunctionName = *req.FunctionName
	}
	if req.AuthStrategy != nil {
		existing.AuthStrategy = *req.AuthStrategy
	}
	if req.AuthConfig != nil {
		existing.AuthConfig = req.AuthConfig
	}
	if req.RequestSchema != nil {
		existing.RequestSchema = req.RequestSchema
	}
	if req.RateLimit != nil {
		existing.RateLimit = req.RateLimit
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	if err := h.Store.UpdateGatewayRoute(r.Context(), id, existing); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(existing)
}

func (h *GatewayHandler) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.Store.DeleteGatewayRoute(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "id": id})
}
