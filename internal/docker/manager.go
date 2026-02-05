// Package docker provides the Docker container backend for function execution.
package docker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	agentPort      = 9999
	defaultTimeout = 30 * time.Second
)

// Config holds Docker backend configuration.
type Config struct {
	CodeDir        string        // Base directory for function code
	AgentPath      string        // Path to nova-agent binary (for mounting)
	ImagePrefix    string        // Docker image prefix (e.g., "nova-runtime")
	Network        string        // Docker network name (optional)
	PortRangeMin   int           // Minimum host port for agent mapping
	PortRangeMax   int           // Maximum host port for agent mapping
	CPULimit       float64       // CPU limit per container (default: 1.0)
	DefaultTimeout time.Duration // Default operation timeout (default: 30s)
	AgentTimeout   time.Duration // Agent startup timeout (default: 10s)
}

// DefaultConfig returns sensible defaults for Docker backend.
func DefaultConfig() *Config {
	codeDir := os.Getenv("NOVA_CODE_DIR")
	if codeDir == "" {
		codeDir = "/tmp/nova/code"
	}
	agentPath := os.Getenv("NOVA_AGENT_PATH")
	if agentPath == "" {
		agentPath = "/opt/nova/bin/nova-agent"
	}
	imagePrefix := os.Getenv("NOVA_DOCKER_IMAGE_PREFIX")
	if imagePrefix == "" {
		imagePrefix = "nova-runtime"
	}

	return &Config{
		CodeDir:        codeDir,
		AgentPath:      agentPath,
		ImagePrefix:    imagePrefix,
		Network:        os.Getenv("NOVA_DOCKER_NETWORK"),
		PortRangeMin:   20000,
		PortRangeMax:   30000,
		CPULimit:       1.0,
		DefaultTimeout: 30 * time.Second,
		AgentTimeout:   10 * time.Second,
	}
}

// Manager manages Docker containers for function execution.
type Manager struct {
	config     *Config
	containers map[string]*backend.VM
	mu         sync.RWMutex
	nextPort   int32
}

// NewManager creates a new Docker backend manager.
func NewManager(cfg *Config) (*Manager, error) {
	if err := os.MkdirAll(cfg.CodeDir, 0755); err != nil {
		return nil, fmt.Errorf("create code dir: %w", err)
	}

	// Verify docker is available
	if err := exec.Command("docker", "version").Run(); err != nil {
		return nil, fmt.Errorf("docker not available: %w", err)
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

// CreateVM creates a new Docker container for the function.
func (m *Manager) CreateVM(ctx context.Context, fn *domain.Function, codeContent []byte) (*backend.VM, error) {
	vmID := uuid.New().String()[:12]
	port := m.allocatePort()

	// Prepare code directory
	codeDir := filepath.Join(m.config.CodeDir, vmID)
	if err := os.MkdirAll(codeDir, 0755); err != nil {
		return nil, fmt.Errorf("create code dir: %w", err)
	}

	// Write code to container code directory
	if len(codeContent) > 0 {
		handlerPath := filepath.Join(codeDir, "handler")
		if err := os.WriteFile(handlerPath, codeContent, 0755); err != nil {
			os.RemoveAll(codeDir)
			return nil, fmt.Errorf("write code file: %w", err)
		}
	}

	var image string
	if fn.RuntimeImageName != "" {
		image = fn.RuntimeImageName
	} else {
		image = imageForRuntime(fn.Runtime, m.config.ImagePrefix)
	}
	containerName := fmt.Sprintf("nova-%s", vmID)

	// Build docker run command
	cpuLimit := m.config.CPULimit
	if cpuLimit <= 0 {
		cpuLimit = 1.0
	}
	args := []string{
		"run", "-d",
		"--name", containerName,
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", port, agentPort),
		"-v", fmt.Sprintf("%s:/code:ro", codeDir),
		"-e", "NOVA_AGENT_MODE=tcp",
		"-e", "NOVA_SKIP_MOUNT=true",
		"--memory", fmt.Sprintf("%dm", fn.MemoryMB),
		"--cpus", fmt.Sprintf("%.2f", cpuLimit),
	}

	// Add network if specified
	if m.config.Network != "" {
		args = append(args, "--network", m.config.Network)
	}

	// Add environment variables
	for k, v := range fn.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, image)

	logging.Op().Debug("starting Docker container", "image", image, "name", containerName, "port", port)

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(codeDir)
		return nil, fmt.Errorf("docker run failed: %w: %s", err, output)
	}

	containerID := strings.TrimSpace(string(output))

	vm := &backend.VM{
		ID:                vmID,
		Runtime:           fn.Runtime,
		State:             backend.VMStateCreating,
		DockerContainerID: containerID,
		AssignedPort:      port,
		CodeDir:           codeDir,
		CreatedAt:         time.Now(),
		LastUsed:          time.Now(),
	}

	// Wait for agent to be ready
	agentTimeout := m.config.AgentTimeout
	if agentTimeout == 0 {
		agentTimeout = 10 * time.Second
	}
	if err := waitForAgent(port, agentTimeout); err != nil {
		m.stopContainer(containerID, codeDir)
		return nil, fmt.Errorf("agent not ready: %w", err)
	}

	vm.State = backend.VMStateRunning
	metrics.Global().RecordVMCreated()

	m.mu.Lock()
	m.containers[vmID] = vm
	m.mu.Unlock()

	logging.Op().Info("Docker container ready", "container", containerID[:12], "port", port)
	return vm, nil
}

