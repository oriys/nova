package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oriys/nova/internal/docker"
	"github.com/oriys/nova/internal/firecracker"
)

// PostgresConfig holds Postgres connection settings
type PostgresConfig struct {
	DSN string `json:"dsn"`
}

// PoolConfig holds VM pool settings
type PoolConfig struct {
	IdleTTL             time.Duration `json:"idle_ttl"`
	CleanupInterval     time.Duration `json:"cleanup_interval"`      // Interval for expired VM cleanup (default: 10s)
	HealthCheckInterval time.Duration `json:"health_check_interval"` // Interval for idle VM health checks (default: 30s)
	MaxPreWarmWorkers   int           `json:"max_prewarm_workers"`   // Max concurrent pre-warm goroutines (default: 8)
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

// ExecutorConfig holds executor settings
type ExecutorConfig struct {
	LogBatchSize     int           `json:"log_batch_size"`     // Number of logs batched before flushing (default: 100)
	LogBufferSize    int           `json:"log_buffer_size"`    // Channel buffer for pending logs (default: 1000)
	LogFlushInterval time.Duration `json:"log_flush_interval"` // Periodic flush interval (default: 500ms)
	LogTimeout       time.Duration `json:"log_timeout"`        // Database persistence timeout (default: 5s)
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

// AuthConfig holds authentication settings
type AuthConfig struct {
	Enabled     bool         `json:"enabled"`      // Default: false
	JWT         JWTConfig    `json:"jwt"`          // JWT authentication settings
	APIKeys     APIKeyConfig `json:"api_keys"`     // API Key authentication settings
	PublicPaths []string     `json:"public_paths"` // Paths that skip authentication
}

// JWTConfig holds JWT authentication settings
type JWTConfig struct {
	Enabled       bool   `json:"enabled"`         // Enable JWT authentication
	Algorithm     string `json:"algorithm"`       // HS256, RS256
	Secret        string `json:"secret"`          // HMAC secret key
	PublicKeyFile string `json:"public_key_file"` // RSA public key file path
	Issuer        string `json:"issuer"`          // Optional issuer claim validation
}

// APIKeyConfig holds API key authentication settings
type APIKeyConfig struct {
	Enabled    bool           `json:"enabled"`     // Enable API key authentication
	StaticKeys []StaticAPIKey `json:"static_keys"` // Static keys from config file
}

// StaticAPIKey represents an API key defined in config
type StaticAPIKey struct {
	Name string `json:"name"` // Key name/identifier
	Key  string `json:"key"`  // The API key value
	Tier string `json:"tier"` // Rate limit tier
}

// RateLimitConfig holds rate limiting settings
type RateLimitConfig struct {
	Enabled bool                       `json:"enabled"` // Default: false
	Tiers   map[string]TierLimitConfig `json:"tiers"`   // Named rate limit tiers
	Default TierLimitConfig            `json:"default"` // Default tier for unauthenticated/unmatched
}

// TierLimitConfig holds rate limit settings for a tier
type TierLimitConfig struct {
	RequestsPerSecond float64 `json:"requests_per_second"` // Token refill rate
	BurstSize         int     `json:"burst_size"`          // Maximum tokens (burst capacity)
}

// SecretsConfig holds secrets management settings
type SecretsConfig struct {
	Enabled       bool   `json:"enabled"`         // Default: false
	MasterKey     string `json:"master_key"`      // Hex-encoded 256-bit key
	MasterKeyFile string `json:"master_key_file"` // Path to file containing master key
}

// Config is the central configuration struct embedding all component configs
type Config struct {
	Firecracker   firecracker.Config  `json:"firecracker"`
	Docker        docker.Config       `json:"docker"`
	Postgres      PostgresConfig      `json:"postgres"`
	Pool          PoolConfig          `json:"pool"`
	Executor      ExecutorConfig      `json:"executor"`
	Daemon        DaemonConfig        `json:"daemon"`
	Observability ObservabilityConfig `json:"observability"`
	GRPC          GRPCConfig          `json:"grpc"`
	Auth          AuthConfig          `json:"auth"`
	RateLimit     RateLimitConfig     `json:"rate_limit"`
	Secrets       SecretsConfig       `json:"secrets"`
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	fcCfg := firecracker.DefaultConfig()
	dockerCfg := docker.DefaultConfig()
	return &Config{
		Firecracker: *fcCfg,
		Docker:      *dockerCfg,
		Postgres: PostgresConfig{
			DSN: "postgres://nova:nova@localhost:5432/nova?sslmode=disable",
		},
		Pool: PoolConfig{
			IdleTTL:             60 * time.Second,
			CleanupInterval:     10 * time.Second,
			HealthCheckInterval: 30 * time.Second,
			MaxPreWarmWorkers:   8,
		},
		Executor: ExecutorConfig{
			LogBatchSize:     100,
			LogBufferSize:    1000,
			LogFlushInterval: 500 * time.Millisecond,
			LogTimeout:       5 * time.Second,
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
		Auth: AuthConfig{
			Enabled: false,
			JWT: JWTConfig{
				Enabled:   false,
				Algorithm: "HS256",
			},
			APIKeys: APIKeyConfig{
				Enabled: false,
			},
			PublicPaths: []string{
				"/health",
				"/health/live",
				"/health/ready",
				"/health/startup",
			},
		},
		RateLimit: RateLimitConfig{
			Enabled: false,
			Tiers:   make(map[string]TierLimitConfig),
			Default: TierLimitConfig{
				RequestsPerSecond: 100,
				BurstSize:         200,
			},
		},
		Secrets: SecretsConfig{
			Enabled: false,
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
	if v := os.Getenv("NOVA_PG_DSN"); v != "" {
		cfg.Postgres.DSN = v
	}
	if v := os.Getenv("NOVA_POSTGRES_DSN"); v != "" {
		cfg.Postgres.DSN = v
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

	// Auth overrides
	if v := os.Getenv("NOVA_AUTH_ENABLED"); v != "" {
		cfg.Auth.Enabled = parseBool(v)
	}
	if v := os.Getenv("NOVA_AUTH_JWT_ENABLED"); v != "" {
		cfg.Auth.JWT.Enabled = parseBool(v)
	}
	if v := os.Getenv("NOVA_AUTH_JWT_SECRET"); v != "" {
		cfg.Auth.JWT.Secret = v
		cfg.Auth.JWT.Enabled = true
	}
	if v := os.Getenv("NOVA_AUTH_JWT_ALGORITHM"); v != "" {
		cfg.Auth.JWT.Algorithm = v
	}
	if v := os.Getenv("NOVA_AUTH_JWT_PUBLIC_KEY_FILE"); v != "" {
		cfg.Auth.JWT.PublicKeyFile = v
	}
	if v := os.Getenv("NOVA_AUTH_JWT_ISSUER"); v != "" {
		cfg.Auth.JWT.Issuer = v
	}
	if v := os.Getenv("NOVA_AUTH_APIKEYS_ENABLED"); v != "" {
		cfg.Auth.APIKeys.Enabled = parseBool(v)
	}

	// Rate limit overrides
	if v := os.Getenv("NOVA_RATELIMIT_ENABLED"); v != "" {
		cfg.RateLimit.Enabled = parseBool(v)
	}
	if v := os.Getenv("NOVA_RATELIMIT_DEFAULT_RPS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.RateLimit.Default.RequestsPerSecond = f
		}
	}
	if v := os.Getenv("NOVA_RATELIMIT_DEFAULT_BURST"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RateLimit.Default.BurstSize = n
		}
	}

	// Secrets overrides
	if v := os.Getenv("NOVA_SECRETS_ENABLED"); v != "" {
		cfg.Secrets.Enabled = parseBool(v)
	}
	if v := os.Getenv("NOVA_MASTER_KEY"); v != "" {
		cfg.Secrets.MasterKey = v
		cfg.Secrets.Enabled = true
	}
	if v := os.Getenv("NOVA_MASTER_KEY_FILE"); v != "" {
		cfg.Secrets.MasterKeyFile = v
	}

	// Firecracker VM overrides
	if v := os.Getenv("NOVA_CODE_DRIVE_SIZE_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Firecracker.CodeDriveSizeMB = n
		}
	}
	if v := os.Getenv("NOVA_MIN_CODE_DRIVE_SIZE_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Firecracker.MinCodeDriveSizeMB = n
		}
	}
	if v := os.Getenv("NOVA_VSOCK_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Firecracker.VsockPort = n
		}
	}
	if v := os.Getenv("NOVA_MAX_VSOCK_MESSAGE_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Firecracker.MaxVsockMessageMB = n
		}
	}

	// Pool overrides
	if v := os.Getenv("NOVA_POOL_IDLE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Pool.IdleTTL = d
		}
	}
	if v := os.Getenv("NOVA_POOL_CLEANUP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Pool.CleanupInterval = d
		}
	}
	if v := os.Getenv("NOVA_POOL_HEALTH_CHECK_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Pool.HealthCheckInterval = d
		}
	}
	if v := os.Getenv("NOVA_POOL_MAX_PREWARM_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Pool.MaxPreWarmWorkers = n
		}
	}

	// Docker backend overrides
	if v := os.Getenv("NOVA_DOCKER_PORT_RANGE_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Docker.PortRangeMin = n
		}
	}
	if v := os.Getenv("NOVA_DOCKER_PORT_RANGE_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Docker.PortRangeMax = n
		}
	}
	if v := os.Getenv("NOVA_DOCKER_CPU_LIMIT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Docker.CPULimit = f
		}
	}
	if v := os.Getenv("NOVA_DOCKER_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Docker.DefaultTimeout = d
		}
	}
	if v := os.Getenv("NOVA_DOCKER_AGENT_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Docker.AgentTimeout = d
		}
	}

	// Executor log batching overrides
	if v := os.Getenv("NOVA_EXECUTOR_LOG_BATCH_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Executor.LogBatchSize = n
		}
	}
	if v := os.Getenv("NOVA_EXECUTOR_LOG_BUFFER_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Executor.LogBufferSize = n
		}
	}
	if v := os.Getenv("NOVA_EXECUTOR_LOG_FLUSH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Executor.LogFlushInterval = d
		}
	}
	if v := os.Getenv("NOVA_EXECUTOR_LOG_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Executor.LogTimeout = d
		}
	}
}

func parseBool(s string) bool {
	s = strings.ToLower(s)
	return s == "true" || s == "1" || s == "yes"
}
