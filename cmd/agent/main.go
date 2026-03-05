package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oriys/nova/api/proto/agentpb"
	"github.com/oriys/nova/internal/pkg/codefile"
	"github.com/oriys/nova/internal/pkg/vsock"
	"google.golang.org/protobuf/proto"
)

const (
	MsgTypeInit   = 1
	MsgTypeExec   = 2
	MsgTypeResp   = 3
	MsgTypePing   = 4
	MsgTypeStop   = 5
	MsgTypeReload = 6 // Hot reload code files
	MsgTypeStream = 7 // Streaming response chunk

	// State operations: function ↔ host state proxy via vsock
	MsgTypeStateGet    = 8  // Get state key
	MsgTypeStatePut    = 9  // Put state key
	MsgTypeStateDelete = 10 // Delete state key
	MsgTypeStateList   = 11 // List state keys
	MsgTypeStateResp   = 12 // State operation response

	// Durable execution step operations
	MsgTypeDurableStep     = 13 // Register/complete a durable step
	MsgTypeDurableStepResp = 14 // Durable step response

	// Sandbox operations
	MsgTypeShellExec   = 20 // Execute shell command (single shot)
	MsgTypeShellStream = 21 // Open interactive shell session
	MsgTypeShellInput  = 22 // Write stdin to shell session
	MsgTypeShellResize = 23 // Terminal window resize
	MsgTypeFileRead    = 30 // Read file
	MsgTypeFileWrite   = 31 // Write file
	MsgTypeFileList    = 32 // List directory
	MsgTypeFileDelete  = 33 // Delete file
	MsgTypeFileResp    = 34 // File operation response
	MsgTypeProcessList = 40 // List processes
	MsgTypeProcessKill = 41 // Kill process
	MsgTypeProcessResp = 42 // Process operation response

	VsockPort = 9999

	defaultPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

	// internalInvokeEndpoint is the vsock endpoint exposed to user functions
	// for direct function-to-function calls. CID 2 is the host in Firecracker,
	// port 9090 is the Comet gRPC listener.
	internalInvokeEndpoint = "vsock://2:9090/invoke"

	// stateProxyPort is the local HTTP server for in-VM state access
	stateProxyPort = 8399

	pythonPath   = "/usr/bin/python3"
	wasmtimePath = "/usr/local/bin/wasmtime"
	nodePath     = "/usr/bin/node"
	rubyPath     = "/usr/bin/ruby"
	javaPath     = "/usr/bin/java"
	phpPath      = "/usr/bin/php"
	luaPath      = "/usr/bin/lua5.4"
	denoPath     = "/usr/local/bin/deno"
	bunPath      = "/usr/local/bin/bun"
	elixirPath   = "/usr/local/bin/elixir"
	perlPath     = "/usr/local/bin/perl"
	rscriptPath  = "/usr/bin/Rscript"
	juliaPath    = "/usr/local/bin/julia"

	maxPersistentResponseBytes = 4 * 1024 * 1024 // 4MB
)

var (
	CodeMountPoint = "/code"
	CodePath       = "/code/handler"
)

// ExecutionMode determines how functions are executed
type ExecutionMode string

const (
	// ModeProcess: Fork new process for each invocation (default, isolated)
	ModeProcess ExecutionMode = "process"
	// ModePersistent: Keep function process alive, send requests via stdin/stdout
	// Enables connection reuse for databases, etc.
	ModePersistent ExecutionMode = "persistent"
	// ModeDurable: Stateful execution with in-VM state access and checkpoint/replay
	ModeDurable ExecutionMode = "durable"
)

