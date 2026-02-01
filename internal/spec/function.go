package spec

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/oriys/nova/internal/domain"
	"gopkg.in/yaml.v3"
)

// FunctionSpec defines the YAML specification for a function
type FunctionSpec struct {
	// API version for future compatibility
	APIVersion string `yaml:"apiVersion,omitempty"`
	// Kind is always "Function"
	Kind string `yaml:"kind,omitempty"`

	// Metadata
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`

	// Runtime configuration
	Runtime string `yaml:"runtime"` // python, go, rust, wasm, node, ruby, java, php, dotnet, deno, bun
	Handler string `yaml:"handler,omitempty"`
	Code    string `yaml:"code"` // Path to code file or directory

	// Resource configuration
	Memory  int `yaml:"memory,omitempty"`  // Memory in MB (default: 128)
	Timeout int `yaml:"timeout,omitempty"` // Timeout in seconds (default: 30)

	// Scaling configuration
	MinReplicas int `yaml:"minReplicas,omitempty"` // Minimum warm replicas
	MaxReplicas int `yaml:"maxReplicas,omitempty"` // Maximum concurrent VMs (0 = unlimited)

	// Execution mode
	Mode string `yaml:"mode,omitempty"` // process, persistent

	// Environment variables (supports $SECRET:name references)
	Env map[string]string `yaml:"env,omitempty"`

	// Resource limits
	Limits *ResourceLimitsSpec `yaml:"limits,omitempty"`
}

// ResourceLimitsSpec defines resource limits in YAML
type ResourceLimitsSpec struct {
	VCPUs          int    `yaml:"vcpus,omitempty"`
	DiskIOPS       int64  `yaml:"diskIOPS,omitempty"`
	DiskBandwidth  string `yaml:"diskBandwidth,omitempty"`  // e.g., "100MB/s", "1GB/s"
	NetRxBandwidth string `yaml:"netRxBandwidth,omitempty"` // e.g., "100MB/s"
	NetTxBandwidth string `yaml:"netTxBandwidth,omitempty"` // e.g., "100MB/s"
}

// MultiSpec holds multiple function specs from a single file
type MultiSpec struct {
	Functions []FunctionSpec
}

// ParseFile parses a YAML file containing one or more function specs
func ParseFile(path string) (*MultiSpec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	return Parse(f, filepath.Dir(path))
}

// Parse parses YAML content containing one or more function specs
func Parse(r io.Reader, baseDir string) (*MultiSpec, error) {
	decoder := yaml.NewDecoder(r)
	var specs []FunctionSpec

	for {
		var spec FunctionSpec
		err := decoder.Decode(&spec)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode yaml: %w", err)
		}

		// Skip empty documents
		if spec.Name == "" && spec.Runtime == "" {
			continue
		}

		// Resolve relative code paths
		if spec.Code != "" && !filepath.IsAbs(spec.Code) {
			spec.Code = filepath.Join(baseDir, spec.Code)
		}

		specs = append(specs, spec)
	}

	if len(specs) == 0 {
		return nil, fmt.Errorf("no valid function specs found")
	}

	return &MultiSpec{Functions: specs}, nil
}

// Validate validates a function spec
func (s *FunctionSpec) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}
	if s.Runtime == "" {
		return fmt.Errorf("runtime is required")
	}
	if s.Code == "" {
		return fmt.Errorf("code path is required")
	}

	// Validate runtime
	rt := domain.Runtime(s.Runtime)
	if !rt.IsValid() {
		return fmt.Errorf("invalid runtime: %s (valid: python, go, rust, wasm, node, ruby, java, php, dotnet, deno, bun)", s.Runtime)
	}

	// Check code path exists
	if _, err := os.Stat(s.Code); os.IsNotExist(err) {
		return fmt.Errorf("code path not found: %s", s.Code)
	}

	// Validate mode if specified
	if s.Mode != "" && s.Mode != "process" && s.Mode != "persistent" {
		return fmt.Errorf("invalid mode: %s (valid: process, persistent)", s.Mode)
	}

	return nil
}

// ToFunction converts a FunctionSpec to a domain.Function
func (s *FunctionSpec) ToFunction(id string) (*domain.Function, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	fn := &domain.Function{
		ID:          id,
		Name:        s.Name,
		Runtime:     domain.Runtime(s.Runtime),
		Handler:     s.Handler,
		CodePath:    s.Code,
		MemoryMB:    s.Memory,
		TimeoutS:    s.Timeout,
		MinReplicas: s.MinReplicas,
		MaxReplicas: s.MaxReplicas,
		Mode:        domain.ExecutionMode(s.Mode),
		EnvVars:     s.Env,
	}

	// Apply defaults
	if fn.Handler == "" {
		fn.Handler = "main.handler"
	}
	if fn.MemoryMB == 0 {
		fn.MemoryMB = 128
	}
	if fn.TimeoutS == 0 {
		fn.TimeoutS = 30
	}
	if fn.Mode == "" {
		fn.Mode = domain.ModeProcess
	}

	// Convert resource limits
	if s.Limits != nil {
		fn.Limits = &domain.ResourceLimits{
			VCPUs:          s.Limits.VCPUs,
			DiskIOPS:       s.Limits.DiskIOPS,
			DiskBandwidth:  parseBandwidth(s.Limits.DiskBandwidth),
			NetRxBandwidth: parseBandwidth(s.Limits.NetRxBandwidth),
			NetTxBandwidth: parseBandwidth(s.Limits.NetTxBandwidth),
		}
	}

	// Calculate code hash
	if hash, err := domain.HashCodeFile(fn.CodePath); err == nil {
		fn.CodeHash = hash
	}

	return fn, nil
}

// parseBandwidth parses bandwidth strings like "100MB/s", "1GB/s" to bytes/s
func parseBandwidth(s string) int64 {
	if s == "" {
		return 0
	}

	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "/s")
	s = strings.ToUpper(s)

	var multiplier int64 = 1
	if strings.HasSuffix(s, "GB") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	} else if strings.HasSuffix(s, "MB") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	} else if strings.HasSuffix(s, "KB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	} else if strings.HasSuffix(s, "B") {
		s = strings.TrimSuffix(s, "B")
	}

	var value int64
	fmt.Sscanf(s, "%d", &value)
	return value * multiplier
}

// ExampleYAML returns an example YAML spec
func ExampleYAML() string {
	return `# Nova Function Specification
apiVersion: nova/v1
kind: Function

name: hello-world
description: A simple hello world function
runtime: python
handler: main.handler
code: ./handler.py

# Resources
memory: 256      # MB
timeout: 30      # seconds

# Scaling
minReplicas: 1
maxReplicas: 10

# Execution mode: process (default) or persistent
mode: process

# Environment variables (use $SECRET:name for secrets)
env:
  LOG_LEVEL: info
  DATABASE_URL: $SECRET:database_url

# Resource limits (optional)
limits:
  vcpus: 2
  diskIOPS: 1000
  diskBandwidth: 100MB/s
  netRxBandwidth: 50MB/s
  netTxBandwidth: 50MB/s
`
}
