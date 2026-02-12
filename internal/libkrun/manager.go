package libkrun

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
)

const (
	agentPort = 9999
)

// Manager manages libkrun microVMs for function execution.
type Manager struct {
	config *Config
	vms    map[string]*backend.VM
	mu     sync.RWMutex
	nextPort int32
}

// NewManager creates a new libkrun backend manager.
func NewManager(cfg *Config) (*Manager, error) {
	if err := os.MkdirAll(cfg.CodeDir, 0755); err != nil {
		return nil, fmt.Errorf("create code dir: %w", err)
	}

	return &Manager{
		config:   cfg,
		vms:      make(map[string]*backend.VM),
		nextPort: int32(cfg.PortRangeMin),
	}, nil
}

// allocatePort returns the next available port in the range.
func (m *Manager) allocatePort() int {
	port := atomic.AddInt32(&m.nextPort, 1) - 1
	if int(port) > m.config.PortRangeMax {
		atomic.StoreInt32(&m.nextPort, int32(m.config.PortRangeMin))
		port = int32(m.config.PortRangeMin)
	}
	return int(port)
}

// imageForRuntime maps runtime to rootfs image name.
func imageForRuntime(rt domain.Runtime) string {
	r := string(rt)
	switch {
	case r == string(domain.RuntimePython) || strings.HasPrefix(r, "python"):
		return "python.ext4"
	case r == string(domain.RuntimeNode) || strings.HasPrefix(r, "node"):
		return "node.ext4"
	case r == string(domain.RuntimeGo) || strings.HasPrefix(r, "go"):
		return "base.ext4"
	case r == string(domain.RuntimeRust):
		return "base.ext4"
	case r == string(domain.RuntimeRuby) || strings.HasPrefix(r, "ruby"):
		return "ruby.ext4"
	case r == string(domain.RuntimeJava) || strings.HasPrefix(r, "java") ||
		r == string(domain.RuntimeKotlin) || r == string(domain.RuntimeScala):
		return "java.ext4"
	case r == string(domain.RuntimePHP) || strings.HasPrefix(r, "php"):
		return "php.ext4"
	case r == string(domain.RuntimeDeno) || strings.HasPrefix(r, "deno"):
		return "deno.ext4"
	case r == string(domain.RuntimeBun) || strings.HasPrefix(r, "bun"):
		return "bun.ext4"
	case r == string(domain.RuntimeWasm) || strings.HasPrefix(r, "wasm"):
		return "wasm.ext4"
	default:
		return "base.ext4"
	}
}

// CreateVM creates a new libkrun microVM for the function.
func (m *Manager) CreateVM(ctx context.Context, fn *domain.Function, codeContent []byte) (*backend.VM, error) {
	vmID := uuid.New().String()[:12]
	port := m.allocatePort()

	// Prepare code directory
	codeDir := filepath.Join(m.config.CodeDir, vmID)
	if err := os.MkdirAll(codeDir, 0755); err != nil {
		return nil, fmt.Errorf("create code dir: %w", err)
	}

	// Write code to local code directory
	if len(codeContent) > 0 {
		handlerPath := filepath.Join(codeDir, "handler")
		if err := os.WriteFile(handlerPath, codeContent, 0755); err != nil {
			os.RemoveAll(codeDir)
			return nil, fmt.Errorf("write code file: %w", err)
		}
	}

	vm, err := m.startVM(ctx, fn, vmID, port, codeDir)
	if err != nil {
		os.RemoveAll(codeDir)
		return nil, err
	}

	return vm, nil
}

// CreateVMWithFiles creates a new libkrun microVM with multiple code files.
func (m *Manager) CreateVMWithFiles(ctx context.Context, fn *domain.Function, files map[string][]byte) (*backend.VM, error) {
	vmID := uuid.New().String()[:12]
	port := m.allocatePort()

	// Prepare code directory
	codeDir := filepath.Join(m.config.CodeDir, vmID)
	if err := os.MkdirAll(codeDir, 0755); err != nil {
		return nil, fmt.Errorf("create code dir: %w", err)
	}

	// Write all files to local code directory
	for path, content := range files {
		fullPath := filepath.Join(codeDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			os.RemoveAll(codeDir)
			return nil, fmt.Errorf("create dir for %s: %w", path, err)
		}
		if err := os.WriteFile(fullPath, content, 0755); err != nil {
			os.RemoveAll(codeDir)
			return nil, fmt.Errorf("write file %s: %w", path, err)
		}
	}

	vm, err := m.startVM(ctx, fn, vmID, port, codeDir)
	if err != nil {
		os.RemoveAll(codeDir)
		return nil, err
	}

	return vm, nil
}