type Message struct {
	Type    int             `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type InitPayload struct {
	Runtime         string            `json:"runtime"`
	Handler         string            `json:"handler"`
	EnvVars         map[string]string `json:"env_vars"`
	Command         []string          `json:"command,omitempty"`
	Extension       string            `json:"extension,omitempty"`
	Mode            ExecutionMode     `json:"mode,omitempty"` // "process", "persistent", or "durable"
	FunctionName    string            `json:"function_name,omitempty"`
	FunctionID      string            `json:"function_id,omitempty"`
	FunctionVersion int               `json:"function_version,omitempty"`
	MemoryMB        int               `json:"memory_mb,omitempty"`
	TimeoutS        int               `json:"timeout_s,omitempty"`
	LayerCount      int               `json:"layer_count,omitempty"`
	VolumeMounts    []VolumeMountInfo `json:"volume_mounts,omitempty"`

	// InternalInvokeEnabled tells the agent to expose NOVA_INVOKE_ENDPOINT
	// so user functions can call other functions through the host.
	InternalInvokeEnabled bool `json:"internal_invoke_enabled,omitempty"`

	// StateEnabled tells the agent to start the local state proxy HTTP server.
	StateEnabled bool `json:"state_enabled,omitempty"`
}

// VolumeMountInfo tells the agent where to mount a volume drive inside the VM.
type VolumeMountInfo struct {
	MountPath string `json:"mount_path"` // guest mount point (e.g., /mnt/data)
	ReadOnly  bool   `json:"read_only"`
}

type ExecPayload struct {
	RequestID   string          `json:"request_id"`
	Input       json.RawMessage `json:"input"`
	TimeoutS    int             `json:"timeout_s"`
	TraceParent string          `json:"traceparent,omitempty"`
	TraceState  string          `json:"tracestate,omitempty"`
	Stream      bool            `json:"stream,omitempty"` // Enable streaming response

	// InternalInvoke enables the invoke capability for this execution.
	InternalInvoke bool `json:"internal_invoke,omitempty"`
}

type RespPayload struct {
	RequestID  string          `json:"request_id"`
	Output     json.RawMessage `json:"output"`
	Error      string          `json:"error,omitempty"`
	DurationMs int64           `json:"duration_ms"`
	Stdout     string          `json:"stdout,omitempty"`
	Stderr     string          `json:"stderr,omitempty"`
}

// ReloadPayload is sent to hot-reload function code
type ReloadPayload struct {
	Files map[string][]byte `json:"files"` // relative path -> content
}

// StreamChunkPayload is sent for streaming responses
type StreamChunkPayload struct {
	RequestID string `json:"request_id"`
	Data      []byte `json:"data"`            // Chunk of data
	IsLast    bool   `json:"is_last"`         // True if this is the final chunk
	Error     string `json:"error,omitempty"` // Error message if execution failed
}

// StateRequestPayload is sent from agent to host for state operations
type StateRequestPayload struct {
	Key             string          `json:"key"`
	Value           json.RawMessage `json:"value,omitempty"`
	Prefix          string          `json:"prefix,omitempty"`
	TTLSeconds      int             `json:"ttl_s,omitempty"`
	ExpectedVersion int64           `json:"expected_version,omitempty"`
	Limit           int             `json:"limit,omitempty"`
}

// StateResponsePayload is the host response to a state operation
type StateResponsePayload struct {
	Key     string          `json:"key,omitempty"`
	Value   json.RawMessage `json:"value,omitempty"`
	Version int64           `json:"version,omitempty"`
	Error   string          `json:"error,omitempty"`
	Entries []StateEntry    `json:"entries,omitempty"`
}

// StateEntry is a single state entry in list responses
type StateEntry struct {
	Key     string          `json:"key"`
	Value   json.RawMessage `json:"value"`
	Version int64           `json:"version"`
}

// DurableStepPayload is sent from agent to host for durable step operations
type DurableStepPayload struct {
	ExecutionID string          `json:"execution_id"`
	StepName    string          `json:"step_name"`
	Action      string          `json:"action"` // "start", "complete", "fail"
	StepID      string          `json:"step_id,omitempty"`
	Input       json.RawMessage `json:"input,omitempty"`
	Output      json.RawMessage `json:"output,omitempty"`
	Error       string          `json:"error,omitempty"`
	DurationMs  int64           `json:"duration_ms,omitempty"`
}

// DurableStepResponsePayload is the host response to a durable step operation
type DurableStepResponsePayload struct {
	StepID string          `json:"step_id,omitempty"`
	Cached bool            `json:"cached,omitempty"` // True if step was already completed (replay)
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type Agent struct {
	function         *InitPayload
	persistentProc   *exec.Cmd
	persistentIn     io.WriteCloser
	persistentOut    *bufio.Reader
	persistentOutRaw io.ReadCloser
	useProtobuf      bool     // true when this connection uses protobuf codec
	stateConn        net.Conn // vsock connection for state proxy requests to host
	stateConnMu      sync.Mutex
}

func main() {
	fmt.Println("[agent] Nova guest agent starting...")

	// Allow overriding code directory (used by WASM backend on host)
	if dir := os.Getenv("NOVA_CODE_DIR"); dir != "" {
		CodeMountPoint = dir
		CodePath = filepath.Join(dir, "handler")
		fmt.Printf("[agent] Code directory overridden to %s\n", dir)
	}

	ensurePath()
	mountCodeDrive()
	mountLayerDrives()
	mountLayerOverlay(countMountedLayers())

	listenPort := VsockPort
	if p := os.Getenv("NOVA_AGENT_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			listenPort = v
		}
	}

	listener, err := listen(listenPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[agent] Failed to listen: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("[agent] Listening on port %d\n", listenPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[agent] Accept error: %v\n", err)
			continue
		}
		go handleConnection(conn)
	}
}

// listen creates a listener based on environment and runtime:
// - NOVA_AGENT_MODE=tcp: TCP socket (for Docker containers)
// - Linux with vsock: AF_VSOCK (for Firecracker VMs)
// - Otherwise: Unix socket (for local dev)
func listen(port int) (net.Listener, error) {
	mode := os.Getenv("NOVA_AGENT_MODE")

	// TCP mode for Docker containers
	if mode == "tcp" {
		addr := fmt.Sprintf("0.0.0.0:%d", port)
		fmt.Printf("[agent] Using TCP: %s\n", addr)
		return net.Listen("tcp", addr)
	}

	if runtime.GOOS == "linux" {
		// Try vsock first (works inside Firecracker VM)
		l, err := vsock.Listen(uint32(port), nil)
		if err == nil {
			fmt.Println("[agent] Using AF_VSOCK")
			return l, nil
		}
		fmt.Fprintf(os.Stderr, "[agent] vsock unavailable: %v, falling back to unix socket\n", err)
	}

	// Fallback: Unix socket for local dev/testing
	sockPath := fmt.Sprintf("/tmp/nova-agent-%d.sock", port)
	os.Remove(sockPath)
	fmt.Printf("[agent] Using unix socket: %s\n", sockPath)
	return net.Listen("unix", sockPath)
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	agent := &Agent{}

	// Protocol detection: auto-detect JSON vs protobuf on the first frame.
	// Once detected, all subsequent messages on this connection use the same codec.
	useProtobuf := false
	protocolDetected := false

	for {
		// Read raw frame for protocol detection on first message
		data, err := readRawFrame(conn)
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "[agent] Read error: %v\n", err)
			}
			return
		}

		if !protocolDetected {
			useProtobuf = isProtobuf(data)
			protocolDetected = true
			agent.useProtobuf = useProtobuf
			if useProtobuf {
				fmt.Println("[agent] Detected protobuf protocol")
			}
		}

		var msg *Message
		if useProtobuf {
			pbMsg := &agentpb.VsockMessage{}
			if err := proto.Unmarshal(data, pbMsg); err != nil {
				fmt.Fprintf(os.Stderr, "[agent] Protobuf unmarshal error: %v\n", err)
				return
			}
			msg, err = pbToMessage(pbMsg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[agent] Protobuf conversion error: %v\n", err)
				return
			}
		} else {
			msg = &Message{}
			if err := json.Unmarshal(data, msg); err != nil {
				fmt.Fprintf(os.Stderr, "[agent] JSON unmarshal error: %v\n", err)
				return
			}
		}

		// Select the appropriate writer for responses
		writer := writeMessage
		if useProtobuf {
			writer = writeProtobufMessage
		}

		if msg.Type == MsgTypeStop {
			fmt.Println("[agent] Received stop, shutting down...")
			writer(conn, &Message{Type: MsgTypeResp, Payload: json.RawMessage(`{"status":"stopping"}`)})
			os.Exit(0)
		}

		resp, err := agent.handleMessage(conn, msg)
		if err != nil {
			resp = &Message{
				Type:    MsgTypeResp,
				Payload: json.RawMessage(fmt.Sprintf(`{"error":"%s"}`, err.Error())),
			}
		}

		// For streaming, response is already sent via chunks, no final response needed
		if msg.Type == MsgTypeExec {
			var execPayload ExecPayload
			if err := json.Unmarshal(msg.Payload, &execPayload); err == nil && execPayload.Stream {
				continue // Skip writing final response, already streamed
			}
		}

		if err := writer(conn, resp); err != nil {
			fmt.Fprintf(os.Stderr, "[agent] Write error: %v\n", err)
			return
		}
	}
}

func (a *Agent) handleMessage(conn net.Conn, msg *Message) (*Message, error) {
	switch msg.Type {
	case MsgTypeInit:
		return a.handleInit(msg.Payload)
	case MsgTypeExec:
		return a.handleExec(conn, msg.Payload)
	case MsgTypePing:
		return &Message{Type: MsgTypeResp, Payload: json.RawMessage(`{"status":"ok"}`)}, nil
	case MsgTypeReload:
		return a.handleReload(msg.Payload)
	// Sandbox operations
	case MsgTypeShellExec:
		return a.handleShellExec(msg.Payload)
	case MsgTypeFileRead:
		return a.handleFileRead(msg.Payload)
	case MsgTypeFileWrite:
		return a.handleFileWrite(msg.Payload)
	case MsgTypeFileList:
		return a.handleFileList(msg.Payload)
	case MsgTypeFileDelete:
		return a.handleFileDelete(msg.Payload)
	case MsgTypeProcessList:
		return a.handleProcessList()
	case MsgTypeProcessKill:
		return a.handleProcessKill(msg.Payload)
	default:
		return nil, fmt.Errorf("unknown message type: %d", msg.Type)
	}
}

func (a *Agent) handleInit(payload json.RawMessage) (*Message, error) {
	var init InitPayload
	if err := json.Unmarshal(payload, &init); err != nil {
		return nil, err
	}

	// Normalize versioned runtime IDs (e.g., python3.12, node24, php8.4)
	// for legacy hardcoded paths only. Dynamic command mode bypasses this.
	if len(init.Command) == 0 {
		init.Runtime = normalizeRuntime(init.Runtime)
	}

	// Default to process mode
	if init.Mode == "" {
		init.Mode = ModeProcess
	}

	a.function = &init
	fmt.Printf("[agent] Init: runtime=%s handler=%s mode=%s\n", init.Runtime, init.Handler, init.Mode)

	// Mount volume drives if configured
	if len(init.VolumeMounts) > 0 {
		mountVolumeDrives(init.LayerCount, init.VolumeMounts)
	}

	// Write bootstrap script for interpreted runtimes
	if err := a.writeBootstrap(); err != nil {
		fmt.Fprintf(os.Stderr, "[agent] Warning: failed to write bootstrap: %v\n", err)
	}

	// For persistent mode, start the process now
	if init.Mode == ModePersistent {
		if err := a.startPersistentProcess(); err != nil {
			return nil, fmt.Errorf("start persistent process: %w", err)
		}
	}

	// For durable mode, treat as persistent + start state proxy
	if init.Mode == ModeDurable {
		init.StateEnabled = true
		if err := a.startPersistentProcess(); err != nil {
			return nil, fmt.Errorf("start persistent process: %w", err)
		}
	}

	// Start state proxy HTTP server if state is enabled
	if init.StateEnabled {
		go a.startStateProxy()
	}

	return &Message{
		Type:    MsgTypeResp,
		Payload: json.RawMessage(`{"status":"initialized"}`),
	}, nil
}

// writeBootstrap writes the runtime-appropriate bootstrap script to /tmp
func (a *Agent) writeBootstrap() error {
	ext := bootstrapExtension(a.function.Runtime)
	if ext == "" {
		return nil // compiled runtimes don't need a bootstrap
	}
	content := bootstrapContent(a.function.Runtime)
	if content == "" {
		return nil
	}
	path := "/tmp/_bootstrap" + ext
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return err
	}
	fmt.Printf("[agent] Wrote bootstrap: %s\n", path)
	return nil
}

// startPersistentProcess starts a long-running function process for connection reuse
func (a *Agent) startPersistentProcess() error {
	var cmd *exec.Cmd
	switch a.function.Runtime {
	case "python":
		cmd = exec.Command(resolveBinary(pythonPath, "python3"), "/tmp/_bootstrap.py", "--persistent")
	case "node":
		cmd = exec.Command(resolveBinary(nodePath, "node"), "/tmp/_bootstrap.js", "--persistent")
	case "ruby":
		cmd = exec.Command(resolveBinary(rubyPath, "ruby"), "/tmp/_bootstrap.rb", "--persistent")
	case "php":
		cmd = exec.Command(resolveBinary(phpPath, "php"), "/tmp/_bootstrap.php", "--persistent")
	case "deno":
		cmd = exec.Command(resolveBinary(denoPath, "deno"), "run", "--allow-read", "--allow-env", "/tmp/_bootstrap.ts", "--persistent")
	case "bun":
		cmd = exec.Command(resolveBinary(bunPath, "bun"), "run", "/tmp/_bootstrap.js", "--persistent")
	case "lua":
		cmd = exec.Command(resolveBinary(luaPath, "lua"), "/tmp/_bootstrap.lua", "--persistent")
	case "elixir":
		cmd = exec.Command(resolveBinary(elixirPath, "elixir"), "/tmp/_bootstrap.exs", "--persistent")
	case "perl":
		cmd = exec.Command(resolveBinary(perlPath, "perl"), "/tmp/_bootstrap.pl", "--persistent")
	case "r":
		cmd = exec.Command(resolveBinary(rscriptPath, "Rscript"), "/tmp/_bootstrap.R", "--persistent")
	case "julia":
		cmd = exec.Command(resolveBinary(juliaPath, "julia"), "/tmp/_bootstrap.jl", "--persistent")
	case "go", "rust", "zig", "swift", "c", "cpp", "graalvm":
		cmd = exec.Command(CodePath, "--persistent")
	case "wasm":
		cmd = exec.Command(resolveBinary(wasmtimePath, "wasmtime"), CodePath, "--", "--persistent")
	default:
		return fmt.Errorf("persistent mode not supported for runtime: %s", a.function.Runtime)
	}

	cmd.Env = append(defaultEnv(), "NOVA_MODE=persistent", "NOVA_CODE_DIR="+CodeMountPoint)
	// Inject context env vars for persistent bootstraps
	cmd.Env = append(cmd.Env,
		"NOVA_FUNCTION_NAME="+a.function.FunctionName,
		"NOVA_FUNCTION_ID="+a.function.FunctionID,
		fmt.Sprintf("NOVA_FUNCTION_VERSION=%d", a.function.FunctionVersion),
		fmt.Sprintf("NOVA_MEMORY_LIMIT_MB=%d", a.function.MemoryMB),
		fmt.Sprintf("NOVA_TIMEOUT_S=%d", a.function.TimeoutS),
		"NOVA_RUNTIME="+a.function.Runtime,
	)
	if a.function.StateEnabled {
		cmd.Env = append(cmd.Env, "NOVA_STATE_URL=http://127.0.0.1:8399")
	}
	if a.function.Runtime == "swift" {
		cmd.Env = append(cmd.Env, "SWIFT_ROOT=/usr")
	}
	// Add dependency paths based on runtime
	cmd.Env = appendDependencyEnv(cmd.Env, a.function.Runtime)
	for k, v := range a.function.EnvVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	a.persistentProc = cmd
	a.persistentIn = stdin
	a.persistentOutRaw = stdout
	a.persistentOut = bufio.NewReader(stdout)

	fmt.Printf("[agent] Persistent process started (pid=%d)\n", cmd.Process.Pid)
	return nil
}

// handleReload replaces the code files in /code without destroying the VM
func (a *Agent) handleReload(payload json.RawMessage) (*Message, error) {
	var req ReloadPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("unmarshal reload payload: %w", err)
	}

	if len(req.Files) == 0 {
		return nil, fmt.Errorf("no files to reload")
	}

	fmt.Printf("[agent] Reloading %d files\n", len(req.Files))

	// 1. Stop persistent process if running
	if a.persistentProc != nil && a.persistentProc.Process != nil {
		fmt.Println("[agent] Stopping persistent process for reload")
		a.persistentProc.Process.Kill()
		a.persistentProc.Wait()
		a.persistentProc = nil
		a.persistentIn = nil
		a.persistentOut = nil
		a.persistentOutRaw = nil
	}

	// 2. Remount code drive as read-write (it's mounted read-only at boot)
	if err := remountCodeDriveRW(); err != nil {
		return nil, fmt.Errorf("remount code drive rw: %w", err)
	}
	defer remountCodeDriveRO()

	// 3. Clear /code directory contents
	entries, err := os.ReadDir(CodeMountPoint)
	if err == nil {
		for _, entry := range entries {
			os.RemoveAll(filepath.Join(CodeMountPoint, entry.Name()))
		}
	}

	// 3b. Clean /tmp to prevent cross-function data leakage when reusing
	// template VMs. Bootstrap scripts and other transient files are regenerated
	// during the subsequent Init call.
	if tmpEntries, err := os.ReadDir("/tmp"); err == nil {
		for _, entry := range tmpEntries {
			os.RemoveAll(filepath.Join("/tmp", entry.Name()))
		}
	}

	// 4. Write new files
	for name, content := range req.Files {
		// Validate path to prevent traversal attacks
		if filepath.IsAbs(name) || strings.Contains(filepath.Clean(name), "..") {
			return nil, fmt.Errorf("unsafe file path: %s", name)
		}
		path := filepath.Join(CodeMountPoint, filepath.Clean(name))
		// Create parent directories
		if dir := filepath.Dir(path); dir != CodeMountPoint {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("create dir for %s: %w", name, err)
			}
		}
		mode := os.FileMode(0644)
		if codefile.ShouldBeExecutable(name, content) {
			mode = 0755
		}
		if err := os.WriteFile(path, content, mode); err != nil {
			return nil, fmt.Errorf("write file %s: %w", name, err)
		}
		fmt.Printf("[agent] Wrote file: %s (%d bytes)\n", name, len(content))
	}

	// 5. Restart persistent process if in persistent mode
	if a.function != nil && a.function.Mode == ModePersistent {
		fmt.Println("[agent] Restarting persistent process after reload")
		if err := a.startPersistentProcess(); err != nil {
			return nil, fmt.Errorf("restart persistent process: %w", err)
		}
	}

	return &Message{
		Type:    MsgTypeResp,
		Payload: json.RawMessage(`{"status":"reloaded"}`),
	}, nil
}

func (a *Agent) handleExec(conn net.Conn, payload json.RawMessage) (*Message, error) {
	if a.function == nil {
		return nil, fmt.Errorf("function not initialized")
	}

	var req ExecPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	// Log trace context if available
	if req.TraceParent != "" {
		fmt.Printf("[agent] exec request_id=%s trace_parent=%s stream=%v\n", req.RequestID, req.TraceParent, req.Stream)
	} else {
		fmt.Printf("[agent] exec request_id=%s stream=%v\n", req.RequestID, req.Stream)
	}

	// Handle streaming mode
	if req.Stream {
		return a.handleStreamingExec(conn, &req)
	}

	// Normal non-streaming mode
	start := time.Now()
	output, stdout, stderr, execErr := a.executeFunction(req.Input, req.TimeoutS, req.RequestID, req.TraceParent, req.TraceState, req.InternalInvoke)
	duration := time.Since(start).Milliseconds()

	// Log execution result
	if execErr != nil {
		fmt.Printf("[agent] exec completed request_id=%s duration_ms=%d error=%s\n", req.RequestID, duration, execErr.Error())
	} else {
		fmt.Printf("[agent] exec completed request_id=%s duration_ms=%d\n", req.RequestID, duration)
	}

	resp := RespPayload{
		RequestID:  req.RequestID,
		DurationMs: duration,
		Stdout:     stdout,
		Stderr:     stderr,
	}
	if execErr != nil {
		resp.Error = execErr.Error()
	} else {
		resp.Output = output
	}

	respData, _ := json.Marshal(resp)
	return &Message{Type: MsgTypeResp, Payload: respData}, nil
}

func (a *Agent) executeFunction(input json.RawMessage, timeoutS int, requestID string, traceParent string, traceState string, internalInvoke bool) (json.RawMessage, string, string, error) {
	// Use persistent mode if available
	if a.function.Mode == ModePersistent && a.persistentProc != nil {
		output, err := a.executePersistent(input, timeoutS)
		return output, "", "", err
	}

	// Process mode: fork new process each time
	if err := os.WriteFile("/tmp/input.json", input, 0644); err != nil {
		return nil, "", "", err
	}

	var cmd *exec.Cmd
	var cancel context.CancelFunc
	ctx := context.Background()
	if timeoutS > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutS)*time.Second)
		defer cancel()
	}
	if len(a.function.Command) > 0 {
		args := append([]string(nil), a.function.Command...)
		if len(args) == 0 {
			return nil, "", "", fmt.Errorf("invalid command: empty")
		}
		// Validate the executable against allowed runtime binaries
		// To prevent path traversal (e.g. /tmp/malicious/python3), we ensure that:
		// 1. If it's a known runtime binary, it must be specified by name only (no path components), OR
		// 2. It must be inside the allowed code mount point.
		exe := filepath.Base(args[0])
		allowed := map[string]bool{
			"python3": true, "python": true, "node": true, "ruby": true,
			"php": true, "deno": true, "bun": true, "lua": true,
			"java": true, "wasmtime": true, "dotnet": true, "swift": true,
			"perl": true, "r": true, "Rscript": true, "julia": true,
			"elixir": true, "handler": true, "bootstrap": true,
		}

		isAllowedBinary := allowed[exe]
		isSimpleName := args[0] == exe
		isInCodeDir := strings.HasPrefix(args[0], CodeMountPoint)

		if !isInCodeDir {
			if !isAllowedBinary || !isSimpleName {
				return nil, "", "", fmt.Errorf("command not allowed: %s (must be in %s or be a simple system binary name)", args[0], CodeMountPoint)
			}
		}
		if a.function.Extension != "" {
			args = append(args, CodePath+a.function.Extension)
		} else {
			args = append(args, CodePath)
		}
		args = append(args, "/tmp/input.json")
		cmd = exec.CommandContext(ctx, args[0], args[1:]...)
	} else {
		switch a.function.Runtime {
		case "python":
			cmd = exec.CommandContext(ctx, resolveBinary(pythonPath, "python3"), "/tmp/_bootstrap.py", "/tmp/input.json")
		case "node":
			cmd = exec.CommandContext(ctx, resolveBinary(nodePath, "node"), "/tmp/_bootstrap.js", "/tmp/input.json")
		case "ruby":
			cmd = exec.CommandContext(ctx, resolveBinary(rubyPath, "ruby"), "/tmp/_bootstrap.rb", "/tmp/input.json")
		case "php":
			cmd = exec.CommandContext(ctx, resolveBinary(phpPath, "php"), "/tmp/_bootstrap.php", "/tmp/input.json")
		case "deno":
			cmd = exec.CommandContext(ctx, resolveBinary(denoPath, "deno"), "run", "--allow-read", "--allow-env", "/tmp/_bootstrap.ts", "/tmp/input.json")
		case "bun":
			cmd = exec.CommandContext(ctx, resolveBinary(bunPath, "bun"), "run", "/tmp/_bootstrap.js", "/tmp/input.json")
		case "lua":
			cmd = exec.CommandContext(ctx, resolveBinary(luaPath, "lua"), "/tmp/_bootstrap.lua", "/tmp/input.json")
		case "elixir":
			cmd = exec.CommandContext(ctx, resolveBinary(elixirPath, "elixir"), "/tmp/_bootstrap.exs", "/tmp/input.json")
		case "perl":
			cmd = exec.CommandContext(ctx, resolveBinary(perlPath, "perl"), "/tmp/_bootstrap.pl", "/tmp/input.json")
		case "r":
			cmd = exec.CommandContext(ctx, resolveBinary(rscriptPath, "Rscript"), "/tmp/_bootstrap.R", "/tmp/input.json")
		case "julia":
			cmd = exec.CommandContext(ctx, resolveBinary(juliaPath, "julia"), "/tmp/_bootstrap.jl", "/tmp/input.json")
		case "go", "rust", "zig", "swift", "c", "cpp", "graalvm":
			cmd = exec.CommandContext(ctx, CodePath, "/tmp/input.json")
		case "wasm":
			cmd = exec.CommandContext(ctx, resolveBinary(wasmtimePath, "wasmtime"), CodePath, "--", "/tmp/input.json")
		case "java", "kotlin", "scala":
			cmd = exec.CommandContext(ctx, resolveBinary(javaPath, "java"), "-jar", CodePath, "/tmp/input.json")
		case "custom", "provided":
			bootstrapPath := filepath.Join(CodeMountPoint, "bootstrap")
			cmd = exec.CommandContext(ctx, bootstrapPath, "/tmp/input.json")
		default:
			return nil, "", "", fmt.Errorf("unsupported runtime: %s", a.function.Runtime)
		}
	}

	cmd.Env = append(defaultEnv(), "NOVA_CODE_DIR="+CodeMountPoint)
	// Inject context env vars
	cmd.Env = append(cmd.Env,
		"NOVA_REQUEST_ID="+requestID,
		"NOVA_FUNCTION_NAME="+a.function.FunctionName,
		"NOVA_FUNCTION_ID="+a.function.FunctionID,
		fmt.Sprintf("NOVA_FUNCTION_VERSION=%d", a.function.FunctionVersion),
		fmt.Sprintf("NOVA_MEMORY_LIMIT_MB=%d", a.function.MemoryMB),
		fmt.Sprintf("NOVA_TIMEOUT_S=%d", a.function.TimeoutS),
		"NOVA_RUNTIME="+a.function.Runtime,
	)
	if a.function.StateEnabled {
		cmd.Env = append(cmd.Env, "NOVA_STATE_URL=http://127.0.0.1:8399")
	}
	if a.function.Runtime == "swift" {
		cmd.Env = append(cmd.Env, "SWIFT_ROOT=/usr")
	}
	// Inject W3C trace context for automatic propagation (Feature 5)
	if traceParent != "" {
		cmd.Env = append(cmd.Env, "TRACEPARENT="+traceParent)
	}
	if traceState != "" {
		cmd.Env = append(cmd.Env, "TRACESTATE="+traceState)
	}
	// Expose internal invoke endpoint for function-to-function calls (Feature 4)
	if internalInvoke || a.function.InternalInvokeEnabled {
		cmd.Env = append(cmd.Env, "NOVA_INVOKE_ENDPOINT="+internalInvokeEndpoint)
	}
	// Add dependency paths based on runtime
	cmd.Env = appendDependencyEnv(cmd.Env, a.function.Runtime)
	for k, v := range a.function.EnvVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Capture stdout and stderr separately
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, stdout, stderr, fmt.Errorf("timeout after %ds", timeoutS)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, stdout, stderr, fmt.Errorf("exit %d: %s", exitErr.ExitCode(), stderr)
		}
		return nil, stdout, stderr, enrichExecError(err, a.function.Runtime)
	}

	output := stdoutBuf.Bytes()
	if json.Valid(output) {
		return output, "", stderr, nil
	}
	result, _ := json.Marshal(string(output))
	return result, "", stderr, nil
}

// handleStreamingExec executes function in streaming mode, sending chunks via MsgTypeStream
func (a *Agent) handleStreamingExec(conn net.Conn, req *ExecPayload) (*Message, error) {
	start := time.Now()

	// Select writer based on connection protocol
	writer := writeMessage
	if a.useProtobuf {
		writer = writeProtobufMessage
	}

	// Helper to send stream chunk
	sendChunk := func(data []byte, isLast bool, errMsg string) error {
		chunk := StreamChunkPayload{
			RequestID: req.RequestID,
			Data:      data,
			IsLast:    isLast,
			Error:     errMsg,
		}
		chunkData, _ := json.Marshal(chunk)
		msg := &Message{Type: MsgTypeStream, Payload: chunkData}
		return writer(conn, msg)
	}

	// Write input to file
	if err := os.WriteFile("/tmp/input.json", req.Input, 0644); err != nil {
		sendChunk(nil, true, err.Error())
		return nil, err
	}

	// Build command
	var cmd *exec.Cmd
	var cancel context.CancelFunc
	ctx := context.Background()
	if req.TimeoutS > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutS)*time.Second)
		defer cancel()
	}

	if len(a.function.Command) > 0 {
		args := append([]string(nil), a.function.Command...)
		if a.function.Extension != "" {
			args = append(args, CodePath+a.function.Extension)
		} else {
			args = append(args, CodePath)
		}
		args = append(args, "/tmp/input.json")
		cmd = exec.CommandContext(ctx, args[0], args[1:]...)
	} else {
		switch a.function.Runtime {
		case "python":
			cmd = exec.CommandContext(ctx, resolveBinary(pythonPath, "python3"), "/tmp/_bootstrap.py", "/tmp/input.json")
		case "node":
			cmd = exec.CommandContext(ctx, resolveBinary(nodePath, "node"), "/tmp/_bootstrap.js", "/tmp/input.json")
		case "ruby":
			cmd = exec.CommandContext(ctx, resolveBinary(rubyPath, "ruby"), "/tmp/_bootstrap.rb", "/tmp/input.json")
		case "php":
			cmd = exec.CommandContext(ctx, resolveBinary(phpPath, "php"), "/tmp/_bootstrap.php", "/tmp/input.json")
		case "deno":
			cmd = exec.CommandContext(ctx, resolveBinary(denoPath, "deno"), "run", "--allow-read", "--allow-env", "/tmp/_bootstrap.ts", "/tmp/input.json")
		case "bun":
			cmd = exec.CommandContext(ctx, resolveBinary(bunPath, "bun"), "run", "/tmp/_bootstrap.js", "/tmp/input.json")
		case "lua":
			cmd = exec.CommandContext(ctx, resolveBinary(luaPath, "lua"), "/tmp/_bootstrap.lua", "/tmp/input.json")
		case "elixir":
			cmd = exec.CommandContext(ctx, resolveBinary(elixirPath, "elixir"), "/tmp/_bootstrap.exs", "/tmp/input.json")
		case "perl":
			cmd = exec.CommandContext(ctx, resolveBinary(perlPath, "perl"), "/tmp/_bootstrap.pl", "/tmp/input.json")
		case "r":
			cmd = exec.CommandContext(ctx, resolveBinary(rscriptPath, "Rscript"), "/tmp/_bootstrap.R", "/tmp/input.json")
		case "julia":
			cmd = exec.CommandContext(ctx, resolveBinary(juliaPath, "julia"), "/tmp/_bootstrap.jl", "/tmp/input.json")
		case "go", "rust", "zig", "swift", "c", "cpp", "graalvm":
			cmd = exec.CommandContext(ctx, CodePath, "/tmp/input.json")
		case "wasm":
			cmd = exec.CommandContext(ctx, resolveBinary(wasmtimePath, "wasmtime"), CodePath, "--", "/tmp/input.json")
		case "java", "kotlin", "scala":
			cmd = exec.CommandContext(ctx, resolveBinary(javaPath, "java"), "-jar", CodePath, "/tmp/input.json")
		default:
			errMsg := fmt.Sprintf("streaming not supported for runtime: %s", a.function.Runtime)
			sendChunk(nil, true, errMsg)
			return nil, errors.New(errMsg)
		}
	}

	// Setup environment
	cmd.Env = append(defaultEnv(), "NOVA_CODE_DIR="+CodeMountPoint, "NOVA_STREAMING=true")
	cmd.Env = append(cmd.Env,
		"NOVA_REQUEST_ID="+req.RequestID,
		"NOVA_FUNCTION_NAME="+a.function.FunctionName,
		"NOVA_FUNCTION_ID="+a.function.FunctionID,
		fmt.Sprintf("NOVA_FUNCTION_VERSION=%d", a.function.FunctionVersion),
		fmt.Sprintf("NOVA_MEMORY_LIMIT_MB=%d", a.function.MemoryMB),
		fmt.Sprintf("NOVA_TIMEOUT_S=%d", a.function.TimeoutS),
		"NOVA_RUNTIME="+a.function.Runtime,
	)
	if a.function.StateEnabled {
		cmd.Env = append(cmd.Env, "NOVA_STATE_URL=http://127.0.0.1:8399")
	}
	if a.function.Runtime == "swift" {
		cmd.Env = append(cmd.Env, "SWIFT_ROOT=/usr")
	}
	// Inject W3C trace context for automatic propagation
	if req.TraceParent != "" {
		cmd.Env = append(cmd.Env, "TRACEPARENT="+req.TraceParent)
	}
	if req.TraceState != "" {
		cmd.Env = append(cmd.Env, "TRACESTATE="+req.TraceState)
	}
	// Expose internal invoke endpoint for function-to-function calls
	if req.InternalInvoke || a.function.InternalInvokeEnabled {
		cmd.Env = append(cmd.Env, "NOVA_INVOKE_ENDPOINT="+internalInvokeEndpoint)
	}
	cmd.Env = appendDependencyEnv(cmd.Env, a.function.Runtime)
	for k, v := range a.function.EnvVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Get stdout pipe for streaming
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		sendChunk(nil, true, err.Error())
		return nil, err
	}
	cmd.Stderr = os.Stderr

	// Start process
	if err := cmd.Start(); err != nil {
		sendChunk(nil, true, err.Error())
		return nil, err
	}

	// Stream stdout in chunks (4KB buffer)
	buf := make([]byte, 4096)
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if sendErr := sendChunk(chunk, false, ""); sendErr != nil {
				fmt.Fprintf(os.Stderr, "[agent] Failed to send chunk: %v\n", sendErr)
				cmd.Process.Kill()
				return nil, sendErr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			sendChunk(nil, true, err.Error())
			return nil, err
		}
	}

	// Wait for process completion
	execErr := cmd.Wait()
	duration := time.Since(start).Milliseconds()

	// Send final chunk
	if execErr != nil {
		errMsg := execErr.Error()
		if ctx.Err() == context.DeadlineExceeded {
			errMsg = fmt.Sprintf("timeout after %ds", req.TimeoutS)
		}
		sendChunk(nil, true, errMsg)
		fmt.Printf("[agent] streaming exec completed request_id=%s duration_ms=%d error=%s\n", req.RequestID, duration, errMsg)
	} else {
		sendChunk(nil, true, "")
		fmt.Printf("[agent] streaming exec completed request_id=%s duration_ms=%d\n", req.RequestID, duration)
	}

	return nil, nil
}

func enrichExecError(err error, runtime string) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && errors.Is(pathErr.Err, syscall.ENOEXEC) {
		if runtime == "go" || runtime == "rust" || runtime == "zig" || runtime == "swift" || runtime == "graalvm" {
			return fmt.Errorf("%w (exec format error: /code/handler must be a Linux executable for the VM architecture, e.g. x86_64-unknown-linux-musl for amd64 or aarch64-unknown-linux-musl for arm64)", err)
		}
		return fmt.Errorf("%w (exec format error: check shebang and executable format of /code/handler)", err)
	}
	if errors.As(err, &pathErr) && errors.Is(pathErr.Err, syscall.ENOENT) && pathErr.Path == CodePath {
		if _, statErr := os.Stat(CodePath); statErr == nil {
			if runtime == "go" || runtime == "rust" || runtime == "zig" || runtime == "swift" || runtime == "graalvm" {
				return fmt.Errorf("%w (/code/handler exists but required dynamic loader is missing; build a static Linux binary, e.g. Rust target x86_64-unknown-linux-musl or aarch64-unknown-linux-musl)", err)
			}
			return fmt.Errorf("%w (/code/handler exists but its interpreter is missing; check shebang/interpreter path)", err)
		}
		return fmt.Errorf("%w (/code/handler not found in code package; ensure entry file is named 'handler' or provide a handler alias)", err)
	}
	return err
}

// executePersistent sends request to long-running process via stdin/stdout
func (a *Agent) executePersistent(input json.RawMessage, timeoutS int) (json.RawMessage, error) {
	// Protocol: write JSON line to stdin, read JSON line from stdout
	// Format: {"input": ..., "context": {...}}\n -> {"output": ...}\n

	req := map[string]interface{}{
		"input": json.RawMessage(input),
		"context": map[string]interface{}{
			"request_id":       os.Getenv("NOVA_REQUEST_ID"),
			"function_name":    a.function.FunctionName,
			"function_version": a.function.FunctionVersion,
			"memory_limit_mb":  a.function.MemoryMB,
			"timeout_s":        a.function.TimeoutS,
			"runtime":          a.function.Runtime,
		},
	}
	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')

	if _, err := a.persistentIn.Write(reqBytes); err != nil {
		// Process may have died, try to restart
		a.stopPersistentProcess()
		if err := a.startPersistentProcess(); err != nil {
			return nil, fmt.Errorf("restart persistent process: %w", err)
		}
		if _, err := a.persistentIn.Write(reqBytes); err != nil {
			return nil, fmt.Errorf("write to persistent process: %w", err)
		}
	}

	// Read response line with timeout/size enforcement
	lineCh := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := readLineWithLimit(a.persistentOut, maxPersistentResponseBytes)
		if err != nil {
			errCh <- err
			return
		}
		lineCh <- line
	}()

	if timeoutS > 0 {
		select {
		case line := <-lineCh:
			return parsePersistentResponse(line)
		case err := <-errCh:
			return nil, fmt.Errorf("read from persistent process: %w", err)
		case <-time.After(time.Duration(timeoutS) * time.Second):
			a.stopPersistentProcess()
			return nil, fmt.Errorf("timeout after %ds", timeoutS)
		}
	}

	select {
	case line := <-lineCh:
		return parsePersistentResponse(line)
	case err := <-errCh:
		return nil, fmt.Errorf("read from persistent process: %w", err)
	}

}

func parsePersistentResponse(line []byte) (json.RawMessage, error) {
	var resp struct {
		Output json.RawMessage `json:"output"`
		Error  string          `json:"error,omitempty"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("parse persistent response: %w", err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return resp.Output, nil
}

func readLineWithLimit(r *bufio.Reader, limit int) ([]byte, error) {
	var out []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(out)+len(chunk) > limit {
			return nil, fmt.Errorf("response too large (limit %d bytes)", limit)
		}
		out = append(out, chunk...)
		if err == nil {
			return out, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return nil, err
	}
}

func defaultEnv() []string {
	env := os.Environ()
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			if len(kv) > len("PATH=") {
				return env
			}
			env[i] = "PATH=" + defaultPath
			return env
		}
	}
	return append(env, "PATH="+defaultPath)
}