// CreateVMWithFiles creates a new Docker container with multiple code files.
func (m *Manager) CreateVMWithFiles(ctx context.Context, fn *domain.Function, files map[string][]byte) (*backend.VM, error) {
	vmID := uuid.New().String()[:12]
	port := m.allocatePort()

	// Prepare code directory
	codeDir := filepath.Join(m.config.CodeDir, vmID)
	if err := os.MkdirAll(codeDir, 0755); err != nil {
		return nil, fmt.Errorf("create code dir: %w", err)
	}

	// Write all files to container code directory
	for path, content := range files {
		fullPath := filepath.Join(codeDir, path)
		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			os.RemoveAll(codeDir)
			return nil, fmt.Errorf("create dir for %s: %w", path, err)
		}
		// Make files executable by default
		if err := os.WriteFile(fullPath, content, 0755); err != nil {
			os.RemoveAll(codeDir)
			return nil, fmt.Errorf("write file %s: %w", path, err)
		}
	}

	var image string
	if fn.RuntimeImageName != "" {
		image = fn.RuntimeImageName
	} else {
		image = imageForRuntime(fn.Runtime, m.config.ImagePrefix)
	}
	containerName := fmt.Sprintf("nova-%s", vmID)

	// Build docker run command
	cpuLimit := m.config.CPULimit
	if cpuLimit <= 0 {
		cpuLimit = 1.0
	}
	args := []string{
		"run", "-d",
		"--name", containerName,
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", port, agentPort),
		"-v", fmt.Sprintf("%s:/code:ro", codeDir),
		"-e", "NOVA_AGENT_MODE=tcp",
		"-e", "NOVA_SKIP_MOUNT=true",
		"--memory", fmt.Sprintf("%dm", fn.MemoryMB),
		"--cpus", fmt.Sprintf("%.2f", cpuLimit),
	}

	if m.config.Network != "" {
		args = append(args, "--network", m.config.Network)
	}

	for k, v := range fn.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, image)

	logging.Op().Debug("starting Docker container with files", "image", image, "name", containerName, "port", port, "files", len(files))

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(codeDir)
		return nil, fmt.Errorf("docker run failed: %w: %s", err, output)
	}

	containerID := strings.TrimSpace(string(output))

	vm := &backend.VM{
		ID:                vmID,
		Runtime:           fn.Runtime,
		State:             backend.VMStateCreating,
		DockerContainerID: containerID,
		AssignedPort:      port,
		CodeDir:           codeDir,
		CreatedAt:         time.Now(),
		LastUsed:          time.Now(),
	}

	agentTimeout := m.config.AgentTimeout
	if agentTimeout == 0 {
		agentTimeout = 10 * time.Second
	}
	if err := waitForAgent(port, agentTimeout); err != nil {
		m.stopContainer(containerID, codeDir)
		return nil, fmt.Errorf("agent not ready: %w", err)
	}

	vm.State = backend.VMStateRunning
	metrics.Global().RecordVMCreated()

	m.mu.Lock()
	m.containers[vmID] = vm
	m.mu.Unlock()

	logging.Op().Info("Docker container ready", "container", containerID[:12], "port", port, "files", len(files))
	return vm, nil
}

