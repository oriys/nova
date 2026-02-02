// Package backend defines the interface for VM execution backends.
// Implementations include Firecracker (microVMs) and Docker (containers).
package backend

import (
	"context"
	"encoding/json"

	"github.com/oriys/nova/internal/domain"
)

// Backend manages the lifecycle of VMs or containers for function execution.
type Backend interface {
	// CreateVM creates a new VM/container for the given function.
	CreateVM(ctx context.Context, fn *domain.Function) (*VM, error)

	// StopVM stops and cleans up a VM/container.
	StopVM(vmID string) error

	// NewClient creates a client for communicating with the VM/container.
	NewClient(vm *VM) (Client, error)

	// Shutdown stops all VMs/containers.
	Shutdown()

	// SnapshotDir returns the snapshot directory (empty for Docker backend).
	SnapshotDir() string
}

// Client communicates with the agent inside a VM/container.
type Client interface {
	// Init sends the init message to the agent.
	Init(fn *domain.Function) error

	// Execute runs a function invocation.
	Execute(reqID string, input json.RawMessage, timeoutS int) (*RespPayload, error)

	// ExecuteWithTrace runs a function invocation with W3C trace context.
	ExecuteWithTrace(reqID string, input json.RawMessage, timeoutS int, traceParent, traceState string) (*RespPayload, error)

	// Ping checks if the agent is responsive.
	Ping() error

	// Close closes the client connection.
	Close() error
}