// appendDependencyEnv adds runtime-specific environment variables for dependency paths
func appendDependencyEnv(env []string, runtime string) []string {
	// Collect mounted layer paths
	var layerPaths []string
	for i := 0; i < 6; i++ {
		mountPoint := fmt.Sprintf("/layers/%d", i)
		if _, err := os.Stat(mountPoint); err != nil {
			break
		}
		layerPaths = append(layerPaths, mountPoint)
	}

	switch runtime {
	case "python":
		paths := []string{"/code/deps", "/code"}
		for _, lp := range layerPaths {
			paths = append(paths, lp+"/lib/python3/site-packages")
		}
		// Add merged overlay if available
		if _, err := os.Stat("/layers/merged"); err == nil {
			paths = append(paths, "/layers/merged/lib/python3/site-packages")
		}
		env = append(env, "PYTHONPATH="+strings.Join(paths, ":"))
	case "node":
		paths := []string{"/code/node_modules"}
		for _, lp := range layerPaths {
			paths = append(paths, lp+"/node_modules")
		}
		// Add merged overlay if available
		if _, err := os.Stat("/layers/merged"); err == nil {
			paths = append(paths, "/layers/merged/node_modules")
		}
		env = append(env, "NODE_PATH="+strings.Join(paths, ":"))
	case "ruby":
		paths := []string{}
		for _, lp := range layerPaths {
			paths = append(paths, lp+"/lib/ruby")
		}
		// Add merged overlay if available
		if _, err := os.Stat("/layers/merged"); err == nil {
			paths = append(paths, "/layers/merged/lib/ruby")
		}
		if len(paths) > 0 {
			env = append(env, "RUBYLIB="+strings.Join(paths, ":"))
		}
		// Support bundler-installed gems in /code/vendor/bundle
		for _, vendorBase := range []string{"/code/vendor/bundle", "/code/vendor"} {
			matches, _ := filepath.Glob(vendorBase + "/ruby/*/gems")
			if len(matches) > 0 {
				gemDir := filepath.Dir(matches[0])
				env = append(env, "GEM_PATH="+gemDir)
				env = append(env, "GEM_HOME="+gemDir)
				env = append(env, "BUNDLE_PATH="+vendorBase)
				env = append(env, "BUNDLE_DISABLE_SHARED_GEMS=1")
				break
			}
		}
	case "deno":
		env = append(env, "DENO_DIR=/code/.deno")
		// libresolv_stub.so is only present in VM rootfs images; skip in Docker mode.
		if _, err := os.Stat("/lib/libresolv_stub.so"); err == nil {
			env = append(env, "LD_PRELOAD=/lib/libresolv_stub.so")
		}
	case "bun":
		paths := []string{"/code/node_modules"}
		for _, lp := range layerPaths {
			paths = append(paths, lp+"/node_modules")
		}
		// Add merged overlay if available
		if _, err := os.Stat("/layers/merged"); err == nil {
			paths = append(paths, "/layers/merged/node_modules")
		}
		env = append(env, "NODE_PATH="+strings.Join(paths, ":"))
		// Bun needs a writable cache directory
		env = append(env, "BUN_INSTALL_CACHE_DIR=/tmp/.bun-cache")
	}

	// Add layer paths to generic PATH for compiled binaries
	if len(layerPaths) > 0 {
		for _, kv := range env {
			if strings.HasPrefix(kv, "PATH=") {
				// Already has PATH, append layer paths
				return env
			}
		}
	}

	// Add merged overlay bin to PATH if available
	if _, err := os.Stat("/layers/merged/bin"); err == nil {
		for i, kv := range env {
			if strings.HasPrefix(kv, "PATH=") {
				env[i] = kv + ":/layers/merged/bin"
				return env
			}
		}
	}

	return env
}

