package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/oriys/nova/internal/api/controlplane"
	"github.com/oriys/nova/internal/api/dataplane"
	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/authz"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/compiler"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/gateway"
	"github.com/oriys/nova/internal/layer"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/ratelimit"
	"github.com/oriys/nova/internal/scheduler"
	"github.com/oriys/nova/internal/secrets"
	"github.com/oriys/nova/internal/service"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/workflow"
)

// ServerConfig contains dependencies for the HTTP server.
type ServerConfig struct {
	Store           *store.Store
	Exec            *executor.Executor
	Pool            *pool.Pool
	Backend         backend.Backend
	FCAdapter       *firecracker.Adapter // Optional: for Firecracker-specific features (snapshots)
	AuthCfg         *config.AuthConfig
	RateLimitCfg    *config.RateLimitConfig
	GatewayCfg      *config.GatewayConfig
	WorkflowService *workflow.Service
	APIKeyManager   *auth.APIKeyManager
	SecretsStore    *secrets.Store
	Scheduler       *scheduler.Scheduler
	RootfsDir       string
	LayerManager    *layer.Manager
}

// StartHTTPServer creates and starts the HTTP server with control plane and data plane handlers.
func StartHTTPServer(addr string, cfg ServerConfig) *http.Server {
	mux := http.NewServeMux()

	// Create compiler
	comp := compiler.New(cfg.Store)

	// Create services
	funcService := service.NewFunctionService(cfg.Store, comp)

	gatewayEnabled := cfg.GatewayCfg != nil && cfg.GatewayCfg.Enabled

	// Register control plane routes
	cpHandler := &controlplane.Handler{
		Store:           cfg.Store,
		Pool:            cfg.Pool,
		Backend:         cfg.Backend,
		FCAdapter:       cfg.FCAdapter,
		Compiler:        comp,
		FunctionService: funcService,
		WorkflowService: cfg.WorkflowService,
		APIKeyManager:   cfg.APIKeyManager,
		SecretsStore:    cfg.SecretsStore,
		Scheduler:       cfg.Scheduler,
		RootfsDir:       cfg.RootfsDir,
		GatewayEnabled:  gatewayEnabled,
		LayerManager:    cfg.LayerManager,
	}
	cpHandler.RegisterRoutes(mux)

	// Register data plane routes
	dpHandler := &dataplane.Handler{
		Store: cfg.Store,
		Exec:  cfg.Exec,
		Pool:  cfg.Pool,
	}
	dpHandler.RegisterRoutes(mux)

	// Wrap with tracing middleware
	var handler http.Handler = mux
	handler = observability.HTTPMiddleware(handler)

	// Add rate limiting middleware
	if cfg.RateLimitCfg != nil && cfg.RateLimitCfg.Enabled {
		tiers := make(map[string]ratelimit.TierConfig)
		for name, tier := range cfg.RateLimitCfg.Tiers {
			tiers[name] = ratelimit.TierConfig{
				RequestsPerSecond: tier.RequestsPerSecond,
				BurstSize:         tier.BurstSize,
			}
		}
		limiter := ratelimit.New(cfg.Store, tiers, ratelimit.TierConfig{
			RequestsPerSecond: cfg.RateLimitCfg.Default.RequestsPerSecond,
			BurstSize:         cfg.RateLimitCfg.Default.BurstSize,
		})
		publicPaths := []string{"/health", "/health/live", "/health/ready", "/health/startup"}
		if cfg.AuthCfg != nil {
			publicPaths = cfg.AuthCfg.PublicPaths
		}
		handler = ratelimit.Middleware(limiter, publicPaths)(handler)
		logging.Op().Info("rate limiting enabled", "default_rps", cfg.RateLimitCfg.Default.RequestsPerSecond)
	}

	// Add auth middleware
	if cfg.AuthCfg != nil && cfg.AuthCfg.Enabled {
		authenticators := buildAuthenticators(cfg.AuthCfg, cfg.Store)

		// Add authorization middleware
		if cfg.AuthCfg.Authorization.Enabled {
			defaultRole := domain.Role(cfg.AuthCfg.Authorization.DefaultRole)
			authorizer := authz.New(defaultRole)
			handler = authz.Middleware(authorizer)(handler)
			logging.Op().Info("authorization enabled", "default_role", cfg.AuthCfg.Authorization.DefaultRole)
		}

		// Resolve/enforce effective tenant scope after authentication and before handlers.
		handler = tenantScopeMiddleware(handler)

		if len(authenticators) > 0 {
			handler = auth.Middleware(authenticators, cfg.AuthCfg.PublicPaths)(handler)
			logging.Op().Info("authentication enabled", "public_paths", cfg.AuthCfg.PublicPaths)
		}
	}

	// Set up gateway host router if enabled
	if gatewayEnabled && cfg.Exec != nil {
		var authenticators []auth.Authenticator
		if cfg.AuthCfg != nil && cfg.AuthCfg.Enabled {
			authenticators = buildAuthenticators(cfg.AuthCfg, cfg.Store)
		}
		gw := gateway.New(cfg.Store, cfg.Exec, authenticators)
		if err := gw.ReloadRoutes(context.Background()); err != nil {
			logging.Op().Warn("failed to load gateway routes", "error", err)
		}
		handler = &hostRouter{
			gateway:    gw,
			defaultMux: handler,
		}
		logging.Op().Info("gateway enabled")
	}

	// Outer tenant scope pass for all paths (including gateway custom domains).
	// For authenticated control-plane requests, an inner tenantScopeMiddleware pass runs
	// after auth and enforces identity-bound scope.
	handler = tenantScopeMiddleware(handler)

	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Op().Error("HTTP server error", "error", err)
		}
	}()

	return server
}

func tenantScopeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedTenant, requestedNamespace, explicit, err := requestedScopeFromHeaders(r)
		if err != nil {
			writeTenantScopeError(w, http.StatusBadRequest, "invalid tenant scope headers")
			logging.Op().Warn("tenant scope rejected: invalid header", "path", r.URL.Path, "method", r.Method, "error", err.Error())
			return
		}

		identity := auth.GetIdentity(r.Context())
		effectiveTenant := requestedTenant
		effectiveNamespace := requestedNamespace

		if identity != nil && identity.ScopeRestricted() {
			if !explicit {
				primary, ok := identity.PrimaryScope()
				if !ok {
					writeTenantScopeError(w, http.StatusForbidden, "tenant scope is required")
					logging.Op().Warn("tenant scope denied", "subject", identity.Subject, "path", r.URL.Path, "method", r.Method, "reason", "missing_allowed_scope")
					return
				}
				if primary.TenantID == "*" || primary.Namespace == "*" {
					writeTenantScopeError(w, http.StatusBadRequest, "explicit X-Nova-Tenant and X-Nova-Namespace headers are required")
					logging.Op().Warn("tenant scope denied", "subject", identity.Subject, "path", r.URL.Path, "method", r.Method, "reason", "ambiguous_scope")
					return
				}
				effectiveTenant = primary.TenantID
				effectiveNamespace = primary.Namespace
			}

			if !identity.AllowsScope(effectiveTenant, effectiveNamespace) {
				writeTenantScopeError(w, http.StatusForbidden, "tenant scope is not allowed for this identity")
				logging.Op().Warn("tenant scope denied", "subject", identity.Subject, "path", r.URL.Path, "method", r.Method, "tenant_id", effectiveTenant, "namespace", effectiveNamespace, "reason", "out_of_scope")
				return
			}
		}

		logging.Op().Debug("tenant scope resolved", "subject", subjectOrAnonymous(identity), "path", r.URL.Path, "method", r.Method, "tenant_id", effectiveTenant, "namespace", effectiveNamespace, "explicit", explicit)
		ctx := store.WithTenantScope(r.Context(), effectiveTenant, effectiveNamespace)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestedScopeFromHeaders(r *http.Request) (tenantID string, namespace string, explicit bool, err error) {
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

	if !store.IsValidTenantScopePart(tenantID) {
		return "", "", true, &tenantScopeHeaderError{Field: "X-Nova-Tenant"}
	}
	if !store.IsValidTenantScopePart(namespace) {
		return "", "", true, &tenantScopeHeaderError{Field: "X-Nova-Namespace"}
	}
	return tenantID, namespace, true, nil
}

type tenantScopeHeaderError struct {
	Field string
}

func (e *tenantScopeHeaderError) Error() string {
	return "invalid header: " + e.Field
}

func writeTenantScopeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "tenant_scope_error",
		"message": msg,
	})
}

func subjectOrAnonymous(identity *auth.Identity) string {
	if identity == nil || strings.TrimSpace(identity.Subject) == "" {
		return "anonymous"
	}
	return identity.Subject
}

