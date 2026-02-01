package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	apikeyPrefix = "nova:apikey:"
	apikeyIndex  = "nova:apikeys"
)

// APIKey represents a stored API key
type APIKey struct {
	Name      string     `json:"name"`
	KeyHash   string     `json:"key_hash"`   // SHA256 hash of the key
	Tier      string     `json:"tier"`       // Rate limit tier
	Enabled   bool       `json:"enabled"`    // Whether the key is active
	ExpiresAt *time.Time `json:"expires_at"` // Optional expiration
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// APIKeyAuthenticator validates API keys
type APIKeyAuthenticator struct {
	redis      *redis.Client
	staticKeys map[string]staticKey // hash -> key info
}

type staticKey struct {
	name string
	tier string
}

// APIKeyAuthConfig holds API key authenticator configuration
type APIKeyAuthConfig struct {
	Redis      *redis.Client
	StaticKeys []StaticKeyConfig
}

// StaticKeyConfig represents a static API key from config
type StaticKeyConfig struct {
	Name string
	Key  string
	Tier string
}

// NewAPIKeyAuthenticator creates a new API key authenticator
func NewAPIKeyAuthenticator(cfg APIKeyAuthConfig) *APIKeyAuthenticator {
	auth := &APIKeyAuthenticator{
		redis:      cfg.Redis,
		staticKeys: make(map[string]staticKey),
	}

	// Index static keys by their hash
	for _, k := range cfg.StaticKeys {
		hash := hashAPIKey(k.Key)
		tier := k.Tier
		if tier == "" {
			tier = "default"
		}
		auth.staticKeys[hash] = staticKey{
			name: k.Name,
			tier: tier,
		}
	}

	return auth
}

// Authenticate implements Authenticator
func (a *APIKeyAuthenticator) Authenticate(r *http.Request) *Identity {
	// Extract key from header
	key := r.Header.Get("X-API-Key")
	if key == "" {
		// Also check Authorization: ApiKey xxx
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "ApiKey " {
			key = authHeader[7:]
		}
	}

	if key == "" {
		return nil
	}

	keyHash := hashAPIKey(key)

	// Check static keys first
	if sk, ok := a.staticKeys[keyHash]; ok {
		return &Identity{
			Subject: "apikey:" + sk.name,
			KeyName: sk.name,
			Tier:    sk.tier,
			Claims:  map[string]any{"source": "static"},
		}
	}

	// Check Redis
	if a.redis != nil {
		if id := a.checkRedisKey(r.Context(), keyHash); id != nil {
			return id
		}
	}

	return nil
}

// checkRedisKey looks up the key hash in Redis
func (a *APIKeyAuthenticator) checkRedisKey(ctx context.Context, keyHash string) *Identity {
	data, err := a.redis.Get(ctx, apikeyPrefix+keyHash).Bytes()
	if err != nil {
		return nil
	}

	var apiKey APIKey
	if err := json.Unmarshal(data, &apiKey); err != nil {
		return nil
	}

	// Check if key is enabled
	if !apiKey.Enabled {
		return nil
	}

	// Check expiration
	if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
		return nil
	}

	tier := apiKey.Tier
	if tier == "" {
		tier = "default"
	}

	return &Identity{
		Subject: "apikey:" + apiKey.Name,
		KeyName: apiKey.Name,
		Tier:    tier,
		Claims:  map[string]any{"source": "redis"},
	}
}

// hashAPIKey creates a SHA256 hash of the API key
func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// APIKeyStore manages API keys in Redis
type APIKeyStore struct {
	redis *redis.Client
}

// NewAPIKeyStore creates a new API key store
func NewAPIKeyStore(redis *redis.Client) *APIKeyStore {
	return &APIKeyStore{redis: redis}
}

// Create creates a new API key and returns the plaintext key
func (s *APIKeyStore) Create(ctx context.Context, name, tier string) (string, error) {
	// Generate random key
	key := generateAPIKey()
	keyHash := hashAPIKey(key)

	// Check if name already exists
	existing, _ := s.redis.HGet(ctx, apikeyIndex, name).Result()
	if existing != "" {
		return "", fmt.Errorf("API key with name '%s' already exists", name)
	}

	if tier == "" {
		tier = "default"
	}

	apiKey := APIKey{
		Name:      name,
		KeyHash:   keyHash,
		Tier:      tier,
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	data, err := json.Marshal(apiKey)
	if err != nil {
		return "", err
	}

	// Store key and index atomically
	pipe := s.redis.Pipeline()
	pipe.Set(ctx, apikeyPrefix+keyHash, data, 0)
	pipe.HSet(ctx, apikeyIndex, name, keyHash)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", err
	}

	return key, nil
}

// Get retrieves an API key by name
func (s *APIKeyStore) Get(ctx context.Context, name string) (*APIKey, error) {
	keyHash, err := s.redis.HGet(ctx, apikeyIndex, name).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("API key not found: %s", name)
	}
	if err != nil {
		return nil, err
	}

	data, err := s.redis.Get(ctx, apikeyPrefix+keyHash).Bytes()
	if err != nil {
		return nil, err
	}

	var apiKey APIKey
	if err := json.Unmarshal(data, &apiKey); err != nil {
		return nil, err
	}

	return &apiKey, nil
}

// List returns all API keys
func (s *APIKeyStore) List(ctx context.Context) ([]*APIKey, error) {
	hashes, err := s.redis.HGetAll(ctx, apikeyIndex).Result()
	if err != nil {
		return nil, err
	}

	keys := make([]*APIKey, 0, len(hashes))
	for _, hash := range hashes {
		data, err := s.redis.Get(ctx, apikeyPrefix+hash).Bytes()
		if err != nil {
			continue
		}

		var apiKey APIKey
		if err := json.Unmarshal(data, &apiKey); err != nil {
			continue
		}
		keys = append(keys, &apiKey)
	}

	return keys, nil
}

// Revoke disables an API key
func (s *APIKeyStore) Revoke(ctx context.Context, name string) error {
	apiKey, err := s.Get(ctx, name)
	if err != nil {
		return err
	}

	apiKey.Enabled = false
	apiKey.UpdatedAt = time.Now()

	data, err := json.Marshal(apiKey)
	if err != nil {
		return err
	}

	return s.redis.Set(ctx, apikeyPrefix+apiKey.KeyHash, data, 0).Err()
}

// Delete removes an API key
func (s *APIKeyStore) Delete(ctx context.Context, name string) error {
	keyHash, err := s.redis.HGet(ctx, apikeyIndex, name).Result()
	if err == redis.Nil {
		return fmt.Errorf("API key not found: %s", name)
	}
	if err != nil {
		return err
	}

	pipe := s.redis.Pipeline()
	pipe.Del(ctx, apikeyPrefix+keyHash)
	pipe.HDel(ctx, apikeyIndex, name)
	_, err = pipe.Exec(ctx)
	return err
}

// generateAPIKey creates a random API key with sk_ prefix
func generateAPIKey() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 24)
	randomBytes := make([]byte, 24)
	rand.Read(randomBytes)
	for i := range b {
		b[i] = charset[randomBytes[i]%byte(len(charset))]
	}
	return "sk_" + string(b)
}

// VerifyAPIKey checks if a plaintext key matches a hash
func VerifyAPIKey(plaintext, hash string) bool {
	computed := hashAPIKey(plaintext)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(hash)) == 1
}
