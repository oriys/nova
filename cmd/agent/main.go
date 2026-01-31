package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
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
	RequestID string          `json:"request_id"`
	Input     json.RawMessage `json:"input"`
	TimeoutS  int             `json:"timeout_s"`
}

type RespPayload struct {
	RequestID  string          `json:"request_id"`
	Output     json.RawMessage `json:"output"`
	Error      string          `json:"error,omitempty"`
	DurationMs int64           `json:"duration_ms"`
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

// listen creates a vsock listener in a VM, or falls back to Unix socket for dev.
func listen(port int) (net.Listener, error) {
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
		cmd = exec.Command("python3", "-u", CodePath, "--persistent")
	case "go", "rust":
		cmd = exec.Command(CodePath, "--persistent")
	case "wasm":
		// WASM persistent mode: wasmtime with stdin/stdout communication
		// The WASM module must implement a loop reading JSON from stdin
		cmd = exec.Command("wasmtime", CodePath, "--", "--persistent")
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

	start := time.Now()
	output, execErr := a.executeFunction(req.Input)
	duration := time.Since(start).Milliseconds()

	resp := RespPayload{
		RequestID:  req.RequestID,
		DurationMs: duration,
	}
	if execErr != nil {
		resp.Error = execErr.Error()
	} else {
		resp.Output = output
	}

	respData, _ := json.Marshal(resp)
	return &Message{Type: MsgTypeResp, Payload: respData}, nil
}

func (a *Agent) executeFunction(input json.RawMessage) (json.RawMessage, error) {
	// Use persistent mode if available
	if a.function.Mode == ModePersistent && a.persistentProc != nil {
		return a.executePersistent(input)
	}

	// Process mode: fork new process each time
	if err := os.WriteFile("/tmp/input.json", input, 0644); err != nil {
		return nil, err
	}

	var cmd *exec.Cmd
	switch a.function.Runtime {
	case "python":
		cmd = exec.Command("python3", CodePath, "/tmp/input.json")
	case "go", "rust":
		cmd = exec.Command(CodePath, "/tmp/input.json")
	case "wasm":
		cmd = exec.Command("wasmtime", CodePath, "--", "/tmp/input.json")
	default:
		return nil, fmt.Errorf("unsupported runtime: %s", a.function.Runtime)
	}

	cmd.Env = append(defaultEnv(), "NOVA_CODE_DIR="+CodeMountPoint)
	for k, v := range a.function.EnvVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("exit %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, err
	}

	if json.Valid(output) {
		return output, nil
	}
	result, _ := json.Marshal(string(output))
	return result, nil
}

// executePersistent sends request to long-running process via stdin/stdout
func (a *Agent) executePersistent(input json.RawMessage) (json.RawMessage, error) {
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

	// Read response line
	line, err := a.persistentOut.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read from persistent process: %w", err)
	}

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
