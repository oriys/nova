package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrFunctionStateNotFound        = errors.New("function state not found")
	ErrFunctionStateVersionConflict = errors.New("function state version conflict")
)

type FunctionStateEntry struct {
	FunctionID string          `json:"function_id"`
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value"`
	Version    int64           `json:"version"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	ExpiresAt  *time.Time      `json:"expires_at,omitempty"`
}

type FunctionStatePutOptions struct {
	TTL             time.Duration `json:"ttl,omitempty"`
	ExpectedVersion int64         `json:"expected_version,omitempty"`
}

type FunctionStateListOptions struct {
	Prefix string `json:"prefix,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

func normalizeFunctionStateListOptions(opts *FunctionStateListOptions) FunctionStateListOptions {
	out := FunctionStateListOptions{}
	if opts != nil {
		out = *opts
	}
	out.Prefix = strings.TrimSpace(out.Prefix)
	if out.Limit <= 0 {
		out.Limit = 100
	}
	if out.Limit > 500 {
		out.Limit = 500
	}
	if out.Offset < 0 {
		out.Offset = 0
	}
	return out
}

func (s *PostgresStore) GetFunctionState(ctx context.Context, functionID, key string) (*FunctionStateEntry, error) {
	scope := tenantScopeFromContext(ctx)
	key = strings.TrimSpace(key)
	if functionID == "" || key == "" {
		return nil, fmt.Errorf("function id and key are required")
	}

	entry := &FunctionStateEntry{}
	var value []byte
	err := s.pool.QueryRow(ctx, `
		SELECT function_id, key, value, version, created_at, updated_at, expires_at
		FROM function_states
		WHERE tenant_id = $1
		  AND namespace = $2
		  AND function_id = $3
		  AND key = $4
		  AND (expires_at IS NULL OR expires_at > NOW())
	`, scope.TenantID, scope.Namespace, functionID, key).Scan(
		&entry.FunctionID,
		&entry.Key,
		&value,
		&entry.Version,
		&entry.CreatedAt,
		&entry.UpdatedAt,
		&entry.ExpiresAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrFunctionStateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get function state: %w", err)
	}
	entry.Value = json.RawMessage(value)
	return entry, nil
}

func (s *PostgresStore) PutFunctionState(ctx context.Context, functionID, key string, value json.RawMessage, opts *FunctionStatePutOptions) (*FunctionStateEntry, error) {
	scope := tenantScopeFromContext(ctx)
	key = strings.TrimSpace(key)
	if functionID == "" || key == "" {
		return nil, fmt.Errorf("function id and key are required")
	}
	if len(value) == 0 || !json.Valid(value) {
		return nil, fmt.Errorf("state value must be valid JSON")
	}

	putOpts := &FunctionStatePutOptions{}
	if opts != nil {
		*putOpts = *opts
	}

	var expiresAt *time.Time
	if putOpts.TTL > 0 {
		t := time.Now().Add(putOpts.TTL)
		expiresAt = &t
	}

	entry := &FunctionStateEntry{}
	var rawValue []byte

	if putOpts.ExpectedVersion > 0 {
		err := s.pool.QueryRow(ctx, `
			UPDATE function_states
			SET value = $5::jsonb,
			    version = version + 1,
			    updated_at = NOW(),
			    expires_at = $6
			WHERE tenant_id = $1
			  AND namespace = $2
			  AND function_id = $3
			  AND key = $4
			  AND version = $7
			RETURNING function_id, key, value, version, created_at, updated_at, expires_at
		`, scope.TenantID, scope.Namespace, functionID, key, value, expiresAt, putOpts.ExpectedVersion).Scan(
			&entry.FunctionID,
			&entry.Key,
			&rawValue,
			&entry.Version,
			&entry.CreatedAt,
			&entry.UpdatedAt,
			&entry.ExpiresAt,
		)
		if err == pgx.ErrNoRows {
			return nil, ErrFunctionStateVersionConflict
		}
		if err != nil {
			return nil, fmt.Errorf("put function state (conditional): %w", err)
		}
		entry.Value = json.RawMessage(rawValue)
		return entry, nil
	}

	err := s.pool.QueryRow(ctx, `
		INSERT INTO function_states (tenant_id, namespace, function_id, key, value, version, created_at, updated_at, expires_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, 1, NOW(), NOW(), $6)
		ON CONFLICT (tenant_id, namespace, function_id, key)
		DO UPDATE SET
			value = EXCLUDED.value,
			version = function_states.version + 1,
			updated_at = NOW(),
			expires_at = EXCLUDED.expires_at
		RETURNING function_id, key, value, version, created_at, updated_at, expires_at
	`, scope.TenantID, scope.Namespace, functionID, key, value, expiresAt).Scan(
		&entry.FunctionID,
		&entry.Key,
		&rawValue,
		&entry.Version,
		&entry.CreatedAt,
		&entry.UpdatedAt,
		&entry.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("put function state: %w", err)
	}
	entry.Value = json.RawMessage(rawValue)
	return entry, nil
}

func (s *PostgresStore) DeleteFunctionState(ctx context.Context, functionID, key string) error {
	scope := tenantScopeFromContext(ctx)
	key = strings.TrimSpace(key)
	if functionID == "" || key == "" {
		return fmt.Errorf("function id and key are required")
	}
	if _, err := s.pool.Exec(ctx, `
		DELETE FROM function_states
		WHERE tenant_id = $1 AND namespace = $2 AND function_id = $3 AND key = $4
	`, scope.TenantID, scope.Namespace, functionID, key); err != nil {
		return fmt.Errorf("delete function state: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListFunctionStates(ctx context.Context, functionID string, opts *FunctionStateListOptions) ([]*FunctionStateEntry, error) {
	scope := tenantScopeFromContext(ctx)
	if functionID == "" {
		return nil, fmt.Errorf("function id is required")
	}
	listOpts := normalizeFunctionStateListOptions(opts)
	rows, err := s.pool.Query(ctx, `
		SELECT function_id, key, value, version, created_at, updated_at, expires_at
		FROM function_states
		WHERE tenant_id = $1
		  AND namespace = $2
		  AND function_id = $3
		  AND ($4 = '' OR key LIKE $4 || '%')
		  AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY key
		LIMIT $5 OFFSET $6
	`, scope.TenantID, scope.Namespace, functionID, listOpts.Prefix, listOpts.Limit, listOpts.Offset)
	if err != nil {
		return nil, fmt.Errorf("list function states: %w", err)
	}
	defer rows.Close()

	entries := make([]*FunctionStateEntry, 0, listOpts.Limit)
	for rows.Next() {
		entry := &FunctionStateEntry{}
		var rawValue []byte
		if err := rows.Scan(
			&entry.FunctionID,
			&entry.Key,
			&rawValue,
			&entry.Version,
			&entry.CreatedAt,
			&entry.UpdatedAt,
			&entry.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("list function states scan: %w", err)
		}
		entry.Value = json.RawMessage(rawValue)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list function states rows: %w", err)
	}
	return entries, nil
}

func (s *PostgresStore) CountFunctionStates(ctx context.Context, functionID, prefix string) (int64, error) {
	scope := tenantScopeFromContext(ctx)
	if functionID == "" {
		return 0, fmt.Errorf("function id is required")
	}
	prefix = strings.TrimSpace(prefix)
	var total int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM function_states
		WHERE tenant_id = $1
		  AND namespace = $2
		  AND function_id = $3
		  AND ($4 = '' OR key LIKE $4 || '%')
		  AND (expires_at IS NULL OR expires_at > NOW())
	`, scope.TenantID, scope.Namespace, functionID, prefix).Scan(&total); err != nil {
		return 0, fmt.Errorf("count function states: %w", err)
	}
	return total, nil
}
