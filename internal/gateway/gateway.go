package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

// GatewayStore is the interface the gateway needs from the store
type GatewayStore interface {
	ListGatewayRoutes(ctx context.Context, limit, offset int) ([]*domain.GatewayRoute, error)
	GetRouteByDomainPath(ctx context.Context, domain, path string) (*domain.GatewayRoute, error)
}

// WorkflowInvoker triggers a workflow run. Implemented by *workflow.Service.
type WorkflowInvoker interface {
	TriggerRun(ctx context.Context, workflowName string, input json.RawMessage, triggerType string) (*domain.WorkflowRun, error)
}

// Gateway handles domain-based routing to functions
type Gateway struct {
	store           GatewayStore
	exec            *executor.Executor
	workflowSvc     WorkflowInvoker      // optional: for workflow routes
	authenticators  []auth.Authenticator // global authenticators for "inherit" strategy
	routes          sync.Map             // "domain:path" -> *domain.GatewayRoute
	rateLimiters    sync.Map             // route ID -> *rateLimiter
	circuitBreakers sync.Map             // route ID -> *circuitState
	schemas         sync.Map             // route ID -> *compiledSchema
	paramRoutes     sync.Map             // "domain" -> []*paramRoute (routes with path parameters)
}

// compiledSchema holds a pre-parsed JSON Schema for fast validation
type compiledSchema struct {
	schema map[string]any
}

// paramRoute holds a route with path parameters (e.g. "/v1/users/{id}")
type paramRoute struct {
	segments []string // e.g. ["v1", "users", "{id}"]
	route    *domain.GatewayRoute
}

type rateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
}

func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens = min(rl.maxTokens, rl.tokens+elapsed*rl.refillRate)
	rl.lastRefill = now

	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}
	return false
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// circuitState tracks consecutive failures for a route's circuit breaker.
type circuitState struct {
	failures  atomic.Int64
	openUntil atomic.Int64 // unix nano; 0 = closed
}

// isOpen returns true if the circuit is currently open (blocking requests).
func (cs *circuitState) isOpen() bool {
	until := cs.openUntil.Load()
	if until == 0 {
		return false
	}
	return time.Now().UnixNano() < until
}

// recordSuccess resets the failure counter and closes the circuit.
func (cs *circuitState) recordSuccess() {
	cs.failures.Store(0)
	cs.openUntil.Store(0)
}

// recordFailure increments failures and opens the circuit if threshold is reached.
func (cs *circuitState) recordFailure(maxFailures, timeoutSec int) {
	count := cs.failures.Add(1)
	if int(count) >= maxFailures {
		cs.openUntil.Store(time.Now().Add(time.Duration(timeoutSec) * time.Second).UnixNano())
		cs.failures.Store(0) // reset counter for next cycle
	}
}

// New creates a new Gateway
func New(store GatewayStore, exec *executor.Executor, authenticators []auth.Authenticator, opts ...GatewayOption) *Gateway {
	gw := &Gateway{
		store:          store,
		exec:           exec,
		authenticators: authenticators,
	}
	for _, opt := range opts {
		opt(gw)
	}
	return gw
}

// GatewayOption configures optional Gateway dependencies.
type GatewayOption func(*Gateway)

// WithWorkflowInvoker sets the workflow invoker for workflow-based gateway routes.
func WithWorkflowInvoker(svc WorkflowInvoker) GatewayOption {
	return func(g *Gateway) { g.workflowSvc = svc }
}

