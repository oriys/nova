package store

import (
	"context"
	"encoding/json"
	"fmt"

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
