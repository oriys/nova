// Package stateproxy provides a TCP/vsock listener that handles state
// requests from guest agents running inside VMs. Each VM connects to
// this listener to proxy state operations (get/put/delete/list) to the
// host-side PostgreSQL state store.
package stateproxy

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

const (
	MsgTypeStateGet    = 8
	MsgTypeStatePut    = 9
	MsgTypeStateDelete = 10
	MsgTypeStateList   = 11
	MsgTypeStateResp   = 12

	maxMessageSize = 1 << 20 // 1MB
)

type Message struct {
	Type    int             `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type StateRequest struct {
	Key             string          `json:"key"`
	Value           json.RawMessage `json:"value,omitempty"`
	Prefix          string          `json:"prefix,omitempty"`
	TTLSeconds      int             `json:"ttl_s,omitempty"`
	ExpectedVersion int64           `json:"expected_version,omitempty"`
	Limit           int             `json:"limit,omitempty"`
}

type StateResponse struct {
	Key     string          `json:"key,omitempty"`
	Value   json.RawMessage `json:"value,omitempty"`
	Version int64           `json:"version,omitempty"`
	Error   string          `json:"error,omitempty"`
	Entries []StateEntry    `json:"entries,omitempty"`
}

type StateEntry struct {
	Key     string          `json:"key"`
	Value   json.RawMessage `json:"value"`
	Version int64           `json:"version"`
}

// Listener manages the state proxy TCP listener.
type Listener struct {
	store      *store.Store
	listener   net.Listener
	functionID string // default function ID for state scoping
}

// Start begins listening on the given address for state proxy connections.
func Start(addr string, s *store.Store) (*Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("state proxy listen: %w", err)
	}

	l := &Listener{
		store:    s,
		listener: ln,
	}
	go l.acceptLoop()
	return l, nil
}

// Close stops the listener.
func (l *Listener) Close() error {
	return l.listener.Close()
}

func (l *Listener) acceptLoop() {
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			return // listener closed
		}
		go l.handleConn(conn)
	}
}

func (l *Listener) handleConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// Read single request-response per connection
	msg, err := readMessage(conn)
	if err != nil {
		logging.Op().Warn("state proxy read error", "error", err)
		return
	}

	resp := l.handleMessage(msg)
	if err := writeMessage(conn, resp); err != nil {
		logging.Op().Warn("state proxy write error", "error", err)
	}
}

func (l *Listener) handleMessage(msg *Message) *Message {
	var req StateRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return stateError("invalid request: " + err.Error())
	}

	ctx := context.Background()

	switch msg.Type {
	case MsgTypeStateGet:
		return l.handleGet(ctx, &req)
	case MsgTypeStatePut:
		return l.handlePut(ctx, &req)
	case MsgTypeStateDelete:
		return l.handleDelete(ctx, &req)
	case MsgTypeStateList:
		return l.handleList(ctx, &req)
	default:
		return stateError(fmt.Sprintf("unknown state message type: %d", msg.Type))
	}
}

func (l *Listener) handleGet(ctx context.Context, req *StateRequest) *Message {
	// Extract function ID from request context or use default
	funcID := l.resolveFunctionID(req)
	if funcID == "" {
		return stateError("function ID not available")
	}

	entry, err := l.store.GetFunctionState(ctx, funcID, req.Key)
	if err != nil {
		return stateError("not found")
	}

	return stateResponse(&StateResponse{
		Key:     entry.Key,
		Value:   entry.Value,
		Version: entry.Version,
	})
}

func (l *Listener) handlePut(ctx context.Context, req *StateRequest) *Message {
	funcID := l.resolveFunctionID(req)
	if funcID == "" {
		return stateError("function ID not available")
	}

	opts := &store.FunctionStatePutOptions{
		ExpectedVersion: req.ExpectedVersion,
	}
	if req.TTLSeconds > 0 {
		opts.TTL = time.Duration(req.TTLSeconds) * time.Second
	}

	entry, err := l.store.PutFunctionState(ctx, funcID, req.Key, req.Value, opts)
	if err != nil {
		if err.Error() == store.ErrFunctionStateVersionConflict.Error() {
			return stateError("conflict")
		}
		return stateError("put failed: " + err.Error())
	}

	return stateResponse(&StateResponse{
		Key:     entry.Key,
		Value:   entry.Value,
		Version: entry.Version,
	})
}

func (l *Listener) handleDelete(ctx context.Context, req *StateRequest) *Message {
	funcID := l.resolveFunctionID(req)
	if funcID == "" {
		return stateError("function ID not available")
	}

	if err := l.store.DeleteFunctionState(ctx, funcID, req.Key); err != nil {
		return stateError("delete failed: " + err.Error())
	}

	return stateResponse(&StateResponse{})
}

func (l *Listener) handleList(ctx context.Context, req *StateRequest) *Message {
	funcID := l.resolveFunctionID(req)
	if funcID == "" {
		return stateError("function ID not available")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	entries, err := l.store.ListFunctionStates(ctx, funcID, &store.FunctionStateListOptions{
		Prefix: req.Prefix,
		Limit:  limit,
	})
	if err != nil {
		return stateError("list failed: " + err.Error())
	}

	result := make([]StateEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, StateEntry{
			Key:     e.Key,
			Value:   e.Value,
			Version: e.Version,
		})
	}

	return stateResponse(&StateResponse{Entries: result})
}

func (l *Listener) resolveFunctionID(_ *StateRequest) string {
	// In a production setup, the function ID would be extracted from the
	// connection metadata (e.g. the VM's vsock CID maps to a function).
	// For now, we use a default context-based approach where the function
	// ID is embedded in the connection context by the pool.
	return l.functionID
}

// SetFunctionID sets the default function ID for this listener.
// In production, this would be per-connection based on VM identity.
func (l *Listener) SetFunctionID(id string) {
	l.functionID = id
}

func stateError(msg string) *Message {
	resp := &StateResponse{Error: msg}
	data, _ := json.Marshal(resp)
	return &Message{Type: MsgTypeStateResp, Payload: data}
}

func stateResponse(resp *StateResponse) *Message {
	data, _ := json.Marshal(resp)
	return &Message{Type: MsgTypeStateResp, Payload: data}
}

func readMessage(conn net.Conn) (*Message, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint32(lenBuf)
	if msgLen > maxMessageSize {
		return nil, fmt.Errorf("message too large: %d", msgLen)
	}

	data := make([]byte, msgLen)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}

	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func writeMessage(conn net.Conn, msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	buf := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(data)))
	copy(buf[4:], data)
	_, err = conn.Write(buf)
	return err
}
