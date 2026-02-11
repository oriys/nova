// Package wasm provides the WebAssembly (WASM) backend for function execution.
// It runs the nova-agent as a host process (no VM or container) and executes
// WASM modules using a WASM runtime such as wasmtime.
package wasm

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
	defaultTimeout = 30 * time.Second
)

// Config holds WASM backend configuration.
type Config struct {
	CodeDir        string        // Base directory for function code
	AgentPath      string        // Path to nova-agent binary
	PortRangeMin   int           // Minimum host port for agent mapping
	PortRangeMax   int           // Maximum host port for agent mapping
	DefaultTimeout time.Duration // Default operation timeout (default: 30s)
	AgentTimeout   time.Duration // Agent startup timeout (default: 10s)
}

// DefaultConfig returns sensible defaults for WASM backend.
func DefaultConfig() *Config {
	codeDir := os.Getenv("NOVA_WASM_CODE_DIR")
	if codeDir == "" {
		codeDir = "/tmp/nova/wasm-code"
	}
	agentPath := os.Getenv("NOVA_AGENT_PATH")
	if agentPath == "" {
		agentPath = "/opt/nova/bin/nova-agent"
	}

	return &Config{
		CodeDir:        codeDir,
		AgentPath:      agentPath,
		PortRangeMin:   30000,
		PortRangeMax:   40000,
		DefaultTimeout: 30 * time.Second,
		AgentTimeout:   10 * time.Second,
	}
}

// Manager manages WASM function execution via host-process agents.
type Manager struct {
	config   *Config
	agents   map[string]*agentProcess
	mu       sync.RWMutex
	nextPort int32
}

// agentProcess tracks a running nova-agent host process.
type agentProcess struct {
	vm  *backend.VM
	cmd *exec.Cmd
}

// NewManager creates a new WASM backend manager.
func NewManager(cfg *Config) (*Manager, error) {
	if err := os.MkdirAll(cfg.CodeDir, 0755); err != nil {
		return nil, fmt.Errorf("create code dir: %w", err)
	}

	// Verify agent binary exists
	if _, err := os.Stat(cfg.AgentPath); err != nil {
		return nil, fmt.Errorf("nova-agent not found at %s: %w", cfg.AgentPath, err)
	}

	return &Manager{
		config:   cfg,
		agents:   make(map[string]*agentProcess),
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

// CreateVM creates a host-process agent for the function.
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

	vm, err := m.startAgent(ctx, vmID, fn, port, codeDir)
	if err != nil {
		os.RemoveAll(codeDir)
		return nil, err
	}

	return vm, nil
}

// CreateVMWithFiles creates a host-process agent with multiple code files.
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

	vm, err := m.startAgent(ctx, vmID, fn, port, codeDir)
	if err != nil {
		os.RemoveAll(codeDir)
		return nil, err
	}

	return vm, nil
}

