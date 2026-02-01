package ratelimit

import (
	"context"
	"time"
)

// Backend defines the storage interface for rate limiting
type Backend interface {
	CheckRateLimit(ctx context.Context, key string, maxTokens int, refillRate float64, requested int) (bool, int, error)
}

// TierConfig holds rate limit configuration for a tier
type TierConfig struct {
	RequestsPerSecond float64
	BurstSize         int
}

// Limiter implements Postgres-backed token bucket rate limiting
type Limiter struct {
	backend  Backend
	tiers    map[string]TierConfig
	default_ TierConfig
}

// New creates a new rate limiter
func New(backend Backend, tiers map[string]TierConfig, defaultTier TierConfig) *Limiter {
	if tiers == nil {
		tiers = make(map[string]TierConfig)
	}
	return &Limiter{
		backend:  backend,
		tiers:    tiers,
		default_: defaultTier,
	}
}

// Result contains the result of a rate limit check
type Result struct {
	Allowed   bool
	Remaining int
	ResetAt   time.Time
}

// Allow checks if a request is allowed for the given key and tier
func (l *Limiter) Allow(ctx context.Context, key, tier string) (Result, error) {
	return l.AllowN(ctx, key, tier, 1)
}

// AllowN checks if N requests are allowed
func (l *Limiter) AllowN(ctx context.Context, key, tier string, n int) (Result, error) {
	cfg := l.getTierConfig(tier)

	allowed, remaining, err := l.backend.CheckRateLimit(ctx, key, cfg.BurstSize, cfg.RequestsPerSecond, n)
	if err != nil {
		return Result{}, err
	}

	// Calculate when bucket will be full again
	tokensNeeded := float64(cfg.BurstSize) - float64(remaining)
	refillSeconds := tokensNeeded / cfg.RequestsPerSecond
	resetAt := time.Now().Add(time.Duration(refillSeconds) * time.Second)

	return Result{
		Allowed:   allowed,
		Remaining: remaining,
		ResetAt:   resetAt,
	}, nil
}

// getTierConfig returns the config for a tier, falling back to default
func (l *Limiter) getTierConfig(tier string) TierConfig {
	if cfg, ok := l.tiers[tier]; ok {
		return cfg
	}
	return l.default_
}

// KeyForAPIKey returns the rate limit key for an API key
func KeyForAPIKey(name string) string {
	return "nova:rl:apikey:" + name
}

// KeyForIP returns the rate limit key for an IP address
func KeyForIP(ip string) string {
	return "nova:rl:ip:" + ip
}

// KeyForGlobal returns the rate limit key for anonymous/global requests
func KeyForGlobal(ip string) string {
	return "nova:rl:global:" + ip
}
