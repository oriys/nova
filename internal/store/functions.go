package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oriys/nova/internal/domain"
)

func (s *PostgresStore) SaveFunction(ctx context.Context, fn *domain.Function) error {
	if fn.ID == "" || fn.Name == "" {
		return fmt.Errorf("function id and name are required")
	}
	scope := tenantScopeFromContext(ctx)
	fn.TenantID = scope.TenantID
	fn.Namespace = scope.Namespace

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
		INSERT INTO functions (id, tenant_id, namespace, name, data, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			tenant_id = EXCLUDED.tenant_id,
			namespace = EXCLUDED.namespace,
			name = EXCLUDED.name,
			data = EXCLUDED.data,
			updated_at = EXCLUDED.updated_at
	`, fn.ID, fn.TenantID, fn.Namespace, fn.Name, data, fn.CreatedAt, fn.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save function: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetFunction(ctx context.Context, id string) (*domain.Function, error) {
	scope := tenantScopeFromContext(ctx)
	var data []byte
	err := s.pool.QueryRow(ctx, `
		SELECT data
		FROM functions
		WHERE id = $1 AND tenant_id = $2 AND namespace = $3
	`, id, scope.TenantID, scope.Namespace).Scan(&data)
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
	if fn.TenantID == "" {
		fn.TenantID = scope.TenantID
	}
	if fn.Namespace == "" {
		fn.Namespace = scope.Namespace
	}
	return &fn, nil
}

func (s *PostgresStore) GetFunctionByName(ctx context.Context, name string) (*domain.Function, error) {
	scope := tenantScopeFromContext(ctx)
	var data []byte
	err := s.pool.QueryRow(ctx, `
		SELECT data
		FROM functions
		WHERE name = $1 AND tenant_id = $2 AND namespace = $3
	`, name, scope.TenantID, scope.Namespace).Scan(&data)
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
	if fn.TenantID == "" {
		fn.TenantID = scope.TenantID
	}
	if fn.Namespace == "" {
		fn.Namespace = scope.Namespace
	}
	return &fn, nil
}

func (s *PostgresStore) DeleteFunction(ctx context.Context, id string) error {
	scope := tenantScopeFromContext(ctx)
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM functions
		WHERE id = $1 AND tenant_id = $2 AND namespace = $3
	`, id, scope.TenantID, scope.Namespace)
	if err != nil {
		return fmt.Errorf("delete function: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("function not found: %s", id)
	}
	return nil
}

func (s *PostgresStore) ListFunctions(ctx context.Context, limit, offset int) ([]*domain.Function, error) {
	scope := tenantScopeFromContext(ctx)
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `
		SELECT data
		FROM functions
		WHERE tenant_id = $1 AND namespace = $2
		ORDER BY name
		LIMIT $3 OFFSET $4
	`, scope.TenantID, scope.Namespace, limit, offset)
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
		if fn.TenantID == "" {
			fn.TenantID = scope.TenantID
		}
		if fn.Namespace == "" {
			fn.Namespace = scope.Namespace
		}
		functions = append(functions, &fn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list functions rows: %w", err)
	}
	return functions, nil
}

