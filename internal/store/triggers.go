package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TriggerRecord represents a trigger row in the store.
type TriggerRecord struct {
	ID           string                 `json:"id"`
	TenantID     string                 `json:"tenant_id"`
	Namespace    string                 `json:"namespace"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	FunctionID   string                 `json:"function_id"`
	FunctionName string                 `json:"function_name"`
	Enabled      bool                   `json:"enabled"`
	Config       map[string]interface{} `json:"config"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// TriggerUpdate contains optional fields for updating a trigger.
type TriggerUpdate struct {
	Name         *string                 `json:"name,omitempty"`
	Enabled      *bool                   `json:"enabled,omitempty"`
	FunctionID   *string                 `json:"function_id,omitempty"`
	FunctionName *string                 `json:"function_name,omitempty"`
	Config       map[string]interface{}  `json:"config,omitempty"`
}

// CreateTrigger inserts a new trigger record.
func (s *PostgresStore) CreateTrigger(ctx context.Context, trigger *TriggerRecord) error {
	if trigger.ID == "" {
		trigger.ID = uuid.New().String()
	}
	now := time.Now()
	trigger.CreatedAt = now
	trigger.UpdatedAt = now

	scope := TenantScopeFromContext(ctx)
	if trigger.TenantID == "" {
		trigger.TenantID = scope.TenantID
	}
	if trigger.Namespace == "" {
		trigger.Namespace = scope.Namespace
	}

	configJSON, err := json.Marshal(trigger.Config)
	if err != nil {
		return fmt.Errorf("marshal trigger config: %w", err)
	}

	query := `
		INSERT INTO triggers (id, tenant_id, namespace, name, type, function_id, function_name, enabled, config, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err = s.pool.Exec(ctx, query,
		trigger.ID, trigger.TenantID, trigger.Namespace, trigger.Name, trigger.Type,
		trigger.FunctionID, trigger.FunctionName, trigger.Enabled, configJSON,
		trigger.CreatedAt, trigger.UpdatedAt,
	)
	return err
}

// GetTrigger retrieves a trigger by ID.
func (s *PostgresStore) GetTrigger(ctx context.Context, id string) (*TriggerRecord, error) {
	scope := TenantScopeFromContext(ctx)
	query := `
		SELECT id, tenant_id, namespace, name, type, function_id, function_name, enabled, config, created_at, updated_at
		FROM triggers
		WHERE id = $1 AND tenant_id = $2 AND namespace = $3
	`
	return s.scanTrigger(s.pool.QueryRow(ctx, query, id, scope.TenantID, scope.Namespace))
}

// GetTriggerByName retrieves a trigger by name.
func (s *PostgresStore) GetTriggerByName(ctx context.Context, name string) (*TriggerRecord, error) {
	scope := TenantScopeFromContext(ctx)
	query := `
		SELECT id, tenant_id, namespace, name, type, function_id, function_name, enabled, config, created_at, updated_at
		FROM triggers
		WHERE name = $1 AND tenant_id = $2 AND namespace = $3
	`
	return s.scanTrigger(s.pool.QueryRow(ctx, query, name, scope.TenantID, scope.Namespace))
}

// ListTriggers lists triggers for the current tenant/namespace.
func (s *PostgresStore) ListTriggers(ctx context.Context, limit, offset int) ([]*TriggerRecord, error) {
	scope := TenantScopeFromContext(ctx)
	query := `
		SELECT id, tenant_id, namespace, name, type, function_id, function_name, enabled, config, created_at, updated_at
		FROM triggers
		WHERE tenant_id = $1 AND namespace = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := s.pool.Query(ctx, query, scope.TenantID, scope.Namespace, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var triggers []*TriggerRecord
	for rows.Next() {
		var t TriggerRecord
		var configJSON []byte
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.Namespace, &t.Name, &t.Type,
			&t.FunctionID, &t.FunctionName, &t.Enabled, &configJSON,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if len(configJSON) > 0 {
			if err := json.Unmarshal(configJSON, &t.Config); err != nil {
				return nil, fmt.Errorf("unmarshal trigger config: %w", err)
			}
		}
		triggers = append(triggers, &t)
	}
	return triggers, rows.Err()
}

// UpdateTrigger updates a trigger by ID and returns the updated record.
func (s *PostgresStore) UpdateTrigger(ctx context.Context, id string, update *TriggerUpdate) (*TriggerRecord, error) {
	scope := TenantScopeFromContext(ctx)
	now := time.Now()

	var configJSON []byte
	if update.Config != nil {
		var err error
		configJSON, err = json.Marshal(update.Config)
		if err != nil {
			return nil, fmt.Errorf("marshal trigger config: %w", err)
		}
	}

	query := `
		UPDATE triggers SET
			name          = COALESCE($1, name),
			enabled       = COALESCE($2, enabled),
			function_id   = COALESCE($3, function_id),
			function_name = COALESCE($4, function_name),
			config        = COALESCE($5, config),
			updated_at    = $6
		WHERE id = $7 AND tenant_id = $8 AND namespace = $9
		RETURNING id, tenant_id, namespace, name, type, function_id, function_name, enabled, config, created_at, updated_at
	`
	return s.scanTrigger(s.pool.QueryRow(ctx, query,
		update.Name, update.Enabled, update.FunctionID, update.FunctionName,
		configJSON, now, id, scope.TenantID, scope.Namespace,
	))
}

// DeleteTrigger deletes a trigger by ID.
func (s *PostgresStore) DeleteTrigger(ctx context.Context, id string) error {
	scope := TenantScopeFromContext(ctx)
	query := `DELETE FROM triggers WHERE id = $1 AND tenant_id = $2 AND namespace = $3`
	result, err := s.pool.Exec(ctx, query, id, scope.TenantID, scope.Namespace)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("trigger not found: %s", id)
	}
	return nil
}

// scanTrigger is a helper that scans a single trigger row.
type triggerScanner interface {
	Scan(dest ...interface{}) error
}

func (s *PostgresStore) scanTrigger(row triggerScanner) (*TriggerRecord, error) {
	var t TriggerRecord
	var configJSON []byte
	err := row.Scan(
		&t.ID, &t.TenantID, &t.Namespace, &t.Name, &t.Type,
		&t.FunctionID, &t.FunctionName, &t.Enabled, &configJSON,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &t.Config); err != nil {
			return nil, fmt.Errorf("unmarshal trigger config: %w", err)
		}
	}
	return &t, nil
}