// waitForAgent polls the agent until it's ready.
func waitForAgent(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for agent on port %d", port)
}

// StopVM stops and removes a Docker container.
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
	// Stop container
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exec.CommandContext(ctx, "docker", "stop", "-t", "2", containerID).Run()
	exec.CommandContext(ctx, "docker", "rm", "-f", containerID).Run()

	// Clean up code directory
	if codeDir != "" {
		os.RemoveAll(codeDir)
	}
	return nil
}

// NewClient creates a TCP client for the container.
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

// SnapshotDir returns empty string - Docker backend doesn't support snapshots.
func (m *Manager) SnapshotDir() string {
	return ""
}

// imageForRuntime maps runtime to Docker image name.
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
	case r == string(domain.RuntimeDotnet) || strings.HasPrefix(r, "dotnet"):
		return prefix + "-dotnet"
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

// Client communicates with the agent inside a Docker container via TCP.
type Client struct {
	vm          *backend.VM
	conn        net.Conn
	mu          sync.Mutex
	initPayload json.RawMessage
}

// NewClient creates a new TCP client for the container.
func NewClient(vm *backend.VM) (*Client, error) {
	return &Client{vm: vm}, nil
}

// Init sends the init message to the agent.
func (c *Client) Init(fn *domain.Function) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, _ := json.Marshal(&backend.InitPayload{
		Runtime:   string(fn.Runtime),
		Handler:   fn.Handler,
		EnvVars:   fn.EnvVars,
		Command:   fn.RuntimeCommand,
		Extension: fn.RuntimeExtension,
	})
	c.initPayload = payload

	if err := c.redialAndInit(5 * time.Second); err != nil {
		return err
	}
	return c.closeLocked()
}

// Execute runs a function invocation.
func (c *Client) Execute(reqID string, input json.RawMessage, timeoutS int) (*backend.RespPayload, error) {
	return c.ExecuteWithTrace(reqID, input, timeoutS, "", "")
}

// ExecuteWithTrace runs a function with trace context.
func (c *Client) ExecuteWithTrace(reqID string, input json.RawMessage, timeoutS int, traceParent, traceState string) (*backend.RespPayload, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, _ := json.Marshal(&backend.ExecPayload{
		RequestID:   reqID,
		Input:       input,
		TimeoutS:    timeoutS,
		TraceParent: traceParent,
		TraceState:  traceState,
	})

	execMsg := &backend.VsockMessage{Type: backend.MsgTypeExec, Payload: payload}

	// Retry with backoff
	backoff := []time.Duration{10 * time.Millisecond, 25 * time.Millisecond, 50 * time.Millisecond}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := c.redialAndInit(5 * time.Second); err != nil {
			lastErr = err
			if attempt < 2 {
				time.Sleep(backoff[attempt])
			}
			continue
		}

		deadline := time.Now().Add(time.Duration(timeoutS+5) * time.Second)
		_ = c.conn.SetDeadline(deadline)

		if err := c.sendLocked(execMsg); err != nil {
			lastErr = err
			_ = c.closeLocked()
			if isBrokenConnErr(err) && attempt < 2 {
				time.Sleep(backoff[attempt])
				continue
			}
			return nil, err
		}

		resp, err := c.receiveLocked()
		_ = c.conn.SetDeadline(time.Time{})
		if err != nil {
			lastErr = err
			_ = c.closeLocked()
			if isBrokenConnErr(err) && attempt < 2 {
				time.Sleep(backoff[attempt])
				continue
			}
			return nil, err
		}

		var result backend.RespPayload
		if err := json.Unmarshal(resp.Payload, &result); err != nil {
			_ = c.closeLocked()
			return nil, err
		}

		_ = c.closeLocked()
		return &result, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("execute failed")
}

