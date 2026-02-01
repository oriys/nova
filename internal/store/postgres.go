package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oriys/nova/internal/domain"
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
	}

	for _, stmt := range stmts {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
	}
	return nil
}

func (s *PostgresStore) SaveFunction(ctx context.Context, fn *domain.Function) error {
	if fn.ID == "" || fn.Name == "" {
		return fmt.Errorf("function id and name are required")
	}

	now := time.Now()
	if fn.CreatedAt.IsZero() {
		fn.CreatedAt = now
	}
	if fn.UpdatedAt.IsZero() {
		fn.UpdatedAt = now
	}

	data, err := json.Marshal(fn)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO functions (id, name, data, created_at, updated_at)
		VALUES ($1, $2, $3::jsonb, $4, $5)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			data = EXCLUDED.data,
			updated_at = EXCLUDED.updated_at
	`, fn.ID, fn.Name, data, fn.CreatedAt, fn.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save function: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetFunction(ctx context.Context, id string) (*domain.Function, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, `SELECT data FROM functions WHERE id = $1`, id).Scan(&data)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("function not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get function: %w", err)
	}

	var fn domain.Function
	if err := json.Unmarshal(data, &fn); err != nil {
		return nil, err
	}
	return &fn, nil
}

func (s *PostgresStore) GetFunctionByName(ctx context.Context, name string) (*domain.Function, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, `SELECT data FROM functions WHERE name = $1`, name).Scan(&data)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("function not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("get function by name: %w", err)
	}

	var fn domain.Function
	if err := json.Unmarshal(data, &fn); err != nil {
		return nil, err
	}
	return &fn, nil
}

func (s *PostgresStore) DeleteFunction(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM functions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete function: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("function not found: %s", id)
	}
	return nil
}

func (s *PostgresStore) ListFunctions(ctx context.Context) ([]*domain.Function, error) {
	rows, err := s.pool.Query(ctx, `SELECT data FROM functions ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list functions: %w", err)
	}
	defer rows.Close()

	var functions []*domain.Function
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("list functions scan: %w", err)
		}
		var fn domain.Function
		if err := json.Unmarshal(data, &fn); err != nil {
			continue
		}
		functions = append(functions, &fn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list functions rows: %w", err)
	}
	return functions, nil
}

func (s *PostgresStore) UpdateFunction(ctx context.Context, name string, update *FunctionUpdate) (*domain.Function, error) {
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

func (s *PostgresStore) PublishVersion(ctx context.Context, funcID string, version *domain.FunctionVersion) error {
	if funcID == "" || version == nil {
		return fmt.Errorf("function id and version are required")
	}
	if version.CreatedAt.IsZero() {
		version.CreatedAt = time.Now()
	}

	data, err := json.Marshal(version)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO function_versions (function_id, version, data, created_at)
		VALUES ($1, $2, $3::jsonb, $4)
		ON CONFLICT (function_id, version) DO UPDATE SET
			data = EXCLUDED.data
	`, funcID, version.Version, data, version.CreatedAt)
	if err != nil {
		return fmt.Errorf("publish version: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetVersion(ctx context.Context, funcID string, version int) (*domain.FunctionVersion, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, `
		SELECT data FROM function_versions
		WHERE function_id = $1 AND version = $2
	`, funcID, version).Scan(&data)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("version not found: %s v%d", funcID, version)
	}
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}

	var v domain.FunctionVersion
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *PostgresStore) ListVersions(ctx context.Context, funcID string) ([]*domain.FunctionVersion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT data FROM function_versions
		WHERE function_id = $1
		ORDER BY version ASC
	`, funcID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	defer rows.Close()

	var versions []*domain.FunctionVersion
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("list versions scan: %w", err)
		}
		var v domain.FunctionVersion
		if err := json.Unmarshal(data, &v); err != nil {
			continue
		}
		versions = append(versions, &v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list versions rows: %w", err)
	}
	return versions, nil
}