// startVM launches the libkrun microVM process and waits for the agent.
func (m *Manager) startVM(ctx context.Context, fn *domain.Function, vmID string, port int, codeDir string) (*backend.VM, error) {
	rootfs := filepath.Join(m.config.RootfsDir, imageForRuntime(fn.Runtime))

	// Build krun command arguments.
	// krun uses chroot-style invocation:
	//   krun --rootfs <rootfs> --port <host:guest> --volume <host:guest> <command>
	args := []string{
		"--rootfs", rootfs,
		"--port", fmt.Sprintf("%d:%d", port, agentPort),
		"--volume", fmt.Sprintf("%s:/code", codeDir),
	}

	if fn.MemoryMB > 0 {
		args = append(args, "--mem", fmt.Sprintf("%d", fn.MemoryMB))
	}

	// The agent binary is expected to be present inside the rootfs or mounted.
	if m.config.AgentPath != "" {
		args = append(args, "--volume", fmt.Sprintf("%s:/opt/nova/bin/nova-agent", m.config.AgentPath))
	}

	// Set environment variables
	for k, v := range fn.EnvVars {
		args = append(args, "--env", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, "--env", "NOVA_AGENT_MODE=tcp")

	// Command to execute inside the VM
	args = append(args, "/opt/nova/bin/nova-agent")

	logging.Op().Debug("creating libkrun VM", "vmID", vmID, "rootfs", rootfs, "port", port)

	cmd := exec.CommandContext(ctx, "krun", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("krun start failed: %w", err)
	}

	agentAddr := fmt.Sprintf("127.0.0.1:%d", port)

	vm := &backend.VM{
		ID:           vmID,
		Runtime:      fn.Runtime,
		State:        backend.VMStateCreating,
		AssignedPort: port,
		GuestIP:      agentAddr,
		CodeDir:      codeDir,
		Cmd:          cmd,
		CreatedAt:    time.Now(),
		LastUsed:     time.Now(),
	}

	// Wait for agent to be ready
	agentTimeout := m.config.AgentTimeout
	if agentTimeout == 0 {
		agentTimeout = 10 * time.Second
	}
	if err := waitForAgent(agentAddr, agentTimeout); err != nil {
		m.stopVM(cmd, codeDir)
		return nil, fmt.Errorf("agent not ready: %w", err)
	}

	vm.State = backend.VMStateRunning
	metrics.Global().RecordVMCreated()

	m.mu.Lock()
	m.vms[vmID] = vm
	m.mu.Unlock()

	logging.Op().Info("libkrun VM ready", "vmID", vmID, "addr", agentAddr)
	return vm, nil
}

// waitForAgent polls the agent until it's ready.
func waitForAgent(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for agent on %s", addr)
}

// StopVM stops and cleans up a libkrun microVM.
func (m *Manager) StopVM(vmID string) error {
	m.mu.Lock()
	vm, ok := m.vms[vmID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("VM not found: %s", vmID)
	}
	delete(m.vms, vmID)
	m.mu.Unlock()

	metrics.Global().RecordVMStopped()
	return m.stopVM(vm.Cmd, vm.CodeDir)
}

func (m *Manager) stopVM(cmd *exec.Cmd, codeDir string) error {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	if codeDir != "" {
		os.RemoveAll(codeDir)
	}
	return nil
}

// NewClient creates a TCP client for the VM.
func (m *Manager) NewClient(vm *backend.VM) (backend.Client, error) {
	return NewClient(vm)
}

// Shutdown stops all VMs.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.vms))
	for id := range m.vms {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(vmID string) {
			defer wg.Done()
			m.StopVM(vmID)
		}(id)
	}
	wg.Wait()
}

// SnapshotDir returns empty string — libkrun backend doesn't support snapshots.
func (m *Manager) SnapshotDir() string {
	return ""
}
