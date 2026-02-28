package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oriys/nova/internal/domain"
)

// CreateDurableExecution inserts a new durable execution record.
func (s *PostgresStore) CreateDurableExecution(ctx context.Context, exec *domain.DurableExecution) error {
	scope := tenantScopeFromContext(ctx)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO durable_executions (id, tenant_id, namespace, function_id, function_name, status, input, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, exec.ID, scope.TenantID, scope.Namespace, exec.FunctionID, exec.FunctionName,
		exec.Status, exec.Input, exec.CreatedAt, exec.UpdatedAt)
	return err
}

// GetDurableExecution retrieves a durable execution by ID, including its steps.
func (s *PostgresStore) GetDurableExecution(ctx context.Context, id string) (*domain.DurableExecution, error) {
	scope := tenantScopeFromContext(ctx)
	exec := &domain.DurableExecution{}
	var input, output []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, function_id, function_name, status, input, output, error_message,
		       created_at, updated_at, completed_at
		FROM durable_executions
		WHERE id = $1 AND tenant_id = $2 AND namespace = $3
	`, id, scope.TenantID, scope.Namespace).Scan(
		&exec.ID, &exec.FunctionID, &exec.FunctionName, &exec.Status,
		&input, &output, &exec.Error,
		&exec.CreatedAt, &exec.UpdatedAt, &exec.CompletedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("durable execution not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get durable execution: %w", err)
	}
	if input != nil {
		exec.Input = json.RawMessage(input)
	}
	if output != nil {
		exec.Output = json.RawMessage(output)
	}

	// Load steps
	steps, err := s.ListDurableSteps(ctx, id)
	if err != nil {
		return nil, err
	}
	exec.Steps = steps
	return exec, nil
}

// UpdateDurableExecution updates the status and output of a durable execution.
func (s *PostgresStore) UpdateDurableExecution(ctx context.Context, exec *domain.DurableExecution) error {
	scope := tenantScopeFromContext(ctx)
	_, err := s.pool.Exec(ctx, `
		UPDATE durable_executions
		SET status = $1, output = $2, error_message = $3, updated_at = $4, completed_at = $5
		WHERE id = $6 AND tenant_id = $7 AND namespace = $8
	`, exec.Status, exec.Output, exec.Error, exec.UpdatedAt, exec.CompletedAt,
		exec.ID, scope.TenantID, scope.Namespace)
	return err
}

// ListDurableExecutions returns durable executions for a function.
func (s *PostgresStore) ListDurableExecutions(ctx context.Context, functionID string, limit, offset int) ([]*domain.DurableExecution, error) {
	scope := tenantScopeFromContext(ctx)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, function_id, function_name, status, input, output, error_message,
		       created_at, updated_at, completed_at
		FROM durable_executions
		WHERE tenant_id = $1 AND namespace = $2 AND function_id = $3
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5
	`, scope.TenantID, scope.Namespace, functionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list durable executions: %w", err)
	}
	defer rows.Close()

	var result []*domain.DurableExecution
	for rows.Next() {
		exec := &domain.DurableExecution{}
		var input, output []byte
		if err := rows.Scan(
			&exec.ID, &exec.FunctionID, &exec.FunctionName, &exec.Status,
			&input, &output, &exec.Error,
			&exec.CreatedAt, &exec.UpdatedAt, &exec.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan durable execution: %w", err)
		}
		if input != nil {
			exec.Input = json.RawMessage(input)
		}
		if output != nil {
			exec.Output = json.RawMessage(output)
		}
		result = append(result, exec)
	}
	return result, rows.Err()
}

// CountDurableExecutions returns the total count for a function.
func (s *PostgresStore) CountDurableExecutions(ctx context.Context, functionID string) (int64, error) {
	scope := tenantScopeFromContext(ctx)
	var count int64
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM durable_executions
		WHERE tenant_id = $1 AND namespace = $2 AND function_id = $3
	`, scope.TenantID, scope.Namespace, functionID).Scan(&count)
	return count, err
}

// CreateDurableStep inserts a step record for a durable execution.
func (s *PostgresStore) CreateDurableStep(ctx context.Context, step *domain.DurableStep) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO durable_steps (id, execution_id, name, status, input, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, step.ID, step.ExecutionID, step.Name, step.Status, step.Input, step.CreatedAt)
	return err
}

// CompleteDurableStep marks a step as completed with output.
func (s *PostgresStore) CompleteDurableStep(ctx context.Context, stepID string, output json.RawMessage, durationMs int64) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx, `
		UPDATE durable_steps
		SET status = 'completed', output = $1, duration_ms = $2, completed_at = $3
		WHERE id = $4
	`, output, durationMs, now, stepID)
	return err
}

// FailDurableStep marks a step as failed with error message.
func (s *PostgresStore) FailDurableStep(ctx context.Context, stepID string, errMsg string, durationMs int64) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx, `
		UPDATE durable_steps
		SET status = 'failed', error_message = $1, duration_ms = $2, completed_at = $3
		WHERE id = $4
	`, errMsg, durationMs, now, stepID)
	return err
}

// ListDurableSteps returns all steps for a durable execution.
func (s *PostgresStore) ListDurableSteps(ctx context.Context, executionID string) ([]domain.DurableStep, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, execution_id, name, status, input, output, error_message, duration_ms, created_at, completed_at
		FROM durable_steps
		WHERE execution_id = $1
		ORDER BY created_at ASC
	`, executionID)
	if err != nil {
		return nil, fmt.Errorf("list durable steps: %w", err)
	}
	defer rows.Close()

	var steps []domain.DurableStep
	for rows.Next() {
		var s domain.DurableStep
		var input, output []byte
		if err := rows.Scan(
			&s.ID, &s.ExecutionID, &s.Name, &s.Status,
			&input, &output, &s.Error, &s.DurationMs,
			&s.CreatedAt, &s.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan durable step: %w", err)
		}
		if input != nil {
			s.Input = json.RawMessage(input)
		}
		if output != nil {
			s.Output = json.RawMessage(output)
		}
		steps = append(steps, s)
	}
	return steps, rows.Err()
}
