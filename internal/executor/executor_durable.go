package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
)

// InvokeDurable executes a function with durable execution tracking.
// It creates a DurableExecution record, invokes the function normally,
// and updates the execution record with the result.
func (e *Executor) InvokeDurable(ctx context.Context, funcName string, payload json.RawMessage) (*domain.DurableExecution, error) {
	fn, err := e.store.GetFunctionByName(ctx, funcName)
	if err != nil {
		return nil, fmt.Errorf("get function: %w", err)
	}

	execID := uuid.New().String()[:12]
	now := time.Now()

	exec := &domain.DurableExecution{
		ID:           execID,
		FunctionID:   fn.ID,
		FunctionName: fn.Name,
		Status:       domain.DurableStatusRunning,
		Input:        payload,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := e.store.CreateDurableExecution(ctx, exec); err != nil {
		return nil, fmt.Errorf("create durable execution: %w", err)
	}

	// Execute the function
	resp, err := e.Invoke(ctx, funcName, payload)

	now = time.Now()
	exec.UpdatedAt = now
	exec.CompletedAt = &now

	if err != nil {
		exec.Status = domain.DurableStatusFailed
		exec.Error = err.Error()
		if updateErr := e.store.UpdateDurableExecution(ctx, exec); updateErr != nil {
			logging.Op().Warn("failed to update durable execution", "id", execID, "error", updateErr)
		}
		return exec, err
	}

	if resp.Error != "" {
		exec.Status = domain.DurableStatusFailed
		exec.Error = resp.Error
	} else {
		exec.Status = domain.DurableStatusCompleted
		exec.Output = resp.Output
	}

	if updateErr := e.store.UpdateDurableExecution(ctx, exec); updateErr != nil {
		logging.Op().Warn("failed to update durable execution", "id", execID, "error", updateErr)
	}

	return exec, nil
}

// GetDurableExecution retrieves a durable execution by ID.
func (e *Executor) GetDurableExecution(ctx context.Context, id string) (*domain.DurableExecution, error) {
	return e.store.GetDurableExecution(ctx, id)
}

// ListDurableExecutions retrieves durable executions for a function.
func (e *Executor) ListDurableExecutions(ctx context.Context, functionID string, limit, offset int) ([]*domain.DurableExecution, error) {
	return e.store.ListDurableExecutions(ctx, functionID, limit, offset)
}
