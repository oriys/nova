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
	"runtime"
	"strings"
	"time"

	"github.com/oriys/nova/internal/pkg/vsock"
)

const (
	MsgTypeInit = 1
	MsgTypeExec = 2
	MsgTypeResp = 3
	MsgTypePing = 4
	MsgTypeStop = 5

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
	Runtime string            `json:"runtime"`
	Handler string            `json:"handler"`
	EnvVars map[string]string `json:"env_vars"`
	Mode    ExecutionMode     `json:"mode,omitempty"` // "process" or "persistent"
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
	init.Runtime = normalizeRuntime(init.Runtime)

	// Default to process mode
	if init.Mode == "" {
		init.Mode = ModeProcess
	}

	a.function = &init
	fmt.Printf("[agent] Init: runtime=%s handler=%s mode=%s\n", init.Runtime, init.Handler, init.Mode)

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

// startPersistentProcess starts a long-running function process for connection reuse
func (a *Agent) startPersistentProcess() error {
	var cmd *exec.Cmd
	switch a.function.Runtime {
	case "python":
		// Python persistent mode: reads JSON lines from stdin, writes to stdout
		cmd = exec.Command(resolveBinary(pythonPath, "python3"), "-u", CodePath, "--persistent")
	case "go", "rust":
		cmd = exec.Command(CodePath, "--persistent")
	case "wasm":
		// WASM persistent mode: wasmtime with stdin/stdout communication
		// The WASM module must implement a loop reading JSON from stdin
		cmd = exec.Command(resolveBinary(wasmtimePath, "wasmtime"), CodePath, "--", "--persistent")
	default:
		return fmt.Errorf("persistent mode not supported for runtime: %s", a.function.Runtime)
	}

	cmd.Env = append(defaultEnv(), "NOVA_MODE=persistent", "NOVA_CODE_DIR="+CodeMountPoint)
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
	output, stdout, stderr, execErr := a.executeFunction(req.Input, req.TimeoutS)
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

func (a *Agent) executeFunction(input json.RawMessage, timeoutS int) (json.RawMessage, string, string, error) {
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
	switch a.function.Runtime {
	case "python":
		cmd = exec.CommandContext(ctx, resolveBinary(pythonPath, "python3"), CodePath, "/tmp/input.json")
	case "go", "rust":
		cmd = exec.CommandContext(ctx, CodePath, "/tmp/input.json")
	case "wasm":
		cmd = exec.CommandContext(ctx, resolveBinary(wasmtimePath, "wasmtime"), CodePath, "--", "/tmp/input.json")
	case "node":
		cmd = exec.CommandContext(ctx, resolveBinary(nodePath, "node"), CodePath, "/tmp/input.json")
	case "ruby":
		cmd = exec.CommandContext(ctx, resolveBinary(rubyPath, "ruby"), CodePath, "/tmp/input.json")
	case "java":
		// Java expects a JAR file: java -jar /code/handler.jar input.json
		cmd = exec.CommandContext(ctx, resolveBinary(javaPath, "java"), "-jar", CodePath, "/tmp/input.json")
	case "php":
		cmd = exec.CommandContext(ctx, resolveBinary(phpPath, "php"), CodePath, "/tmp/input.json")
	case "deno":
		// Deno needs --allow-read for input file
		cmd = exec.CommandContext(ctx, resolveBinary(denoPath, "deno"), "run", "--allow-read", CodePath, "/tmp/input.json")
	case "bun":
		cmd = exec.CommandContext(ctx, resolveBinary(bunPath, "bun"), "run", CodePath, "/tmp/input.json")
	case "dotnet":
		// Expect a single-file apphost at /code/handler (PublishSingleFile=true).
		cmd = exec.CommandContext(ctx, CodePath, "/tmp/input.json")
	default:
		return nil, "", "", fmt.Errorf("unsupported runtime: %s", a.function.Runtime)
	}

	cmd.Env = append(defaultEnv(), "NOVA_CODE_DIR="+CodeMountPoint)
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
		return nil, stdout, stderr, err
	}

	output := stdoutBuf.Bytes()
	if json.Valid(output) {
		return output, "", stderr, nil
	}
	result, _ := json.Marshal(string(output))
	return result, "", stderr, nil
}

// executePersistent sends request to long-running process via stdin/stdout
func (a *Agent) executePersistent(input json.RawMessage, timeoutS int) (json.RawMessage, error) {
	// Protocol: write JSON line to stdin, read JSON line from stdout
	// Format: {"input": ...}\n -> {"output": ...}\n

	req := map[string]json.RawMessage{"input": input}
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
