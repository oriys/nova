// Package libkrun provides a backend that uses libkrun for KVM-based
// process isolation. It launches each function inside a lightweight
// microVM managed through the krun CLI, communicating with the
// nova-agent over TCP.
package libkrun

import (
	"os"
	"time"
)

// Config holds libkrun backend configuration.
type Config struct {
	CodeDir        string        // Base directory for function code
	RootfsDir      string        // Directory containing rootfs images
	AgentPath      string        // Path to nova-agent binary (mounted into VM)
	PortRangeMin   int           // Minimum host port for agent mapping
	PortRangeMax   int           // Maximum host port for agent mapping
	DefaultTimeout time.Duration // Default operation timeout
	AgentTimeout   time.Duration // Agent startup timeout
}

// DefaultConfig returns sensible defaults for the libkrun backend.
func DefaultConfig() *Config {
	codeDir := os.Getenv("NOVA_LIBKRUN_CODE_DIR")
	if codeDir == "" {
		codeDir = "/tmp/nova/libkrun-code"
	}
	rootfsDir := os.Getenv("NOVA_LIBKRUN_ROOTFS_DIR")
	if rootfsDir == "" {
		rootfsDir = "/opt/nova/rootfs"
	}
	agentPath := os.Getenv("NOVA_AGENT_PATH")
	if agentPath == "" {
		agentPath = "/opt/nova/bin/nova-agent"
	}

	return &Config{
		CodeDir:        codeDir,
		RootfsDir:      rootfsDir,
		AgentPath:      agentPath,
		PortRangeMin:   40000,
		PortRangeMax:   50000,
		DefaultTimeout: 30 * time.Second,
		AgentTimeout:   10 * time.Second,
	}
}
