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
	id, err := s.client.HGet(ctx, funcListKey, name).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("function not found: %s", name)
	}
	if err != nil {
		return nil, err
	}
	return s.GetFunction(ctx, id)
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
