package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	if dsn == "" {
		return nil, fmt.Errorf("postgres DSN is required")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	s := &PostgresStore{pool: pool}

	if err := s.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	if err := s.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return s, nil
}

func (s *PostgresStore) Close() error {
	if s.pool != nil {
		s.pool.Close()
	}
	return nil
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	if s.pool == nil {
		return fmt.Errorf("postgres not initialized")
	}
	return s.pool.Ping(ctx)
}

func (s *PostgresStore) ensureSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS functions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			data JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_functions_runtime ON functions ((data->>'runtime'))`,
		`CREATE TABLE IF NOT EXISTS function_versions (
			function_id TEXT NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
			version INTEGER NOT NULL,
			data JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (function_id, version)
		)`,
		`CREATE TABLE IF NOT EXISTS function_aliases (
			function_id TEXT NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			data JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (function_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS invocation_logs (
			id TEXT PRIMARY KEY,
			function_id TEXT NOT NULL,
			function_name TEXT NOT NULL,
			runtime TEXT NOT NULL,
			duration_ms BIGINT NOT NULL,
			cold_start BOOLEAN NOT NULL DEFAULT FALSE,
			success BOOLEAN NOT NULL DEFAULT TRUE,
			error_message TEXT,
			input_size INTEGER DEFAULT 0,
			output_size INTEGER DEFAULT 0,
			stdout TEXT,
			stderr TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_invocation_logs_function_id ON invocation_logs(function_id)`,
		`CREATE INDEX IF NOT EXISTS idx_invocation_logs_created_at ON invocation_logs(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_invocation_logs_func_time ON invocation_logs(function_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS runtimes (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			version TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'available',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			name TEXT PRIMARY KEY,
			key_hash TEXT NOT NULL UNIQUE,
			tier TEXT NOT NULL DEFAULT 'default',
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			expires_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash)`,
		`CREATE TABLE IF NOT EXISTS secrets (
			name TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS rate_limit_buckets (
			key TEXT PRIMARY KEY,
			tokens DOUBLE PRECISION NOT NULL,
			last_refill TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS function_code (
			function_id TEXT NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
			source_code TEXT NOT NULL,
			compiled_binary BYTEA,
			source_hash TEXT NOT NULL,
			binary_hash TEXT,
			compile_status TEXT NOT NULL DEFAULT 'pending',
			compile_error TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (function_id)
		)`,
	}

	for _, stmt := range stmts {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
	}
	return nil
}