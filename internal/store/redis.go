package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/go-redis/redis/v8"
)

const (
	funcKeyPrefix = "nova:func:"
	funcListKey   = "nova:funcs"
)

// Lua script for atomic name->function lookup (single RTT instead of 2)
var getFunctionByNameScript = redis.NewScript(`
local id = redis.call('HGET', KEYS[1], ARGV[1])
if not id then
    return nil
end
return redis.call('GET', KEYS[2] .. id)
`)

type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(addr, password string, db int) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisStore{client: client}, nil
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}

// Client returns the underlying Redis client for direct access
func (s *RedisStore) Client() *redis.Client {
	return s.client
}

// Ping checks Redis connectivity
func (s *RedisStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *RedisStore) SaveFunction(ctx context.Context, fn *domain.Function) error {
	data, err := json.Marshal(fn)
	if err != nil {
		return err
	}

	pipe := s.client.Pipeline()
	pipe.Set(ctx, funcKeyPrefix+fn.ID, data, 0)
	pipe.HSet(ctx, funcListKey, fn.Name, fn.ID)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *RedisStore) GetFunction(ctx context.Context, id string) (*domain.Function, error) {
	data, err := s.client.Get(ctx, funcKeyPrefix+id).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("function not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	var fn domain.Function
	if err := json.Unmarshal(data, &fn); err != nil {
		return nil, err
	}
	return &fn, nil
}

func (s *RedisStore) GetFunctionByName(ctx context.Context, name string) (*domain.Function, error) {
	// Use Lua script for atomic lookup (single RTT)
	result, err := getFunctionByNameScript.Run(ctx, s.client, []string{funcListKey, funcKeyPrefix}, name).Result()
	if err == redis.Nil || result == nil {
		return nil, fmt.Errorf("function not found: %s", name)
	}
	if err != nil {
		return nil, err
	}

	data, ok := result.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected result type from Lua script")
	}

	var fn domain.Function
	if err := json.Unmarshal([]byte(data), &fn); err != nil {
		return nil, err
	}
	return &fn, nil
}

func (s *RedisStore) DeleteFunction(ctx context.Context, id string) error {
	fn, err := s.GetFunction(ctx, id)
	if err != nil {
		return err
	}

	pipe := s.client.Pipeline()
	pipe.Del(ctx, funcKeyPrefix+id)
	pipe.HDel(ctx, funcListKey, fn.Name)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *RedisStore) ListFunctions(ctx context.Context) ([]*domain.Function, error) {
	ids, err := s.client.HVals(ctx, funcListKey).Result()
	if err != nil {
		return nil, err
	}

	functions := make([]*domain.Function, 0, len(ids))
	for _, id := range ids {
		fn, err := s.GetFunction(ctx, id)
		if err != nil {
			continue
		}
		functions = append(functions, fn)
	}
	return functions, nil
}

// UpdateFunction updates an existing function with partial data.
// Only non-zero fields in the update struct will be applied.
type FunctionUpdate struct {
	Handler        *string
	CodePath       *string
	MemoryMB       *int
	TimeoutS       *int
	MinReplicas    *int
	Mode           *domain.ExecutionMode
	Limits         *domain.ResourceLimits
	EnvVars        map[string]string
	MergeEnvVars   bool // If true, merge envvars instead of replace
}

func (s *RedisStore) UpdateFunction(ctx context.Context, name string, update *FunctionUpdate) (*domain.Function, error) {
	fn, err := s.GetFunctionByName(ctx, name)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if update.Handler != nil {
		fn.Handler = *update.Handler
	}
	if update.CodePath != nil {
		fn.CodePath = *update.CodePath
	}
	if update.MemoryMB != nil {
		fn.MemoryMB = *update.MemoryMB
	}
	if update.TimeoutS != nil {
		fn.TimeoutS = *update.TimeoutS
	}
	if update.MinReplicas != nil {
		fn.MinReplicas = *update.MinReplicas
	}
	if update.Mode != nil {
		fn.Mode = *update.Mode
	}
	if update.Limits != nil {
		fn.Limits = update.Limits
	}
	if update.EnvVars != nil {
		if update.MergeEnvVars && fn.EnvVars != nil {
			for k, v := range update.EnvVars {
				fn.EnvVars[k] = v
			}
		} else {
			fn.EnvVars = update.EnvVars
		}
	}

	fn.UpdatedAt = time.Now()

	if err := s.SaveFunction(ctx, fn); err != nil {
		return nil, err
	}

	return fn, nil
}

