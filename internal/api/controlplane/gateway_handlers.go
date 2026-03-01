package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// GatewayHandler handles gateway route management endpoints
type GatewayHandler struct {
	Store *store.Store
}

const (
	gatewayDefaultRPSKey    = "gateway.default_rate_limit_rps"
	gatewayDefaultBurstKey  = "gateway.default_rate_limit_burst"
	gatewayDefaultEnableKey = "gateway.default_rate_limit_enabled"
	gatewayMaxRouteTimeout  = 120000
	gatewayMaxRetryBackoff  = 30000
)

type gatewayRateLimitTemplate struct {
	Enabled           bool    `json:"enabled"`
	RequestsPerSecond float64 `json:"requests_per_second"`
	BurstSize         int     `json:"burst_size"`
}

func (h *GatewayHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /gateway/routes", h.CreateRoute)
	mux.HandleFunc("GET /gateway/routes", h.ListRoutes)
	mux.HandleFunc("GET /gateway/routes/{id}", h.GetRoute)
	mux.HandleFunc("PATCH /gateway/routes/{id}", h.UpdateRoute)
	mux.HandleFunc("DELETE /gateway/routes/{id}", h.DeleteRoute)
	mux.HandleFunc("GET /gateway/rate-limit-template", h.GetRateLimitTemplate)
	mux.HandleFunc("PUT /gateway/rate-limit-template", h.UpdateRateLimitTemplate)
}