func (s *PostgresStore) SearchFunctions(ctx context.Context, query string, limit, offset int) ([]*domain.Function, error) {
	scope := tenantScopeFromContext(ctx)
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `
		SELECT data
		FROM functions
		WHERE tenant_id = $1 AND namespace = $2 AND name ILIKE $3
		ORDER BY name
		LIMIT $4 OFFSET $5
	`, scope.TenantID, scope.Namespace, "%"+query+"%", limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search functions: %w", err)
	}
	defer rows.Close()

	var functions []*domain.Function
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("search functions scan: %w", err)
		}
		var fn domain.Function
		if err := json.Unmarshal(data, &fn); err != nil {
			continue
		}
		if fn.TenantID == "" {
			fn.TenantID = scope.TenantID
		}
		if fn.Namespace == "" {
			fn.Namespace = scope.Namespace
		}
		functions = append(functions, &fn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search functions rows: %w", err)
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
	if update.MemoryMB != nil {
		fn.MemoryMB = *update.MemoryMB
	}
	if update.TimeoutS != nil {
		fn.TimeoutS = *update.TimeoutS
	}
	if update.MinReplicas != nil {
		fn.MinReplicas = *update.MinReplicas
	}
	if update.MaxReplicas != nil {
		fn.MaxReplicas = *update.MaxReplicas
	}
	if update.InstanceConcurrency != nil {
		fn.InstanceConcurrency = *update.InstanceConcurrency
	}
	if update.Mode != nil {
		fn.Mode = *update.Mode
	}
	if update.Limits != nil {
		fn.Limits = update.Limits
	}
	if update.NetworkPolicy != nil {
		fn.NetworkPolicy = update.NetworkPolicy
	}
	if update.AutoScalePolicy != nil {
		fn.AutoScalePolicy = update.AutoScalePolicy
	}
	if update.CapacityPolicy != nil {
		fn.CapacityPolicy = update.CapacityPolicy
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

func (s *PostgresStore) ListVersions(ctx context.Context, funcID string, limit, offset int) ([]*domain.FunctionVersion, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `
		SELECT data FROM function_versions
		WHERE function_id = $1
		ORDER BY version ASC
		LIMIT $2 OFFSET $3
	`, funcID, limit, offset)
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

func (s *PostgresStore) ListAliases(ctx context.Context, funcID string, limit, offset int) ([]*domain.FunctionAlias, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `
		SELECT data FROM function_aliases
		WHERE function_id = $1
		ORDER BY name ASC
		LIMIT $2 OFFSET $3
	`, funcID, limit, offset)
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

// ─── Function Code ──────────────────────────────────────────────────────────

func (s *PostgresStore) SaveFunctionCode(ctx context.Context, funcID, sourceCode, sourceHash string) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO function_code (function_id, source_code, source_hash, compile_status, created_at, updated_at)
		VALUES ($1, $2, $3, 'pending', $4, $5)
		ON CONFLICT (function_id) DO UPDATE SET
			source_code = EXCLUDED.source_code,
			source_hash = EXCLUDED.source_hash,
			compile_status = 'pending',
			compile_error = NULL,
			compiled_binary = NULL,
			binary_hash = NULL,
			updated_at = EXCLUDED.updated_at
	`, funcID, sourceCode, sourceHash, now, now)
	if err != nil {
		return fmt.Errorf("save function code: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetFunctionCode(ctx context.Context, funcID string) (*domain.FunctionCode, error) {
	var fc domain.FunctionCode
	var compiledBinary []byte
	var binaryHash, compileError *string
	err := s.pool.QueryRow(ctx, `
		SELECT function_id, source_code, compiled_binary, source_hash, binary_hash, compile_status, compile_error, created_at, updated_at
		FROM function_code
		WHERE function_id = $1
	`, funcID).Scan(&fc.FunctionID, &fc.SourceCode, &compiledBinary, &fc.SourceHash, &binaryHash, &fc.CompileStatus, &compileError, &fc.CreatedAt, &fc.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get function code: %w", err)
	}
	fc.CompiledBinary = compiledBinary
	if binaryHash != nil {
		fc.BinaryHash = *binaryHash
	}
	if compileError != nil {
		fc.CompileError = *compileError
	}
	return &fc, nil
}

func (s *PostgresStore) UpdateFunctionCode(ctx context.Context, funcID, sourceCode, sourceHash string) error {
	now := time.Now()
	ct, err := s.pool.Exec(ctx, `
		UPDATE function_code
		SET source_code = $2, source_hash = $3, compile_status = 'pending',
		    compile_error = NULL, compiled_binary = NULL, binary_hash = NULL, updated_at = $4
		WHERE function_id = $1
	`, funcID, sourceCode, sourceHash, now)
	if err != nil {
		return fmt.Errorf("update function code: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return s.SaveFunctionCode(ctx, funcID, sourceCode, sourceHash)
	}
	return nil
}

func (s *PostgresStore) UpdateCompileResult(ctx context.Context, funcID string, binary []byte, binaryHash string, status domain.CompileStatus, compileError string) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx, `
		UPDATE function_code
		SET compiled_binary = $2, binary_hash = $3, compile_status = $4, compile_error = $5, updated_at = $6
		WHERE function_id = $1
	`, funcID, binary, binaryHash, string(status), compileError, now)
	if err != nil {
		return fmt.Errorf("update compile result: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteFunctionCode(ctx context.Context, funcID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM function_code WHERE function_id = $1`, funcID)
	if err != nil {
		return fmt.Errorf("delete function code: %w", err)
	}
	return nil
}

