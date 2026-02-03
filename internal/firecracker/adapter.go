// Package firecracker provides the Firecracker microVM backend adapter.
package firecracker

import (
	"context"
	"encoding/json"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
)

// Adapter wraps the Firecracker Manager to implement backend.Backend.
type Adapter struct {
	manager *Manager
}

// NewAdapter creates a new Firecracker backend adapter.
func NewAdapter(cfg *Config) (*Adapter, error) {
	mgr, err := NewManager(cfg)
	if err != nil {
		return nil, err
	}
	return &Adapter{manager: mgr}, nil
}

// CreateVM creates a new Firecracker microVM.
func (a *Adapter) CreateVM(ctx context.Context, fn *domain.Function, codeContent []byte) (*backend.VM, error) {
	vm, err := a.manager.CreateVM(ctx, fn, codeContent)
	if err != nil {
		return nil, err
	}
	return vmToBackend(vm), nil
}

// StopVM stops a Firecracker microVM.
func (a *Adapter) StopVM(vmID string) error {
	return a.manager.StopVM(vmID)
}

// NewClient creates a vsock client for the VM.
func (a *Adapter) NewClient(vm *backend.VM) (backend.Client, error) {
	fcVM := backendToVM(vm)
	client, err := NewVsockClient(fcVM)
	if err != nil {
		return nil, err
	}
	return &VsockClientAdapter{client: client}, nil
}

// Shutdown stops all VMs.
func (a *Adapter) Shutdown() {
	a.manager.Shutdown()
}

// SnapshotDir returns the snapshot directory.
func (a *Adapter) SnapshotDir() string {
	return a.manager.SnapshotDir()
}

// Manager returns the underlying firecracker manager (for snapshot operations).
func (a *Adapter) Manager() *Manager {
	return a.manager
}

// vmToBackend converts firecracker.VM to backend.VM.
func vmToBackend(vm *VM) *backend.VM {
	return &backend.VM{
		ID:                vm.ID,
		Runtime:           vm.Runtime,
		State:             backend.VMState(vm.State),
		CID:               vm.CID,
		SocketPath:        vm.SocketPath,
		VsockPath:         vm.VsockPath,
		CodeDrive:         vm.CodeDrive,
		TapDevice:         vm.TapDevice,
		GuestIP:           vm.GuestIP,
		GuestMAC:          vm.GuestMAC,
		Cmd:               vm.Cmd,
		DockerContainerID: vm.DockerContainerID,
		AssignedPort:      vm.AssignedPort,
		CreatedAt:         vm.CreatedAt,
		LastUsed:          vm.LastUsed,
	}
}

// backendToVM converts backend.VM back to firecracker.VM for internal use.
func backendToVM(vm *backend.VM) *VM {
	return &VM{
		ID:                vm.ID,
		Runtime:           vm.Runtime,
		State:             VMState(vm.State),
		CID:               vm.CID,
		SocketPath:        vm.SocketPath,
		VsockPath:         vm.VsockPath,
		CodeDrive:         vm.CodeDrive,
		TapDevice:         vm.TapDevice,
		GuestIP:           vm.GuestIP,
		GuestMAC:          vm.GuestMAC,
		Cmd:               vm.Cmd,
		DockerContainerID: vm.DockerContainerID,
		AssignedPort:      vm.AssignedPort,
		CreatedAt:         vm.CreatedAt,
		LastUsed:          vm.LastUsed,
	}
}

// VsockClientAdapter wraps VsockClient to implement backend.Client.
type VsockClientAdapter struct {
	client *VsockClient
}

// Init sends the init message.
func (c *VsockClientAdapter) Init(fn *domain.Function) error {
	return c.client.Init(fn)
}

// Execute runs a function invocation.
func (c *VsockClientAdapter) Execute(reqID string, input json.RawMessage, timeoutS int) (*backend.RespPayload, error) {
	resp, err := c.client.Execute(reqID, input, timeoutS)
	if err != nil {
		return nil, err
	}
	return &backend.RespPayload{
		RequestID:  resp.RequestID,
		Output:     resp.Output,
		Error:      resp.Error,
		DurationMs: resp.DurationMs,
		Stdout:     resp.Stdout,
		Stderr:     resp.Stderr,
	}, nil
}

// ExecuteWithTrace runs a function with trace context.
func (c *VsockClientAdapter) ExecuteWithTrace(reqID string, input json.RawMessage, timeoutS int, traceParent, traceState string) (*backend.RespPayload, error) {
	resp, err := c.client.ExecuteWithTrace(reqID, input, timeoutS, traceParent, traceState)
	if err != nil {
		return nil, err
	}
	return &backend.RespPayload{
		RequestID:  resp.RequestID,
		Output:     resp.Output,
		Error:      resp.Error,
		DurationMs: resp.DurationMs,
		Stdout:     resp.Stdout,
		Stderr:     resp.Stderr,
	}, nil
}

// Ping checks if the agent is responsive.
func (c *VsockClientAdapter) Ping() error {
	return c.client.Ping()
}

// Close closes the client.
func (c *VsockClientAdapter) Close() error {
	return c.client.Close()
}