func countMountedLayers() int {
	count := 0
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(fmt.Sprintf("/layers/%d", i)); err != nil {
			break
		}
		count++
	}
	return count
}

func ensurePath() {
	if os.Getenv("PATH") == "" {
		_ = os.Setenv("PATH", defaultPath)
	}
}

func resolveBinary(preferredPath, name string) string {
	if preferredPath != "" {
		if fi, err := os.Stat(preferredPath); err == nil && !fi.IsDir() {
			return preferredPath
		}
	}
	// Check well-known extra locations (e.g. homebrew, user-local installs)
	home, _ := os.UserHomeDir()
	extras := []string{
		"/opt/homebrew/bin/" + name,
		"/usr/local/bin/" + name,
	}
	if home != "" {
		extras = append(extras, home+"/.bun/bin/"+name, home+"/.deno/bin/"+name)
	}
	for _, p := range extras {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return name
}

func normalizeRuntime(rt string) string {
	rt = strings.TrimSpace(rt)
	switch {
	case strings.HasPrefix(rt, "python"):
		return "python"
	case strings.HasPrefix(rt, "node"):
		return "node"
	case strings.HasPrefix(rt, "ruby"):
		return "ruby"
	case strings.HasPrefix(rt, "java"):
		return "java"
	case strings.HasPrefix(rt, "php"):
		return "php"
	case strings.HasPrefix(rt, "go"):
		return "go"
	case strings.HasPrefix(rt, "rust"):
		return "rust"
	case strings.HasPrefix(rt, "swift"):
		return "swift"
	case strings.HasPrefix(rt, "zig"):
		return "zig"
	case strings.HasPrefix(rt, "graalvm"):
		return "graalvm"
	case strings.HasPrefix(rt, "elixir"):
		return "elixir"
	case strings.HasPrefix(rt, "perl"):
		return "perl"
	case strings.HasPrefix(rt, "julia"):
		return "julia"
	case rt == "r":
		return "r"
	default:
		return rt
	}
}

func (a *Agent) stopPersistentProcess() {
	if a.persistentProc != nil {
		a.persistentIn.Close()
		a.persistentOutRaw.Close()
		a.persistentProc.Process.Kill()
		a.persistentProc.Wait()
		a.persistentProc = nil
		a.persistentIn = nil
		a.persistentOut = nil
		a.persistentOutRaw = nil
	}
}

func readMessage(conn net.Conn) (*Message, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(lenBuf)
	// Guard against oversized messages that could cause OOM
	const maxMessageSize = 16 * 1024 * 1024 // 16MB
	if size > maxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", size, maxMessageSize)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}
	var msg Message
	return &msg, json.Unmarshal(data, &msg)
}

func writeMessage(conn net.Conn, msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	if _, err := conn.Write(lenBuf); err != nil {
		return err
	}
	_, err = conn.Write(data)
	return err
}

// readRawFrame reads a length-prefixed frame from the connection without
// decoding the payload. Returns the raw bytes for protocol detection.
func readRawFrame(conn net.Conn) ([]byte, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, err
	}
	msgLen := binary.BigEndian.Uint32(lenBuf)
	if msgLen > 8*1024*1024 {
		return nil, fmt.Errorf("message too large: %d bytes", msgLen)
	}
	data := make([]byte, msgLen)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}
	return data, nil
}

