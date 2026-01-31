package config

import (
	"encoding/json"
	"os"
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

// Config is the central configuration struct embedding all component configs
type Config struct {
	Firecracker firecracker.Config `json:"firecracker"`
	Redis       RedisConfig        `json:"redis"`
	Pool        PoolConfig         `json:"pool"`
	Daemon      DaemonConfig       `json:"daemon"`
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
}
