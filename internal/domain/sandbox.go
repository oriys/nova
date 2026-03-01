package domain

import "time"

// SandboxStatus represents the lifecycle state of a sandbox.
type SandboxStatus string

const (
	SandboxStatusCreating SandboxStatus = "creating"
	SandboxStatusRunning  SandboxStatus = "running"
	SandboxStatusPaused   SandboxStatus = "paused"
	SandboxStatusStopped  SandboxStatus = "stopped"
	SandboxStatusError    SandboxStatus = "error"
)

// Sandbox represents a long-lived VM session for LLM/AI agent code execution.
type Sandbox struct {
	ID            string            `json:"id"`
	Template      string            `json:"template"`       // runtime template: python, node, ubuntu, go
	Status        SandboxStatus     `json:"status"`
	MemoryMB      int               `json:"memory_mb"`
	VCPUs         int               `json:"vcpus"`
	TimeoutS      int               `json:"timeout_s"`      // max lifetime in seconds
	OnIdleS       int               `json:"on_idle_s"`      // auto-destroy after idle seconds
	NetworkPolicy string            `json:"network_policy"` // "restricted", "open"
	EnvVars       map[string]string `json:"env_vars,omitempty"`
	VMID          string            `json:"vm_id,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	LastActiveAt  time.Time         `json:"last_active_at"`
	ExpiresAt     time.Time         `json:"expires_at"`
	Error         string            `json:"error,omitempty"`
}

// CreateSandboxRequest is the request body for POST /sandboxes.
type CreateSandboxRequest struct {
	Template      string            `json:"template"`
	MemoryMB      int               `json:"memory_mb,omitempty"`
	VCPUs         int               `json:"vcpus,omitempty"`
	TimeoutS      int               `json:"timeout_s,omitempty"`
	NetworkPolicy string            `json:"network_policy,omitempty"`
	EnvVars       map[string]string `json:"env_vars,omitempty"`
	OnIdleS       int               `json:"on_idle_s,omitempty"`
}

// SandboxExecRequest is the request body for POST /sandboxes/{id}/exec.
type SandboxExecRequest struct {
	Command  string `json:"command"`
	TimeoutS int    `json:"timeout_s,omitempty"`
	WorkDir  string `json:"workdir,omitempty"`
}

// SandboxExecResponse is the response for shell execution.
type SandboxExecResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// SandboxCodeRequest is the request body for POST /sandboxes/{id}/code.
type SandboxCodeRequest struct {
	Code     string `json:"code"`
	Language string `json:"language"` // python, javascript, bash, etc.
	TimeoutS int    `json:"timeout_s,omitempty"`
}

// SandboxFileInfo represents a file or directory entry.
type SandboxFileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
}

// SandboxFileWriteRequest is the request body for PUT /sandboxes/{id}/files.
type SandboxFileWriteRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"` // base64-encoded for binary, raw for text
	IsB64   bool   `json:"is_base64,omitempty"`
	Perm    int    `json:"perm,omitempty"` // file permissions, default 0644
}

// SandboxProcessInfo represents a running process inside the sandbox.
type SandboxProcessInfo struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
	CPU     string `json:"cpu,omitempty"`
	Memory  string `json:"memory,omitempty"`
}