// isProtobuf detects whether a raw frame is protobuf-encoded (as opposed to
// JSON). JSON messages always start with '{', while protobuf varint-encoded
// fields start with a field tag (never 0x7B which is '{').
func isProtobuf(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	return data[0] != '{'
}

// pbToMessage converts a protobuf VsockMessage to the internal JSON-based Message.
func pbToMessage(pb *agentpb.VsockMessage) (*Message, error) {
	msg := &Message{Type: int(pb.Type)}
	switch p := pb.Payload.(type) {
	case *agentpb.VsockMessage_Init:
		envVars := p.Init.EnvVars
		if envVars == nil {
			envVars = make(map[string]string)
		}
		payload := InitPayload{
			Runtime:         p.Init.Runtime,
			Handler:         p.Init.Handler,
			EnvVars:         envVars,
			Command:         p.Init.Command,
			Extension:       p.Init.Extension,
			Mode:            ExecutionMode(p.Init.Mode),
			FunctionName:    p.Init.FunctionName,
			FunctionVersion: int(p.Init.FunctionVersion),
			MemoryMB:        int(p.Init.MemoryMb),
			TimeoutS:        int(p.Init.TimeoutS),
			LayerCount:      int(p.Init.LayerCount),
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		msg.Payload = data
	case *agentpb.VsockMessage_Exec:
		payload := ExecPayload{
			RequestID:   p.Exec.RequestId,
			Input:       p.Exec.Input,
			TimeoutS:    int(p.Exec.TimeoutS),
			TraceParent: p.Exec.TraceParent,
			TraceState:  p.Exec.TraceState,
			Stream:      p.Exec.Stream,
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		msg.Payload = data
	case *agentpb.VsockMessage_Reload:
		payload := ReloadPayload{Files: p.Reload.Files}
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		msg.Payload = data
	case nil:
		// No payload (e.g., PING, STOP)
		msg.Payload = json.RawMessage(`{}`)
	default:
		return nil, fmt.Errorf("unsupported protobuf payload type: %T", p)
	}
	return msg, nil
}

// messageToPb converts the internal JSON-based Message to a protobuf VsockMessage.
func messageToPb(msg *Message) (*agentpb.VsockMessage, error) {
	pb := &agentpb.VsockMessage{
		Type: agentpb.VsockMessage_Type(msg.Type),
	}
	switch msg.Type {
	case MsgTypeResp:
		var resp RespPayload
		if err := json.Unmarshal(msg.Payload, &resp); err != nil {
			// If payload doesn't match RespPayload, send raw bytes as output
			pb.Payload = &agentpb.VsockMessage_Resp{
				Resp: &agentpb.RespPayload{Output: msg.Payload},
			}
			return pb, nil
		}
		pb.Payload = &agentpb.VsockMessage_Resp{
			Resp: &agentpb.RespPayload{
				RequestId:  resp.RequestID,
				Output:     resp.Output,
				Error:      resp.Error,
				DurationMs: resp.DurationMs,
				Stdout:     resp.Stdout,
				Stderr:     resp.Stderr,
			},
		}
	case MsgTypeStream:
		var chunk StreamChunkPayload
		if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
			return nil, fmt.Errorf("unmarshal stream chunk for pb: %w", err)
		}
		pb.Payload = &agentpb.VsockMessage_StreamChunk{
			StreamChunk: &agentpb.StreamChunkPayload{
				RequestId: chunk.RequestID,
				Data:      chunk.Data,
				IsLast:    chunk.IsLast,
				Error:     chunk.Error,
			},
		}
	}
	return pb, nil
}

// writeProtobufMessage marshals and sends a protobuf-encoded message.
func writeProtobufMessage(conn net.Conn, msg *Message) error {
	pb, err := messageToPb(msg)
	if err != nil {
		return err
	}
	data, err := proto.Marshal(pb)
	if err != nil {
		return err
	}
	buf := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(data)))
	copy(buf[4:], data)
	_, err = conn.Write(buf)
	return err
}

