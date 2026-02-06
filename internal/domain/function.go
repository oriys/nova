package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

type Runtime string

var functionNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

const (
	RuntimePython   Runtime = "python"
	RuntimeGo       Runtime = "go"
	RuntimeRust     Runtime = "rust"
	RuntimeWasm     Runtime = "wasm"
	RuntimeNode     Runtime = "node"
	RuntimeRuby     Runtime = "ruby"
	RuntimeJava     Runtime = "java"
	RuntimeDeno     Runtime = "deno"
	RuntimeBun      Runtime = "bun"
	RuntimePHP      Runtime = "php"
	RuntimeDotnet   Runtime = "dotnet"
	RuntimeElixir   Runtime = "elixir"
	RuntimeKotlin   Runtime = "kotlin"
	RuntimeSwift    Runtime = "swift"
	RuntimeZig      Runtime = "zig"
	RuntimeLua      Runtime = "lua"
	RuntimePerl     Runtime = "perl"
	RuntimeR        Runtime = "r"
	RuntimeJulia    Runtime = "julia"
	RuntimeScala    Runtime = "scala"
	RuntimeCustom   Runtime = "custom"
	RuntimeProvided Runtime = "provided"
)

// ValidateFunctionName enforces the accepted function name format.
func ValidateFunctionName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if !functionNamePattern.MatchString(name) {
		return fmt.Errorf("invalid name: must match %s", functionNamePattern.String())
	}
	return nil
}

// ExecutionMode determines how functions are executed
type ExecutionMode string

const (
	// ModeProcess forks a new process for each invocation (default)
	ModeProcess ExecutionMode = "process"
	// ModePersistent keeps function process alive for connection reuse
	ModePersistent ExecutionMode = "persistent"
)

func (r Runtime) IsValid() bool {
	// Base runtime IDs
	validRuntimes := map[Runtime]bool{
		RuntimePython: true, RuntimeGo: true, RuntimeRust: true, RuntimeWasm: true,
		RuntimeNode: true, RuntimeRuby: true, RuntimeJava: true, RuntimeDeno: true, RuntimeBun: true,
		RuntimePHP: true, RuntimeDotnet: true, RuntimeElixir: true, RuntimeKotlin: true, RuntimeSwift: true,
		RuntimeZig: true, RuntimeLua: true, RuntimePerl: true, RuntimeR: true, RuntimeJulia: true, RuntimeScala: true,
		RuntimeCustom: true, RuntimeProvided: true,
	}
	if validRuntimes[r] {
		return true
	}
	// Versioned runtime IDs (e.g., python3.11, node20, go1.21)
	versionedPrefixes := []string{
		"python3.", "go1.", "node", "rust1.", "ruby3.", "ruby2.",
		"java", "php8.", "dotnet", "scala",
	}
	for _, prefix := range versionedPrefixes {
		if len(r) > len(prefix) && string(r)[:len(prefix)] == prefix {
			return true
		}
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
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Runtime             Runtime           `json:"runtime"`
	Handler             string            `json:"handler"`
	CodeHash            string            `json:"code_hash,omitempty"` // SHA256 hash of code for change detection
	MemoryMB            int               `json:"memory_mb"`
	TimeoutS            int               `json:"timeout_s"`
	MinReplicas         int               `json:"min_replicas"`
	MaxReplicas         int               `json:"max_replicas,omitempty"`         // Maximum concurrent VMs (0 = unlimited)
	InstanceConcurrency int               `json:"instance_concurrency,omitempty"` // Max in-flight requests per instance
	Mode                ExecutionMode     `json:"mode,omitempty"`                 // "process" or "persistent"
	Limits              *ResourceLimits   `json:"limits,omitempty"`
	EnvVars             map[string]string `json:"env_vars,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`

	// Versioning
	Version        int         `json:"version"`                   // Current version number (1, 2, 3, ...)
	VersionAlias   string      `json:"version_alias,omitempty"`   // Alias for this version (e.g., "stable", "canary")
	ActiveVersions []int       `json:"active_versions,omitempty"` // List of active versions
	TrafficSplit   map[int]int `json:"traffic_split,omitempty"`   // version -> percentage (must sum to 100)

	// Runtime metadata resolved at invocation time from runtimes table.
	RuntimeCommand   []string `json:"-"`
	RuntimeExtension string   `json:"-"`
	RuntimeImageName string   `json:"-"` // rootfs/image name from custom runtime config
}

// FunctionVersion represents a specific version of a function
type FunctionVersion struct {
	FunctionID  string            `json:"function_id"`
	Version     int               `json:"version"`
	CodeHash    string            `json:"code_hash"`
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
	Name         string      `json:"name"`                    // e.g., "latest", "stable", "canary"
	Version      int         `json:"version,omitempty"`       // Single version target
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

// CompileStatus represents the compilation status of function code
type CompileStatus string

const (
	CompileStatusPending     CompileStatus = "pending"
	CompileStatusCompiling   CompileStatus = "compiling"
	CompileStatusSuccess     CompileStatus = "success"
	CompileStatusFailed      CompileStatus = "failed"
	CompileStatusNotRequired CompileStatus = "not_required"
)

// FunctionCode represents source code and compiled binary for a function
type FunctionCode struct {
	FunctionID     string        `json:"function_id"`
	SourceCode     string        `json:"source_code"`
	CompiledBinary []byte        `json:"-"` // Not exposed in JSON
	SourceHash     string        `json:"source_hash"`
	BinaryHash     string        `json:"binary_hash,omitempty"`
	CompileStatus  CompileStatus `json:"compile_status"`
	CompileError   string        `json:"compile_error,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// NeedsCompilation returns true if the runtime requires compilation
func NeedsCompilation(runtime Runtime) bool {
	compiledRuntimes := map[Runtime]bool{
		RuntimeGo:     true,
		RuntimeRust:   true,
		RuntimeJava:   true,
		RuntimeKotlin: true,
		RuntimeSwift:  true,
		RuntimeZig:    true,
		RuntimeDotnet: true,
		RuntimeScala:  true,
	}
	if compiledRuntimes[runtime] {
		return true
	}

	rt := string(runtime)
	versionedPrefixes := []string{
		"go1.", "rust1.", "java", "dotnet", "scala",
	}
	for _, prefix := range versionedPrefixes {
		if len(rt) > len(prefix) && rt[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
