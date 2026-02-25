// Package libkrun provides a backend that uses libkrun for KVM-based
// process isolation. It launches each function inside a lightweight
// microVM, communicating with the nova-agent over TCP.
//
// On Linux the backend uses the `krun` CLI with ext4 rootfs images.
// On macOS it uses `krunvm` (Apple Virtualization Framework) with OCI
// container images pulled from a registry.
package libkrun

import (
	"os"
	"runtime"
	"time"
)

// Config holds libkrun backend configuration.
type Config struct {
	CodeDir        string        `json:"code_dir"`        // Base directory for function code
	RootfsDir      string        `json:"rootfs_dir"`      // Directory containing rootfs images (Linux/krun only)
	AgentPath      string        `json:"agent_path"`      // Path to nova-agent binary (mounted into VM on Linux)
	ImagePrefix    string        `json:"image_prefix"`    // OCI image prefix for krunvm (macOS), e.g. "localhost:5555/nova-runtime-"
	PortRangeMin   int           `json:"port_range_min"`  // Minimum host port for agent mapping
	PortRangeMax   int           `json:"port_range_max"`  // Maximum host port for agent mapping
	DefaultTimeout time.Duration `json:"default_timeout"` // Default operation timeout
	AgentTimeout   time.Duration `json:"agent_timeout"`   // Agent startup timeout
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
	imagePrefix := os.Getenv("NOVA_LIBKRUN_IMAGE_PREFIX")
	if imagePrefix == "" {
		imagePrefix = "localhost:5555/nova-runtime-"
	}

	return &Config{
		CodeDir:        codeDir,
		RootfsDir:      rootfsDir,
		AgentPath:      agentPath,
		ImagePrefix:    imagePrefix,
		PortRangeMin:   40000,
		PortRangeMax:   50000,
		DefaultTimeout: 30 * time.Second,
		AgentTimeout:   10 * time.Second,
	}
}

// UseKrunVM returns true when the macOS krunvm path should be used
// instead of the Linux krun binary.
func UseKrunVM() bool {
	return runtime.GOOS == "darwin"
}
