package main

import (
	"bufio"
	"bytes"
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
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/pkg/vsock"
)

const (
	MsgTypeInit   = 1
	MsgTypeExec   = 2
	MsgTypeResp   = 3
	MsgTypePing   = 4
	MsgTypeStop   = 5
	MsgTypeReload = 6 // Hot reload code files

	VsockPort = 9999

	CodeMountPoint = "/code"
	CodePath       = "/code/handler"

	defaultPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

	pythonPath   = "/usr/bin/python3"
	wasmtimePath = "/usr/local/bin/wasmtime"
	nodePath     = "/usr/bin/node"
	rubyPath     = "/usr/bin/ruby"
	javaPath     = "/usr/bin/java"
	phpPath      = "/usr/bin/php"
	luaPath      = "/usr/bin/lua"
	denoPath     = "/usr/local/bin/deno"
	bunPath      = "/usr/local/bin/bun"
	dotnetRoot   = "/usr/share/dotnet"

	maxPersistentResponseBytes = 4 * 1024 * 1024 // 4MB
)

// ExecutionMode determines how functions are executed
type ExecutionMode string

const (
	// ModeProcess: Fork new process for each invocation (default, isolated)
	ModeProcess ExecutionMode = "process"
	// ModePersistent: Keep function process alive, send requests via stdin/stdout
	// Enables connection reuse for databases, etc.
	ModePersistent ExecutionMode = "persistent"
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
	Mode            ExecutionMode     `json:"mode,omitempty"` // "process" or "persistent"
	FunctionName    string            `json:"function_name,omitempty"`
	FunctionVersion int               `json:"function_version,omitempty"`
	MemoryMB        int               `json:"memory_mb,omitempty"`
	TimeoutS        int               `json:"timeout_s,omitempty"`
	LayerCount      int               `json:"layer_count,omitempty"`
}

type ExecPayload struct {
	RequestID   string          `json:"request_id"`
	Input       json.RawMessage `json:"input"`
	TimeoutS    int             `json:"timeout_s"`
	TraceParent string          `json:"traceparent,omitempty"`
	TraceState  string          `json:"tracestate,omitempty"`
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

type Agent struct {
	function         *InitPayload
	persistentProc   *exec.Cmd
	persistentIn     io.WriteCloser
	persistentOut    *bufio.Reader
	persistentOutRaw io.ReadCloser
}

func main() {
	fmt.Println("[agent] Nova guest agent starting...")

	ensurePath()
	mountCodeDrive()
	mountLayerDrives()

	listener, err := listen(VsockPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[agent] Failed to listen: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("[agent] Listening on port %d\n", VsockPort)

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

	for {
		msg, err := readMessage(conn)
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "[agent] Read error: %v\n", err)
			}
			return
		}

		if msg.Type == MsgTypeStop {
			fmt.Println("[agent] Received stop, shutting down...")
			writeMessage(conn, &Message{Type: MsgTypeResp, Payload: json.RawMessage(`{"status":"stopping"}`)})
			os.Exit(0)
		}

		resp, err := agent.handleMessage(msg)
		if err != nil {
			resp = &Message{
				Type:    MsgTypeResp,
				Payload: json.RawMessage(fmt.Sprintf(`{"error":"%s"}`, err.Error())),
			}
		}

		if err := writeMessage(conn, resp); err != nil {
			fmt.Fprintf(os.Stderr, "[agent] Write error: %v\n", err)
			return
		}
	}
}

func (a *Agent) handleMessage(msg *Message) (*Message, error) {
	switch msg.Type {
	case MsgTypeInit:
		return a.handleInit(msg.Payload)
	case MsgTypeExec:
		return a.handleExec(msg.Payload)
	case MsgTypePing:
		return &Message{Type: MsgTypeResp, Payload: json.RawMessage(`{"status":"ok"}`)}, nil
	case MsgTypeReload:
		return a.handleReload(msg.Payload)
	default:
		return nil, fmt.Errorf("unknown message type: %d", msg.Type)
	}
}

