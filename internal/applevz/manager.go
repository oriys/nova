package applevz

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
	agentVsockPort = 9999
)

// Manager manages Apple Virtualization.framework VMs via vfkit for function execution.
type Manager struct {
	config    *Config
	vms       map[string]*vmInfo
	mu        sync.RWMutex
	vfkitPath string
}

// vmInfo tracks a running vfkit VM.
type vmInfo struct {
	vm         *backend.VM
	cmd        *exec.Cmd
	codeDir    string
	socketPath string
}

// NewManager creates a new Apple VZ backend manager.
func NewManager(cfg *Config) (*Manager, error) {
	if !IsSupported() {
		return nil, fmt.Errorf("Apple Virtualization backend requires macOS")
	}

	// Locate vfkit binary
	vfkitPath := cfg.VfkitPath
	if vfkitPath == "" {
		var err error
		vfkitPath, err = exec.LookPath("vfkit")
		if err != nil {
			return nil, fmt.Errorf("vfkit not found in PATH: %w (install with: brew install vfkit)", err)
		}
	}

	// Verify kernel exists
	if cfg.KernelPath == "" {
		return nil, fmt.Errorf("kernel_path is required for Apple VZ backend")
	}
	if _, err := os.Stat(cfg.KernelPath); err != nil {
		return nil, fmt.Errorf("kernel not found at %s: %w", cfg.KernelPath, err)
	}

	for _, dir := range []string{cfg.CodeDir, cfg.SocketDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return &Manager{
		config:    cfg,
		vms:       make(map[string]*vmInfo),
		vfkitPath: vfkitPath,
	}, nil
}

// rootfsForRuntime maps a runtime to its rootfs disk image path.
func (m *Manager) rootfsForRuntime(rt domain.Runtime) string {
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
	return filepath.Join(m.config.RootfsDir, base+".ext4")
}

// CreateVM creates a new Apple VZ VM for the function.
func (m *Manager) CreateVM(ctx context.Context, fn *domain.Function, codeContent []byte) (*backend.VM, error) {
	vmID := uuid.New().String()[:12]

	// Prepare code directory
	codeDir := filepath.Join(m.config.CodeDir, vmID)
	if err := os.MkdirAll(codeDir, 0755); err != nil {
		return nil, fmt.Errorf("create code dir: %w", err)
	}

	// Write code file
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

	vm, err := m.startVM(ctx, fn, vmID, codeDir)
	if err != nil {
		os.RemoveAll(codeDir)
		return nil, err
	}
	return vm, nil
}

// CreateVMWithFiles creates a new Apple VZ VM with multiple code files.
func (m *Manager) CreateVMWithFiles(ctx context.Context, fn *domain.Function, files map[string][]byte) (*backend.VM, error) {
	vmID := uuid.New().String()[:12]

	// Prepare code directory
	codeDir := filepath.Join(m.config.CodeDir, vmID)
	if err := os.MkdirAll(codeDir, 0755); err != nil {
		return nil, fmt.Errorf("create code dir: %w", err)
	}

	// Write all files
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

	vm, err := m.startVM(ctx, fn, vmID, codeDir)
	if err != nil {
		os.RemoveAll(codeDir)
		return nil, err
	}
	return vm, nil
}

// startVM launches a vfkit VM and waits for the agent.
func (m *Manager) startVM(ctx context.Context, fn *domain.Function, vmID string, codeDir string) (*backend.VM, error) {
	rootfs := m.rootfsForRuntime(fn.Runtime)
	socketPath := filepath.Join(m.config.SocketDir, fmt.Sprintf("nova-vz-%s.sock", vmID))

	// Clean up stale socket
	os.Remove(socketPath)

	memMB := m.config.DefaultMemMB
	if fn.MemoryMB > 0 {
		memMB = fn.MemoryMB
	}
	cpus := m.config.DefaultCPUs
	if cpus <= 0 {
		cpus = 1
	}

	// Build kernel command line
	cmdline := "console=hvc0 root=/dev/vda rw"
	if m.config.KernelCmdline != "" {
		cmdline += " " + m.config.KernelCmdline
	}

	// Build vfkit arguments
	bootloaderArg := fmt.Sprintf("linux,kernel=%s,cmdline=%s", m.config.KernelPath, cmdline)
	if m.config.InitrdPath != "" {
		bootloaderArg = fmt.Sprintf("linux,kernel=%s,initrd=%s,cmdline=%s",
			m.config.KernelPath, m.config.InitrdPath, cmdline)
	}

	args := []string{
		"--cpus", fmt.Sprintf("%d", cpus),
		"--memory", fmt.Sprintf("%d", memMB),
		"--bootloader", bootloaderArg,
		// Root filesystem as virtio block device
		"--device", fmt.Sprintf("virtio-blk,path=%s", rootfs),
		// Share code directory via VirtioFS
		"--device", fmt.Sprintf("virtio-fs,sharedDir=%s,mountTag=code", codeDir),
		// Vsock for agent communication (UNIX socket on host)
		"--device", fmt.Sprintf("virtio-vsock,port=%d,socketURL=%s", agentVsockPort, socketPath),
		// NAT networking for outbound connectivity
		"--device", "virtio-net,nat",
	}

	logging.Op().Info("creating Apple VZ VM",
		"vmID", vmID,
		"rootfs", rootfs,
		"codeDir", codeDir,
		"socket", socketPath,
		"memory", memMB,
		"cpus", cpus,
	)

	cmd := exec.CommandContext(ctx, m.vfkitPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("vfkit start failed: %w", err)
	}

	vm := &backend.VM{
		ID:         vmID,
		Runtime:    fn.Runtime,
		State:      backend.VMStateCreating,
		VsockPath:  socketPath,
		CodeDir:    codeDir,
		Cmd:        cmd,
		CreatedAt:  time.Now(),
		LastUsed:   time.Now(),
	}

	// Wait for the vsock UNIX socket to appear and agent to respond
	agentTimeout := m.config.AgentTimeout
	if agentTimeout == 0 {
		agentTimeout = 15 * time.Second
	}
	if err := waitForVsockAgent(socketPath, agentTimeout); err != nil {
		m.stopVfkit(cmd, codeDir, socketPath)
		return nil, fmt.Errorf("agent not ready: %w", err)
	}

	vm.State = backend.VMStateRunning
	metrics.Global().RecordVMCreated()

	m.mu.Lock()
	m.vms[vmID] = &vmInfo{
		vm:         vm,
		cmd:        cmd,
		codeDir:    codeDir,
		socketPath: socketPath,
	}
	m.mu.Unlock()

	logging.Op().Info("Apple VZ VM ready", "vmID", vmID, "socket", socketPath)
	return vm, nil
}

// waitForVsockAgent polls the vsock UNIX socket until the agent is reachable.
func waitForVsockAgent(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// First check if the socket file exists
		if _, err := os.Stat(socketPath); err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		// Try connecting
		conn, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for agent on vsock socket %s", socketPath)
}

// StopVM stops and cleans up an Apple VZ VM.
func (m *Manager) StopVM(vmID string) error {
	m.mu.Lock()
	info, ok := m.vms[vmID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("VM not found: %s", vmID)
	}
	delete(m.vms, vmID)
	m.mu.Unlock()

	metrics.Global().RecordVMStopped()
	return m.stopVfkit(info.cmd, info.codeDir, info.socketPath)
}

func (m *Manager) stopVfkit(cmd *exec.Cmd, codeDir, socketPath string) error {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	if socketPath != "" {
		os.Remove(socketPath)
	}
	if codeDir != "" {
		os.RemoveAll(codeDir)
	}
	return nil
}

// NewClient creates a vsock UNIX socket client for the VM.
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

// SnapshotDir returns empty string — Apple VZ backend doesn't support snapshots yet.
func (m *Manager) SnapshotDir() string {
	return ""
}