func (s *PostgresStore) DeleteVersion(ctx context.Context, funcID string, version int) error {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM function_versions
		WHERE function_id = $1 AND version = $2
	`, funcID, version)
	if err != nil {
		return fmt.Errorf("delete version: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("version not found: %s v%d", funcID, version)
	}
	return nil
}

func (s *PostgresStore) SetAlias(ctx context.Context, alias *domain.FunctionAlias) error {
	if alias == nil || alias.FunctionID == "" || alias.Name == "" {
		return fmt.Errorf("alias function_id and name are required")
	}

	now := time.Now()
	if alias.CreatedAt.IsZero() {
		alias.CreatedAt = now
	}
	if alias.UpdatedAt.IsZero() {
		alias.UpdatedAt = now
	}

	data, err := json.Marshal(alias)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO function_aliases (function_id, name, data, created_at, updated_at)
		VALUES ($1, $2, $3::jsonb, $4, $5)
		ON CONFLICT (function_id, name) DO UPDATE SET
			data = EXCLUDED.data,
			updated_at = EXCLUDED.updated_at
	`, alias.FunctionID, alias.Name, data, alias.CreatedAt, alias.UpdatedAt)
	if err != nil {
		return fmt.Errorf("set alias: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetAlias(ctx context.Context, funcID, aliasName string) (*domain.FunctionAlias, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, `
		SELECT data FROM function_aliases
		WHERE function_id = $1 AND name = $2
	`, funcID, aliasName).Scan(&data)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("alias not found: %s@%s", funcID, aliasName)
	}
	if err != nil {
		return nil, fmt.Errorf("get alias: %w", err)
	}

	var alias domain.FunctionAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return nil, err
	}
	return &alias, nil
}

func (s *PostgresStore) ListAliases(ctx context.Context, funcID string) ([]*domain.FunctionAlias, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT data FROM function_aliases
		WHERE function_id = $1
		ORDER BY name ASC
	`, funcID)
	if err != nil {
		return nil, fmt.Errorf("list aliases: %w", err)
	}
	defer rows.Close()

	var aliases []*domain.FunctionAlias
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("list aliases scan: %w", err)
		}
		var alias domain.FunctionAlias
		if err := json.Unmarshal(data, &alias); err != nil {
			continue
		}
		aliases = append(aliases, &alias)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list aliases rows: %w", err)
	}
	return aliases, nil
}

func (s *PostgresStore) DeleteAlias(ctx context.Context, funcID, aliasName string) error {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM function_aliases
		WHERE function_id = $1 AND name = $2
	`, funcID, aliasName)
	if err != nil {
		return fmt.Errorf("delete alias: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("alias not found: %s@%s", funcID, aliasName)
	}
	return nil
}

// InvocationLog represents a single function invocation record
type InvocationLog struct {
	ID           string    `json:"id"`
	FunctionID   string    `json:"function_id"`
	FunctionName string    `json:"function_name"`
	Runtime      string    `json:"runtime"`
	DurationMs   int64     `json:"duration_ms"`
	ColdStart    bool      `json:"cold_start"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message,omitempty"`
	InputSize    int       `json:"input_size"`
	OutputSize   int       `json:"output_size"`
	Stdout       string    `json:"stdout,omitempty"`
	Stderr       string    `json:"stderr,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// TimeSeriesBucket represents aggregated metrics for a time period
type TimeSeriesBucket struct {
	Timestamp   time.Time `json:"timestamp"`
	Invocations int64     `json:"invocations"`
	Errors      int64     `json:"errors"`
	AvgDuration float64   `json:"avg_duration"`
}

// RuntimeRecord represents a runtime configuration in the database
type RuntimeRecord struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Version        string    `json:"version"`
	Status         string    `json:"status"`
	FunctionsCount int       `json:"functions_count"`
	CreatedAt      time.Time `json:"created_at"`
}

// SaveInvocationLog saves an invocation log entry to the database
func (s *PostgresStore) SaveInvocationLog(ctx context.Context, log *InvocationLog) error {
	if log.ID == "" {
		return fmt.Errorf("invocation log id is required")
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO invocation_logs (id, function_id, function_name, runtime, duration_ms, cold_start, success, error_message, input_size, output_size, stdout, stderr, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (id) DO NOTHING
	`, log.ID, log.FunctionID, log.FunctionName, log.Runtime, log.DurationMs, log.ColdStart, log.Success, log.ErrorMessage, log.InputSize, log.OutputSize, log.Stdout, log.Stderr, log.CreatedAt)
	if err != nil {
		return fmt.Errorf("save invocation log: %w", err)
	}
	return nil
}