func (a *Agent) handleInit(payload json.RawMessage) (*Message, error) {
	var init InitPayload
	if err := json.Unmarshal(payload, &init); err != nil {
		return nil, err
	}

	// Normalize versioned runtime IDs (e.g., python3.12, node24, php8.4, dotnet8)
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
	case "go", "rust":
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
		fmt.Sprintf("NOVA_FUNCTION_VERSION=%d", a.function.FunctionVersion),
		fmt.Sprintf("NOVA_MEMORY_LIMIT_MB=%d", a.function.MemoryMB),
		fmt.Sprintf("NOVA_TIMEOUT_S=%d", a.function.TimeoutS),
		"NOVA_RUNTIME="+a.function.Runtime,
	)
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

	// 4. Write new files
	for name, content := range req.Files {
		path := filepath.Join(CodeMountPoint, name)
		// Create parent directories
		if dir := filepath.Dir(path); dir != CodeMountPoint {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("create dir for %s: %w", name, err)
			}
		}
		if err := os.WriteFile(path, content, 0755); err != nil {
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

func (a *Agent) handleExec(payload json.RawMessage) (*Message, error) {
	if a.function == nil {
		return nil, fmt.Errorf("function not initialized")
	}

	var req ExecPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	// Log trace context if available
	if req.TraceParent != "" {
		fmt.Printf("[agent] exec request_id=%s trace_parent=%s\n", req.RequestID, req.TraceParent)
	} else {
		fmt.Printf("[agent] exec request_id=%s\n", req.RequestID)
	}

	start := time.Now()
	output, stdout, stderr, execErr := a.executeFunction(req.Input, req.TimeoutS, req.RequestID)
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

func (a *Agent) executeFunction(input json.RawMessage, timeoutS int, requestID string) (json.RawMessage, string, string, error) {
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
		case "go", "rust", "zig", "swift":
			cmd = exec.CommandContext(ctx, CodePath, "/tmp/input.json")
		case "wasm":
			cmd = exec.CommandContext(ctx, resolveBinary(wasmtimePath, "wasmtime"), CodePath, "--", "/tmp/input.json")
		case "java", "kotlin", "scala":
			cmd = exec.CommandContext(ctx, resolveBinary(javaPath, "java"), "-jar", CodePath, "/tmp/input.json")
		case "dotnet":
			cmd = exec.CommandContext(ctx, CodePath, "/tmp/input.json")
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
		fmt.Sprintf("NOVA_FUNCTION_VERSION=%d", a.function.FunctionVersion),
		fmt.Sprintf("NOVA_MEMORY_LIMIT_MB=%d", a.function.MemoryMB),
		fmt.Sprintf("NOVA_TIMEOUT_S=%d", a.function.TimeoutS),
		"NOVA_RUNTIME="+a.function.Runtime,
	)
	// Add dependency paths based on runtime
	cmd.Env = appendDependencyEnv(cmd.Env, a.function.Runtime)
	if a.function.Runtime == "dotnet" {
		cmd.Env = append(cmd.Env,
			"DOTNET_ROOT="+dotnetRoot,
			"DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=true",
		)
	}
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

func enrichExecError(err error, runtime string) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && errors.Is(pathErr.Err, syscall.ENOEXEC) {
		if runtime == "go" || runtime == "rust" || runtime == "zig" || runtime == "swift" || runtime == "dotnet" {
			return fmt.Errorf("%w (exec format error: /code/handler must be a Linux executable for the VM architecture, e.g. x86_64-unknown-linux-musl)", err)
		}
		return fmt.Errorf("%w (exec format error: check shebang and executable format of /code/handler)", err)
	}
	if errors.As(err, &pathErr) && errors.Is(pathErr.Err, syscall.ENOENT) && pathErr.Path == CodePath {
		if _, statErr := os.Stat(CodePath); statErr == nil {
			if runtime == "go" || runtime == "rust" || runtime == "zig" || runtime == "swift" || runtime == "dotnet" {
				return fmt.Errorf("%w (/code/handler exists but required dynamic loader is missing; build a static Linux binary, e.g. Rust target x86_64-unknown-linux-musl)", err)
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
			"request_id":      os.Getenv("NOVA_REQUEST_ID"),
			"function_name":   a.function.FunctionName,
			"function_version": a.function.FunctionVersion,
			"memory_limit_mb": a.function.MemoryMB,
			"timeout_s":       a.function.TimeoutS,
			"runtime":         a.function.Runtime,
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
		env = append(env, "PYTHONPATH="+strings.Join(paths, ":"))
	case "node":
		paths := []string{"/code/node_modules"}
		for _, lp := range layerPaths {
			paths = append(paths, lp+"/node_modules")
		}
		env = append(env, "NODE_PATH="+strings.Join(paths, ":"))
	case "ruby":
		paths := []string{}
		for _, lp := range layerPaths {
			paths = append(paths, lp+"/lib/ruby")
		}
		if len(paths) > 0 {
			env = append(env, "RUBYLIB="+strings.Join(paths, ":"))
		}
	case "deno":
		env = append(env, "DENO_DIR=/code/.deno")
	case "bun":
		paths := []string{"/code/node_modules"}
		for _, lp := range layerPaths {
			paths = append(paths, lp+"/node_modules")
		}
		env = append(env, "NODE_PATH="+strings.Join(paths, ":"))
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

	return env
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
	case strings.HasPrefix(rt, "dotnet"):
		return "dotnet"
	case strings.HasPrefix(rt, "go"):
		return "go"
	case strings.HasPrefix(rt, "rust"):
		return "rust"
	case strings.HasPrefix(rt, "swift"):
		return "swift"
	case strings.HasPrefix(rt, "zig"):
		return "zig"
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
	data := make([]byte, binary.BigEndian.Uint32(lenBuf))
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
