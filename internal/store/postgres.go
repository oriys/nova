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