// ListInvocationLogs returns recent invocation logs for a function
func (s *PostgresStore) ListInvocationLogs(ctx context.Context, functionID string, limit int) ([]*InvocationLog, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, function_id, function_name, runtime, duration_ms, cold_start, success, error_message, input_size, output_size, stdout, stderr, created_at
		FROM invocation_logs
		WHERE function_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, functionID, limit)
	if err != nil {
		return nil, fmt.Errorf("list invocation logs: %w", err)
	}
	defer rows.Close()

	var logs []*InvocationLog
	for rows.Next() {
		var log InvocationLog
		var errorMessage, stdout, stderr *string
		if err := rows.Scan(&log.ID, &log.FunctionID, &log.FunctionName, &log.Runtime, &log.DurationMs, &log.ColdStart, &log.Success, &errorMessage, &log.InputSize, &log.OutputSize, &stdout, &stderr, &log.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan invocation log: %w", err)
		}
		if errorMessage != nil {
			log.ErrorMessage = *errorMessage
		}
		if stdout != nil {
			log.Stdout = *stdout
		}
		if stderr != nil {
			log.Stderr = *stderr
		}
		logs = append(logs, &log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list invocation logs rows: %w", err)
	}
	return logs, nil
}

// ListAllInvocationLogs returns recent invocation logs across all functions
func (s *PostgresStore) ListAllInvocationLogs(ctx context.Context, limit int) ([]*InvocationLog, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, function_id, function_name, runtime, duration_ms, cold_start, success, error_message, input_size, output_size, stdout, stderr, created_at
		FROM invocation_logs
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list all invocation logs: %w", err)
	}
	defer rows.Close()

	var logs []*InvocationLog
	for rows.Next() {
		var log InvocationLog
		var errorMessage, stdout, stderr *string
		if err := rows.Scan(&log.ID, &log.FunctionID, &log.FunctionName, &log.Runtime, &log.DurationMs, &log.ColdStart, &log.Success, &errorMessage, &log.InputSize, &log.OutputSize, &stdout, &stderr, &log.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan invocation log: %w", err)
		}
		if errorMessage != nil {
			log.ErrorMessage = *errorMessage
		}
		if stdout != nil {
			log.Stdout = *stdout
		}
		if stderr != nil {
			log.Stderr = *stderr
		}
		logs = append(logs, &log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list all invocation logs rows: %w", err)
	}
	return logs, nil
}

// GetInvocationLog retrieves a single invocation log by request ID
func (s *PostgresStore) GetInvocationLog(ctx context.Context, requestID string) (*InvocationLog, error) {
	var log InvocationLog
	var errorMessage, stdout, stderr *string
	err := s.pool.QueryRow(ctx, `
		SELECT id, function_id, function_name, runtime, duration_ms, cold_start, success, error_message, input_size, output_size, stdout, stderr, created_at
		FROM invocation_logs
		WHERE id = $1
	`, requestID).Scan(&log.ID, &log.FunctionID, &log.FunctionName, &log.Runtime, &log.DurationMs, &log.ColdStart, &log.Success, &errorMessage, &log.InputSize, &log.OutputSize, &stdout, &stderr, &log.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("invocation log not found: %s", requestID)
	}
	if err != nil {
		return nil, fmt.Errorf("get invocation log: %w", err)
	}
	if errorMessage != nil {
		log.ErrorMessage = *errorMessage
	}
	if stdout != nil {
		log.Stdout = *stdout
	}
	if stderr != nil {
		log.Stderr = *stderr
	}
	return &log, nil
}

