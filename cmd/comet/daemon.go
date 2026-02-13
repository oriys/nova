package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/asyncqueue"
	"github.com/oriys/nova/internal/autoscaler"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/docker"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	novagrpc "github.com/oriys/nova/internal/grpc"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/secrets"
	"github.com/oriys/nova/internal/store"
	"github.com/spf13/cobra"
)

func daemonCmd() *cobra.Command {
	var (
		idleTTL  time.Duration
		grpcAddr string
		logLevel string
	)

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run Comet data plane daemon",
		Long:  "Run comet as a data plane daemon with executor, pool, and gRPC invocation API",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.DefaultConfig()
			if configFile != "" {
				var err error
				cfg, err = config.LoadFromFile(configFile)
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
			}
			config.LoadFromEnv(cfg)

			if cmd.Flags().Changed("pg-dsn") {
				cfg.Postgres.DSN = pgDSN
			}
			if cmd.Flags().Changed("idle-ttl") {
				cfg.Pool.IdleTTL = idleTTL
			}
			if cmd.Flags().Changed("grpc") {
				cfg.GRPC.Addr = grpcAddr
			}
			if cmd.Flags().Changed("log-level") {
				cfg.Daemon.LogLevel = logLevel
			}

			cfg.GRPC.Enabled = true
			if cfg.GRPC.Addr == "" {
				cfg.GRPC.Addr = ":9090"
			}
			if cfg.Observability.Tracing.ServiceName == "" || cfg.Observability.Tracing.ServiceName == "nova" {
				cfg.Observability.Tracing.ServiceName = "comet"
			}

			logging.SetLevelFromString(cfg.Daemon.LogLevel)
			logging.InitStructured(cfg.Observability.Logging.Format, cfg.Observability.Logging.Level)

			if err := observability.Init(context.Background(), observability.Config{
				Enabled:     cfg.Observability.Tracing.Enabled,
				Exporter:    cfg.Observability.Tracing.Exporter,
				Endpoint:    cfg.Observability.Tracing.Endpoint,
				ServiceName: cfg.Observability.Tracing.ServiceName,
				SampleRate:  cfg.Observability.Tracing.SampleRate,
			}); err != nil {
				return fmt.Errorf("init tracing: %w", err)
			}
			defer observability.Shutdown(context.Background())

			if cfg.Observability.Metrics.Enabled {
				metrics.InitPrometheus(cfg.Observability.Metrics.Namespace, cfg.Observability.Metrics.HistogramBuckets)
			}

			if cfg.Observability.OutputCapture.Enabled {
				if err := logging.InitOutputStore(
					cfg.Observability.OutputCapture.StorageDir,
					cfg.Observability.OutputCapture.MaxSize,
					cfg.Observability.OutputCapture.RetentionS,
				); err != nil {
					logging.Op().Warn("failed to init output capture", "error", err)
				}
			}

			pgStore, err := store.NewPostgresStore(context.Background(), cfg.Postgres.DSN)
			if err != nil {
				return err
			}
			cachedStore := store.NewCachedMetadataStore(pgStore, store.DefaultCacheTTL)
			s := store.NewStore(cachedStore)
			defer s.Close()

			var be backend.Backend
			var fcAdapter *firecracker.Adapter
			switch cfg.Firecracker.Backend {
			case "docker":
				logging.Op().Info("using Docker backend")
				dockerMgr, err := docker.NewManager(&cfg.Docker)
				if err != nil {
					return err
				}
				be = dockerMgr
			default:
				logging.Op().Info("using Firecracker backend")
				adapter, err := firecracker.NewAdapter(&cfg.Firecracker)
				if err != nil {
					return err
				}
				fcAdapter = adapter
				be = adapter
			}

			p := pool.NewPool(be, pool.PoolConfig{
				IdleTTL:             cfg.Pool.IdleTTL,
				CleanupInterval:     cfg.Pool.CleanupInterval,
				HealthCheckInterval: cfg.Pool.HealthCheckInterval,
				MaxPreWarmWorkers:   cfg.Pool.MaxPreWarmWorkers,
			})
			if fcAdapter != nil {
				mgr := fcAdapter.Manager()
				p.SetSnapshotCallback(func(ctx context.Context, vmID, funcID string) error {
					if err := mgr.CreateSnapshot(ctx, vmID, funcID); err != nil {
						return err
					}
					return mgr.ResumeVM(ctx, vmID)
				})
			}

			// Load max_global_vms from config store
			if sysConfig, err := s.GetConfig(context.Background()); err == nil {
				if v, ok := sysConfig["max_global_vms"]; ok {
					if n, err := strconv.Atoi(v); err == nil && n >= 0 {
						p.SetMaxGlobalVMs(n)
						logging.Op().Info("loaded max_global_vms from config", "value", n)
					}
				}
			}

			if cfg.AutoScale.Enabled {
				as := autoscaler.New(p, s, cfg.AutoScale.Interval)
				as.Start()
				defer as.Stop()
			}

			var secretsResolver *secrets.Resolver
			if cfg.Secrets.Enabled || cfg.Secrets.MasterKey != "" || cfg.Secrets.MasterKeyFile != "" {
				var cipher *secrets.Cipher
				if cfg.Secrets.MasterKey != "" {
					cipher, err = secrets.NewCipher(cfg.Secrets.MasterKey)
				} else if cfg.Secrets.MasterKeyFile != "" {
					cipher, err = secrets.NewCipherFromFile(cfg.Secrets.MasterKeyFile)
				}
				if err != nil {
					logging.Op().Warn("failed to initialize secrets", "error", err)
				} else if cipher != nil {
					secretsResolver = secrets.NewResolver(secrets.NewStore(s, cipher))
					logging.Op().Info("secrets management enabled")
				}
			}

			execOpts := []executor.Option{
				executor.WithLogBatcherConfig(executor.LogBatcherConfig{
					BatchSize:     cfg.Executor.LogBatchSize,
					BufferSize:    cfg.Executor.LogBufferSize,
					FlushInterval: cfg.Executor.LogFlushInterval,
					Timeout:       cfg.Executor.LogTimeout,
				}),
			}
			if secretsResolver != nil {
				execOpts = append(execOpts, executor.WithSecretsResolver(secretsResolver))
			}
			exec := executor.New(s, p, execOpts...)

			asyncWorkers := asyncqueue.New(s, exec, asyncqueue.Config{})
			asyncWorkers.Start()

			grpcServer := novagrpc.NewServer(s, exec, p)
			if err := grpcServer.Start(cfg.GRPC.Addr); err != nil {
				return fmt.Errorf("start Comet gRPC server: %w", err)
			}
			logging.Op().Info("Comet gRPC API started", "addr", cfg.GRPC.Addr)

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-sigCh:
					logging.Op().Info("shutdown signal received")
					grpcServer.Stop()
					asyncWorkers.Stop()
					exec.Shutdown(10 * time.Second)
					be.Shutdown()
					return nil
				case <-ticker.C:
					ctx := context.Background()
					funcs, err := s.ListFunctions(ctx, 0, 0)
					if err != nil {
						logging.Op().Error("error listing functions", "error", err)
						continue
					}
					for _, fn := range funcs {
						codeRecord, err := s.GetFunctionCode(ctx, fn.ID)
						if err != nil || codeRecord == nil {
							logging.Op().Debug("skipping function with no code", "function", fn.Name)
							continue
						}
						codeContent := codeRecord.CompiledBinary
						if len(codeContent) == 0 {
							codeContent = []byte(codeRecord.SourceCode)
						}
						if err := p.EnsureReady(ctx, fn, codeContent); err != nil {
							logging.Op().Error("error ensuring ready", "function", fn.Name, "error", err)
						}
					}
				}
			}
		},
	}

	cmd.Flags().DurationVar(&idleTTL, "idle-ttl", 60*time.Second, "Worker idle timeout")
	cmd.Flags().StringVar(&grpcAddr, "grpc", ":9090", "Comet gRPC address")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")

	return cmd
}
