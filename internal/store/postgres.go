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
			input JSONB,
			output JSONB,
			stdout TEXT,
			stderr TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE invocation_logs ADD COLUMN IF NOT EXISTS input JSONB`,
		`ALTER TABLE invocation_logs ADD COLUMN IF NOT EXISTS output JSONB`,
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
		`ALTER TABLE runtimes ADD COLUMN IF NOT EXISTS image_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE runtimes ADD COLUMN IF NOT EXISTS entrypoint TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[]`,
		`ALTER TABLE runtimes ADD COLUMN IF NOT EXISTS file_extension VARCHAR(10) NOT NULL DEFAULT ''`,
		`ALTER TABLE runtimes ADD COLUMN IF NOT EXISTS env_vars JSONB NOT NULL DEFAULT '{}'::jsonb`,
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
		`CREATE TABLE IF NOT EXISTS function_files (
			id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			function_id TEXT NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
			path TEXT NOT NULL,
			content BYTEA NOT NULL,
			is_binary BOOLEAN DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(function_id, path)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_function_files_function_id ON function_files(function_id)`,

		// DAG Workflow tables
		`CREATE TABLE IF NOT EXISTS dag_workflows (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			current_version INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS dag_workflow_versions (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL REFERENCES dag_workflows(id) ON DELETE CASCADE,
			version INTEGER NOT NULL,
			definition JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(workflow_id, version)
		)`,
		`CREATE TABLE IF NOT EXISTS dag_workflow_nodes (
			id TEXT PRIMARY KEY,
			version_id TEXT NOT NULL REFERENCES dag_workflow_versions(id) ON DELETE CASCADE,
			node_key TEXT NOT NULL,
			function_name TEXT NOT NULL,
			input_mapping JSONB,
			retry_policy JSONB,
			timeout_s INTEGER NOT NULL DEFAULT 30,
			position INTEGER NOT NULL DEFAULT 0,
			UNIQUE(version_id, node_key)
		)`,
		`CREATE TABLE IF NOT EXISTS dag_workflow_edges (
			id TEXT PRIMARY KEY,
			version_id TEXT NOT NULL REFERENCES dag_workflow_versions(id) ON DELETE CASCADE,
			from_node_id TEXT NOT NULL REFERENCES dag_workflow_nodes(id) ON DELETE CASCADE,
			to_node_id TEXT NOT NULL REFERENCES dag_workflow_nodes(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS dag_runs (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL REFERENCES dag_workflows(id) ON DELETE CASCADE,
			version_id TEXT NOT NULL REFERENCES dag_workflow_versions(id) ON DELETE CASCADE,
			status TEXT NOT NULL DEFAULT 'pending',
			trigger_type TEXT NOT NULL DEFAULT 'manual',
			input JSONB,
			output JSONB,
			error_message TEXT,
			started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			finished_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dag_runs_workflow ON dag_runs(workflow_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS dag_run_nodes (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL REFERENCES dag_runs(id) ON DELETE CASCADE,
			node_id TEXT NOT NULL REFERENCES dag_workflow_nodes(id) ON DELETE CASCADE,
			node_key TEXT NOT NULL,
			function_name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			unresolved_deps INTEGER NOT NULL DEFAULT 0,
			attempt INTEGER NOT NULL DEFAULT 0,
			input JSONB,
			output JSONB,
			error_message TEXT,
			lease_owner TEXT,
			lease_expires_at TIMESTAMPTZ,
			started_at TIMESTAMPTZ,
			finished_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dag_run_nodes_ready ON dag_run_nodes(status, lease_expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_dag_run_nodes_run ON dag_run_nodes(run_id)`,
		`CREATE TABLE IF NOT EXISTS dag_node_attempts (
			id TEXT PRIMARY KEY,
			run_node_id TEXT NOT NULL REFERENCES dag_run_nodes(id) ON DELETE CASCADE,
			attempt INTEGER NOT NULL,
			status TEXT NOT NULL,
			input JSONB,
			output JSONB,
			error TEXT,
			duration_ms BIGINT NOT NULL DEFAULT 0,
			started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			finished_at TIMESTAMPTZ,
			UNIQUE(run_node_id, attempt)
		)`,

		// Schedules table
		`CREATE TABLE IF NOT EXISTS schedules (
			id TEXT PRIMARY KEY,
			function_name TEXT NOT NULL,
			cron_expression TEXT NOT NULL,
			input JSONB,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			last_run_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_schedules_function ON schedules(function_name)`,
	}

	for _, stmt := range stmts {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
	}
	return nil
}