func (h *GatewayHandler) CreateRoute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain          string                       `json:"domain"`
		Path            string                       `json:"path"`
		Methods         []string                     `json:"methods,omitempty"`
		FunctionName    string                       `json:"function_name"`
		WorkflowName    string                       `json:"workflow_name"`
		AuthStrategy    string                       `json:"auth_strategy"`
		AuthConfig      map[string]string            `json:"auth_config,omitempty"`
		RequestSchema   json.RawMessage              `json:"request_schema,omitempty"`
		ParamMapping    []domain.ParamMapping        `json:"param_mapping,omitempty"`
		ResponseMapping []domain.ParamMapping        `json:"response_mapping,omitempty"`
		RateLimit       *domain.RouteRateLimit       `json:"rate_limit,omitempty"`
		TimeoutMs       *int                         `json:"timeout_ms,omitempty"`
		RetryPolicy     *domain.RouteRetryPolicy     `json:"retry_policy,omitempty"`
		IPWhitelist     []string                     `json:"ip_whitelist,omitempty"`
		IPBlacklist     []string                     `json:"ip_blacklist,omitempty"`
		MockResponse    *domain.MockResponseConfig   `json:"mock_response,omitempty"`
		ResponseHeaders map[string]string            `json:"response_headers,omitempty"`
		MaxBodyBytes    *int64                       `json:"max_body_bytes,omitempty"`
		CircuitBreaker  *domain.CircuitBreakerConfig `json:"circuit_breaker,omitempty"`
		Enabled         *bool                        `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	if req.FunctionName == "" && req.WorkflowName == "" {
		http.Error(w, "function_name or workflow_name is required", http.StatusBadRequest)
		return
	}

	// Verify target exists
	if req.FunctionName != "" {
		if _, err := h.Store.GetFunctionByName(r.Context(), req.FunctionName); err != nil {
			http.Error(w, "function not found: "+req.FunctionName, http.StatusBadRequest)
			return
		}
	}
	if req.WorkflowName != "" {
		if _, err := h.Store.GetWorkflowByName(r.Context(), req.WorkflowName); err != nil {
			http.Error(w, "workflow not found: "+req.WorkflowName, http.StatusBadRequest)
			return
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if req.AuthStrategy == "" {
		req.AuthStrategy = "none"
	}
	if req.RateLimit == nil {
		if tpl, err := h.loadRateLimitTemplate(r.Context()); err == nil && tpl.Enabled {
			req.RateLimit = &domain.RouteRateLimit{
				RequestsPerSecond: tpl.RequestsPerSecond,
				BurstSize:         tpl.BurstSize,
			}
		}
	}
	timeoutMs := 0
	if req.TimeoutMs != nil {
		timeoutMs = *req.TimeoutMs
	}
	if err := validateRouteExecutionPolicy(timeoutMs, req.RetryPolicy); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateCircuitBreaker(req.CircuitBreaker); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var maxBodyBytes int64
	if req.MaxBodyBytes != nil {
		maxBodyBytes = *req.MaxBodyBytes
	}

	now := time.Now()
	route := &domain.GatewayRoute{
		ID:              uuid.New().String()[:8],
		Domain:          req.Domain,
		Path:            req.Path,
		Methods:         req.Methods,
		FunctionName:    req.FunctionName,
		WorkflowName:    req.WorkflowName,
		AuthStrategy:    req.AuthStrategy,
		AuthConfig:      req.AuthConfig,
		RequestSchema:   req.RequestSchema,
		ParamMapping:    req.ParamMapping,
		ResponseMapping: req.ResponseMapping,
		RateLimit:       req.RateLimit,
		TimeoutMs:       timeoutMs,
		RetryPolicy:     req.RetryPolicy,
		IPWhitelist:     req.IPWhitelist,
		IPBlacklist:     req.IPBlacklist,
		MockResponse:    req.MockResponse,
		ResponseHeaders: req.ResponseHeaders,
		MaxBodyBytes:    maxBodyBytes,
		CircuitBreaker:  req.CircuitBreaker,
		Enabled:         enabled,
		CreatedAt:       now,
		UpdatedAt:       now,
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
	total := estimatePaginatedTotal(limit, offset, len(routes))
	writePaginatedList(w, limit, offset, len(routes), total, routes)
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
		Domain          *string                      `json:"domain"`
		Path            *string                      `json:"path"`
		Methods         []string                     `json:"methods"`
		FunctionName    *string                      `json:"function_name"`
		WorkflowName    *string                      `json:"workflow_name"`
		AuthStrategy    *string                      `json:"auth_strategy"`
		AuthConfig      map[string]string            `json:"auth_config"`
		RequestSchema   json.RawMessage              `json:"request_schema"`
		ParamMapping    []domain.ParamMapping        `json:"param_mapping"`
		ResponseMapping []domain.ParamMapping        `json:"response_mapping"`
		RateLimit       *domain.RouteRateLimit       `json:"rate_limit"`
		TimeoutMs       *int                         `json:"timeout_ms"`
		RetryPolicy     *domain.RouteRetryPolicy     `json:"retry_policy"`
		IPWhitelist     []string                     `json:"ip_whitelist"`
		IPBlacklist     []string                     `json:"ip_blacklist"`
		MockResponse    *domain.MockResponseConfig   `json:"mock_response"`
		ResponseHeaders map[string]string            `json:"response_headers"`
		MaxBodyBytes    *int64                       `json:"max_body_bytes"`
		CircuitBreaker  *domain.CircuitBreakerConfig `json:"circuit_breaker"`
		Enabled         *bool                        `json:"enabled"`
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
	if req.WorkflowName != nil {
		if *req.WorkflowName != "" {
			if _, err := h.Store.GetWorkflowByName(r.Context(), *req.WorkflowName); err != nil {
				http.Error(w, "workflow not found: "+*req.WorkflowName, http.StatusBadRequest)
				return
			}
		}
		existing.WorkflowName = *req.WorkflowName
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
	if req.ParamMapping != nil {
		existing.ParamMapping = req.ParamMapping
	}
	if req.ResponseMapping != nil {
		existing.ResponseMapping = req.ResponseMapping
	}
	if req.RateLimit != nil {
		existing.RateLimit = req.RateLimit
	}
	if req.TimeoutMs != nil {
		existing.TimeoutMs = *req.TimeoutMs
	}
	if req.RetryPolicy != nil {
		existing.RetryPolicy = req.RetryPolicy
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.IPWhitelist != nil {
		existing.IPWhitelist = req.IPWhitelist
	}
	if req.IPBlacklist != nil {
		existing.IPBlacklist = req.IPBlacklist
	}
	if req.MockResponse != nil {
		existing.MockResponse = req.MockResponse
	}
	if req.ResponseHeaders != nil {
		existing.ResponseHeaders = req.ResponseHeaders
	}
	if req.MaxBodyBytes != nil {
		existing.MaxBodyBytes = *req.MaxBodyBytes
	}
	if req.CircuitBreaker != nil {
		existing.CircuitBreaker = req.CircuitBreaker
	}
	if err := validateRouteExecutionPolicy(existing.TimeoutMs, existing.RetryPolicy); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateCircuitBreaker(existing.CircuitBreaker); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
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

func (h *GatewayHandler) GetRateLimitTemplate(w http.ResponseWriter, r *http.Request) {
	tpl, err := h.loadRateLimitTemplate(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tpl)
}

func (h *GatewayHandler) UpdateRateLimitTemplate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled           *bool    `json:"enabled"`
		RequestsPerSecond *float64 `json:"requests_per_second"`
		BurstSize         *int     `json:"burst_size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	current, err := h.loadRateLimitTemplate(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	next := current
	if req.Enabled != nil {
		next.Enabled = *req.Enabled
	}
	if req.RequestsPerSecond != nil {
		next.RequestsPerSecond = *req.RequestsPerSecond
	}
	if req.BurstSize != nil {
		next.BurstSize = *req.BurstSize
	}

	if next.RequestsPerSecond < 0 {
		http.Error(w, "requests_per_second must be >= 0", http.StatusBadRequest)
		return
	}
	if next.BurstSize < 0 {
		http.Error(w, "burst_size must be >= 0", http.StatusBadRequest)
		return
	}

	if !next.Enabled || next.RequestsPerSecond <= 0 || next.BurstSize <= 0 {
		next.Enabled = false
		next.RequestsPerSecond = 0
		next.BurstSize = 0
	}

	if err := h.Store.SetConfig(r.Context(), gatewayDefaultEnableKey, strconv.FormatBool(next.Enabled)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.Store.SetConfig(r.Context(), gatewayDefaultRPSKey, strconv.FormatFloat(next.RequestsPerSecond, 'f', -1, 64)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.Store.SetConfig(r.Context(), gatewayDefaultBurstKey, strconv.Itoa(next.BurstSize)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(next)
}

func (h *GatewayHandler) loadRateLimitTemplate(ctx context.Context) (*gatewayRateLimitTemplate, error) {
	cfg, err := h.Store.GetConfig(ctx)
	if err != nil {
		return nil, err
	}

	tpl := &gatewayRateLimitTemplate{}
	if v, ok := cfg[gatewayDefaultEnableKey]; ok {
		tpl.Enabled = strings.EqualFold(strings.TrimSpace(v), "true")
	}
	if v, ok := cfg[gatewayDefaultRPSKey]; ok {
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && parsed > 0 {
			tpl.RequestsPerSecond = parsed
		}
	}
	if v, ok := cfg[gatewayDefaultBurstKey]; ok {
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && parsed > 0 {
			tpl.BurstSize = parsed
		}
	}

	if tpl.RequestsPerSecond <= 0 || tpl.BurstSize <= 0 {
		tpl.Enabled = false
		tpl.RequestsPerSecond = 0
		tpl.BurstSize = 0
	}

	return tpl, nil
}

func validateRouteExecutionPolicy(timeoutMs int, retry *domain.RouteRetryPolicy) error {
	if timeoutMs < 0 || timeoutMs > gatewayMaxRouteTimeout {
		return fmt.Errorf("timeout_ms must be between 0 and %d", gatewayMaxRouteTimeout)
	}
	if retry == nil {
		return nil
	}
	if retry.MaxAttempts < 1 || retry.MaxAttempts > domain.MaxRouteRetryAttempts {
		return fmt.Errorf("retry_policy.max_attempts must be between 1 and %d", domain.MaxRouteRetryAttempts)
	}
	if retry.BackoffMs < 0 || retry.BackoffMs > gatewayMaxRetryBackoff {
		return fmt.Errorf("retry_policy.backoff_ms must be between 0 and %d", gatewayMaxRetryBackoff)
	}
	return nil
}

func validateCircuitBreaker(cb *domain.CircuitBreakerConfig) error {
	if cb == nil {
		return nil
	}
	if cb.MaxFailures < 1 || cb.MaxFailures > 1000 {
		return fmt.Errorf("circuit_breaker.max_failures must be between 1 and 1000")
	}
	if cb.TimeoutSec < 1 || cb.TimeoutSec > 3600 {
		return fmt.Errorf("circuit_breaker.timeout_sec must be between 1 and 3600")
	}
	return nil
}
