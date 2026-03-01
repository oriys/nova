package firecracker

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/metrics"
)

// ─── Vsock protocol ─────────────────────────────────────

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
	MsgTypeShellExec    = 20 // Execute shell command (single shot)
	MsgTypeShellStream  = 21 // Open interactive shell session
	MsgTypeShellInput   = 22 // Write stdin to shell session
	MsgTypeShellResize  = 23 // Terminal window resize
	MsgTypeFileRead     = 30 // Read file
	MsgTypeFileWrite    = 31 // Write file
	MsgTypeFileList     = 32 // List directory
	MsgTypeFileDelete   = 33 // Delete file
	MsgTypeFileResp     = 34 // File operation response
	MsgTypeProcessList  = 40 // List processes
	MsgTypeProcessKill  = 41 // Kill process
	MsgTypeProcessResp  = 42 // Process operation response
)

type VsockMessage struct {
	Type    int             `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type ExecPayload struct {
	RequestID   string          `json:"request_id"`
	Input       json.RawMessage `json:"input"`
	TimeoutS    int             `json:"timeout_s"`
	TraceParent string          `json:"traceparent,omitempty"` // W3C TraceContext
	TraceState  string          `json:"tracestate,omitempty"`  // W3C TraceContext
	Stream      bool            `json:"stream,omitempty"`      // Enable streaming response
}

type RespPayload struct {
	RequestID  string          `json:"request_id"`
	Output     json.RawMessage `json:"output"`
	Error      string          `json:"error,omitempty"`
	DurationMs int64           `json:"duration_ms"`
	Stdout     string          `json:"stdout,omitempty"` // Captured stdout
	Stderr     string          `json:"stderr,omitempty"` // Captured stderr
}

// StreamChunkPayload is used for streaming responses
type StreamChunkPayload struct {
	RequestID string `json:"request_id"`
	Data      []byte `json:"data"`            // Chunk of data
	IsLast    bool   `json:"is_last"`         // True if this is the final chunk
	Error     string `json:"error,omitempty"` // Error message if execution failed
}

// ─── Sandbox payload types ──────────────────────────────

// ShellExecPayload is sent to execute a shell command inside a sandbox.
type ShellExecPayload struct {
	Command  string `json:"command"`
	TimeoutS int    `json:"timeout_s,omitempty"`
	WorkDir  string `json:"workdir,omitempty"`
}

// ShellExecRespPayload is the response from a shell command execution.
type ShellExecRespPayload struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}

// FileReadPayload requests reading a file.
type FileReadPayload struct {
	Path string `json:"path"`
}

// FileWritePayload requests writing a file.
type FileWritePayload struct {
	Path    string `json:"path"`
	Content string `json:"content"` // base64-encoded
	Perm    int    `json:"perm,omitempty"`
}

// FileListPayload requests listing a directory.
type FileListPayload struct {
	Path string `json:"path"`
}

// FileDeletePayload requests deleting a file or directory.
type FileDeletePayload struct {
	Path string `json:"path"`
}

// FileRespPayload is the generic response for file operations.
type FileRespPayload struct {
	Content string          `json:"content,omitempty"` // base64-encoded for reads
	Entries []FileEntryInfo `json:"entries,omitempty"` // for directory listing
	Error   string          `json:"error,omitempty"`
}

// FileEntryInfo represents a file/directory in a listing.
type FileEntryInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
}

// ProcessListRespPayload is the response for listing processes.
type ProcessListRespPayload struct {
	Processes []ProcessEntryInfo `json:"processes"`
	Error     string             `json:"error,omitempty"`
}

// ProcessEntryInfo represents a running process.
type ProcessEntryInfo struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
	CPU     string `json:"cpu,omitempty"`
	Memory  string `json:"memory,omitempty"`
}

// ProcessKillPayload requests killing a process.
type ProcessKillPayload struct {
	PID    int `json:"pid"`
	Signal int `json:"signal,omitempty"` // default SIGTERM
}

// ProcessKillRespPayload is the response for killing a process.
type ProcessKillRespPayload struct {
	Error string `json:"error,omitempty"`
}

type VsockClient struct {
	vm          *VM
	conn        net.Conn
	mu          sync.Mutex
	initPayload json.RawMessage
}

func NewVsockClient(vm *VM) (*VsockClient, error) {
	// Dial on demand. In practice, the underlying UDS-backed vsock connection may
	// be short-lived; keeping a long-lived connection is error-prone.
	return &VsockClient{vm: vm}, nil
}

func (c *VsockClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *VsockClient) closeLocked() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *VsockClient) dialLocked(timeout time.Duration) error {
	start := time.Now()
	conn, err := dialVsock(c.vm, timeout)
	if err != nil {
		return err
	}
	metrics.RecordVsockLatency("connect", float64(time.Since(start).Microseconds())/1000.0)
	c.conn = conn
	return nil
}

func (c *VsockClient) initLocked() error {
	if c.initPayload == nil {
		return errors.New("missing init payload")
	}
	if err := c.sendLocked(&VsockMessage{Type: MsgTypeInit, Payload: c.initPayload}); err != nil {
		return err
	}
	resp, err := c.receiveLocked()
	if err != nil {
		return err
	}
	if resp.Type != MsgTypeResp {
		return fmt.Errorf("unexpected response type: %d", resp.Type)
	}
	return nil
}

func (c *VsockClient) redialAndInitLocked(timeout time.Duration) error {
	hadConn := c.conn != nil
	_ = c.closeLocked()
	// Small delay after closing to let the vsock proxy clean up.
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

// Connect establishes a vsock connection without sending an Init message.
// Used for sandbox mode where the agent is already running and doesn't
// require function initialization.
func (c *VsockClient) Connect(timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.closeLocked()
	return c.dialLocked(timeout)
}

// ConnectIfNeeded establishes a vsock connection only if not already connected.
func (c *VsockClient) ConnectIfNeeded(timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return nil
	}
	return c.dialLocked(timeout)
}

func (c *VsockClient) Send(msg *VsockMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sendLocked(msg)
}

func (c *VsockClient) sendLocked(msg *VsockMessage) error {
	if c.conn == nil {
		return errors.New("vsock not connected")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Batch length prefix and data into single write to reduce syscalls
	buf := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(data)))
	copy(buf[4:], data)

	start := time.Now()
	err = writeFull(c.conn, buf)
	metrics.RecordVsockLatency("send", float64(time.Since(start).Microseconds())/1000.0)
	return err
}

func (c *VsockClient) Receive() (*VsockMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.receiveLocked()
}

func (c *VsockClient) receiveLocked() (*VsockMessage, error) {
	if c.conn == nil {
		return nil, errors.New("vsock not connected")
	}

	start := time.Now()

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, lenBuf); err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint32(lenBuf)
	if msgLen > maxVsockMessageBytes {
		return nil, fmt.Errorf("vsock message too large: %d bytes", msgLen)
	}
	data := make([]byte, msgLen)
	if _, err := io.ReadFull(c.conn, data); err != nil {
		return nil, err
	}

	metrics.RecordVsockLatency("receive", float64(time.Since(start).Microseconds())/1000.0)

	var msg VsockMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (c *VsockClient) Init(fn *domain.Function) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, err := backend.MarshalInitPayload(fn)
	if err != nil {
		return fmt.Errorf("marshal init payload: %w", err)
	}
	c.initPayload = payload
	if err := c.redialAndInitLocked(5 * time.Second); err != nil {
		return err
	}
	// Close connection after init. Execute() will establish a fresh connection.
	return c.closeLocked()
}

func (c *VsockClient) Execute(reqID string, input json.RawMessage, timeoutS int) (*RespPayload, error) {
	return c.ExecuteWithTrace(reqID, input, timeoutS, "", "")
}

// ExecuteWithTrace executes a request with optional W3C trace context propagation
func (c *VsockClient) ExecuteWithTrace(reqID string, input json.RawMessage, timeoutS int, traceParent, traceState string) (*RespPayload, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, _ := json.Marshal(&ExecPayload{
		RequestID:   reqID,
		Input:       input,
		TimeoutS:    timeoutS,
		TraceParent: traceParent,
		TraceState:  traceState,
	})

	execMsg := &VsockMessage{Type: MsgTypeExec, Payload: payload}

	// Exponential backoff: 10ms, 25ms, 50ms
	backoff := []time.Duration{10 * time.Millisecond, 25 * time.Millisecond, 50 * time.Millisecond}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := c.redialAndInitLocked(5 * time.Second); err != nil {
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

		var result RespPayload
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
	return nil, errors.New("vsock execute failed")
}

// ExecuteStream executes a function in streaming mode, calling the callback for each chunk
func (c *VsockClient) ExecuteStream(reqID string, input json.RawMessage, timeoutS int, traceParent, traceState string, callback func(chunk []byte, isLast bool, err error) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, _ := json.Marshal(&ExecPayload{
		RequestID:   reqID,
		Input:       input,
		TimeoutS:    timeoutS,
		TraceParent: traceParent,
		TraceState:  traceState,
		Stream:      true,
	})

	execMsg := &VsockMessage{Type: MsgTypeExec, Payload: payload}

	// Connect and send request
	if err := c.redialAndInitLocked(5 * time.Second); err != nil {
		return err
	}

	deadline := time.Now().Add(time.Duration(timeoutS+5) * time.Second)
	_ = c.conn.SetDeadline(deadline)

	if err := c.sendLocked(execMsg); err != nil {
		_ = c.closeLocked()
		return err
	}

	// Receive stream chunks
	for {
		resp, err := c.receiveLocked()
		if err != nil {
			_ = c.closeLocked()
			return err
		}

		if resp.Type != MsgTypeStream {
			_ = c.closeLocked()
			return fmt.Errorf("unexpected message type: %d (expected stream)", resp.Type)
		}

		var chunk StreamChunkPayload
		if err := json.Unmarshal(resp.Payload, &chunk); err != nil {
			_ = c.closeLocked()
			return err
		}

		// Call callback with chunk
		var chunkErr error
		if chunk.Error != "" {
			chunkErr = fmt.Errorf("%s", chunk.Error)
		}
		if err := callback(chunk.Data, chunk.IsLast, chunkErr); err != nil {
			_ = c.closeLocked()
			return err
		}

		// If this is the last chunk, we're done
		if chunk.IsLast {
			break
		}
	}

	_ = c.conn.SetDeadline(time.Time{})
	_ = c.closeLocked()
	return nil
}

func (c *VsockClient) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.redialAndInitLocked(5 * time.Second); err != nil {
		return err
	}
	defer c.closeLocked()

	_ = c.conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err := c.sendLocked(&VsockMessage{Type: MsgTypePing}); err != nil {
		return err
	}
	_, err := c.receiveLocked()
	_ = c.conn.SetDeadline(time.Time{})
	return err
}

// Reload sends new code files to the agent for hot reload
func (c *VsockClient) Reload(files map[string][]byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	type reloadPayload struct {
		Files map[string][]byte `json:"files"`
	}

	payload, err := json.Marshal(&reloadPayload{Files: files})
	if err != nil {
		return err
	}

	if err := c.redialAndInitLocked(5 * time.Second); err != nil {
		return err
	}
	defer c.closeLocked()

	_ = c.conn.SetDeadline(time.Now().Add(30 * time.Second))
	if err := c.sendLocked(&VsockMessage{Type: MsgTypeReload, Payload: payload}); err != nil {
		return err
	}

	resp, err := c.receiveLocked()
	_ = c.conn.SetDeadline(time.Time{})
	if err != nil {
		return err
	}

	if resp.Type != MsgTypeResp {
		return fmt.Errorf("unexpected response type: %d", resp.Type)
	}

	// Check for error in the response payload
	var respBody struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(resp.Payload, &respBody) == nil && respBody.Error != "" {
		return fmt.Errorf("reload failed: %s", respBody.Error)
	}
	return nil
}

func dialVsock(vm *VM, timeout time.Duration) (net.Conn, error) {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.Dial("unix", vm.VsockPath)
	if err != nil {
		return nil, err
	}
	if err := sendVsockConnect(conn, defaultVsockPort, timeout); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func sendVsockConnect(conn net.Conn, port int, timeout time.Duration) error {
	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}
	if _, err := fmt.Fprintf(conn, "CONNECT %d\n", port); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(line, "OK") {
		return fmt.Errorf("vsock connect failed: %s", strings.TrimSpace(line))
	}
	if timeout > 0 {
		_ = conn.SetDeadline(time.Time{})
	}
	return nil
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
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.ENOTCONN))
}
