package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// APIKeyRecord represents an API key in the database
type APIKeyRecord struct {
	Name        string          `json:"name"`
	KeyHash     string          `json:"key_hash"`
	Tier        string          `json:"tier"`
	Enabled     bool            `json:"enabled"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
	Permissions json.RawMessage `json:"permissions,omitempty"` // JSONB policies
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// RateLimitBucket represents a token bucket for rate limiting
type RateLimitBucket struct {
	Key        string
	Tokens     float64
	LastRefill time.Time
}

// SaveAPIKey creates or updates an API key
func (s *PostgresStore) SaveAPIKey(ctx context.Context, key *APIKeyRecord) error {
	permissions := key.Permissions
	if len(permissions) == 0 {
		permissions = json.RawMessage("[]")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO api_keys (name, key_hash, tier, enabled, expires_at, permissions, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (name) DO UPDATE SET
			key_hash = EXCLUDED.key_hash,
			tier = EXCLUDED.tier,
			enabled = EXCLUDED.enabled,
			expires_at = EXCLUDED.expires_at,
			permissions = EXCLUDED.permissions,
			updated_at = NOW()
	`, key.Name, key.KeyHash, key.Tier, key.Enabled, key.ExpiresAt, permissions, key.CreatedAt, key.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save api key: %w", err)
	}
	return nil
}

// GetAPIKeyByHash retrieves an API key by its hash
func (s *PostgresStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKeyRecord, error) {
	var key APIKeyRecord
	err := s.pool.QueryRow(ctx, `
		SELECT name, key_hash, tier, enabled, expires_at, COALESCE(permissions, '[]'::jsonb), created_at, updated_at
		FROM api_keys WHERE key_hash = $1
	`, keyHash).Scan(&key.Name, &key.KeyHash, &key.Tier, &key.Enabled, &key.ExpiresAt, &key.Permissions, &key.CreatedAt, &key.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}
	return &key, nil
}

// GetAPIKeyByName retrieves an API key by name
func (s *PostgresStore) GetAPIKeyByName(ctx context.Context, name string) (*APIKeyRecord, error) {
	var key APIKeyRecord
	err := s.pool.QueryRow(ctx, `
		SELECT name, key_hash, tier, enabled, expires_at, COALESCE(permissions, '[]'::jsonb), created_at, updated_at
		FROM api_keys WHERE name = $1
	`, name).Scan(&key.Name, &key.KeyHash, &key.Tier, &key.Enabled, &key.ExpiresAt, &key.Permissions, &key.CreatedAt, &key.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("api key not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
	}
	return &key, nil
}

// ListAPIKeys returns all API keys
func (s *PostgresStore) ListAPIKeys(ctx context.Context) ([]*APIKeyRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT name, key_hash, tier, enabled, expires_at, COALESCE(permissions, '[]'::jsonb), created_at, updated_at
		FROM api_keys ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []*APIKeyRecord
	for rows.Next() {
		var key APIKeyRecord
		if err := rows.Scan(&key.Name, &key.KeyHash, &key.Tier, &key.Enabled, &key.ExpiresAt, &key.Permissions, &key.CreatedAt, &key.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		keys = append(keys, &key)
	}
	return keys, nil
}

// DeleteAPIKey removes an API key
func (s *PostgresStore) DeleteAPIKey(ctx context.Context, name string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM api_keys WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("api key not found: %s", name)
	}
	return nil
}

// ─── Secrets ────────────────────────────────────────────────────────────────

// SaveSecret stores an encrypted secret
func (s *PostgresStore) SaveSecret(ctx context.Context, name, encryptedValue string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO secrets (name, value, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (name) DO UPDATE SET
			value = EXCLUDED.value,
			updated_at = NOW()
	`, name, encryptedValue)
	if err != nil {
		return fmt.Errorf("save secret: %w", err)
	}
	return nil
}

// GetSecret retrieves an encrypted secret
func (s *PostgresStore) GetSecret(ctx context.Context, name string) (string, error) {
	var value string
	err := s.pool.QueryRow(ctx, `SELECT value FROM secrets WHERE name = $1`, name).Scan(&value)
	if err == pgx.ErrNoRows {
		return "", fmt.Errorf("secret not found: %s", name)
	}
	if err != nil {
		return "", fmt.Errorf("get secret: %w", err)
	}
	return value, nil
}

// DeleteSecret removes a secret
func (s *PostgresStore) DeleteSecret(ctx context.Context, name string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM secrets WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	return nil
}

// ListSecrets returns all secret names with their creation times
func (s *PostgresStore) ListSecrets(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT name, created_at FROM secrets ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var name string
		var createdAt time.Time
		if err := rows.Scan(&name, &createdAt); err != nil {
			return nil, fmt.Errorf("scan secret: %w", err)
		}
		result[name] = createdAt.Format(time.RFC3339)
	}
	return result, nil
}

// SecretExists checks if a secret exists
func (s *PostgresStore) SecretExists(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM secrets WHERE name = $1)`, name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check secret exists: %w", err)
	}
	return exists, nil
}

// ─── Rate Limiting ──────────────────────────────────────────────────────────

// CheckRateLimit performs token bucket rate limiting
func (s *PostgresStore) CheckRateLimit(ctx context.Context, key string, maxTokens int, refillRate float64, requested int) (bool, int, error) {
	now := time.Now()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, 0, fmt.Errorf("begin rate limit tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var tokens float64
	var lastRefill time.Time
	err = tx.QueryRow(ctx, `
		SELECT tokens, last_refill FROM rate_limit_buckets
		WHERE key = $1 FOR UPDATE
	`, key).Scan(&tokens, &lastRefill)

	if err == pgx.ErrNoRows {
		tokens = float64(maxTokens)
		lastRefill = now
	} else if err != nil {
		return false, 0, fmt.Errorf("get rate limit bucket: %w", err)
	}

	elapsed := now.Sub(lastRefill).Seconds()
	tokens = min(float64(maxTokens), tokens+elapsed*refillRate)

	allowed := tokens >= float64(requested)
	if allowed {
		tokens -= float64(requested)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO rate_limit_buckets (key, tokens, last_refill)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET
			tokens = EXCLUDED.tokens,
			last_refill = EXCLUDED.last_refill
	`, key, tokens, now)
	if err != nil {
		return false, 0, fmt.Errorf("update rate limit bucket: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, 0, fmt.Errorf("commit rate limit tx: %w", err)
	}

	return allowed, int(tokens), nil
}

// CleanupRateLimitBuckets removes expired rate limit entries
func (s *PostgresStore) CleanupRateLimitBuckets(ctx context.Context, olderThan time.Duration) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM rate_limit_buckets
		WHERE last_refill < $1
	`, time.Now().Add(-olderThan))
	if err != nil {
		return fmt.Errorf("cleanup rate limit buckets: %w", err)
	}
	return nil
}
