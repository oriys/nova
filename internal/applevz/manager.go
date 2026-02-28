package applevz

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
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

// Manager manages Apple Virtualization.framework VMs via nova-vz or vfkit for function execution.
type Manager struct {
	config    *Config
	vms       map[string]*vmInfo
	mu        sync.RWMutex
	vmTool    string // path to nova-vz or vfkit
	useNovaVZ bool   // true if using nova-vz (supports snapshots)
}

// vmInfo tracks a running VM.
type vmInfo struct {
	vm                *backend.VM
	cmd               *exec.Cmd
	codeDir           string
	socketPath        string
	controlSocketPath string // nova-vz only: control socket for save/restore
}

// NewManager creates a new Apple VZ backend manager.
func NewManager(cfg *Config) (*Manager, error) {
	if !IsSupported() {
		return nil, fmt.Errorf("Apple Virtualization backend requires macOS")
	}

	// Locate VM tool: prefer nova-vz (supports snapshots), fall back to vfkit
	var vmTool string
	var useNovaVZ bool

	if cfg.NovaVZPath != "" {
		vmTool = cfg.NovaVZPath
		useNovaVZ = true
	} else if p, err := exec.LookPath("nova-vz"); err == nil {
		vmTool = p
		useNovaVZ = true
	} else if cfg.VfkitPath != "" {
		vmTool = cfg.VfkitPath
	} else if p, err := exec.LookPath("vfkit"); err == nil {
		vmTool = p
	} else {
		return nil, fmt.Errorf("neither nova-vz nor vfkit found in PATH (build nova-vz via: make nova-vz, or install vfkit via: brew install vfkit)")
	}

	// Verify kernel exists
	if cfg.KernelPath == "" {
		return nil, fmt.Errorf("kernel_path is required for Apple VZ backend")
	}
	if _, err := os.Stat(cfg.KernelPath); err != nil {
		return nil, fmt.Errorf("kernel not found at %s: %w", cfg.KernelPath, err)
	}

	dirs := []string{cfg.CodeDir, cfg.SocketDir}
	if useNovaVZ && cfg.SnapshotDirVal != "" {
		dirs = append(dirs, cfg.SnapshotDirVal)
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return &Manager{
		config:    cfg,
		vms:       make(map[string]*vmInfo),
		vmTool:    vmTool,
		useNovaVZ: useNovaVZ,
	}, nil
}

// rootfsForRuntime maps a runtime to its rootfs disk image path.
// On arm64 (Apple Silicon), looks for <base>-arm64.ext4 first, then falls back to <base>.ext4.
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
	case r == string(domain.RuntimeC) || r == string(domain.RuntimeCpp):
		base = "base"
	}
	// Prefer arch-specific rootfs (e.g. base-arm64.ext4) on Apple Silicon
	if goruntime.GOARCH == "arm64" {
		archPath := filepath.Join(m.config.RootfsDir, base+"-arm64.ext4")
		if _, err := os.Stat(archPath); err == nil {
			return archPath
		}
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

// startVM launches a VM via nova-vz or vfkit and waits for the agent.
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
	cmdline := "console=hvc0 root=/dev/vda rw init=/init modules=virtio_blk,ext4,vsock,vmw_vsock_virtio_transport_common,vmw_vsock_virtio_transport"
	if m.config.KernelCmdline != "" {
		cmdline += " " + m.config.KernelCmdline
	}

	var args []string
	var controlSocketPath string

	if m.useNovaVZ {
		args, controlSocketPath = m.buildNovaVZArgs(vmID, cpus, memMB, cmdline, rootfs, codeDir, socketPath)
	} else {
		args = m.buildVfkitArgs(cpus, memMB, cmdline, rootfs, codeDir, socketPath)
	}

	logging.Op().Info("creating Apple VZ VM",
		"vmID", vmID,
		"tool", filepath.Base(m.vmTool),
		"rootfs", rootfs,
		"codeDir", codeDir,
		"socket", socketPath,
		"memory", memMB,
		"cpus", cpus,
		"snapshots", m.useNovaVZ && m.config.SnapshotDirVal != "",
	)

	cmd := exec.CommandContext(ctx, m.vmTool, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s start failed: %w", filepath.Base(m.vmTool), err)
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
		m.stopProcess(cmd, codeDir, socketPath, controlSocketPath)
		return nil, fmt.Errorf("agent not ready: %w", err)
	}

	vm.State = backend.VMStateRunning
	metrics.Global().RecordVMCreated()

	m.mu.Lock()
	m.vms[vmID] = &vmInfo{
		vm:                vm,
		cmd:               cmd,
		codeDir:           codeDir,
		socketPath:        socketPath,
		controlSocketPath: controlSocketPath,
	}
	m.mu.Unlock()

	logging.Op().Info("Apple VZ VM ready", "vmID", vmID, "socket", socketPath)
	return vm, nil
}

// buildNovaVZArgs builds CLI arguments for nova-vz.
func (m *Manager) buildNovaVZArgs(vmID string, cpus, memMB int, cmdline, rootfs, codeDir, socketPath string) ([]string, string) {
	ctlSocket := filepath.Join(m.config.SocketDir, fmt.Sprintf("nova-vz-%s-ctl.sock", vmID))
	os.Remove(ctlSocket)

	args := []string{
		"--kernel", m.config.KernelPath,
		"--cmdline", cmdline,
		"--rootfs", rootfs,
		"--cpus", fmt.Sprintf("%d", cpus),
		"--memory", fmt.Sprintf("%d", memMB),
		"--shared-dir", codeDir,
		"--mount-tag", "code",
		"--vsock-port", fmt.Sprintf("%d", agentVsockPort),
		"--vsock-socket", socketPath,
		"--control-socket", ctlSocket,
	}
	if m.config.InitrdPath != "" {
		args = append(args, "--initrd", m.config.InitrdPath)
	}
	return args, ctlSocket
}

// buildVfkitArgs builds CLI arguments for vfkit.
func (m *Manager) buildVfkitArgs(cpus, memMB int, cmdline, rootfs, codeDir, socketPath string) []string {
	bootloaderArg := fmt.Sprintf("linux,kernel=%s,cmdline=%s", m.config.KernelPath, cmdline)
	if m.config.InitrdPath != "" {
		bootloaderArg = fmt.Sprintf("linux,kernel=%s,initrd=%s,cmdline=%s",
			m.config.KernelPath, m.config.InitrdPath, cmdline)
	}
	return []string{
		"--cpus", fmt.Sprintf("%d", cpus),
		"--memory", fmt.Sprintf("%d", memMB),
		"--bootloader", bootloaderArg,
		"--device", fmt.Sprintf("virtio-blk,path=%s", rootfs),
		"--device", fmt.Sprintf("virtio-fs,sharedDir=%s,mountTag=code", codeDir),
		"--device", fmt.Sprintf("virtio-vsock,port=%d,socketURL=%s", agentVsockPort, socketPath),
		"--device", "virtio-net,nat",
	}
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
	return m.stopProcess(info.cmd, info.codeDir, info.socketPath, info.controlSocketPath)
}

func (m *Manager) stopProcess(cmd *exec.Cmd, codeDir, socketPath, ctlSocketPath string) error {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	for _, path := range []string{socketPath, ctlSocketPath} {
		if path != "" {
			os.Remove(path)
		}
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

// SnapshotDir returns the snapshot directory if nova-vz is available (supports save/restore).
func (m *Manager) SnapshotDir() string {
	if m.useNovaVZ {
		return m.config.SnapshotDirVal
	}
	return ""
}