// Ping checks if the agent is responsive.
func (c *Client) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.redialAndInit(5 * time.Second); err != nil {
		return err
	}
	defer c.closeLocked()

	_ = c.conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err := c.sendLocked(&backend.VsockMessage{Type: backend.MsgTypePing}); err != nil {
		return err
	}
	_, err := c.receiveLocked()
	_ = c.conn.SetDeadline(time.Time{})
	return err
}

// Reload sends new code files to the agent for hot reload.
func (c *Client) Reload(files map[string][]byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, err := json.Marshal(&backend.ReloadPayload{Files: files})
	if err != nil {
		return err
	}

	if err := c.redialAndInit(5 * time.Second); err != nil {
		return err
	}
	defer c.closeLocked()

	_ = c.conn.SetDeadline(time.Now().Add(30 * time.Second))
	if err := c.sendLocked(&backend.VsockMessage{Type: backend.MsgTypeReload, Payload: payload}); err != nil {
		return err
	}

	resp, err := c.receiveLocked()
	_ = c.conn.SetDeadline(time.Time{})
	if err != nil {
		return err
	}

	if resp.Type != backend.MsgTypeResp {
		return fmt.Errorf("unexpected response type: %d", resp.Type)
	}
	return nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *Client) closeLocked() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) dialLocked(timeout time.Duration) error {
	addr := fmt.Sprintf("127.0.0.1:%d", c.vm.AssignedPort)
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func (c *Client) initLocked() error {
	if c.initPayload == nil {
		return errors.New("missing init payload")
	}
	if err := c.sendLocked(&backend.VsockMessage{Type: backend.MsgTypeInit, Payload: c.initPayload}); err != nil {
		return err
	}
	resp, err := c.receiveLocked()
	if err != nil {
		return err
	}
	if resp.Type != backend.MsgTypeResp {
		return fmt.Errorf("unexpected response type: %d", resp.Type)
	}
	return nil
}

func (c *Client) redialAndInit(timeout time.Duration) error {
	hadConn := c.conn != nil
	_ = c.closeLocked()
	if hadConn {
		time.Sleep(10 * time.Millisecond)
	}
	if err := c.dialLocked(timeout); err != nil {
		return err
	}
	if c.initPayload != nil {
		if err := c.initLocked(); err != nil {
			_ = c.closeLocked()
			return err
		}
	}
	return nil
}

func (c *Client) sendLocked(msg *backend.VsockMessage) error {
	if c.conn == nil {
		return errors.New("not connected")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	buf := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(data)))
	copy(buf[4:], data)

	return writeFull(c.conn, buf)
}

func (c *Client) receiveLocked() (*backend.VsockMessage, error) {
	if c.conn == nil {
		return nil, errors.New("not connected")
	}

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, lenBuf); err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint32(lenBuf)
	data := make([]byte, msgLen)
	if _, err := io.ReadFull(c.conn, data); err != nil {
		return nil, err
	}

	var msg backend.VsockMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func writeFull(conn net.Conn, b []byte) error {
	for len(b) > 0 {
		n, err := conn.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

func isBrokenConnErr(err error) bool {
	return err != nil && (errors.Is(err, io.EOF) ||
		errors.Is(err, net.ErrClosed) ||
		strings.Contains(err.Error(), "connection reset") ||
		strings.Contains(err.Error(), "broken pipe"))
}