// GetFunctionTimeSeries returns hourly aggregated metrics for a function
func (s *PostgresStore) GetFunctionTimeSeries(ctx context.Context, functionID string, hours int) ([]TimeSeriesBucket, error) {
	if hours <= 0 {
		hours = 24
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			date_trunc('hour', created_at) AS bucket,
			COUNT(*) AS invocations,
			COUNT(*) FILTER (WHERE NOT success) AS errors,
			AVG(duration_ms) AS avg_duration
		FROM invocation_logs
		WHERE function_id = $1
		  AND created_at >= NOW() - INTERVAL '1 hour' * $2
		GROUP BY bucket
		ORDER BY bucket ASC
	`, functionID, hours)
	if err != nil {
		return nil, fmt.Errorf("get function time series: %w", err)
	}
	defer rows.Close()

	buckets := make([]TimeSeriesBucket, 0)
	for rows.Next() {
		var bucket TimeSeriesBucket
		if err := rows.Scan(&bucket.Timestamp, &bucket.Invocations, &bucket.Errors, &bucket.AvgDuration); err != nil {
			return nil, fmt.Errorf("scan time series: %w", err)
		}
		buckets = append(buckets, bucket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get function time series rows: %w", err)
	}
	return buckets, nil
}

// GetGlobalTimeSeries returns hourly aggregated metrics across all functions
func (s *PostgresStore) GetGlobalTimeSeries(ctx context.Context, hours int) ([]TimeSeriesBucket, error) {
	if hours <= 0 {
		hours = 24
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			date_trunc('hour', created_at) AS bucket,
			COUNT(*) AS invocations,
			COUNT(*) FILTER (WHERE NOT success) AS errors,
			AVG(duration_ms) AS avg_duration
		FROM invocation_logs
		WHERE created_at >= NOW() - INTERVAL '1 hour' * $1
		GROUP BY bucket
		ORDER BY bucket ASC
	`, hours)
	if err != nil {
		return nil, fmt.Errorf("get global time series: %w", err)
	}
	defer rows.Close()

	buckets := make([]TimeSeriesBucket, 0)
	for rows.Next() {
		var bucket TimeSeriesBucket
		if err := rows.Scan(&bucket.Timestamp, &bucket.Invocations, &bucket.Errors, &bucket.AvgDuration); err != nil {
			return nil, fmt.Errorf("scan time series: %w", err)
		}
		buckets = append(buckets, bucket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get global time series rows: %w", err)
	}
	return buckets, nil
}

// SaveRuntime creates or updates a runtime record
func (s *PostgresStore) SaveRuntime(ctx context.Context, rt *RuntimeRecord) error {
	if rt.ID == "" {
		return fmt.Errorf("runtime id is required")
	}
	if rt.Status == "" {
		rt.Status = "available"
	}
	if rt.CreatedAt.IsZero() {
		rt.CreatedAt = time.Now()
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO runtimes (id, name, version, status, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			version = EXCLUDED.version,
			status = EXCLUDED.status
	`, rt.ID, rt.Name, rt.Version, rt.Status, rt.CreatedAt)
	if err != nil {
		return fmt.Errorf("save runtime: %w", err)
	}
	return nil
}

// GetRuntime retrieves a runtime by ID
func (s *PostgresStore) GetRuntime(ctx context.Context, id string) (*RuntimeRecord, error) {
	var rt RuntimeRecord
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, version, status, created_at
		FROM runtimes
		WHERE id = $1
	`, id).Scan(&rt.ID, &rt.Name, &rt.Version, &rt.Status, &rt.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("runtime not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get runtime: %w", err)
	}
	return &rt, nil
}

// ListRuntimes returns all runtimes with function counts
func (s *PostgresStore) ListRuntimes(ctx context.Context) ([]*RuntimeRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.name, r.version, r.status, r.created_at, COUNT(f.id) as functions_count
		FROM runtimes r
		LEFT JOIN functions f ON f.data->>'runtime' = r.id
		GROUP BY r.id, r.name, r.version, r.status, r.created_at
		ORDER BY r.name, r.version DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list runtimes: %w", err)
	}
	defer rows.Close()

	var runtimes []*RuntimeRecord
	for rows.Next() {
		var rt RuntimeRecord
		if err := rows.Scan(&rt.ID, &rt.Name, &rt.Version, &rt.Status, &rt.CreatedAt, &rt.FunctionsCount); err != nil {
			return nil, fmt.Errorf("scan runtime: %w", err)
		}
		runtimes = append(runtimes, &rt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runtimes rows: %w", err)
	}
	return runtimes, nil
}

// DeleteRuntime removes a runtime by ID
func (s *PostgresStore) DeleteRuntime(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM runtimes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete runtime: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("runtime not found: %s", id)
	}
	return nil
}

// GetConfig retrieves all config key-value pairs
func (s *PostgresStore) GetConfig(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, value FROM config ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}
	defer rows.Close()

	config := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan config: %w", err)
		}
		config[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get config rows: %w", err)
	}
	return config, nil
}

