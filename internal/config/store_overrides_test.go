package config

import "testing"

func TestApplyStoreOverrides_BooleanAndRuntimePoolFields(t *testing.T) {
	cfg := DefaultConfig()

	ApplyStoreOverrides(cfg, map[string]string{
		"gateway_enabled":              "false",
		"auth_enabled":                 "true",
		"auth_jwt_enabled":             "1",
		"auth_apikeys_enabled":         "yes",
		"authz_enabled":                "on",
		"rate_limit_enabled":           "true",
		"secrets_enabled":              "true",
		"network_policy_enabled":       "true",
		"auto_scale_enabled":           "true",
		"layers_enabled":               "true",
		"volumes_enabled":              "true",
		"grpc_enabled":                 "true",
		"tracing_enabled":              "true",
		"output_capture_enabled":       "true",
		"queue_adaptive_enabled":       "true",
		"cache_redis_enabled":          "true",
		"cache_invalidation":           "true",
		"runtime_pool_enabled":         "false",
		"runtime_pool_size":            "3",
		"runtime_pool_refill_interval": "45s",
		"runtime_pool_runtimes":        "python, node,go",
	})

	if cfg.Gateway.Enabled {
		t.Fatalf("expected gateway to be disabled by store override")
	}
	if !cfg.Auth.Enabled || !cfg.Auth.JWT.Enabled || !cfg.Auth.APIKeys.Enabled {
		t.Fatalf("expected auth/auth.jwt/auth.apikeys to be enabled")
	}
	if !cfg.Auth.Authorization.Enabled {
		t.Fatalf("expected authz to be enabled")
	}
	if !cfg.RateLimit.Enabled || !cfg.Secrets.Enabled || !cfg.NetworkPolicy.Enabled {
		t.Fatalf("expected rate_limit/secrets/network_policy to be enabled")
	}
	if !cfg.AutoScale.Enabled || !cfg.Layers.Enabled || !cfg.Volumes.Enabled {
		t.Fatalf("expected auto_scale/layers/volumes to be enabled")
	}
	if !cfg.GRPC.Enabled || !cfg.Observability.Tracing.Enabled || !cfg.Observability.OutputCapture.Enabled {
		t.Fatalf("expected grpc/tracing/output_capture to be enabled")
	}
	if !cfg.Queue.AdaptiveEnabled || !cfg.Cache.RedisEnabled || !cfg.Cache.Invalidation {
		t.Fatalf("expected queue/cache flags to be enabled")
	}
	if cfg.RuntimePool.Enabled {
		t.Fatalf("expected runtime pool to be disabled")
	}
	if cfg.RuntimePool.PoolSize != 3 {
		t.Fatalf("expected runtime pool size 3, got %d", cfg.RuntimePool.PoolSize)
	}
	if cfg.RuntimePool.RefillInterval != "45s" {
		t.Fatalf("expected runtime pool refill interval 45s, got %s", cfg.RuntimePool.RefillInterval)
	}
	if got, want := len(cfg.RuntimePool.Runtimes), 3; got != want {
		t.Fatalf("expected %d runtime pool runtimes, got %d", want, got)
	}
}

func TestApplyStoreOverrides_InvalidValuesAreIgnored(t *testing.T) {
	cfg := DefaultConfig()
	origGateway := cfg.Gateway.Enabled
	origRuntimePoolSize := cfg.RuntimePool.PoolSize
	origRuntimePoolRefill := cfg.RuntimePool.RefillInterval

	ApplyStoreOverrides(cfg, map[string]string{
		"gateway_enabled":              "not-a-bool",
		"runtime_pool_size":            "-1",
		"runtime_pool_refill_interval": "bad-duration",
	})

	if cfg.Gateway.Enabled != origGateway {
		t.Fatalf("expected invalid gateway_enabled to be ignored")
	}
	if cfg.RuntimePool.PoolSize != origRuntimePoolSize {
		t.Fatalf("expected invalid runtime_pool_size to be ignored")
	}
	if cfg.RuntimePool.RefillInterval != origRuntimePoolRefill {
		t.Fatalf("expected invalid runtime_pool_refill_interval to be ignored")
	}
}
