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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin schema transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Serialize schema initialization across processes to avoid Postgres catalog
	// races when multiple services start at the same time.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(0x6e6f7661)); err != nil {
		return fmt.Errorf("acquire schema lock: %w", err)
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			tier TEXT NOT NULL DEFAULT 'default',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`INSERT INTO tenants (id, name, status, tier) VALUES ('default', 'Default Tenant', 'active', 'default') ON CONFLICT (id) DO NOTHING`,
		`CREATE TABLE IF NOT EXISTS namespaces (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(tenant_id, name)
		)`,
		`INSERT INTO namespaces (id, tenant_id, name) VALUES ('default/default', 'default', 'default') ON CONFLICT (id) DO NOTHING`,
		`CREATE TABLE IF NOT EXISTS tenant_members (
			tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			subject TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (tenant_id, subject)
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_quotas (
			tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			dimension TEXT NOT NULL,
			hard_limit BIGINT NOT NULL DEFAULT 0,
			soft_limit BIGINT NOT NULL DEFAULT 0,
			burst BIGINT NOT NULL DEFAULT 0,
			window_s INTEGER NOT NULL DEFAULT 60,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (tenant_id, dimension)
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_usage_current (
			tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			dimension TEXT NOT NULL,
			used BIGINT NOT NULL DEFAULT 0,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (tenant_id, dimension)
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_usage_timeseries (
			tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			dimension TEXT NOT NULL,
			ts TIMESTAMPTZ NOT NULL,
			used BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (tenant_id, dimension, ts)
		)`,
		`CREATE TABLE IF NOT EXISTS functions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			data JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
		`ALTER TABLE functions ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE functions ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT 'default'`,
		`UPDATE functions SET tenant_id = 'default' WHERE tenant_id IS NULL OR tenant_id = ''`,
		`UPDATE functions SET namespace = 'default' WHERE namespace IS NULL OR namespace = ''`,
		`ALTER TABLE functions DROP CONSTRAINT IF EXISTS functions_name_key`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_functions_tenant_namespace_name ON functions(tenant_id, namespace, name)`,
		`CREATE INDEX IF NOT EXISTS idx_functions_tenant_namespace ON functions(tenant_id, namespace)`,
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
		`ALTER TABLE invocation_logs ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE invocation_logs ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE invocation_logs ADD COLUMN IF NOT EXISTS input JSONB`,
		`ALTER TABLE invocation_logs ADD COLUMN IF NOT EXISTS output JSONB`,
		`CREATE INDEX IF NOT EXISTS idx_invocation_logs_tenant_namespace_created ON invocation_logs(tenant_id, namespace, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_invocation_logs_function_id ON invocation_logs(function_id)`,
		`CREATE INDEX IF NOT EXISTS idx_invocation_logs_created_at ON invocation_logs(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_invocation_logs_func_time ON invocation_logs(function_id, created_at DESC)`,
		`DELETE FROM invocation_logs l
			WHERE NOT EXISTS (
				SELECT 1
				FROM functions f
				WHERE f.id = l.function_id
				  AND f.tenant_id = l.tenant_id
				  AND f.namespace = l.namespace
			)`,
		`CREATE TABLE IF NOT EXISTS async_invocations (
			id TEXT PRIMARY KEY,
			function_id TEXT NOT NULL,
			function_name TEXT NOT NULL,
			payload JSONB NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			attempt INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			backoff_base_ms INTEGER NOT NULL DEFAULT 1000,
			backoff_max_ms INTEGER NOT NULL DEFAULT 60000,
			next_run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			locked_by TEXT,
			locked_until TIMESTAMPTZ,
			request_id TEXT,
			output JSONB,
			duration_ms BIGINT NOT NULL DEFAULT 0,
			cold_start BOOLEAN NOT NULL DEFAULT FALSE,
			last_error TEXT,
			started_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE async_invocations ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE async_invocations ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT 'default'`,
		`CREATE INDEX IF NOT EXISTS idx_async_invocations_tenant_namespace_created ON async_invocations(tenant_id, namespace, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_async_invocations_status_next_run ON async_invocations(status, next_run_at)`,
		`CREATE INDEX IF NOT EXISTS idx_async_invocations_function_created ON async_invocations(function_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_async_invocations_created ON async_invocations(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS idempotency_keys (
			scope TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			idempotency_key TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (scope, scope_id, idempotency_key)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_idempotency_keys_expires_at ON idempotency_keys(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_idempotency_keys_resource_id ON idempotency_keys(resource_id)`,
		`CREATE TABLE IF NOT EXISTS event_topics (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			retention_hours INTEGER NOT NULL DEFAULT 168,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE event_topics ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE event_topics ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE event_topics DROP CONSTRAINT IF EXISTS event_topics_name_key`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_event_topics_tenant_namespace_name ON event_topics(tenant_id, namespace, name)`,
		`CREATE INDEX IF NOT EXISTS idx_event_topics_tenant_namespace_created ON event_topics(tenant_id, namespace, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS event_subscriptions (
			id TEXT PRIMARY KEY,
			topic_id TEXT NOT NULL REFERENCES event_topics(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			consumer_group TEXT NOT NULL,
			function_id TEXT NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
			function_name TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			backoff_base_ms INTEGER NOT NULL DEFAULT 1000,
			backoff_max_ms INTEGER NOT NULL DEFAULT 60000,
			max_inflight INTEGER NOT NULL DEFAULT 0,
			rate_limit_per_sec INTEGER NOT NULL DEFAULT 0,
			last_dispatch_at TIMESTAMPTZ,
			last_acked_sequence BIGINT NOT NULL DEFAULT 0,
			last_acked_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(topic_id, name),
			UNIQUE(topic_id, consumer_group)
		)`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT 'default'`,
		`UPDATE event_subscriptions s
			SET tenant_id = t.tenant_id, namespace = t.namespace
			FROM event_topics t
			WHERE s.topic_id = t.id`,
		`CREATE INDEX IF NOT EXISTS idx_event_subscriptions_topic ON event_subscriptions(topic_id)`,
		`CREATE INDEX IF NOT EXISTS idx_event_subscriptions_tenant_namespace_topic ON event_subscriptions(tenant_id, namespace, topic_id)`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS max_inflight INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS rate_limit_per_sec INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS last_dispatch_at TIMESTAMPTZ`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS last_acked_sequence BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS last_acked_at TIMESTAMPTZ`,
		`CREATE TABLE IF NOT EXISTS event_messages (
			id TEXT PRIMARY KEY,
			topic_id TEXT NOT NULL REFERENCES event_topics(id) ON DELETE CASCADE,
			sequence BIGSERIAL UNIQUE,
			source_outbox_id TEXT UNIQUE,
			ordering_key TEXT NOT NULL DEFAULT '',
			payload JSONB NOT NULL,
			headers JSONB NOT NULL DEFAULT '{}'::jsonb,
			published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE event_messages ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE event_messages ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT 'default'`,
		`UPDATE event_messages m
			SET tenant_id = t.tenant_id, namespace = t.namespace
			FROM event_topics t
			WHERE m.topic_id = t.id`,
		`ALTER TABLE event_messages ADD COLUMN IF NOT EXISTS source_outbox_id TEXT UNIQUE`,
		`CREATE INDEX IF NOT EXISTS idx_event_messages_topic_sequence ON event_messages(topic_id, sequence DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_event_messages_topic_created ON event_messages(topic_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_event_messages_tenant_namespace_topic_sequence ON event_messages(tenant_id, namespace, topic_id, sequence DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_event_messages_source_outbox ON event_messages(source_outbox_id)`,
		`CREATE TABLE IF NOT EXISTS event_deliveries (
			id TEXT PRIMARY KEY,
			topic_id TEXT NOT NULL REFERENCES event_topics(id) ON DELETE CASCADE,
			subscription_id TEXT NOT NULL REFERENCES event_subscriptions(id) ON DELETE CASCADE,
			message_id TEXT NOT NULL REFERENCES event_messages(id) ON DELETE CASCADE,
			function_id TEXT NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
			function_name TEXT NOT NULL,
			ordering_key TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'queued',
			attempt INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			backoff_base_ms INTEGER NOT NULL DEFAULT 1000,
			backoff_max_ms INTEGER NOT NULL DEFAULT 60000,
			next_run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			locked_by TEXT,
			locked_until TIMESTAMPTZ,
			request_id TEXT,
			output JSONB,
			duration_ms BIGINT NOT NULL DEFAULT 0,
			cold_start BOOLEAN NOT NULL DEFAULT FALSE,
			last_error TEXT,
			started_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE event_deliveries ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE event_deliveries ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT 'default'`,
		`UPDATE event_deliveries d
			SET tenant_id = s.tenant_id, namespace = s.namespace
			FROM event_subscriptions s
			WHERE d.subscription_id = s.id`,
		`CREATE INDEX IF NOT EXISTS idx_event_deliveries_status_next_run ON event_deliveries(status, next_run_at)`,
		`CREATE INDEX IF NOT EXISTS idx_event_deliveries_subscription_created ON event_deliveries(subscription_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_event_deliveries_ordering ON event_deliveries(subscription_id, ordering_key, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_event_deliveries_tenant_namespace_status_next_run ON event_deliveries(tenant_id, namespace, status, next_run_at)`,
		`CREATE INDEX IF NOT EXISTS idx_event_deliveries_message ON event_deliveries(message_id)`,
		`CREATE TABLE IF NOT EXISTS event_outbox (
			id TEXT PRIMARY KEY,
			topic_id TEXT NOT NULL REFERENCES event_topics(id) ON DELETE CASCADE,
			topic_name TEXT NOT NULL,
			ordering_key TEXT NOT NULL DEFAULT '',
			payload JSONB NOT NULL,
			headers JSONB NOT NULL DEFAULT '{}'::jsonb,
			status TEXT NOT NULL DEFAULT 'pending',
			attempt INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 5,
			backoff_base_ms INTEGER NOT NULL DEFAULT 1000,
			backoff_max_ms INTEGER NOT NULL DEFAULT 60000,
			next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			locked_by TEXT,
			locked_until TIMESTAMPTZ,
			message_id TEXT,
			last_error TEXT,
			published_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE event_outbox ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE event_outbox ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT 'default'`,
		`UPDATE event_outbox o
			SET tenant_id = t.tenant_id, namespace = t.namespace
			FROM event_topics t
			WHERE o.topic_id = t.id`,
		`CREATE INDEX IF NOT EXISTS idx_event_outbox_status_next_attempt ON event_outbox(status, next_attempt_at)`,
		`CREATE INDEX IF NOT EXISTS idx_event_outbox_tenant_namespace_status_next_attempt ON event_outbox(tenant_id, namespace, status, next_attempt_at)`,
		`CREATE INDEX IF NOT EXISTS idx_event_outbox_topic_created ON event_outbox(topic_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS event_inbox (
			subscription_id TEXT NOT NULL REFERENCES event_subscriptions(id) ON DELETE CASCADE,
			message_id TEXT NOT NULL REFERENCES event_messages(id) ON DELETE CASCADE,
			delivery_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'processing',
			request_id TEXT,
			output JSONB,
			duration_ms BIGINT NOT NULL DEFAULT 0,
			cold_start BOOLEAN NOT NULL DEFAULT FALSE,
			last_error TEXT,
			succeeded_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (subscription_id, message_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_event_inbox_delivery ON event_inbox(delivery_id)`,
		`CREATE INDEX IF NOT EXISTS idx_event_inbox_status_updated ON event_inbox(status, updated_at DESC)`,

		// Webhook subscription support
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'function'`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS workflow_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS workflow_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS webhook_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS webhook_method TEXT NOT NULL DEFAULT 'POST'`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS webhook_headers JSONB NOT NULL DEFAULT '{}'::jsonb`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS webhook_signing_secret TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE event_subscriptions ADD COLUMN IF NOT EXISTS webhook_timeout_ms INTEGER NOT NULL DEFAULT 30000`,
		`ALTER TABLE event_subscriptions DROP COLUMN IF EXISTS transform_function_id`,
		`ALTER TABLE event_subscriptions DROP COLUMN IF EXISTS transform_function_name`,
		// Relax function FK constraints for non-function subscriptions (function_id may be empty)
		`ALTER TABLE event_subscriptions DROP CONSTRAINT IF EXISTS event_subscriptions_function_id_fkey`,
		`ALTER TABLE event_deliveries DROP CONSTRAINT IF EXISTS event_deliveries_function_id_fkey`,

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
		`ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT '[]'`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_tenant_namespace_name ON api_keys(tenant_id, namespace, name)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_tenant_namespace_hash ON api_keys(tenant_id, namespace, key_hash)`,
		`CREATE TABLE IF NOT EXISTS secrets (
			name TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE secrets ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE secrets ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT 'default'`,
		`CREATE INDEX IF NOT EXISTS idx_secrets_tenant_namespace_name ON secrets(tenant_id, namespace, name)`,
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
		`ALTER TABLE dag_workflows ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE dag_workflows ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT 'default'`,
		`CREATE INDEX IF NOT EXISTS idx_dag_workflows_tenant_namespace_created ON dag_workflows(tenant_id, namespace, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_dag_workflows_tenant_namespace_name ON dag_workflows(tenant_id, namespace, name)`,
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
		`ALTER TABLE schedules ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE schedules ADD COLUMN IF NOT EXISTS namespace TEXT NOT NULL DEFAULT 'default'`,
		`CREATE INDEX IF NOT EXISTS idx_schedules_function ON schedules(function_name)`,
		`CREATE INDEX IF NOT EXISTS idx_schedules_tenant_namespace_function ON schedules(tenant_id, namespace, function_name)`,
		`CREATE INDEX IF NOT EXISTS idx_schedules_tenant_namespace_id ON schedules(tenant_id, namespace, id)`,

		// Gateway routes table
		`CREATE TABLE IF NOT EXISTS gateway_routes (
			id TEXT PRIMARY KEY,
			domain TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL,
			function_name TEXT NOT NULL,
			data JSONB NOT NULL,
			enabled BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(domain, path)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gateway_routes_domain ON gateway_routes(domain)`,

		// Layers tables
		`CREATE TABLE IF NOT EXISTS layers (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			runtime TEXT NOT NULL,
			version TEXT NOT NULL DEFAULT '1.0',
			size_mb INTEGER NOT NULL DEFAULT 0,
			files TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
			image_path TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE layers ADD COLUMN IF NOT EXISTS content_hash TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS function_layers (
			function_id TEXT NOT NULL,
			layer_id TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (function_id, layer_id)
		)`,

		// Volumes tables
		`CREATE TABLE IF NOT EXISTS volumes (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			namespace TEXT NOT NULL DEFAULT 'default',
			name TEXT NOT NULL,
			size_mb INTEGER NOT NULL,
			image_path TEXT NOT NULL,
			shared BOOLEAN NOT NULL DEFAULT FALSE,
			description TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(tenant_id, namespace, name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_volumes_tenant_namespace ON volumes(tenant_id, namespace)`,

		// Triggers tables
		`CREATE TABLE IF NOT EXISTS triggers (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			namespace TEXT NOT NULL DEFAULT 'default',
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			function_id TEXT NOT NULL,
			function_name TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			config JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(tenant_id, namespace, name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_triggers_tenant_namespace ON triggers(tenant_id, namespace)`,
		`CREATE INDEX IF NOT EXISTS idx_triggers_function ON triggers(function_id)`,
		`CREATE INDEX IF NOT EXISTS idx_triggers_enabled ON triggers(enabled)`,

		// Cluster nodes table
		`CREATE TABLE IF NOT EXISTS cluster_nodes (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			address TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'active',
			cpu_cores INTEGER NOT NULL DEFAULT 0,
			memory_mb INTEGER NOT NULL DEFAULT 0,
			max_vms INTEGER NOT NULL DEFAULT 0,
			active_vms INTEGER NOT NULL DEFAULT 0,
			queue_depth INTEGER NOT NULL DEFAULT 0,
			version TEXT NOT NULL DEFAULT '',
			labels JSONB NOT NULL DEFAULT '{}'::jsonb,
			last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cluster_nodes_state ON cluster_nodes(state)`,
		`CREATE INDEX IF NOT EXISTS idx_cluster_nodes_heartbeat ON cluster_nodes(last_heartbeat DESC)`,

		// Marketplace/App Store tables
		`CREATE TABLE IF NOT EXISTS app_store_apps (
			id TEXT PRIMARY KEY,
			slug TEXT NOT NULL UNIQUE,
			owner TEXT NOT NULL,
			visibility TEXT NOT NULL DEFAULT 'public',
			title TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			tags TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
			icon_url TEXT,
			source_url TEXT,
			homepage_url TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_apps_owner ON app_store_apps(owner)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_apps_visibility ON app_store_apps(visibility)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_apps_created ON app_store_apps(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS app_store_releases (
			id TEXT PRIMARY KEY,
			app_id TEXT NOT NULL REFERENCES app_store_apps(id) ON DELETE CASCADE,
			version TEXT NOT NULL,
			manifest_json JSONB NOT NULL,
			artifact_uri TEXT NOT NULL,
			artifact_digest TEXT NOT NULL,
			signature TEXT,
			status TEXT NOT NULL DEFAULT 'draft',
			changelog TEXT,
			requires_version TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(app_id, version)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_releases_app ON app_store_releases(app_id)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_releases_status ON app_store_releases(status)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_releases_created ON app_store_releases(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS app_store_installations (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			namespace TEXT NOT NULL,
			app_id TEXT NOT NULL REFERENCES app_store_apps(id) ON DELETE CASCADE,
			release_id TEXT NOT NULL REFERENCES app_store_releases(id) ON DELETE CASCADE,
			install_name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			values_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_by TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(tenant_id, namespace, install_name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_installations_tenant_namespace ON app_store_installations(tenant_id, namespace)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_installations_app ON app_store_installations(app_id)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_installations_release ON app_store_installations(release_id)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_installations_status ON app_store_installations(status)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_installations_created ON app_store_installations(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS app_store_installation_resources (
			id TEXT PRIMARY KEY,
			installation_id TEXT NOT NULL REFERENCES app_store_installations(id) ON DELETE CASCADE,
			resource_type TEXT NOT NULL,
			resource_name TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			content_digest TEXT NOT NULL,
			managed_mode TEXT NOT NULL DEFAULT 'exclusive',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_installation_resources_installation ON app_store_installation_resources(installation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_installation_resources_resource ON app_store_installation_resources(resource_type, resource_id)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_installation_resources_name ON app_store_installation_resources(resource_type, resource_name)`,
		`CREATE TABLE IF NOT EXISTS app_store_jobs (
			id TEXT PRIMARY KEY,
			installation_id TEXT NOT NULL REFERENCES app_store_installations(id) ON DELETE CASCADE,
			operation TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			step TEXT,
			error TEXT,
			started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			finished_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_jobs_installation ON app_store_jobs(installation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_jobs_operation ON app_store_jobs(operation)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_jobs_status ON app_store_jobs(status)`,
		`CREATE INDEX IF NOT EXISTS idx_app_store_jobs_started ON app_store_jobs(started_at DESC)`,

		// Additional indexes for list query optimization
		`CREATE INDEX IF NOT EXISTS idx_gateway_routes_domain_path ON gateway_routes(domain, path)`,
		`CREATE INDEX IF NOT EXISTS idx_layers_name ON layers(name)`,
		`CREATE INDEX IF NOT EXISTS idx_runtimes_name_version ON runtimes(name, version DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_tenant_namespace ON api_keys(tenant_id, namespace)`,
		`CREATE INDEX IF NOT EXISTS idx_secrets_tenant_namespace ON secrets(tenant_id, namespace)`,
		`CREATE INDEX IF NOT EXISTS idx_invocation_logs_tenant_ns_func_created ON invocation_logs(tenant_id, namespace, function_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_async_invocations_tenant_ns_status ON async_invocations(tenant_id, namespace, status)`,
		`DROP INDEX IF EXISTS idx_dag_workflows_tenant_namespace_name`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_dag_workflows_tenant_namespace_name_unique ON dag_workflows(tenant_id, namespace, name)`,

		// pg_trgm GIN index for ILIKE text search on function names
		`CREATE EXTENSION IF NOT EXISTS pg_trgm`,
		`CREATE INDEX IF NOT EXISTS idx_functions_name_trgm ON functions USING gin(name gin_trgm_ops)`,
	}

	for _, stmt := range stmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit schema transaction: %w", err)
	}
	return nil
}