// ReloadRoutes refreshes the in-memory route cache from the database
func (g *Gateway) ReloadRoutes(ctx context.Context) error {
	routes, err := g.store.ListGatewayRoutes(ctx, 0, 0)
	if err != nil {
		return err
	}

	// Clear old entries
	g.routes.Range(func(key, value any) bool {
		g.routes.Delete(key)
		return true
	})
	g.schemas.Range(func(key, value any) bool {
		g.schemas.Delete(key)
		return true
	})
	g.paramRoutes.Range(func(key, value any) bool {
		g.paramRoutes.Delete(key)
		return true
	})

	// Temporary map to build paramRoutes per domain
	paramMap := make(map[string][]*paramRoute)

	for _, route := range routes {
		if !route.Enabled {
			continue
		}

		// Pre-compile request schema
		if len(route.RequestSchema) > 0 {
			var schema map[string]any
			if err := json.Unmarshal(route.RequestSchema, &schema); err == nil {
				g.schemas.Store(route.ID, &compiledSchema{schema: schema})
			}
		}

		// Check if path has parameters (e.g. "/v1/users/{id}")
		if strings.Contains(route.Path, "{") {
			segments := splitPath(route.Path)
			pr := &paramRoute{segments: segments, route: route}
			paramMap[route.Domain] = append(paramMap[route.Domain], pr)
		}

		key := route.Domain + ":" + route.Path
		g.routes.Store(key, route)
	}

	for dom, prs := range paramMap {
		g.paramRoutes.Store(dom, prs)
	}

	logging.Op().Info("gateway routes reloaded", "count", len(routes))
	return nil
}

// KnownDomains returns the set of domains that have gateway routes
func (g *Gateway) KnownDomains() map[string]struct{} {
	domains := make(map[string]struct{})
	g.routes.Range(func(key, value any) bool {
		route := value.(*domain.GatewayRoute)
		if route.Domain != "" {
			domains[route.Domain] = struct{}{}
		}
		return true
	})
	return domains
}

