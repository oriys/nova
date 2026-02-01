package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-redis/redis/v8"
	"github.com/oriys/nova/internal/domain"
)

// MetadataStore is the durable metadata store (functions, versions, aliases).
type MetadataStore interface {
	Close() error
	Ping(ctx context.Context) error

	SaveFunction(ctx context.Context, fn *domain.Function) error
	GetFunction(ctx context.Context, id string) (*domain.Function, error)
	GetFunctionByName(ctx context.Context, name string) (*domain.Function, error)
	DeleteFunction(ctx context.Context, id string) error
	ListFunctions(ctx context.Context) ([]*domain.Function, error)
	UpdateFunction(ctx context.Context, name string, update *FunctionUpdate) (*domain.Function, error)

	PublishVersion(ctx context.Context, funcID string, version *domain.FunctionVersion) error
	GetVersion(ctx context.Context, funcID string, version int) (*domain.FunctionVersion, error)
	ListVersions(ctx context.Context, funcID string) ([]*domain.FunctionVersion, error)
	DeleteVersion(ctx context.Context, funcID string, version int) error

	SetAlias(ctx context.Context, alias *domain.FunctionAlias) error
	GetAlias(ctx context.Context, funcID, aliasName string) (*domain.FunctionAlias, error)
	ListAliases(ctx context.Context, funcID string) ([]*domain.FunctionAlias, error)
	DeleteAlias(ctx context.Context, funcID, aliasName string) error
}

// Store is the combined backend store: Postgres for metadata + Redis for caching/rate limits/logs/secrets.
type Store struct {
	MetadataStore
	redis *RedisStore
}

func NewStore(meta MetadataStore, redisStore *RedisStore) *Store {
	return &Store{
		MetadataStore: meta,
		redis:         redisStore,
	}
}

func (s *Store) Client() *redis.Client {
	if s.redis == nil {
		return nil
	}
	return s.redis.Client()
}

func (s *Store) PingPostgres(ctx context.Context) error {
	if s.MetadataStore == nil {
		return fmt.Errorf("postgres not configured")
	}
	return s.MetadataStore.Ping(ctx)
}

func (s *Store) PingRedis(ctx context.Context) error {
	if s.redis == nil {
		return fmt.Errorf("redis not configured")
	}
	return s.redis.Ping(ctx)
}

func (s *Store) Ping(ctx context.Context) error {
	var errs []error
	if err := s.PingPostgres(ctx); err != nil {
		errs = append(errs, fmt.Errorf("postgres: %w", err))
	}
	if err := s.PingRedis(ctx); err != nil {
		errs = append(errs, fmt.Errorf("redis: %w", err))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (s *Store) Close() error {
	var errs []error
	if s.MetadataStore != nil {
		if err := s.MetadataStore.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.redis != nil {
		if err := s.redis.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