// ─── State Proxy HTTP Server ─────────────────────────────────
// The state proxy runs at 127.0.0.1:8399 inside the VM and provides
// a simple HTTP API for function code to access durable state. Requests
// are proxied to the host's state API via vsock/TCP.

func (a *Agent) startStateProxy() {
	mux := http.NewServeMux()

	// GET /state?key=xxx → get single state entry
	mux.HandleFunc("GET /state", a.handleStateGet)
	// PUT /state?key=xxx → put state entry
	mux.HandleFunc("PUT /state", a.handleStatePut)
	// DELETE /state?key=xxx → delete state entry
	mux.HandleFunc("DELETE /state", a.handleStateDelete)
	// GET /state/list → list state entries
	mux.HandleFunc("GET /state/list", a.handleStateList)

	addr := fmt.Sprintf("127.0.0.1:%d", stateProxyPort)
	fmt.Printf("[agent] State proxy starting on %s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "[agent] State proxy error: %v\n", err)
	}
}

func (a *Agent) sendStateRequest(msgType int, payload interface{}) (*StateResponsePayload, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	msg := &Message{Type: msgType, Payload: data}
	msgData, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	a.stateConnMu.Lock()
	defer a.stateConnMu.Unlock()

	conn, err := a.dialStateConn()
	if err != nil {
		return nil, fmt.Errorf("dial state conn: %w", err)
	}
	defer conn.Close()

	// Send message with length prefix
	buf := make([]byte, 4+len(msgData))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(msgData)))
	copy(buf[4:], msgData)
	if _, err := conn.Write(buf); err != nil {
		return nil, fmt.Errorf("write state request: %w", err)
	}

	// Read response
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, fmt.Errorf("read state response length: %w", err)
	}
	respLen := binary.BigEndian.Uint32(lenBuf)
	respData := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respData); err != nil {
		return nil, fmt.Errorf("read state response: %w", err)
	}

	var respMsg Message
	if err := json.Unmarshal(respData, &respMsg); err != nil {
		return nil, fmt.Errorf("unmarshal state response message: %w", err)
	}

	var resp StateResponsePayload
	if err := json.Unmarshal(respMsg.Payload, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal state response: %w", err)
	}
	return &resp, nil
}

