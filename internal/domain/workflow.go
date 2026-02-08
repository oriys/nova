package domain

import (
	"encoding/json"
	"time"
)

// Workflow status
type WorkflowStatus string

const (
	WorkflowStatusActive   WorkflowStatus = "active"
	WorkflowStatusInactive WorkflowStatus = "inactive"
	WorkflowStatusDeleted  WorkflowStatus = "deleted"
)

// Run status
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

// RunNode status
type NodeStatus string

const (
	NodeStatusPending   NodeStatus = "pending"
	NodeStatusReady     NodeStatus = "ready"
	NodeStatusRunning   NodeStatus = "running"
	NodeStatusSucceeded NodeStatus = "succeeded"
	NodeStatusFailed    NodeStatus = "failed"
	NodeStatusSkipped   NodeStatus = "skipped"
)

// Workflow is the top-level DAG workflow entity.
type Workflow struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	Status         WorkflowStatus `json:"status"`
	CurrentVersion int            `json:"current_version"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// WorkflowVersion is a published, immutable version of a workflow DAG.
type WorkflowVersion struct {
	ID         string          `json:"id"`
	WorkflowID string          `json:"workflow_id"`
	Version    int             `json:"version"`
	Definition json.RawMessage `json:"definition"` // Full definition JSONB
	Nodes      []WorkflowNode  `json:"nodes,omitempty"`
	Edges      []WorkflowEdge  `json:"edges,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// WorkflowNode is a single step in a workflow version's DAG.
type WorkflowNode struct {
	ID           string          `json:"id"`
	VersionID    string          `json:"version_id"`
	NodeKey      string          `json:"node_key"`
	FunctionName string          `json:"function_name"`
	InputMapping json.RawMessage `json:"input_mapping,omitempty"` // Static input template
	RetryPolicy  *RetryPolicy    `json:"retry_policy,omitempty"`
	TimeoutS     int             `json:"timeout_s,omitempty"`
	Position     int             `json:"position"` // Topological order
}

// WorkflowEdge is a directed edge in the DAG (from -> to).
type WorkflowEdge struct {
	ID         string `json:"id"`
	VersionID  string `json:"version_id"`
	FromNodeID string `json:"from_node_id"`
	ToNodeID   string `json:"to_node_id"`
}

// RetryPolicy controls retry behavior for a node.
type RetryPolicy struct {
	MaxAttempts  int `json:"max_attempts"`
	BaseMS       int `json:"base_ms"`
	MaxBackoffMS int `json:"max_backoff_ms"`
}

// WorkflowDefinition is the user-submitted DAG definition for publishing a version.
type WorkflowDefinition struct {
	Nodes []NodeDefinition `json:"nodes"`
	Edges []EdgeDefinition `json:"edges"`
}

// NodeDefinition is a node in a user-submitted workflow definition.
type NodeDefinition struct {
	NodeKey      string          `json:"node_key"`
	FunctionName string          `json:"function_name"`
	InputMapping json.RawMessage `json:"input_mapping,omitempty"`
	RetryPolicy  *RetryPolicy    `json:"retry_policy,omitempty"`
	TimeoutS     int             `json:"timeout_s,omitempty"`
}

// EdgeDefinition is an edge in a user-submitted workflow definition.
type EdgeDefinition struct {
	From string `json:"from"` // node_key
	To   string `json:"to"`   // node_key
}

// WorkflowRun is a single execution of a workflow version.
type WorkflowRun struct {
	ID           string          `json:"id"`
	WorkflowID   string          `json:"workflow_id"`
	WorkflowName string          `json:"workflow_name,omitempty"` // Denormalized for display
	VersionID    string          `json:"version_id"`
	Version      int             `json:"version,omitempty"` // Denormalized
	Status       RunStatus       `json:"status"`
	TriggerType  string          `json:"trigger_type"` // "manual", "api", "schedule"
	Input        json.RawMessage `json:"input,omitempty"`
	Output       json.RawMessage `json:"output,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	StartedAt    time.Time       `json:"started_at"`
	FinishedAt   *time.Time      `json:"finished_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`

	// Populated on detail queries
	Nodes []RunNode `json:"nodes,omitempty"`
}

// RunNode tracks the status of a single node in a run.
type RunNode struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id,omitempty"`
	Namespace      string          `json:"namespace,omitempty"`
	RunID          string          `json:"run_id"`
	NodeID         string          `json:"node_id"` // FK to dag_workflow_nodes
	NodeKey        string          `json:"node_key"`
	FunctionName   string          `json:"function_name"`
	Status         NodeStatus      `json:"status"`
	UnresolvedDeps int             `json:"unresolved_deps"`
	Attempt        int             `json:"attempt"`
	Input          json.RawMessage `json:"input,omitempty"`
	Output         json.RawMessage `json:"output,omitempty"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	LeaseOwner     string          `json:"lease_owner,omitempty"`
	LeaseExpiresAt *time.Time      `json:"lease_expires_at,omitempty"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	FinishedAt     *time.Time      `json:"finished_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`

	Attempts []NodeAttempt `json:"attempts,omitempty"`
}

// NodeAttempt records a single execution attempt of a run node.
type NodeAttempt struct {
	ID         string          `json:"id"`
	RunNodeID  string          `json:"run_node_id"`
	Attempt    int             `json:"attempt"`
	Status     NodeStatus      `json:"status"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	DurationMs int64           `json:"duration_ms"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
}