// ServeHTTP handles incoming gateway requests
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := extractHost(r)
	route, pathParams := g.matchRouteWithParams(host, r.URL.Path, r.Method)
	if route == nil {
		http.Error(w, `{"error":"not_found","message":"no matching gateway route"}`, http.StatusNotFound)
		return
	}

	// ── IP whitelist / blacklist ─────────────────────────────────────────
	if len(route.IPBlacklist) > 0 || len(route.IPWhitelist) > 0 {
		clientIP := extractClientIP(r)
		if clientIP != nil {
			if len(route.IPBlacklist) > 0 && ipMatchesAny(clientIP, route.IPBlacklist) {
				writeJSON(w, http.StatusForbidden, `{"error":"forbidden","message":"IP address is blocked"}`)
				return
			}
			if len(route.IPWhitelist) > 0 && !ipMatchesAny(clientIP, route.IPWhitelist) {
				writeJSON(w, http.StatusForbidden, `{"error":"forbidden","message":"IP address is not allowed"}`)
				return
			}
		}
	}

	// Handle CORS
	if route.CORS != nil {
		if r.Method == http.MethodOptions {
			g.handlePreflight(w, r, route)
			return
		}
		g.setCORSHeaders(w, r, route)
	}

	// Check HTTP method
	if len(route.Methods) > 0 && !methodAllowed(route.Methods, r.Method) {
		w.Header().Set("Allow", strings.Join(route.Methods, ", "))
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Authentication based on route strategy
	if err := g.authenticateRequest(route, w, r); err != nil {
		return // authenticateRequest already wrote the error response
	}

	// Rate limiting
	if route.RateLimit != nil {
		rl := g.getOrCreateLimiter(route)
		if !rl.allow() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate_limit_exceeded","message":"too many requests for this route"}`))
			return
		}
	}

	// ── Mock response — return fixed response without calling backend ────
	if route.MockResponse != nil {
		if err := g.writeMockResponse(w, route); err != nil {
			g.writeResponseMappingError(w, err)
		}
		return
	}

	// ── Circuit breaker check ────────────────────────────────────────────
	if route.CircuitBreaker != nil {
		cs := g.getOrCreateBreaker(route)
		if cs.isOpen() {
			writeJSON(w, http.StatusServiceUnavailable, `{"error":"circuit_open","message":"route is temporarily unavailable due to repeated failures"}`)
			return
		}
	}

	// ── Request body validation ──────────────────────────────────────────
	maxBody := int64(10 << 20) // default 10MB
	if route.MaxBodyBytes > 0 {
		maxBody = route.MaxBodyBytes
	}
	var payload json.RawMessage
	if r.ContentLength > 0 {
		if r.ContentLength > maxBody {
			writeJSON(w, http.StatusRequestEntityTooLarge, fmt.Sprintf(`{"error":"request_too_large","message":"body exceeds %d bytes limit"}`, maxBody))
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
		if err != nil {
			http.Error(w, `{"error":"read_body_failed"}`, http.StatusBadRequest)
			return
		}
		if int64(len(body)) > maxBody {
			writeJSON(w, http.StatusRequestEntityTooLarge, fmt.Sprintf(`{"error":"request_too_large","message":"body exceeds %d bytes limit"}`, maxBody))
			return
		}

		if len(route.RequestSchema) > 0 {
			// Use pre-compiled schema if available
			if cs, ok := g.schemas.Load(route.ID); ok {
				if err := validateValueDirect("$", cs.(*compiledSchema).schema, body); err != nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]string{
						"error":   "validation_failed",
						"message": FormatValidationError(err),
					})
					return
				}
			} else if err := ValidateRequestBody(route.RequestSchema, body); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{
					"error":   "validation_failed",
					"message": FormatValidationError(err),
				})
				return
			}
		}
		payload = body
	} else {
		payload = json.RawMessage("{}")
	}

	// Inject path parameters into payload if present
	if len(pathParams) > 0 {
		payload = injectPathParams(payload, pathParams)
	}

	// Apply parameter mapping rules (query, path, body, header → payload)
	if len(route.ParamMapping) > 0 {
		mapped, err := applyParamMappings(payload, r, pathParams, route.ParamMapping)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "param_mapping_failed",
				"message": err.Error(),
			})
			return
		}
		payload = mapped
	}

	// ── Execute function or workflow ─────────────────────────────────────
	invokeCtx := r.Context()
	var cancel context.CancelFunc
	if route.TimeoutMs > 0 {
		invokeCtx, cancel = context.WithTimeout(invokeCtx, time.Duration(route.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	if route.WorkflowName != "" {
		// Workflow route
		if g.workflowSvc == nil {
			http.Error(w, `{"error":"not_configured","message":"workflow invocation not available"}`, http.StatusServiceUnavailable)
			return
		}
		run, err := g.workflowSvc.TriggerRun(invokeCtx, route.WorkflowName, payload, "gateway")
		if err != nil {
			g.recordCircuitResult(route, false)
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(invokeCtx.Err(), context.DeadlineExceeded) {
				http.Error(w, `{"error":"invoke_timeout","message":"gateway route timed out"}`, http.StatusGatewayTimeout)
				return
			}
			http.Error(w, `{"error":"workflow_failed","message":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		g.recordCircuitResult(route, true)
		body, err := json.Marshal(run)
		if err != nil {
			http.Error(w, `{"error":"response_encode_failed","message":"failed to encode workflow response"}`, http.StatusInternalServerError)
			return
		}
		body, err = g.mapRouteResponsePayload(route, body)
		if err != nil {
			g.writeResponseMappingError(w, err)
			return
		}
		setResponseHeaders(w, route)
		writeJSONPayload(w, http.StatusOK, body)
		return
	}

	// Function route
	attempts, backoff := routeExecutionPolicy(route)
	resp, err := invokeWithRetry(invokeCtx, attempts, backoff, func(ctx context.Context) (*domain.InvokeResponse, error) {
		return g.exec.Invoke(ctx, route.FunctionName, payload)
	})
	if err != nil {
		g.recordCircuitResult(route, false)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(invokeCtx.Err(), context.DeadlineExceeded) {
			http.Error(w, `{"error":"invoke_timeout","message":"gateway route timed out"}`, http.StatusGatewayTimeout)
			return
		}
		http.Error(w, `{"error":"invoke_failed","message":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	g.recordCircuitResult(route, true)

	if len(route.ResponseMapping) > 0 {
		mappedOutput, err := g.mapRouteResponsePayload(route, resp.Output)
		if err != nil {
			g.writeResponseMappingError(w, err)
			return
		}
		resp.Output = mappedOutput
	}

	body, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, `{"error":"response_encode_failed","message":"failed to encode function response"}`, http.StatusInternalServerError)
		return
	}

	setResponseHeaders(w, route)
	writeJSONPayload(w, http.StatusOK, body)
}

// matchRouteWithParams finds a route and extracts path parameters.
func (g *Gateway) matchRouteWithParams(host, reqPath, method string) (*domain.GatewayRoute, map[string]string) {
	// Try exact domain+path match first
	key := host + ":" + reqPath
	if v, ok := g.routes.Load(key); ok {
		return v.(*domain.GatewayRoute), nil
	}

	// Try parameterized routes for this domain
	if v, ok := g.paramRoutes.Load(host); ok {
		prs := v.([]*paramRoute)
		reqSegments := splitPath(reqPath)
		for _, pr := range prs {
			if params, ok := matchParamRoute(pr.segments, reqSegments); ok {
				return pr.route, params
			}
		}
	}

	// Try prefix matching: walk up path segments
	p := reqPath
	for p != "" && p != "/" {
		idx := strings.LastIndex(p, "/")
		if idx <= 0 {
			break
		}
		p = p[:idx]
		key = host + ":" + p
		if v, ok := g.routes.Load(key); ok {
			return v.(*domain.GatewayRoute), nil
		}
	}

	// Try root path
	key = host + ":/"
	if v, ok := g.routes.Load(key); ok {
		return v.(*domain.GatewayRoute), nil
	}

	// Fall back to database lookup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	route, err := g.store.GetRouteByDomainPath(ctx, host, reqPath)
	if err != nil || route == nil {
		return nil, nil
	}
	// Cache it
	cacheKey := route.Domain + ":" + route.Path
	g.routes.Store(cacheKey, route)
	return route, nil
}

func (g *Gateway) authenticateRequest(route *domain.GatewayRoute, w http.ResponseWriter, r *http.Request) error {
	switch route.AuthStrategy {
	case "none", "":
		return nil

	case "inherit":
		for _, authenticator := range g.authenticators {
			if id := authenticator.Authenticate(r); id != nil {
				return bindGatewayIdentityScope(w, r, id)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("WWW-Authenticate", `Bearer realm="nova-gateway"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized","message":"authentication required"}`))
		return errUnauthorized

	case "apikey":
		key := r.Header.Get("X-API-Key")
		if key == "" {
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "ApiKey " {
				key = authHeader[7:]
			}
		}
		if key == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized","message":"API key required"}`))
			return errUnauthorized
		}
		// Delegate to existing API key authenticators
		requireExplicitScope := routeRequiresExplicitAPIKeyScope(route)
		for _, authenticator := range g.authenticators {
			if id := authenticator.Authenticate(r); id != nil {
				// Check that the API key's policy allows access to the route target
				if !policyAllowsRouteTarget(id.Policies, route.FunctionName, route.WorkflowName, requireExplicitScope) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					w.Write([]byte(`{"error":"forbidden","message":"API key does not have permission for this resource"}`))
					return errUnauthorized
				}
				return bindGatewayIdentityScope(w, r, id)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized","message":"invalid API key"}`))
		return errUnauthorized

	case "jwt":
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("WWW-Authenticate", `Bearer realm="nova-gateway"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized","message":"Bearer token required"}`))
			return errUnauthorized
		}
		for _, authenticator := range g.authenticators {
			if id := authenticator.Authenticate(r); id != nil {
				return bindGatewayIdentityScope(w, r, id)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized","message":"invalid token"}`))
		return errUnauthorized
	}

	return nil
}

func bindGatewayIdentityScope(w http.ResponseWriter, r *http.Request, identity *auth.Identity) error {
	requestedTenant, requestedNamespace, explicit, err := gatewayScopeFromHeaders(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"tenant_scope_error","message":"invalid tenant scope headers"}`))
		return errUnauthorized
	}

	effectiveTenant := requestedTenant
	effectiveNamespace := requestedNamespace

	if identity != nil && identity.ScopeRestricted() {
		if !explicit {
			primary, ok := identity.PrimaryScope()
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"forbidden","message":"tenant scope is required"}`))
				return errUnauthorized
			}
			if primary.TenantID == "*" || primary.Namespace == "*" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"tenant_scope_error","message":"explicit X-Nova-Tenant and X-Nova-Namespace headers are required"}`))
				return errUnauthorized
			}
			effectiveTenant = primary.TenantID
			effectiveNamespace = primary.Namespace
		}

		if !identity.AllowsScope(effectiveTenant, effectiveNamespace) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"forbidden","message":"tenant scope is not allowed for this identity"}`))
			return errUnauthorized
		}
	}

	ctx := auth.WithIdentity(r.Context(), identity)
	ctx = store.WithTenantScope(ctx, effectiveTenant, effectiveNamespace)
	*r = *r.WithContext(ctx)
	return nil
}