func (a *Agent) dialStateConn() (net.Conn, error) {
	mode := os.Getenv("NOVA_AGENT_MODE")

	// TCP mode for Docker containers — connect back to host state port
	if mode == "tcp" {
		hostAddr := os.Getenv("NOVA_HOST_ADDR")
		if hostAddr == "" {
			hostAddr = "host.docker.internal:9998"
		}
		return net.DialTimeout("tcp", hostAddr, 5*time.Second)
	}

	// In Firecracker VM, connect to host via TCP over bridge network.
	// The host's state proxy listens on port 9998.
	// Use the gateway IP (172.30.x.1) which is set as default route.
	if runtime.GOOS == "linux" {
		// Try connecting to common gateway addresses
		for _, addr := range []string{"172.30.0.1:9998", "10.0.0.1:9998"} {
			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err == nil {
				return conn, nil
			}
		}
	}

	// Fallback: unix socket for local dev
	sockPath := "/tmp/nova-state-9998.sock"
	return net.DialTimeout("unix", sockPath, 5*time.Second)
}

func (a *Agent) handleStateGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, `{"error":"key parameter required"}`, http.StatusBadRequest)
		return
	}

	resp, err := a.sendStateRequest(MsgTypeStateGet, &StateRequestPayload{Key: key})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	if resp.Error != "" {
		status := http.StatusInternalServerError
		if resp.Error == "not found" {
			status = http.StatusNotFound
		}
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, resp.Error), status)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (a *Agent) handleStatePut(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, `{"error":"key parameter required"}`, http.StatusBadRequest)
		return
	}

	var req StateRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	req.Key = key

	resp, err := a.sendStateRequest(MsgTypeStatePut, &req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	if resp.Error != "" {
		status := http.StatusInternalServerError
		if resp.Error == "conflict" {
			status = http.StatusConflict
		}
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, resp.Error), status)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (a *Agent) handleStateDelete(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, `{"error":"key parameter required"}`, http.StatusBadRequest)
		return
	}

	resp, err := a.sendStateRequest(MsgTypeStateDelete, &StateRequestPayload{Key: key})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	if resp.Error != "" {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, resp.Error), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *Agent) handleStateList(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	resp, err := a.sendStateRequest(MsgTypeStateList, &StateRequestPayload{
		Prefix: prefix,
		Limit:  limit,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	if resp.Error != "" {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, resp.Error), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ─── Sandbox handlers ───────────────────────────────────

type shellExecPayload struct {
	Command  string `json:"command"`
	TimeoutS int    `json:"timeout_s,omitempty"`
	WorkDir  string `json:"workdir,omitempty"`
}

type shellExecRespPayload struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}

func (a *Agent) handleShellExec(payload json.RawMessage) (*Message, error) {
	var req shellExecPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	timeout := time.Duration(req.TimeoutS) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", req.Command)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	} else {
		cmd.Dir = "/home/sandbox"
	}
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			resp := &shellExecRespPayload{
				ExitCode: -1,
				Error:    err.Error(),
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
			}
			data, _ := json.Marshal(resp)
			return &Message{Type: MsgTypeResp, Payload: data}, nil
		}
	}

	resp := &shellExecRespPayload{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
	data, _ := json.Marshal(resp)
	return &Message{Type: MsgTypeResp, Payload: data}, nil
}