// startAgent launches the nova-agent as a host process.
func (m *Manager) startAgent(ctx context.Context, vmID string, fn *domain.Function, port int, codeDir string) (*backend.VM, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// Use a background context for the agent process so it outlives the caller's request context.
	cmd := exec.Command(m.config.AgentPath)
	cmd.Env = append(os.Environ(),
		"NOVA_AGENT_MODE=tcp",
		fmt.Sprintf("NOVA_AGENT_PORT=%d", port),
		fmt.Sprintf("NOVA_CODE_DIR=%s", codeDir),
		"NOVA_SKIP_MOUNT=true",
	)
	// Add function environment variables
	for k, v := range fn.EnvVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	logging.Op().Debug("starting WASM agent process", "vmID", vmID, "port", port, "codeDir", codeDir)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start agent process: %w", err)
	}

	vm := &backend.VM{
		ID:           vmID,
		Runtime:      fn.Runtime,
		State:        backend.VMStateCreating,
		AssignedPort: port,
		GuestIP:      addr,
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
	if err := waitForAgent(addr, agentTimeout); err != nil {
		if killErr := cmd.Process.Kill(); killErr != nil {
			logging.Op().Debug("agent kill error during cleanup", "error", killErr)
		}
		cmd.Wait()
		return nil, fmt.Errorf("agent not ready: %w", err)
	}

	vm.State = backend.VMStateRunning
	metrics.Global().RecordVMCreated()

	m.mu.Lock()
	m.agents[vmID] = &agentProcess{vm: vm, cmd: cmd}
	m.mu.Unlock()

	logging.Op().Info("WASM agent process ready", "vmID", vmID, "addr", addr)
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

// StopVM stops and cleans up an agent process.
func (m *Manager) StopVM(vmID string) error {
	m.mu.Lock()
	ap, ok := m.agents[vmID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("agent not found: %s", vmID)
	}
	delete(m.agents, vmID)
	m.mu.Unlock()

	metrics.Global().RecordVMStopped()
	return m.stopAgent(ap)
}

func (m *Manager) stopAgent(ap *agentProcess) error {
	if ap.cmd != nil && ap.cmd.Process != nil {
		if err := ap.cmd.Process.Kill(); err != nil {
			logging.Op().Debug("agent kill error (may have already exited)", "error", err)
		}
		if err := ap.cmd.Wait(); err != nil {
			logging.Op().Debug("agent wait error", "error", err)
		}
	}

	// Clean up code directory
	if ap.vm.CodeDir != "" {
		os.RemoveAll(ap.vm.CodeDir)
	}
	return nil
}

// NewClient creates a TCP client for the agent process.
func (m *Manager) NewClient(vm *backend.VM) (backend.Client, error) {
	return NewClient(vm), nil
}

// Shutdown stops all agent processes.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.agents))
	for id := range m.agents {
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

// SnapshotDir returns empty string - WASM backend doesn't support snapshots.
func (m *Manager) SnapshotDir() string {
	return ""
}

// Client communicates with the agent via TCP, reusing the same protocol as Docker.
type Client struct {
	vm          *backend.VM
	conn        net.Conn
	mu          sync.Mutex
	initPayload json.RawMessage
}

// NewClient creates a new TCP client for the agent process.
func NewClient(vm *backend.VM) *Client {
	return &Client{vm: vm}
}

// Init sends the init message to the agent.
func (c *Client) Init(fn *domain.Function) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, _ := json.Marshal(&backend.InitPayload{
		Runtime:         string(fn.Runtime),
		Handler:         fn.Handler,
		EnvVars:         fn.EnvVars,
		Command:         fn.RuntimeCommand,
		Extension:       fn.RuntimeExtension,
		Mode:            string(fn.Mode),
		FunctionName:    fn.Name,
		FunctionVersion: fn.Version,
		MemoryMB:        fn.MemoryMB,
		TimeoutS:        fn.TimeoutS,
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

// ExecuteStream executes a function in streaming mode.
func (c *Client) ExecuteStream(reqID string, input json.RawMessage, timeoutS int, traceParent, traceState string, callback func(chunk []byte, isLast bool, err error) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, _ := json.Marshal(&backend.ExecPayload{
		RequestID:   reqID,
		Input:       input,
		TimeoutS:    timeoutS,
		TraceParent: traceParent,
		TraceState:  traceState,
		Stream:      true,
	})

	execMsg := &backend.VsockMessage{Type: backend.MsgTypeExec, Payload: payload}

	if err := c.redialAndInit(5 * time.Second); err != nil {
		return err
	}

	deadline := time.Now().Add(time.Duration(timeoutS+5) * time.Second)
	_ = c.conn.SetDeadline(deadline)

	if err := c.sendLocked(execMsg); err != nil {
		_ = c.closeLocked()
		return err
	}

	for {
		resp, err := c.receiveLocked()
		if err != nil {
			_ = c.closeLocked()
			return err
		}

		if resp.Type != backend.MsgTypeStream {
			_ = c.closeLocked()
			return fmt.Errorf("unexpected message type: %d (expected stream)", resp.Type)
		}

		var chunk backend.StreamChunkPayload
		if err := json.Unmarshal(resp.Payload, &chunk); err != nil {
			_ = c.closeLocked()
			return err
		}

		var chunkErr error
		if chunk.Error != "" {
			chunkErr = fmt.Errorf("%s", chunk.Error)
		}
		if err := callback(chunk.Data, chunk.IsLast, chunkErr); err != nil {
			_ = c.closeLocked()
			return err
		}

		if chunk.IsLast {
			break
		}
	}

	_ = c.conn.SetDeadline(time.Time{})
	_ = c.closeLocked()
	return nil
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
	addr := c.vm.GuestIP
	if addr == "" {
		addr = fmt.Sprintf("127.0.0.1:%d", c.vm.AssignedPort)
	}
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