func gatewayScopeFromHeaders(r *http.Request) (tenantID string, namespace string, explicit bool, err error) {
	tenantID = strings.TrimSpace(r.Header.Get("X-Nova-Tenant"))
	namespace = strings.TrimSpace(r.Header.Get("X-Nova-Namespace"))

	if tenantID == "" && namespace == "" {
		return "", "", false, nil
	}
	explicit = true

	if tenantID == "" {
		tenantID = store.DefaultTenantID
	}
	if namespace == "" {
		namespace = store.DefaultNamespace
	}
	if !store.IsValidTenantScopePart(tenantID) || !store.IsValidTenantScopePart(namespace) {
		return "", "", true, fmt.Errorf("invalid tenant scope headers")
	}
	return tenantID, namespace, true, nil
}

func (g *Gateway) getOrCreateLimiter(route *domain.GatewayRoute) *rateLimiter {
	if v, ok := g.rateLimiters.Load(route.ID); ok {
		return v.(*rateLimiter)
	}
	rl := &rateLimiter{
		tokens:     float64(route.RateLimit.BurstSize),
		maxTokens:  float64(route.RateLimit.BurstSize),
		refillRate: route.RateLimit.RequestsPerSecond,
		lastRefill: time.Now(),
	}
	actual, _ := g.rateLimiters.LoadOrStore(route.ID, rl)
	return actual.(*rateLimiter)
}

