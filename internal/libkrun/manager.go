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
	"github.com/oriys/nova/internal/pkg/codefile"
	"github.com/oriys/nova/internal/pkg/safepath"
)

const (
	agentPort = 9999
)

// Manager manages libkrun microVMs for function execution.
type Manager struct {
	config   *Config
	vms      map[string]*backend.VM
	mu       sync.RWMutex
	nextPort int32
	useMacOS bool // true = use krunvm (macOS), false = use krun (Linux)
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
		useMacOS: UseKrunVM(),
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
		mode := os.FileMode(0644)
		if codefile.ShouldBeExecutable("handler", codeContent) {
			mode = 0755
		}
		if err := os.WriteFile(handlerPath, codeContent, mode); err != nil {
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
		fullPath, err := safepath.Join(codeDir, path)
		if err != nil {
			os.RemoveAll(codeDir)
			return nil, fmt.Errorf("unsafe file path %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			os.RemoveAll(codeDir)
			return nil, fmt.Errorf("create dir for %s: %w", path, err)
		}
		mode := os.FileMode(0644)
		if codefile.ShouldBeExecutable(path, content) {
			mode = 0755
		}
		if err := os.WriteFile(fullPath, content, mode); err != nil {
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

// startVM launches a libkrun microVM process and waits for the agent.
// Dispatches to krun (Linux) or krunvm (macOS) based on the platform.
func (m *Manager) startVM(ctx context.Context, fn *domain.Function, vmID string, port int, codeDir string) (*backend.VM, error) {
	if m.useMacOS {
		return m.startVMKrunVM(ctx, fn, vmID, port, codeDir)
	}
	return m.startVMKrun(ctx, fn, vmID, port, codeDir)
}

// startVMKrun launches a microVM via the Linux krun binary (ext4 rootfs).
func (m *Manager) startVMKrun(ctx context.Context, fn *domain.Function, vmID string, port int, codeDir string) (*backend.VM, error) {
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

	// VM lifecycle is managed by the pool (StopVM/Shutdown), not by a single
	// invocation context. Binding the process to request ctx would terminate
	// warm VMs after the first invocation.
	cmd := exec.Command("krun", args...)
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
	if err := waitForAgent(ctx, agentAddr, agentTimeout); err != nil {
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
func waitForAgent(ctx context.Context, addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for agent on %s", addr)
}

// ociImageForRuntime returns the OCI image reference for a runtime,
// used by the krunvm (macOS) path.
func (m *Manager) ociImageForRuntime(rt domain.Runtime) string {
	r := string(rt)
	base := "base"
	switch {
	case r == string(domain.RuntimePython) || strings.HasPrefix(r, "python"):
		base = "python"
	case r == string(domain.RuntimeNode) || strings.HasPrefix(r, "node"):
		base = "node"
	case r == string(domain.RuntimeGo) || strings.HasPrefix(r, "go"):
		base = "base"
	case r == string(domain.RuntimeRust):
		base = "base"
	case r == string(domain.RuntimeRuby) || strings.HasPrefix(r, "ruby"):
		base = "ruby"
	case r == string(domain.RuntimeJava) || strings.HasPrefix(r, "java") ||
		r == string(domain.RuntimeKotlin) || r == string(domain.RuntimeScala):
		base = "java"
	case r == string(domain.RuntimePHP) || strings.HasPrefix(r, "php"):
		base = "php"
	case r == string(domain.RuntimeDeno) || strings.HasPrefix(r, "deno"):
		base = "deno"
	case r == string(domain.RuntimeBun) || strings.HasPrefix(r, "bun"):
		base = "bun"
	case r == string(domain.RuntimeLua):
		base = "lua"
	case r == string(domain.RuntimeWasm) || strings.HasPrefix(r, "wasm"):
		base = "wasm"
	}
	return m.config.ImagePrefix + base + ":latest"
}

// krunvmName returns a deterministic krunvm VM name for a VM ID.
func krunvmName(vmID string) string {
	return "nova-" + vmID
}

// startVMKrunVM launches a microVM via krunvm (macOS / Apple Virtualization).
// krunvm create: registers a VM from an OCI image with port/volume mapping.
// krunvm start: runs the agent inside the VM (blocking in a goroutine).
func (m *Manager) startVMKrunVM(ctx context.Context, fn *domain.Function, vmID string, port int, codeDir string) (*backend.VM, error) {
	image := m.ociImageForRuntime(fn.Runtime)
	name := krunvmName(vmID)

	// Step 1: krunvm create
	mem := 256
	if fn.MemoryMB > 0 {
		mem = fn.MemoryMB
	}
	createArgs := []string{
		"create", image,
		"--name", name,
		"--cpus", "1",
		"--mem", fmt.Sprintf("%d", mem),
		"--port", fmt.Sprintf("%d:%d", port, agentPort),
		"--volume", fmt.Sprintf("%s:/code", codeDir),
	}

	logging.Op().Info("creating krunvm VM", "vmID", vmID, "image", image, "name", name, "port", port, "codeDir", codeDir)
	logging.Op().Debug("krunvm create args", "args", createArgs)

	createCmd := exec.CommandContext(ctx, "krunvm", createArgs...)
	createOut, err := createCmd.CombinedOutput()
	logging.Op().Debug("krunvm create result", "output", string(createOut), "err", err)
	if err != nil {
		return nil, fmt.Errorf("krunvm create failed: %s: %w", string(createOut), err)
	}

	// Step 2: krunvm start (runs in background goroutine, blocks until VM exits)
	startArgs := []string{
		"start", name,
		"--env", "NOVA_AGENT_MODE=tcp",
		"--env", "NOVA_SKIP_MOUNT=true",
	}
	for k, v := range fn.EnvVars {
		startArgs = append(startArgs, "--env", fmt.Sprintf("%s=%s", k, v))
	}
	startArgs = append(startArgs, "--", "/usr/local/bin/nova-agent")

	logging.Op().Debug("krunvm start args", "args", startArgs)

	// Keep VM lifetime decoupled from request ctx; pool owns process lifetime.
	startCmd := exec.Command("krunvm", startArgs...)
	startCmd.Stdout = os.Stdout
	startCmd.Stderr = os.Stderr
	startCmd.Stdin = nil

	if err := startCmd.Start(); err != nil {
		// Clean up the created VM
		exec.Command("krunvm", "delete", name).Run()
		return nil, fmt.Errorf("krunvm start failed: %w", err)
	}

	agentAddr := fmt.Sprintf("127.0.0.1:%d", port)

	vm := &backend.VM{
		ID:           vmID,
		Runtime:      fn.Runtime,
		State:        backend.VMStateCreating,
		AssignedPort: port,
		GuestIP:      agentAddr,
		CodeDir:      codeDir,
		Cmd:          startCmd,
		CreatedAt:    time.Now(),
		LastUsed:     time.Now(),
	}

	// For krunvm we skip waitForAgent (which opens a probe TCP connection
	// that poisons krunvm's single-connection port forwarding). Instead we
	// mark the VM as running immediately; the caller (pool.createVM) will
	// call NewClient → Init which retries the dial until the agent is ready.
	vm.State = backend.VMStateRunning
	metrics.Global().RecordVMCreated()

	m.mu.Lock()
	m.vms[vmID] = vm
	m.mu.Unlock()

	logging.Op().Info("krunvm VM ready", "vmID", vmID, "addr", agentAddr, "image", image)
	return vm, nil
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
	if m.useMacOS {
		return m.stopKrunVM(vm.Cmd, vm.CodeDir, krunvmName(vmID))
	}
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

func (m *Manager) stopKrunVM(cmd *exec.Cmd, codeDir, name string) error {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	// Delete the krunvm VM registration
	exec.Command("krunvm", "delete", name).Run()
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
