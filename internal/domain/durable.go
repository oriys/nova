package domain

import (
	"encoding/json"
	"time"
)

// DurableExecutionStatus represents the lifecycle state of a durable execution.
type DurableExecutionStatus string

const (
	DurableStatusRunning   DurableExecutionStatus = "running"
	DurableStatusCompleted DurableExecutionStatus = "completed"
	DurableStatusFailed    DurableExecutionStatus = "failed"
	DurableStatusSuspended DurableExecutionStatus = "suspended"
)

// DurableExecution tracks a multi-step function invocation with checkpoint support.
type DurableExecution struct {
	ID           string                 `json:"id"`
	FunctionID   string                 `json:"function_id"`
	FunctionName string                 `json:"function_name"`
	Status       DurableExecutionStatus `json:"status"`
	Input        json.RawMessage        `json:"input,omitempty"`
	Output       json.RawMessage        `json:"output,omitempty"`
	Error        string                 `json:"error,omitempty"`
	Steps        []DurableStep          `json:"steps,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
}

// DurableStepStatus represents the lifecycle state of a single step.
type DurableStepStatus string

const (
	StepStatusRunning   DurableStepStatus = "running"
	StepStatusCompleted DurableStepStatus = "completed"
	StepStatusFailed    DurableStepStatus = "failed"
)

// DurableStep records a single checkpointed step within a durable execution.
type DurableStep struct {
	ID          string            `json:"id"`
	ExecutionID string            `json:"execution_id"`
	Name        string            `json:"name"`
	Status      DurableStepStatus `json:"status"`
	Input       json.RawMessage   `json:"input,omitempty"`
	Output      json.RawMessage   `json:"output,omitempty"`
	Error       string            `json:"error,omitempty"`
	DurationMs  int64             `json:"duration_ms"`
	CreatedAt   time.Time         `json:"created_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
}
