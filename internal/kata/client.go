package kata

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
)

const (
	// maxMessageSize is the maximum allowed message size (16MB).
	maxMessageSize = 16 * 1024 * 1024
)
// Client communicates with the nova-agent inside a Kata Container via TCP.
type Client struct {
	vm          *backend.VM
	conn        net.Conn
	mu          sync.Mutex
	initPayload json.RawMessage
}

// NewClient creates a new TCP client for the Kata Container.
func NewClient(vm *backend.VM) (*Client, error) {
	return &Client{vm: vm}, nil
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

// ExecuteStream runs a function in streaming mode, calling callback for each chunk.
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
			chunkErr = errors.New(chunk.Error)
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
	if msgLen > maxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", msgLen, maxMessageSize)
	}
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
