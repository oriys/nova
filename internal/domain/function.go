package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"time"
)

type Runtime string

const (
	RuntimePython Runtime = "python"
	RuntimeGo     Runtime = "go"
	RuntimeRust   Runtime = "rust"
	RuntimeWasm   Runtime = "wasm"
)

// ExecutionMode determines how functions are executed
type ExecutionMode string

const (
	// ModeProcess forks a new process for each invocation (default)
	ModeProcess ExecutionMode = "process"
	// ModePersistent keeps function process alive for connection reuse
	ModePersistent ExecutionMode = "persistent"
)

func (r Runtime) IsValid() bool {
	switch r {
	case RuntimePython, RuntimeGo, RuntimeRust, RuntimeWasm:
		return true
	}
	return false
}

// ResourceLimits defines VM resource constraints
type ResourceLimits struct {
	VCPUs          int   `json:"vcpus,omitempty"`            // vCPU count (1-32, default: 1)
	DiskIOPS       int64 `json:"disk_iops,omitempty"`        // Max disk IOPS (0 = unlimited)
	DiskBandwidth  int64 `json:"disk_bandwidth,omitempty"`   // Max disk bandwidth bytes/s (0 = unlimited)
	NetRxBandwidth int64 `json:"net_rx_bandwidth,omitempty"` // Max network RX bytes/s (0 = unlimited)
	NetTxBandwidth int64 `json:"net_tx_bandwidth,omitempty"` // Max network TX bytes/s (0 = unlimited)
}

type Function struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Runtime     Runtime           `json:"runtime"`
	Handler     string            `json:"handler"`
	CodePath    string            `json:"code_path"`
	CodeHash    string            `json:"code_hash,omitempty"` // SHA256 hash of code file for change detection
	MemoryMB    int               `json:"memory_mb"`
	TimeoutS    int               `json:"timeout_s"`
	MinReplicas int               `json:"min_replicas"`
	MaxReplicas int               `json:"max_replicas,omitempty"` // Maximum concurrent VMs (0 = unlimited)
	Mode        ExecutionMode     `json:"mode,omitempty"`         // "process" or "persistent"
	Limits      *ResourceLimits   `json:"limits,omitempty"`
	EnvVars     map[string]string `json:"env_vars,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`

	// Versioning
	Version        int               `json:"version"`                   // Current version number (1, 2, 3, ...)
	VersionAlias   string            `json:"version_alias,omitempty"`   // Alias for this version (e.g., "stable", "canary")
	ActiveVersions []int             `json:"active_versions,omitempty"` // List of active versions
	TrafficSplit   map[int]int       `json:"traffic_split,omitempty"`   // version -> percentage (must sum to 100)
}

// FunctionVersion represents a specific version of a function
type FunctionVersion struct {
	FunctionID  string            `json:"function_id"`
	Version     int               `json:"version"`
	CodePath    string            `json:"code_path"`
	Handler     string            `json:"handler"`
	MemoryMB    int               `json:"memory_mb"`
	TimeoutS    int               `json:"timeout_s"`
	Mode        ExecutionMode     `json:"mode,omitempty"`
	Limits      *ResourceLimits   `json:"limits,omitempty"`
	EnvVars     map[string]string `json:"env_vars,omitempty"`
	Description string            `json:"description,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// FunctionAlias maps an alias name to a specific version or traffic split
type FunctionAlias struct {
	FunctionID   string      `json:"function_id"`
	Name         string      `json:"name"`  // e.g., "latest", "stable", "canary"
	Version      int         `json:"version,omitempty"` // Single version target
	TrafficSplit map[int]int `json:"traffic_split,omitempty"` // version -> percentage for gradual rollout
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

func (f *Function) MarshalBinary() ([]byte, error) {
	return json.Marshal(f)
}

func (f *Function) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, f)
}

type InvokeRequest struct {
	FunctionID string          `json:"function_id"`
	Payload    json.RawMessage `json:"payload"`
}

type InvokeResponse struct {
	RequestID  string          `json:"request_id"`
	Output     json.RawMessage `json:"output"`
	Error      string          `json:"error,omitempty"`
	DurationMs int64           `json:"duration_ms"`
	ColdStart  bool            `json:"cold_start"`
	Version    int             `json:"version,omitempty"` // Which version handled this request
}

// HashCodeFile calculates SHA256 hash of a code file for change detection.
func HashCodeFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil))[:16], nil // Use first 16 chars for brevity
}

// CodeHashChanged checks if the code file has changed since the function was registered.
func (f *Function) CodeHashChanged() bool {
	if f.CodeHash == "" {
		return false // No hash stored, can't detect change
	}

	currentHash, err := HashCodeFile(f.CodePath)
	if err != nil {
		return false // Can't read file, assume unchanged
	}

	return currentHash != f.CodeHash
}
