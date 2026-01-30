package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"time"
)

const (
	MsgTypeInit = 1
	MsgTypeExec = 2
	MsgTypeResp = 3
	MsgTypePing = 4

	VsockPort = 9999

	// Function code is on second drive, mounted at /code
	CodeMountPoint = "/code"
	CodePath       = "/code/handler"
)

type Message struct {
	Type    int             `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type InitPayload struct {
	Runtime string            `json:"runtime"`
	Handler string            `json:"handler"`
	EnvVars map[string]string `json:"env_vars"`
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
	function *InitPayload
}

func main() {
	fmt.Println("[agent] Nova guest agent starting...")

	// Mount code drive (second block device /dev/vdb)
	mountCodeDrive()

	listener, err := listenVsock(VsockPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[agent] Failed to listen: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("[agent] Listening on vsock port %d\n", VsockPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[agent] Accept error: %v\n", err)
			continue
		}
		go handleConnection(conn)
	}
}

func mountCodeDrive() {
	os.MkdirAll(CodeMountPoint, 0755)
	cmd := exec.Command("mount", "-t", "ext4", "-o", "ro", "/dev/vdb", CodeMountPoint)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "[agent] Mount /dev/vdb failed: %s: %v\n", out, err)
		// Non-fatal: might be running outside a VM for testing
	} else {
		fmt.Printf("[agent] Mounted code drive at %s\n", CodeMountPoint)
	}
}

func listenVsock(port int) (net.Listener, error) {
	// In real Firecracker VM: use vsock
	// For dev/testing: fallback to unix socket
	sockPath := fmt.Sprintf("/tmp/nova-agent-%d.sock", port)
	os.Remove(sockPath)
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
	a.function = &init
	fmt.Printf("[agent] Init: runtime=%s handler=%s\n", init.Runtime, init.Handler)

	return &Message{
		Type:    MsgTypeResp,
		Payload: json.RawMessage(`{"status":"initialized"}`),
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
	// Write input to temp file
	if err := os.WriteFile("/tmp/input.json", input, 0644); err != nil {
		return nil, err
	}

	// Code is always at /code/handler (from code drive)
	var cmd *exec.Cmd
	switch a.function.Runtime {
	case "python":
		cmd = exec.Command("python3", CodePath, "/tmp/input.json")
	case "go", "rust":
		// Static binary, execute directly
		os.Chmod(CodePath, 0755)
		cmd = exec.Command(CodePath, "/tmp/input.json")
	case "wasm":
		cmd = exec.Command("wasmtime", CodePath, "--", "/tmp/input.json")
	default:
		return nil, fmt.Errorf("unsupported runtime: %s", a.function.Runtime)
	}

	// Set env vars
	cmd.Env = append(os.Environ(), "NOVA_CODE_DIR="+CodeMountPoint)
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
