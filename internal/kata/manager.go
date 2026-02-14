package kata

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

// Manager manages Kata Containers for function execution.
type Manager struct {
	config     *Config
	containers map[string]*backend.VM
	mu         sync.RWMutex
	nextPort   int32
}

// NewManager creates a new Kata Containers backend manager.
func NewManager(cfg *Config) (*Manager, error) {
	if err := os.MkdirAll(cfg.CodeDir, 0755); err != nil {
		return nil, fmt.Errorf("create code dir: %w", err)
	}

	// Verify that the container runtime is available
	runtimeBin := "docker"
	if err := exec.Command(runtimeBin, "version").Run(); err != nil {
		return nil, fmt.Errorf("container runtime not available: %w", err)
	}

	return &Manager{
		config:     cfg,
		containers: make(map[string]*backend.VM),
		nextPort:   int32(cfg.PortRangeMin),
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

// agentAddr returns the address to reach the agent on.
func (m *Manager) agentAddr(containerName string, port int) string {
	if m.config.Network != "" {
		return fmt.Sprintf("%s:%d", containerName, agentPort)
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}

// CreateVM creates a new Kata Container for the function.
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

	vm, err := m.startContainer(ctx, fn, vmID, port, codeDir)
	if err != nil {
		os.RemoveAll(codeDir)
		return nil, err
	}

	return vm, nil
}

// CreateVMWithFiles creates a new Kata Container with multiple code files.
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

	vm, err := m.startContainer(ctx, fn, vmID, port, codeDir)
	if err != nil {
		os.RemoveAll(codeDir)
		return nil, err
	}

	return vm, nil
}

// startContainer launches a Kata Container and waits for the agent.
func (m *Manager) startContainer(ctx context.Context, fn *domain.Function, vmID string, port int, codeDir string) (*backend.VM, error) {
	var image string
	if fn.RuntimeImageName != "" {
		image = fn.RuntimeImageName
	} else {
		image = imageForRuntime(fn.Runtime, m.config.ImagePrefix)
	}
	containerName := fmt.Sprintf("nova-kata-%s", vmID)

	cpuLimit := m.config.CPULimit
	if cpuLimit <= 0 {
		cpuLimit = 1.0
	}

	// Build docker create command with --runtime flag for Kata
	args := []string{
		"create",
		"--name", containerName,
		"--runtime", m.config.RuntimeName,
		"-e", "NOVA_AGENT_MODE=tcp",
		"-e", "NOVA_SKIP_MOUNT=true",
		"--memory", fmt.Sprintf("%dm", fn.MemoryMB),
		"--cpus", fmt.Sprintf("%.2f", cpuLimit),
	}

	if m.config.Network != "" {
		args = append(args, "--network", m.config.Network)
	} else {
		args = append(args, "-p", fmt.Sprintf("127.0.0.1:%d:%d", port, agentPort))
	}

	// Add environment variables
	for k, v := range fn.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, image)

	logging.Op().Debug("creating Kata container", "image", image, "name", containerName, "port", port, "runtime", m.config.RuntimeName)

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker create with kata runtime failed: %w: %s", err, output)
	}

	containerID := strings.TrimSpace(string(output))

	// Copy code files into container
	cpCmd := exec.CommandContext(ctx, "docker", "cp", codeDir+"/.", containerName+":/code/")
	if cpOut, err := cpCmd.CombinedOutput(); err != nil {
		m.stopContainer(containerID, codeDir)
		return nil, fmt.Errorf("docker cp failed: %w: %s", err, cpOut)
	}

	// Start container
	startCmd := exec.CommandContext(ctx, "docker", "start", containerName)
	if startOut, err := startCmd.CombinedOutput(); err != nil {
		m.stopContainer(containerID, codeDir)
		return nil, fmt.Errorf("docker start failed: %w: %s", err, startOut)
	}

	agentAddress := m.agentAddr(containerName, port)

	vm := &backend.VM{
		ID:                vmID,
		Runtime:           fn.Runtime,
		State:             backend.VMStateCreating,
		DockerContainerID: containerID,
		AssignedPort:      port,
		GuestIP:           agentAddress,
		CodeDir:           codeDir,
		CreatedAt:         time.Now(),
		LastUsed:          time.Now(),
	}

	// Wait for agent to be ready (Kata VMs may take slightly longer to boot)
	agentTimeout := m.config.AgentTimeout
	if agentTimeout == 0 {
		agentTimeout = 15 * time.Second
	}
	if err := waitForAgent(agentAddress, agentTimeout); err != nil {
		m.stopContainer(containerID, codeDir)
		return nil, fmt.Errorf("agent not ready: %w", err)
	}

	vm.State = backend.VMStateRunning
	metrics.Global().RecordVMCreated()

	m.mu.Lock()
	m.containers[vmID] = vm
	m.mu.Unlock()

	logging.Op().Info("Kata container ready", "container", containerID[:12], "addr", agentAddress, "runtime", m.config.RuntimeName)
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

// StopVM stops and removes a Kata Container.
func (m *Manager) StopVM(vmID string) error {
	m.mu.Lock()
	vm, ok := m.containers[vmID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("container not found: %s", vmID)
	}
	delete(m.containers, vmID)
	m.mu.Unlock()

	metrics.Global().RecordVMStopped()
	return m.stopContainer(vm.DockerContainerID, vm.CodeDir)
}

func (m *Manager) stopContainer(containerID, codeDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exec.CommandContext(ctx, "docker", "stop", "-t", "2", containerID).Run()
	exec.CommandContext(ctx, "docker", "rm", "-f", containerID).Run()

	if codeDir != "" {
		os.RemoveAll(codeDir)
	}
	return nil
}

// NewClient creates a TCP client for the Kata Container.
func (m *Manager) NewClient(vm *backend.VM) (backend.Client, error) {
	return NewClient(vm)
}

// Shutdown stops all containers.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.containers))
	for id := range m.containers {
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

// SnapshotDir returns empty string — Kata backend doesn't support snapshots.
func (m *Manager) SnapshotDir() string {
	return ""
}

// imageForRuntime maps runtime to container image name.
func imageForRuntime(rt domain.Runtime, prefix string) string {
	r := string(rt)
	switch {
	case r == string(domain.RuntimePython) || strings.HasPrefix(r, "python"):
		return prefix + "-python"
	case r == string(domain.RuntimeNode) || strings.HasPrefix(r, "node"):
		return prefix + "-node"
	case r == string(domain.RuntimeGo) || strings.HasPrefix(r, "go"):
		return prefix + "-base"
	case r == string(domain.RuntimeRust):
		return prefix + "-base"
	case r == string(domain.RuntimeRuby) || strings.HasPrefix(r, "ruby"):
		return prefix + "-ruby"
	case r == string(domain.RuntimeJava) || strings.HasPrefix(r, "java") ||
		r == string(domain.RuntimeKotlin) || r == string(domain.RuntimeScala):
		return prefix + "-java"
	case r == string(domain.RuntimePHP) || strings.HasPrefix(r, "php"):
		return prefix + "-php"
	case r == string(domain.RuntimeLua) || strings.HasPrefix(r, "lua"):
		return prefix + "-lua"
	case r == string(domain.RuntimeDeno) || strings.HasPrefix(r, "deno"):
		return prefix + "-deno"
	case r == string(domain.RuntimeBun) || strings.HasPrefix(r, "bun"):
		return prefix + "-bun"
	case r == string(domain.RuntimeWasm) || strings.HasPrefix(r, "wasm"):
		return prefix + "-wasm"
	default:
		return prefix + "-base"
	}
}
