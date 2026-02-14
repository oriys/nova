// Package kata provides a backend that uses Kata Containers for
// hardware-virtualised container isolation. Each function runs inside
// a lightweight VM managed through the kata-runtime OCI runtime,
// communicating with the nova-agent over TCP.
package kata

import (
	"os"
	"time"
)

// Config holds Kata Containers backend configuration.
type Config struct {
	CodeDir        string        // Base directory for function code
	ImagePrefix    string        // Container image prefix (e.g., "nova-runtime")
	RuntimeName    string        // OCI runtime name (default: "kata")
	Network        string        // Container network name (optional)
	PortRangeMin   int           // Minimum host port for agent mapping
	PortRangeMax   int           // Maximum host port for agent mapping
	CPULimit       float64       // CPU limit per container (default: 1.0)
	DefaultTimeout time.Duration // Default operation timeout
	AgentTimeout   time.Duration // Agent startup timeout
}

// DefaultConfig returns sensible defaults for the Kata Containers backend.
func DefaultConfig() *Config {
	codeDir := os.Getenv("NOVA_KATA_CODE_DIR")
	if codeDir == "" {
		codeDir = "/tmp/nova/kata-code"
	}
	imagePrefix := os.Getenv("NOVA_KATA_IMAGE_PREFIX")
	if imagePrefix == "" {
		imagePrefix = "nova-runtime"
	}
	runtimeName := os.Getenv("NOVA_KATA_RUNTIME")
	if runtimeName == "" {
		runtimeName = "kata"
	}

	return &Config{
		CodeDir:        codeDir,
		ImagePrefix:    imagePrefix,
		RuntimeName:    runtimeName,
		Network:        os.Getenv("NOVA_KATA_NETWORK"),
		PortRangeMin:   50000,
		PortRangeMax:   60000,
		CPULimit:       1.0,
		DefaultTimeout: 30 * time.Second,
		AgentTimeout:   15 * time.Second,
	}
}
