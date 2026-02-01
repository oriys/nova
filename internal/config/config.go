package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oriys/nova/internal/firecracker"
)

// RedisConfig holds Redis connection settings
type RedisConfig struct {
	Addr     string `json:"addr"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

// PoolConfig holds VM pool settings
type PoolConfig struct {
	IdleTTL time.Duration `json:"idle_ttl"`
}

// DaemonConfig holds daemon-specific settings
type DaemonConfig struct {
	HTTPAddr string `json:"http_addr"`
	LogLevel string `json:"log_level"`
}

// TracingConfig holds OpenTelemetry tracing settings
type TracingConfig struct {
	Enabled     bool    `json:"enabled"`      // Default: false
	Exporter    string  `json:"exporter"`     // otlp-http, otlp-grpc, stdout
	Endpoint    string  `json:"endpoint"`     // localhost:4318
	ServiceName string  `json:"service_name"` // nova
	SampleRate  float64 `json:"sample_rate"`  // 1.0
}

// MetricsConfig holds Prometheus metrics settings
type MetricsConfig struct {
	Enabled          bool      `json:"enabled"`           // Default: true
	Namespace        string    `json:"namespace"`         // nova
	HistogramBuckets []float64 `json:"histogram_buckets"` // Latency buckets in ms
}

// LoggingConfig holds structured logging settings
type LoggingConfig struct {
	Level          string `json:"level"`            // debug, info, warn, error
	Format         string `json:"format"`           // text, json
	IncludeTraceID bool   `json:"include_trace_id"` // Correlate with traces
}

// OutputCaptureConfig holds function output capture settings
type OutputCaptureConfig struct {
	Enabled    bool   `json:"enabled"`     // Default: false
	MaxSize    int64  `json:"max_size"`    // 1MB
	StorageDir string `json:"storage_dir"` // /tmp/nova/output
	RetentionS int    `json:"retention_s"` // 3600
}

// ObservabilityConfig holds all observability-related settings
type ObservabilityConfig struct {
	Tracing       TracingConfig       `json:"tracing"`
	Metrics       MetricsConfig       `json:"metrics"`
	Logging       LoggingConfig       `json:"logging"`
	OutputCapture OutputCaptureConfig `json:"output_capture"`
}

// GRPCConfig holds gRPC server settings
type GRPCConfig struct {
	Enabled bool   `json:"enabled"` // Default: false
	Addr    string `json:"addr"`    // :9090
}

// Config is the central configuration struct embedding all component configs
type Config struct {
	Firecracker   firecracker.Config  `json:"firecracker"`
	Redis         RedisConfig         `json:"redis"`
	Pool          PoolConfig          `json:"pool"`
	Daemon        DaemonConfig        `json:"daemon"`
	Observability ObservabilityConfig `json:"observability"`
	GRPC          GRPCConfig          `json:"grpc"`
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	fcCfg := firecracker.DefaultConfig()
	return &Config{
		Firecracker: *fcCfg,
		Redis: RedisConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		},
		Pool: PoolConfig{
			IdleTTL: 60 * time.Second,
		},
		Daemon: DaemonConfig{
			HTTPAddr: "",
			LogLevel: "info",
		},
		Observability: ObservabilityConfig{
			Tracing: TracingConfig{
				Enabled:     false,
				Exporter:    "otlp-http",
				Endpoint:    "localhost:4318",
				ServiceName: "nova",
				SampleRate:  1.0,
			},
			Metrics: MetricsConfig{
				Enabled:          true,
				Namespace:        "nova",
				HistogramBuckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
			},
			Logging: LoggingConfig{
				Level:          "info",
				Format:         "text",
				IncludeTraceID: true,
			},
			OutputCapture: OutputCaptureConfig{
				Enabled:    false,
				MaxSize:    1 << 20, // 1MB
				StorageDir: "/tmp/nova/output",
				RetentionS: 3600,
			},
		},
		GRPC: GRPCConfig{
			Enabled: false,
			Addr:    ":9090",
		},
	}
}

// LoadFromFile loads configuration from a JSON file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadFromEnv applies environment variable overrides to the config
func LoadFromEnv(cfg *Config) {
	if v := os.Getenv("NOVA_REDIS_ADDR"); v != "" {
		cfg.Redis.Addr = v
	}
	if v := os.Getenv("NOVA_REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
	if v := os.Getenv("NOVA_HTTP_ADDR"); v != "" {
		cfg.Daemon.HTTPAddr = v
	}
	if v := os.Getenv("NOVA_LOG_LEVEL"); v != "" {
		cfg.Daemon.LogLevel = v
	}
	if v := os.Getenv("NOVA_FIRECRACKER_BIN"); v != "" {
		cfg.Firecracker.FirecrackerBin = v
	}
	if v := os.Getenv("NOVA_KERNEL_PATH"); v != "" {
		cfg.Firecracker.KernelPath = v
	}
	if v := os.Getenv("NOVA_ROOTFS_DIR"); v != "" {
		cfg.Firecracker.RootfsDir = v
	}
	if v := os.Getenv("NOVA_SNAPSHOT_DIR"); v != "" {
		cfg.Firecracker.SnapshotDir = v
	}

	// Observability overrides
	if v := os.Getenv("NOVA_TRACING_ENABLED"); v != "" {
		cfg.Observability.Tracing.Enabled = parseBool(v)
	}
	if v := os.Getenv("NOVA_TRACING_ENDPOINT"); v != "" {
		cfg.Observability.Tracing.Endpoint = v
	}
	if v := os.Getenv("NOVA_TRACING_EXPORTER"); v != "" {
		cfg.Observability.Tracing.Exporter = v
	}
	if v := os.Getenv("NOVA_TRACING_SERVICE_NAME"); v != "" {
		cfg.Observability.Tracing.ServiceName = v
	}
	if v := os.Getenv("NOVA_TRACING_SAMPLE_RATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Observability.Tracing.SampleRate = f
		}
	}
	if v := os.Getenv("NOVA_METRICS_ENABLED"); v != "" {
		cfg.Observability.Metrics.Enabled = parseBool(v)
	}
	if v := os.Getenv("NOVA_METRICS_NAMESPACE"); v != "" {
		cfg.Observability.Metrics.Namespace = v
	}
	if v := os.Getenv("NOVA_LOG_FORMAT"); v != "" {
		cfg.Observability.Logging.Format = v
	}
	if v := os.Getenv("NOVA_LOG_INCLUDE_TRACE_ID"); v != "" {
		cfg.Observability.Logging.IncludeTraceID = parseBool(v)
	}
	if v := os.Getenv("NOVA_OUTPUT_CAPTURE_ENABLED"); v != "" {
		cfg.Observability.OutputCapture.Enabled = parseBool(v)
	}
	if v := os.Getenv("NOVA_OUTPUT_CAPTURE_DIR"); v != "" {
		cfg.Observability.OutputCapture.StorageDir = v
	}
	if v := os.Getenv("NOVA_OUTPUT_CAPTURE_MAX_SIZE"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Observability.OutputCapture.MaxSize = n
		}
	}
	if v := os.Getenv("NOVA_OUTPUT_CAPTURE_RETENTION"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Observability.OutputCapture.RetentionS = n
		}
	}

	// GRPC overrides
	if v := os.Getenv("NOVA_GRPC_ENABLED"); v != "" {
		cfg.GRPC.Enabled = parseBool(v)
	}
	if v := os.Getenv("NOVA_GRPC_ADDR"); v != "" {
		cfg.GRPC.Addr = v
	}
}

func parseBool(s string) bool {
	s = strings.ToLower(s)
	return s == "true" || s == "1" || s == "yes"
}
