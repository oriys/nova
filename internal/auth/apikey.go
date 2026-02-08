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

	"github.com/oriys/nova/internal/domain"
)

// APIKey represents a stored API key
type APIKey struct {
	Name      string                 `json:"name"`
	KeyHash   string                 `json:"key_hash"`   // SHA256 hash of the key
	Tier      string                 `json:"tier"`       // Rate limit tier
	Enabled   bool                   `json:"enabled"`    // Whether the key is active
	TenantID  string                 `json:"tenant_id"`  // Bound tenant scope
	Namespace string                 `json:"namespace"`  // Bound namespace scope
	ExpiresAt *time.Time             `json:"expires_at"` // Optional expiration
	Policies  []domain.PolicyBinding `json:"policies"`   // Authorization policies
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// APIKeyStore interface for API key operations
type APIKeyStore interface {
	SaveAPIKey(ctx context.Context, key *APIKey) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error)
	GetAPIKeyByName(ctx context.Context, name string) (*APIKey, error)
	ListAPIKeys(ctx context.Context) ([]*APIKey, error)
	DeleteAPIKey(ctx context.Context, name string) error
}

// APIKeyAuthenticator validates API keys
type APIKeyAuthenticator struct {
	store      APIKeyStore
	staticKeys map[string]staticKey // hash -> key info
}

type staticKey struct {
	name string
	tier string
}

// APIKeyAuthConfig holds API key authenticator configuration
type APIKeyAuthConfig struct {
	Store      APIKeyStore
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
		store:      cfg.Store,
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

	// Check database
	if a.store != nil {
		if id := a.checkStoreKey(r.Context(), keyHash); id != nil {
			return id
		}
	}

	return nil
}

// checkStoreKey looks up the key hash in the store
func (a *APIKeyAuthenticator) checkStoreKey(ctx context.Context, keyHash string) *Identity {
	apiKey, err := a.store.GetAPIKeyByHash(ctx, keyHash)
	if err != nil || apiKey == nil {
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
		Subject:  "apikey:" + apiKey.Name,
		KeyName:  apiKey.Name,
		Tier:     tier,
		Claims:   map[string]any{"source": "postgres"},
		Policies: apiKey.Policies,
		AllowedScopes: []TenantScope{normalizeTenantScopeWithWildcard(TenantScope{
			TenantID:  apiKey.TenantID,
			Namespace: apiKey.Namespace,
		})},
	}
}

// hashAPIKey creates a SHA256 hash of the API key
func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// APIKeyManager manages API keys in the database
type APIKeyManager struct {
	store APIKeyStore
}

// NewAPIKeyManager creates a new API key manager
func NewAPIKeyManager(store APIKeyStore) *APIKeyManager {
	return &APIKeyManager{store: store}
}

// Create creates a new API key and returns the plaintext key
func (m *APIKeyManager) Create(ctx context.Context, name, tier string, policies []domain.PolicyBinding) (string, error) {
	// Check if name already exists
	existing, _ := m.store.GetAPIKeyByName(ctx, name)
	if existing != nil {
		return "", fmt.Errorf("API key with name '%s' already exists", name)
	}

	// Generate random key
	key := generateAPIKey()
	keyHash := hashAPIKey(key)

	if tier == "" {
		tier = "default"
	}

	apiKey := &APIKey{
		Name:      name,
		KeyHash:   keyHash,
		Tier:      tier,
		Enabled:   true,
		Policies:  policies,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := m.store.SaveAPIKey(ctx, apiKey); err != nil {
		return "", err
	}

	return key, nil
}

// Get retrieves an API key by name
func (m *APIKeyManager) Get(ctx context.Context, name string) (*APIKey, error) {
	return m.store.GetAPIKeyByName(ctx, name)
}

// List returns all API keys
func (m *APIKeyManager) List(ctx context.Context) ([]*APIKey, error) {
	return m.store.ListAPIKeys(ctx)
}

// Revoke disables an API key
func (m *APIKeyManager) Revoke(ctx context.Context, name string) error {
	apiKey, err := m.store.GetAPIKeyByName(ctx, name)
	if err != nil {
		return err
	}

	apiKey.Enabled = false
	apiKey.UpdatedAt = time.Now()

	return m.store.SaveAPIKey(ctx, apiKey)
}

// Enable re-enables a disabled API key
func (m *APIKeyManager) Enable(ctx context.Context, name string) error {
	apiKey, err := m.store.GetAPIKeyByName(ctx, name)
	if err != nil {
		return err
	}

	apiKey.Enabled = true
	apiKey.UpdatedAt = time.Now()

	return m.store.SaveAPIKey(ctx, apiKey)
}

// Delete removes an API key
func (m *APIKeyManager) Delete(ctx context.Context, name string) error {
	return m.store.DeleteAPIKey(ctx, name)
}

// UpdatePolicies updates the authorization policies for an API key
func (m *APIKeyManager) UpdatePolicies(ctx context.Context, name string, policies []domain.PolicyBinding) error {
	apiKey, err := m.store.GetAPIKeyByName(ctx, name)
	if err != nil {
		return err
	}
	apiKey.Policies = policies
	apiKey.UpdatedAt = time.Now()
	return m.store.SaveAPIKey(ctx, apiKey)
}

// MarshalPolicies serializes policies to JSON
func MarshalPolicies(policies []domain.PolicyBinding) (json.RawMessage, error) {
	if len(policies) == 0 {
		return json.RawMessage("[]"), nil
	}
	return json.Marshal(policies)
}

// UnmarshalPolicies deserializes policies from JSON
func UnmarshalPolicies(data json.RawMessage) ([]domain.PolicyBinding, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var policies []domain.PolicyBinding
	if err := json.Unmarshal(data, &policies); err != nil {
		return nil, err
	}
	return policies, nil
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