// ─── Function Files (Multi-file Support) ─────────────────────────────────────

// FunctionFileInfo represents metadata about a file in a function
type FunctionFileInfo struct {
	Path     string `json:"path"`
	Size     int    `json:"size"`
	IsBinary bool   `json:"is_binary"`
}

// SaveFunctionFiles saves multiple files for a function, replacing any existing files
func (s *PostgresStore) SaveFunctionFiles(ctx context.Context, funcID string, files map[string][]byte) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Delete existing files
	if _, err := tx.Exec(ctx, `DELETE FROM function_files WHERE function_id = $1`, funcID); err != nil {
		return fmt.Errorf("delete existing files: %w", err)
	}

	// Insert new files
	for path, content := range files {
		isBinary := isBinaryContent(content)
		_, err := tx.Exec(ctx, `
			INSERT INTO function_files (function_id, path, content, is_binary)
			VALUES ($1, $2, $3, $4)
		`, funcID, path, content, isBinary)
		if err != nil {
			return fmt.Errorf("insert file %s: %w", path, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// GetFunctionFiles retrieves all files for a function
func (s *PostgresStore) GetFunctionFiles(ctx context.Context, funcID string) (map[string][]byte, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT path, content FROM function_files WHERE function_id = $1
	`, funcID)
	if err != nil {
		return nil, fmt.Errorf("get function files: %w", err)
	}
	defer rows.Close()

	files := make(map[string][]byte)
	for rows.Next() {
		var path string
		var content []byte
		if err := rows.Scan(&path, &content); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		files[path] = content
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return files, nil
}

// ListFunctionFiles returns metadata about all files in a function
func (s *PostgresStore) ListFunctionFiles(ctx context.Context, funcID string) ([]FunctionFileInfo, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT path, LENGTH(content), is_binary FROM function_files WHERE function_id = $1 ORDER BY path
	`, funcID)
	if err != nil {
		return nil, fmt.Errorf("list function files: %w", err)
	}
	defer rows.Close()

	var files []FunctionFileInfo
	for rows.Next() {
		var f FunctionFileInfo
		if err := rows.Scan(&f.Path, &f.Size, &f.IsBinary); err != nil {
			return nil, fmt.Errorf("scan file info: %w", err)
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return files, nil
}

// DeleteFunctionFiles deletes all files for a function
func (s *PostgresStore) DeleteFunctionFiles(ctx context.Context, funcID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM function_files WHERE function_id = $1`, funcID)
	if err != nil {
		return fmt.Errorf("delete function files: %w", err)
	}
	return nil
}

// HasFunctionFiles returns true if a function has any files stored
func (s *PostgresStore) HasFunctionFiles(ctx context.Context, funcID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM function_files WHERE function_id = $1`, funcID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count function files: %w", err)
	}
	return count > 0, nil
}

// isBinaryContent checks if content appears to be binary (contains null bytes)
func isBinaryContent(content []byte) bool {
	// Check first 512 bytes for null bytes (common binary indicator)
	checkLen := len(content)
	if checkLen > 512 {
		checkLen = 512
	}
	for i := 0; i < checkLen; i++ {
		if content[i] == 0 {
			return true
		}
	}
	return false
}
