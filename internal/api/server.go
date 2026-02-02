package api

import (
	"context"
	"net/http"

	"github.com/oriys/nova/internal/api/controlplane"
	"github.com/oriys/nova/internal/api/dataplane"
	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/compiler"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/ratelimit"
	"github.com/oriys/nova/internal/store"
)

// ServerConfig contains dependencies for the HTTP server.
type ServerConfig struct {
	Store        *store.Store
	Exec         *executor.Executor
	Pool         *pool.Pool
	Backend      backend.Backend
	FCAdapter    *firecracker.Adapter // Optional: for Firecracker-specific features (snapshots)
	AuthCfg      *config.AuthConfig
	RateLimitCfg *config.RateLimitConfig
}

// StartHTTPServer creates and starts the HTTP server with control plane and data plane handlers.
func StartHTTPServer(addr string, cfg ServerConfig) *http.Server {
	mux := http.NewServeMux()

	// Create compiler
	comp := compiler.New(cfg.Store)

	// Register control plane routes
	cpHandler := &controlplane.Handler{
		Store:     cfg.Store,
		Pool:      cfg.Pool,
		Backend:   cfg.Backend,
		FCAdapter: cfg.FCAdapter,
		Compiler:  comp,
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
		if len(authenticators) > 0 {
			handler = auth.Middleware(authenticators, cfg.AuthCfg.PublicPaths)(handler)
			logging.Op().Info("authentication enabled", "public_paths", cfg.AuthCfg.PublicPaths)
		}
	}

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
	return a.s.SaveAPIKey(ctx, &store.APIKeyRecord{
		Name: key.Name, KeyHash: key.KeyHash, Tier: key.Tier,
		Enabled: key.Enabled, ExpiresAt: key.ExpiresAt,
		CreatedAt: key.CreatedAt, UpdatedAt: key.UpdatedAt,
	})
}

func (a *apiKeyStoreAdapter) GetAPIKeyByHash(ctx context.Context, keyHash string) (*auth.APIKey, error) {
	rec, err := a.s.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		return nil, err
	}
	return &auth.APIKey{
		Name: rec.Name, KeyHash: rec.KeyHash, Tier: rec.Tier,
		Enabled: rec.Enabled, ExpiresAt: rec.ExpiresAt,
		CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
	}, nil
}

func (a *apiKeyStoreAdapter) GetAPIKeyByName(ctx context.Context, name string) (*auth.APIKey, error) {
	rec, err := a.s.GetAPIKeyByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return &auth.APIKey{
		Name: rec.Name, KeyHash: rec.KeyHash, Tier: rec.Tier,
		Enabled: rec.Enabled, ExpiresAt: rec.ExpiresAt,
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
		keys[i] = &auth.APIKey{
			Name: rec.Name, KeyHash: rec.KeyHash, Tier: rec.Tier,
			Enabled: rec.Enabled, ExpiresAt: rec.ExpiresAt,
			CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
		}
	}
	return keys, nil
}

func (a *apiKeyStoreAdapter) DeleteAPIKey(ctx context.Context, name string) error {
	return a.s.DeleteAPIKey(ctx, name)
}