// buildAuthenticators creates authenticators based on config.
func buildAuthenticators(cfg *config.AuthConfig, s *store.Store) []auth.Authenticator {
	var authenticators []auth.Authenticator

	// Add JWT authenticator if enabled
	if cfg.JWT.Enabled {
		jwtAuth, err := auth.NewJWTAuthenticator(auth.JWTAuthConfig{
			Algorithm:     cfg.JWT.Algorithm,
			Secret:        cfg.JWT.Secret,
			PublicKeyFile: cfg.JWT.PublicKeyFile,
			Issuer:        cfg.JWT.Issuer,
		})
		if err != nil {
			logging.Op().Warn("failed to create JWT authenticator", "error", err)
		} else {
			authenticators = append(authenticators, jwtAuth)
		}
	}

	// Add API Key authenticator if enabled
	if cfg.APIKeys.Enabled {
		var staticKeys []auth.StaticKeyConfig
		for _, k := range cfg.APIKeys.StaticKeys {
			staticKeys = append(staticKeys, auth.StaticKeyConfig{
				Name: k.Name,
				Key:  k.Key,
				Tier: k.Tier,
			})
		}
		apiKeyAuth := auth.NewAPIKeyAuthenticator(auth.APIKeyAuthConfig{
			Store:      &apiKeyStoreAdapter{s: s},
			StaticKeys: staticKeys,
		})
		authenticators = append(authenticators, apiKeyAuth)
	}

	return authenticators
}

// apiKeyStoreAdapter adapts store.Store to auth.APIKeyStore.
type apiKeyStoreAdapter struct {
	s *store.Store
}

func (a *apiKeyStoreAdapter) SaveAPIKey(ctx context.Context, key *auth.APIKey) error {
	permissions, _ := auth.MarshalPolicies(key.Policies)
	return a.s.SaveAPIKey(ctx, &store.APIKeyRecord{
		Name: key.Name, KeyHash: key.KeyHash, Tier: key.Tier,
		TenantID: key.TenantID, Namespace: key.Namespace,
		Enabled: key.Enabled, ExpiresAt: key.ExpiresAt,
		Permissions: permissions,
		CreatedAt:   key.CreatedAt, UpdatedAt: key.UpdatedAt,
	})
}

func (a *apiKeyStoreAdapter) GetAPIKeyByHash(ctx context.Context, keyHash string) (*auth.APIKey, error) {
	rec, err := a.s.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	policies, _ := auth.UnmarshalPolicies(rec.Permissions)
	return &auth.APIKey{
		Name: rec.Name, KeyHash: rec.KeyHash, Tier: rec.Tier,
		TenantID: rec.TenantID, Namespace: rec.Namespace,
		Enabled: rec.Enabled, ExpiresAt: rec.ExpiresAt,
		Policies:  policies,
		CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
	}, nil
}

func (a *apiKeyStoreAdapter) GetAPIKeyByName(ctx context.Context, name string) (*auth.APIKey, error) {
	rec, err := a.s.GetAPIKeyByName(ctx, name)
	if err != nil {
		return nil, err
	}
	policies, _ := auth.UnmarshalPolicies(rec.Permissions)
	return &auth.APIKey{
		Name: rec.Name, KeyHash: rec.KeyHash, Tier: rec.Tier,
		TenantID: rec.TenantID, Namespace: rec.Namespace,
		Enabled: rec.Enabled, ExpiresAt: rec.ExpiresAt,
		Policies:  policies,
		CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
	}, nil
}

func (a *apiKeyStoreAdapter) ListAPIKeys(ctx context.Context) ([]*auth.APIKey, error) {
	recs, err := a.s.ListAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]*auth.APIKey, len(recs))
	for i, rec := range recs {
		policies, _ := auth.UnmarshalPolicies(rec.Permissions)
		keys[i] = &auth.APIKey{
			Name: rec.Name, KeyHash: rec.KeyHash, Tier: rec.Tier,
			TenantID: rec.TenantID, Namespace: rec.Namespace,
			Enabled: rec.Enabled, ExpiresAt: rec.ExpiresAt,
			Policies:  policies,
			CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
		}
	}
	return keys, nil
}

func (a *apiKeyStoreAdapter) DeleteAPIKey(ctx context.Context, name string) error {
	return a.s.DeleteAPIKey(ctx, name)
}

// hostRouter routes requests to the gateway for known custom domains,
// and falls back to the default mux for all other traffic.
type hostRouter struct {
	gateway    *gateway.Gateway
	defaultMux http.Handler
}

func (h *hostRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	// Strip port
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}
	host = strings.ToLower(host)

	// Check if this host has gateway routes
	domains := h.gateway.KnownDomains()
	if _, ok := domains[host]; ok {
		h.gateway.ServeHTTP(w, r)
		return
	}

	h.defaultMux.ServeHTTP(w, r)
}
