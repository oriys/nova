package domain

import (
	"encoding/json"
	"time"
)

type Runtime string

const (
	RuntimePython Runtime = "python"
	RuntimeGo     Runtime = "go"
	RuntimeRust   Runtime = "rust"
	RuntimeWasm   Runtime = "wasm"
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
	VCPUs         int   `json:"vcpus,omitempty"`           // vCPU count (1-32, default: 1)
	DiskIOPS      int64 `json:"disk_iops,omitempty"`       // Max disk IOPS (0 = unlimited)
	DiskBandwidth int64 `json:"disk_bandwidth,omitempty"`  // Max disk bandwidth bytes/s (0 = unlimited)
	NetRxBandwidth int64 `json:"net_rx_bandwidth,omitempty"` // Max network RX bytes/s (0 = unlimited)
	NetTxBandwidth int64 `json:"net_tx_bandwidth,omitempty"` // Max network TX bytes/s (0 = unlimited)
}

type Function struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Runtime     Runtime           `json:"runtime"`
	Handler     string            `json:"handler"`
	CodePath    string            `json:"code_path"`
	MemoryMB    int               `json:"memory_mb"`
	TimeoutS    int               `json:"timeout_s"`
	MinReplicas int               `json:"min_replicas"`
	Limits      *ResourceLimits   `json:"limits,omitempty"`
	EnvVars     map[string]string `json:"env_vars,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
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
}