type fileReadPayload struct {
	Path string `json:"path"`
}

// sandboxSafeDirs are the directories that sandbox file operations are allowed to access.
var sandboxSafeDirs = []string{"/home/sandbox", "/tmp", "/code", "/var/tmp"}

// validateSandboxPath ensures the path is within allowed sandbox directories.
func validateSandboxPath(path string) error {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join("/home/sandbox", cleaned)
	}
	for _, safe := range sandboxSafeDirs {
		if cleaned == safe || strings.HasPrefix(cleaned, safe+"/") {
			return nil
		}
	}
	return fmt.Errorf("path %q is outside sandbox-allowed directories", path)
}

type fileRespPayload struct {
	Content string          `json:"content,omitempty"`
	Entries []fileEntryInfo `json:"entries,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type fileEntryInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
}

func (a *Agent) handleFileRead(payload json.RawMessage) (*Message, error) {
	var req fileReadPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	if err := validateSandboxPath(req.Path); err != nil {
		resp := &fileRespPayload{Error: err.Error()}
		data, _ := json.Marshal(resp)
		return &Message{Type: MsgTypeFileResp, Payload: data}, nil
	}

	content, err := os.ReadFile(req.Path)
	if err != nil {
		resp := &fileRespPayload{Error: err.Error()}
		data, _ := json.Marshal(resp)
		return &Message{Type: MsgTypeFileResp, Payload: data}, nil
	}

	resp := &fileRespPayload{Content: base64.StdEncoding.EncodeToString(content)}
	data, _ := json.Marshal(resp)
	return &Message{Type: MsgTypeFileResp, Payload: data}, nil
}

type fileWritePayload struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Perm    int    `json:"perm,omitempty"`
}

func (a *Agent) handleFileWrite(payload json.RawMessage) (*Message, error) {
	var req fileWritePayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	if err := validateSandboxPath(req.Path); err != nil {
		resp := &fileRespPayload{Error: err.Error()}
		data, _ := json.Marshal(resp)
		return &Message{Type: MsgTypeFileResp, Payload: data}, nil
	}

	content, err := base64.StdEncoding.DecodeString(req.Content)
	if err != nil {
		resp := &fileRespPayload{Error: "invalid base64 content: " + err.Error()}
		data, _ := json.Marshal(resp)
		return &Message{Type: MsgTypeFileResp, Payload: data}, nil
	}

	perm := os.FileMode(0644)
	if req.Perm > 0 {
		perm = os.FileMode(req.Perm)
	}

	dir := filepath.Dir(req.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		resp := &fileRespPayload{Error: err.Error()}
		data, _ := json.Marshal(resp)
		return &Message{Type: MsgTypeFileResp, Payload: data}, nil
	}

	if err := os.WriteFile(req.Path, content, perm); err != nil {
		resp := &fileRespPayload{Error: err.Error()}
		data, _ := json.Marshal(resp)
		return &Message{Type: MsgTypeFileResp, Payload: data}, nil
	}

	resp := &fileRespPayload{}
	data, _ := json.Marshal(resp)
	return &Message{Type: MsgTypeFileResp, Payload: data}, nil
}

type fileListPayload struct {
	Path string `json:"path"`
}

func (a *Agent) handleFileList(payload json.RawMessage) (*Message, error) {
	var req fileListPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	if err := validateSandboxPath(req.Path); err != nil {
		resp := &fileRespPayload{Error: err.Error()}
		data, _ := json.Marshal(resp)
		return &Message{Type: MsgTypeFileResp, Payload: data}, nil
	}

	entries, err := os.ReadDir(req.Path)
	if err != nil {
		resp := &fileRespPayload{Error: err.Error()}
		data, _ := json.Marshal(resp)
		return &Message{Type: MsgTypeFileResp, Payload: data}, nil
	}

	var infos []fileEntryInfo
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		infos = append(infos, fileEntryInfo{
			Name:    e.Name(),
			Path:    filepath.Join(req.Path, e.Name()),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		})
	}

	resp := &fileRespPayload{Entries: infos}
	data, _ := json.Marshal(resp)
	return &Message{Type: MsgTypeFileResp, Payload: data}, nil
}

type fileDeletePayload struct {
	Path string `json:"path"`
}

func (a *Agent) handleFileDelete(payload json.RawMessage) (*Message, error) {
	var req fileDeletePayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	if err := validateSandboxPath(req.Path); err != nil {
		resp := &fileRespPayload{Error: err.Error()}
		data, _ := json.Marshal(resp)
		return &Message{Type: MsgTypeFileResp, Payload: data}, nil
	}

	err := os.RemoveAll(req.Path)
	if err != nil {
		resp := &fileRespPayload{Error: err.Error()}
		data, _ := json.Marshal(resp)
		return &Message{Type: MsgTypeFileResp, Payload: data}, nil
	}

	resp := &fileRespPayload{}
	data, _ := json.Marshal(resp)
	return &Message{Type: MsgTypeFileResp, Payload: data}, nil
}

type processEntryInfo struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
	CPU     string `json:"cpu,omitempty"`
	Memory  string `json:"memory,omitempty"`
}

type processListRespPayload struct {
	Processes []processEntryInfo `json:"processes"`
	Error     string             `json:"error,omitempty"`
}

func (a *Agent) handleProcessList() (*Message, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		resp := &processListRespPayload{Error: err.Error()}
		data, _ := json.Marshal(resp)
		return &Message{Type: MsgTypeProcessResp, Payload: data}, nil
	}

	var procs []processEntryInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		cmd := strings.ReplaceAll(string(cmdline), "\x00", " ")
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		procs = append(procs, processEntryInfo{
			PID:     pid,
			Command: cmd,
		})
	}

	resp := &processListRespPayload{Processes: procs}
	data, _ := json.Marshal(resp)
	return &Message{Type: MsgTypeProcessResp, Payload: data}, nil
}

type processKillPayload struct {
	PID    int `json:"pid"`
	Signal int `json:"signal,omitempty"`
}

type processKillRespPayload struct {
	Error string `json:"error,omitempty"`
}

func (a *Agent) handleProcessKill(payload json.RawMessage) (*Message, error) {
	var req processKillPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	// Protect critical system processes
	if req.PID <= 2 {
		resp := &processKillRespPayload{Error: "cannot kill system process (PID <= 2)"}
		data, _ := json.Marshal(resp)
		return &Message{Type: MsgTypeProcessResp, Payload: data}, nil
	}

	sig := syscall.SIGTERM
	if req.Signal > 0 {
		sig = syscall.Signal(req.Signal)
	}

	err := syscall.Kill(req.PID, sig)
	resp := &processKillRespPayload{}
	if err != nil {
		resp.Error = err.Error()
	}
	data, _ := json.Marshal(resp)
	return &Message{Type: MsgTypeProcessResp, Payload: data}, nil
}
