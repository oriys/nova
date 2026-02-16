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
)

type VsockMessage struct {
	Type    int             `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type InitPayload struct {
	Runtime         string            `json:"runtime"`
	Handler         string            `json:"handler"`
	EnvVars         map[string]string `json:"env_vars"`
	Command         []string          `json:"command,omitempty"`
	Extension       string            `json:"extension,omitempty"`
	Mode            string            `json:"mode,omitempty"`
	FunctionName    string            `json:"function_name,omitempty"`
	FunctionVersion int               `json:"function_version,omitempty"`
	MemoryMB        int               `json:"memory_mb,omitempty"`
	TimeoutS        int               `json:"timeout_s,omitempty"`
	LayerCount      int               `json:"layer_count,omitempty"`
	VolumeMounts    []VolumeMountInfo `json:"volume_mounts,omitempty"`
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
	TraceParent string          `json:"traceparent,omitempty"` // W3C TraceContext
	TraceState  string          `json:"tracestate,omitempty"`  // W3C TraceContext
	Stream      bool            `json:"stream,omitempty"`       // Enable streaming response
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

	// Build volume mount info for the agent
	var volumeMounts []VolumeMountInfo
	for _, rm := range fn.ResolvedMounts {
		volumeMounts = append(volumeMounts, VolumeMountInfo{
			MountPath: rm.MountPath,
			ReadOnly:  rm.ReadOnly,
		})
	}

	payload, _ := json.Marshal(&InitPayload{
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
		LayerCount:      len(fn.LayerPaths),
		VolumeMounts:    volumeMounts,
	})
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
