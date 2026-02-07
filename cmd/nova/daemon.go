package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/api"
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
	"github.com/oriys/nova/internal/workflow"
	"github.com/spf13/cobra"
)

func daemonCmd() *cobra.Command {
	var (
		idleTTL  time.Duration
		httpAddr string
		logLevel string
	)

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as daemon (keeps VMs warm, optional HTTP API)",
		Long:  "Run nova as a daemon that maintains warm VMs and optionally exposes an HTTP API",
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
			if cmd.Flags().Changed("http") {
				cfg.Daemon.HTTPAddr = httpAddr
			}
			if cmd.Flags().Changed("log-level") {
				cfg.Daemon.LogLevel = logLevel
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
				metrics.InitPrometheus(
					cfg.Observability.Metrics.Namespace,
					cfg.Observability.Metrics.HistogramBuckets,
				)
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
			s := store.NewStore(pgStore)
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

			var secretsResolver *secrets.Resolver
			if cfg.Secrets.Enabled || cfg.Secrets.MasterKey != "" || cfg.Secrets.MasterKeyFile != "" {
				var cipher *secrets.Cipher
				var err error
				if cfg.Secrets.MasterKey != "" {
					cipher, err = secrets.NewCipher(cfg.Secrets.MasterKey)
				} else if cfg.Secrets.MasterKeyFile != "" {
					cipher, err = secrets.NewCipherFromFile(cfg.Secrets.MasterKeyFile)
				}
				if err != nil {
					logging.Op().Warn("failed to initialize secrets", "error", err)
				} else if cipher != nil {
					secretsStore := secrets.NewStore(s, cipher)
					secretsResolver = secrets.NewResolver(secretsStore)
					logging.Op().Info("secrets management enabled")
				}
			}

			execOpts := []executor.Option{}
			if secretsResolver != nil {
				execOpts = append(execOpts, executor.WithSecretsResolver(secretsResolver))
			}
			execOpts = append(execOpts, executor.WithLogBatcherConfig(executor.LogBatcherConfig{
				BatchSize:     cfg.Executor.LogBatchSize,
				BufferSize:    cfg.Executor.LogBufferSize,
				FlushInterval: cfg.Executor.LogFlushInterval,
				Timeout:       cfg.Executor.LogTimeout,
			}))
			exec := executor.New(s, p, execOpts...)

			// Create workflow service and engine
			wfService := workflow.NewService(s)
			wfEngine := workflow.NewEngine(s, exec, workflow.EngineConfig{})
			wfEngine.Start()

			var httpServer *http.Server
			if cfg.Daemon.HTTPAddr != "" {
				httpServer = api.StartHTTPServer(cfg.Daemon.HTTPAddr, api.ServerConfig{
					Store:           s,
					Exec:            exec,
					Pool:            p,
					Backend:         be,
					FCAdapter:       fcAdapter,
					AuthCfg:         &cfg.Auth,
					RateLimitCfg:    &cfg.RateLimit,
					WorkflowService: wfService,
					RootfsDir:       cfg.Firecracker.RootfsDir,
				})
				logging.Op().Info("HTTP API started", "addr", cfg.Daemon.HTTPAddr)
			}

			var grpcServer *novagrpc.Server
			if cfg.GRPC.Enabled {
				grpcServer = novagrpc.NewServer(s, exec)
				if err := grpcServer.Start(cfg.GRPC.Addr); err != nil {
					return fmt.Errorf("start gRPC server: %w", err)
				}
				logging.Op().Info("gRPC API started", "addr", cfg.GRPC.Addr)
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-sigCh:
					logging.Op().Info("shutdown signal received")
					if grpcServer != nil {
						grpcServer.Stop()
					}
					if httpServer != nil {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						httpServer.Shutdown(ctx)
						cancel()
					}
					wfEngine.Stop()
					exec.Shutdown(10 * time.Second)
					be.Shutdown()
					return nil
				case <-ticker.C:
					ctx := context.Background()
					funcs, err := s.ListFunctions(ctx)
					if err != nil {
						logging.Op().Error("error listing functions", "error", err)
					} else {
						for _, fn := range funcs {
							// Fetch code content for pre-warming
							codeRecord, err := s.GetFunctionCode(ctx, fn.ID)
							if err != nil || codeRecord == nil {
								logging.Op().Debug("skipping function with no code", "function", fn.Name)
								continue
							}
							var codeContent []byte
							if len(codeRecord.CompiledBinary) > 0 {
								codeContent = codeRecord.CompiledBinary
							} else {
								codeContent = []byte(codeRecord.SourceCode)
							}
							if err := p.EnsureReady(ctx, fn, codeContent); err != nil {
								logging.Op().Error("error ensuring ready", "function", fn.Name, "error", err)
							}
						}
					}
					stats := p.Stats()
					logging.Op().Debug("daemon status", "active_vms", stats["active_vms"])
				}
			}
		},
	}

	cmd.Flags().DurationVar(&idleTTL, "idle-ttl", 60*time.Second, "VM idle timeout")
	cmd.Flags().StringVar(&httpAddr, "http", "", "HTTP API address")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")

	return cmd
}