// ─── Versioning ─────────────────────────────────────────────────────────────

const (
	versionKeyPrefix = "nova:version:"
	aliasKeyPrefix   = "nova:alias:"
)

// PublishVersion creates a new version of a function
func (s *RedisStore) PublishVersion(ctx context.Context, funcID string, version *domain.FunctionVersion) error {
	data, err := json.Marshal(version)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s%s:%d", versionKeyPrefix, funcID, version.Version)
	return s.client.Set(ctx, key, data, 0).Err()
}

// GetVersion retrieves a specific version of a function
func (s *RedisStore) GetVersion(ctx context.Context, funcID string, version int) (*domain.FunctionVersion, error) {
	key := fmt.Sprintf("%s%s:%d", versionKeyPrefix, funcID, version)
	data, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("version not found: %s v%d", funcID, version)
	}
	if err != nil {
		return nil, err
	}

	var v domain.FunctionVersion
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// ListVersions lists all versions of a function
func (s *RedisStore) ListVersions(ctx context.Context, funcID string) ([]*domain.FunctionVersion, error) {
	pattern := fmt.Sprintf("%s%s:*", versionKeyPrefix, funcID)
	keys, err := s.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	versions := make([]*domain.FunctionVersion, 0, len(keys))
	for _, key := range keys {
		data, err := s.client.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		var v domain.FunctionVersion
		if err := json.Unmarshal(data, &v); err != nil {
			continue
		}
		versions = append(versions, &v)
	}
	return versions, nil
}

// DeleteVersion removes a specific version
func (s *RedisStore) DeleteVersion(ctx context.Context, funcID string, version int) error {
	key := fmt.Sprintf("%s%s:%d", versionKeyPrefix, funcID, version)
	return s.client.Del(ctx, key).Err()
}

// SetAlias creates or updates an alias for a function
func (s *RedisStore) SetAlias(ctx context.Context, alias *domain.FunctionAlias) error {
	data, err := json.Marshal(alias)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s%s:%s", aliasKeyPrefix, alias.FunctionID, alias.Name)
	return s.client.Set(ctx, key, data, 0).Err()
}

// GetAlias retrieves an alias configuration
func (s *RedisStore) GetAlias(ctx context.Context, funcID, aliasName string) (*domain.FunctionAlias, error) {
	key := fmt.Sprintf("%s%s:%s", aliasKeyPrefix, funcID, aliasName)
	data, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("alias not found: %s@%s", funcID, aliasName)
	}
	if err != nil {
		return nil, err
	}

	var alias domain.FunctionAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return nil, err
	}
	return &alias, nil
}

// ListAliases lists all aliases for a function
func (s *RedisStore) ListAliases(ctx context.Context, funcID string) ([]*domain.FunctionAlias, error) {
	pattern := fmt.Sprintf("%s%s:*", aliasKeyPrefix, funcID)
	keys, err := s.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	aliases := make([]*domain.FunctionAlias, 0, len(keys))
	for _, key := range keys {
		data, err := s.client.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		var alias domain.FunctionAlias
		if err := json.Unmarshal(data, &alias); err != nil {
			continue
		}
		aliases = append(aliases, &alias)
	}
	return aliases, nil
}

// DeleteAlias removes an alias
func (s *RedisStore) DeleteAlias(ctx context.Context, funcID, aliasName string) error {
	key := fmt.Sprintf("%s%s:%s", aliasKeyPrefix, funcID, aliasName)
	return s.client.Del(ctx, key).Err()
}