func extractHost(r *http.Request) string {
	host := r.Host
	// Strip port
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		// Make sure it's not an IPv6 address
		if !strings.Contains(host, "]") || idx > strings.Index(host, "]") {
			host = host[:idx]
		}
	}
	return strings.ToLower(host)
}

func methodAllowed(allowed []string, method string) bool {
	for _, m := range allowed {
		if strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}

var errUnauthorized = &unauthorizedError{}

type unauthorizedError struct{}

func (e *unauthorizedError) Error() string { return "unauthorized" }

// splitPath splits a URL path into segments, ignoring leading slash.
func splitPath(p string) []string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// matchParamRoute matches request segments against a parameterized route pattern.
// Returns extracted parameters on match, e.g. {id} -> "123".
func matchParamRoute(pattern, segments []string) (map[string]string, bool) {
	if len(pattern) != len(segments) {
		return nil, false
	}
	params := make(map[string]string)
	for i, ps := range pattern {
		if strings.HasPrefix(ps, "{") && strings.HasSuffix(ps, "}") {
			name := ps[1 : len(ps)-1]
			params[name] = segments[i]
		} else if ps != segments[i] {
			return nil, false
		}
	}
	return params, true
}

// injectPathParams merges path parameters into the JSON payload under a "pathParams" key.
func injectPathParams(payload json.RawMessage, params map[string]string) json.RawMessage {
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		obj = make(map[string]any)
	}
	obj["pathParams"] = params
	out, err := json.Marshal(obj)
	if err != nil {
		return payload
	}
	return out
}

// validateValueDirect validates a JSON body against a pre-compiled schema map.
func validateValueDirect(path string, schema map[string]any, body json.RawMessage) error {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return validateValue(path, schema, value)
}