// SetConfig upserts a config key-value pair
func (s *PostgresStore) SetConfig(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO config (key, value, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			updated_at = NOW()
	`, key, value)
	if err != nil {
		return fmt.Errorf("set config: %w", err)
	}
	return nil
}

// ─── API Keys ───────────────────────────────────────────────────────────────

// APIKeyRecord represents an API key in the database
type APIKeyRecord struct {
	Name      string     `json:"name"`
	KeyHash   string     `json:"key_hash"`
	Tier      string     `json:"tier"`
	Enabled   bool       `json:"enabled"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// SaveAPIKey creates or updates an API key
func (s *PostgresStore) SaveAPIKey(ctx context.Context, key *APIKeyRecord) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO api_keys (name, key_hash, tier, enabled, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (name) DO UPDATE SET
			key_hash = EXCLUDED.key_hash,
			tier = EXCLUDED.tier,
			enabled = EXCLUDED.enabled,
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
	`, key.Name, key.KeyHash, key.Tier, key.Enabled, key.ExpiresAt, key.CreatedAt, key.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save api key: %w", err)
	}
	return nil
}

// GetAPIKeyByHash retrieves an API key by its hash
func (s *PostgresStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKeyRecord, error) {
	var key APIKeyRecord
	err := s.pool.QueryRow(ctx, `
		SELECT name, key_hash, tier, enabled, expires_at, created_at, updated_at
		FROM api_keys WHERE key_hash = $1
	`, keyHash).Scan(&key.Name, &key.KeyHash, &key.Tier, &key.Enabled, &key.ExpiresAt, &key.CreatedAt, &key.UpdatedAt)
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
		SELECT name, key_hash, tier, enabled, expires_at, created_at, updated_at
		FROM api_keys WHERE name = $1
	`, name).Scan(&key.Name, &key.KeyHash, &key.Tier, &key.Enabled, &key.ExpiresAt, &key.CreatedAt, &key.UpdatedAt)
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
		SELECT name, key_hash, tier, enabled, expires_at, created_at, updated_at
		FROM api_keys ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []*APIKeyRecord
	for rows.Next() {
		var key APIKeyRecord
		if err := rows.Scan(&key.Name, &key.KeyHash, &key.Tier, &key.Enabled, &key.ExpiresAt, &key.CreatedAt, &key.UpdatedAt); err != nil {
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

// RateLimitBucket represents a token bucket for rate limiting
type RateLimitBucket struct {
	Key        string
	Tokens     float64
	LastRefill time.Time
}

// CheckRateLimit performs token bucket rate limiting
// Returns (allowed, remaining tokens)
func (s *PostgresStore) CheckRateLimit(ctx context.Context, key string, maxTokens int, refillRate float64, requested int) (bool, int, error) {
	now := time.Now()

	// Use a transaction with row-level locking for atomicity
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, 0, fmt.Errorf("begin rate limit tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get or create bucket with lock
	var tokens float64
	var lastRefill time.Time
	err = tx.QueryRow(ctx, `
		SELECT tokens, last_refill FROM rate_limit_buckets
		WHERE key = $1 FOR UPDATE
	`, key).Scan(&tokens, &lastRefill)

	if err == pgx.ErrNoRows {
		// New bucket starts full
		tokens = float64(maxTokens)
		lastRefill = now
	} else if err != nil {
		return false, 0, fmt.Errorf("get rate limit bucket: %w", err)
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(lastRefill).Seconds()
	tokens = min(float64(maxTokens), tokens+elapsed*refillRate)

	// Check if request is allowed
	allowed := tokens >= float64(requested)
	if allowed {
		tokens -= float64(requested)
	}

	// Update bucket
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
