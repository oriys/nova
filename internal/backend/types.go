package backend

import (
	"encoding/json"
	"os/exec"
	"sync"
	"time"

	"github.com/oriys/nova/internal/domain"
)

// VMState represents the state of a VM or container.
type VMState string

const (
	VMStateCreating VMState = "creating"
	VMStateRunning  VMState = "running"
	VMStatePaused   VMState = "paused"
	VMStateStopped  VMState = "stopped"
)

// VM represents a running VM or container.
type VM struct {
	ID                string
	Runtime           domain.Runtime
	State             VMState
	CID               uint32    // Firecracker CID
	SocketPath        string    // Firecracker API socket
	VsockPath         string    // Firecracker vsock path
	CodeDrive         string    // Path to code drive (Firecracker)
	TapDevice         string    // TAP device (Firecracker)
	GuestIP           string    // Guest IP (Firecracker)
	GuestMAC          string    // Guest MAC (Firecracker)
	Cmd               *exec.Cmd // Firecracker process
	DockerContainerID string    // Docker container ID
	AssignedPort      int       // Docker: host port mapped to agent
	CodeDir           string    // Docker: mounted code directory
	KubePod           string    // Kubernetes: pod name
	KubeNS            string    // Kubernetes: namespace
	CreatedAt         time.Time
	LastUsed          time.Time
	mu                sync.RWMutex
}

// Lock acquires the write lock on the VM.
func (vm *VM) Lock() {
	vm.mu.Lock()
}

// Unlock releases the write lock on the VM.
func (vm *VM) Unlock() {
	vm.mu.Unlock()
}

// RLock acquires the read lock on the VM.
func (vm *VM) RLock() {
	vm.mu.RLock()
}

// RUnlock releases the read lock on the VM.
func (vm *VM) RUnlock() {
	vm.mu.RUnlock()
}

// RespPayload is the response from a function execution.
type RespPayload struct {
	RequestID  string          `json:"request_id"`
	Output     json.RawMessage `json:"output"`
	Error      string          `json:"error,omitempty"`
	DurationMs int64           `json:"duration_ms"`
	Stdout     string          `json:"stdout,omitempty"`
	Stderr     string          `json:"stderr,omitempty"`
}

// Protocol message types (shared with agent).
const (
	MsgTypeInit   = 1
	MsgTypeExec   = 2
	MsgTypeResp   = 3
	MsgTypePing   = 4
	MsgTypeStop   = 5
	MsgTypeReload = 6 // Hot reload code files
	MsgTypeStream = 7 // Streaming response chunk
)

// VsockMessage is the wire format for agent communication.
type VsockMessage struct {
	Type    int             `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// InitPayload is sent to initialize the agent.
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

	// InternalInvokeEnabled tells the agent to expose an internal invoke
	// capability (e.g. via environment variable NOVA_INVOKE_ENDPOINT) so
	// that user functions can call other functions through the host without
	// routing through the public gateway.
	InternalInvokeEnabled bool `json:"internal_invoke_enabled,omitempty"`
}

// VolumeMountInfo tells the agent where to mount a volume drive inside the VM.
type VolumeMountInfo struct {
	MountPath string `json:"mount_path"` // guest mount point (e.g., /mnt/data)
	ReadOnly  bool   `json:"read_only"`
}

// ExecPayload is sent to execute a function.
type ExecPayload struct {
	RequestID   string          `json:"request_id"`
	Input       json.RawMessage `json:"input"`
	TimeoutS    int             `json:"timeout_s"`
	TraceParent string          `json:"traceparent,omitempty"`
	TraceState  string          `json:"tracestate,omitempty"`
	Stream      bool            `json:"stream,omitempty"` // Enable streaming response

	// InternalInvoke, when true, indicates that the function context should
	// expose an "invoke" capability so the running function can call other
	// functions via the host (vsock -> Comet) without going through the
	// public gateway.
	InternalInvoke bool `json:"internal_invoke,omitempty"`
}

// ReloadPayload is sent to hot-reload function code.
type ReloadPayload struct {
	Files map[string][]byte `json:"files"` // relative path -> content
}

// StreamChunkPayload is sent for streaming responses.
type StreamChunkPayload struct {
	RequestID string `json:"request_id"`
	Data      []byte `json:"data"`      // Chunk of data (can be stdout, binary, or text)
	IsLast    bool   `json:"is_last"`   // True if this is the final chunk
	Error     string `json:"error,omitempty"` // Error message if execution failed
}