// handlePreflight responds to CORS preflight (OPTIONS) requests.
func (g *Gateway) handlePreflight(w http.ResponseWriter, r *http.Request, route *domain.GatewayRoute) {
	cors := route.CORS
	origin := r.Header.Get("Origin")
	if origin == "" || !originAllowed(cors.AllowOrigins, origin) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", origin)
	methods := cors.AllowMethods
	if len(methods) == 0 {
		methods = route.Methods
	}
	if len(methods) == 0 {
		methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ", "))
	if len(cors.AllowHeaders) > 0 {
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(cors.AllowHeaders, ", "))
	} else {
		// Mirror the request's Access-Control-Request-Headers
		if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
			w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
		}
	}
	if cors.AllowCredentials {
		// CORS spec forbids Access-Control-Allow-Credentials with wildcard origin.
		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
	}
	if cors.MaxAge > 0 {
		w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cors.MaxAge))
	}
	w.WriteHeader(http.StatusNoContent)
}

// setCORSHeaders sets CORS response headers for non-preflight requests.
func (g *Gateway) setCORSHeaders(w http.ResponseWriter, r *http.Request, route *domain.GatewayRoute) {
	cors := route.CORS
	origin := r.Header.Get("Origin")
	if origin == "" || !originAllowed(cors.AllowOrigins, origin) {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	if cors.AllowCredentials {
		// CORS spec forbids Access-Control-Allow-Credentials with wildcard origin.
		if origin != "*" {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
	}
	if len(cors.ExposeHeaders) > 0 {
		w.Header().Set("Access-Control-Expose-Headers", strings.Join(cors.ExposeHeaders, ", "))
	}
}

// originAllowed checks if the request origin is in the allowed list.
// Wildcard "*" is rejected when credentials are in use (enforced at the
// call-site level in handlePreflight/setCORSHeaders); here we simply
// match the origin against the allow-list.
func originAllowed(allowed []string, origin string) bool {
	for _, a := range allowed {
		if a == "*" {
			return true
		}
		if strings.EqualFold(a, origin) {
			return true
		}
	}
	return false
}

func routeExecutionPolicy(route *domain.GatewayRoute) (attempts int, backoff time.Duration) {
	attempts = 1
	if route == nil || route.RetryPolicy == nil {
		return attempts, 0
	}
	attempts = route.RetryPolicy.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	if attempts > domain.MaxRouteRetryAttempts {
		attempts = domain.MaxRouteRetryAttempts
	}
	if route.RetryPolicy.BackoffMs > 0 {
		backoff = time.Duration(route.RetryPolicy.BackoffMs) * time.Millisecond
	}
	return attempts, backoff
}

func invokeWithRetry(
	ctx context.Context,
	attempts int,
	backoff time.Duration,
	invoke func(context.Context) (*domain.InvokeResponse, error),
) (*domain.InvokeResponse, error) {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := invoke(ctx)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if attempt == attempts {
			break
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if backoff <= 0 {
			continue
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

// policyAllowsRouteTarget checks whether the identity's PolicyBindings permit
// access to the given function and/or workflow. If no policies exist (empty slice),
// the key is unrestricted and access is allowed. If policies exist, at least one
// "allow" binding must cover the target, and no "deny" binding must block it.
func policyAllowsRouteTarget(policies []domain.PolicyBinding, functionName, workflowName string, requireExplicitScope bool) bool {
	if len(policies) == 0 {
		return !requireExplicitScope
	}

	for _, p := range policies {
		if p.Effect == domain.EffectDeny {
			if functionName != "" && matchesAny(p.Functions, functionName) {
				return false
			}
			if workflowName != "" && matchesAny(p.Workflows, workflowName) {
				return false
			}
		}
	}

	for _, p := range policies {
		if p.Effect == domain.EffectDeny {
			continue
		}
		// Effect is "allow" or empty (default allow)
		fnOk := functionName == "" || len(p.Functions) == 0 || matchesAny(p.Functions, functionName)
		wfOk := workflowName == "" || len(p.Workflows) == 0 || matchesAny(p.Workflows, workflowName)
		if fnOk && wfOk {
			return true
		}
	}
	return false
}

func routeRequiresExplicitAPIKeyScope(route *domain.GatewayRoute) bool {
	if route == nil || len(route.AuthConfig) == 0 {
		return false
	}

	raw, ok := route.AuthConfig["require_explicit_scope"]
	if !ok {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

// matchesAny checks if name matches any pattern in the list (supports glob via filepath.Match).
func matchesAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
	}
	return false
}

// ── IP whitelist / blacklist helpers ─────────────────────────────────────────

// extractClientIP returns the client IP from X-Forwarded-For, X-Real-IP, or RemoteAddr.
func extractClientIP(r *http.Request) net.IP {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First IP in the chain is the original client
		if idx := strings.IndexByte(xff, ','); idx > 0 {
			xff = xff[:idx]
		}
		if ip := net.ParseIP(strings.TrimSpace(xff)); ip != nil {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := net.ParseIP(strings.TrimSpace(xri)); ip != nil {
			return ip
		}
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return net.ParseIP(host)
}

// ipMatchesAny checks whether ip matches any entry in the list.
// Entries can be plain IPs ("1.2.3.4") or CIDR notation ("10.0.0.0/8").
func ipMatchesAny(ip net.IP, entries []string) bool {
	for _, entry := range entries {
		if strings.Contains(entry, "/") {
			_, cidr, err := net.ParseCIDR(entry)
			if err == nil && cidr.Contains(ip) {
				return true
			}
		} else {
			if net.ParseIP(entry) != nil && net.ParseIP(entry).Equal(ip) {
				return true
			}
		}
	}
	return false
}

// ── Mock response ────────────────────────────────────────────────────────────

func (g *Gateway) writeMockResponse(w http.ResponseWriter, route *domain.GatewayRoute) error {
	mock := route.MockResponse
	body, err := g.mapRouteResponsePayload(route, mock.Body)
	if err != nil {
		return err
	}
	setResponseHeaders(w, route)
	for k, v := range mock.Headers {
		w.Header().Set(k, v)
	}
	statusCode := mock.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	writeJSONPayload(w, statusCode, body)
	return nil
}

// ── Circuit breaker helpers ──────────────────────────────────────────────────

func (g *Gateway) getOrCreateBreaker(route *domain.GatewayRoute) *circuitState {
	if v, ok := g.circuitBreakers.Load(route.ID); ok {
		return v.(*circuitState)
	}
	cs := &circuitState{}
	actual, _ := g.circuitBreakers.LoadOrStore(route.ID, cs)
	return actual.(*circuitState)
}

func (g *Gateway) recordCircuitResult(route *domain.GatewayRoute, success bool) {
	if route.CircuitBreaker == nil {
		return
	}
	cs := g.getOrCreateBreaker(route)
	if success {
		cs.recordSuccess()
	} else {
		cs.recordFailure(route.CircuitBreaker.MaxFailures, route.CircuitBreaker.TimeoutSec)
	}
}

// ── Response headers ─────────────────────────────────────────────────────────

func setResponseHeaders(w http.ResponseWriter, route *domain.GatewayRoute) {
	for k, v := range route.ResponseHeaders {
		w.Header().Set(k, v)
	}
}

func (g *Gateway) mapRouteResponsePayload(route *domain.GatewayRoute, payload json.RawMessage) (json.RawMessage, error) {
	if len(route.ResponseMapping) == 0 {
		return payload, nil
	}
	return applyResponseMappings(payload, route.ResponseMapping)
}

func (g *Gateway) writeResponseMappingError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "response_mapping_failed",
		"message": err.Error(),
	})
}

// ── Shared helpers ───────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(body))
}

func writeJSONPayload(w http.ResponseWriter, status int, payload json.RawMessage) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	if len(payload) == 0 {
		w.Write([]byte("null"))
		return
	}
	w.Write(payload)
}
