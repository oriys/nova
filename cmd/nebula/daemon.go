package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/asyncqueue"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/docker"
	"github.com/oriys/nova/internal/eventbus"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/kubernetes"
	"github.com/oriys/nova/internal/libkrun"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/logsink"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/queue"
	"github.com/oriys/nova/internal/secrets"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/wasm"
	"github.com/oriys/nova/internal/workflow"
	"github.com/spf13/cobra"
)

func daemonCmd() *cobra.Command {
	var (
		logLevel  string
		cometAddr string
	)

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run Nebula event ingestion plane daemon",
		Long:  "Run Nebula as the event ingestion service (event bus, async queue, workflow engine)",
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
			if cmd.Flags().Changed("log-level") {
				cfg.Daemon.LogLevel = logLevel
			}

			logging.SetLevelFromString(cfg.Daemon.LogLevel)
			logging.InitStructured(cfg.Observability.Logging.Format, cfg.Observability.Logging.Level)

			if cfg.Observability.Tracing.ServiceName == "" || cfg.Observability.Tracing.ServiceName == "nova" {
				cfg.Observability.Tracing.ServiceName = "nebula"
			}
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

			pgStore, err := store.NewPostgresStore(context.Background(), cfg.Postgres.DSN)
			if err != nil {
				return err
			}
			cachedStore := store.NewCachedMetadataStore(pgStore, store.DefaultCacheTTL)
			s := store.NewStore(cachedStore)
			defer s.Close()

			// Initialize queue notifier for push-based task triggering
			var notifier queue.Notifier
			switch cfg.Queue.NotifierType {
			case "channel":
				notifier = queue.NewChannelNotifier()
			default:
				notifier = queue.NewNoopNotifier()
			}
			defer notifier.Close()

			// Initialize log sink
			var sink logsink.LogSink
			switch cfg.LogSink.Type {
			case "noop":
				sink = logsink.NewNoopSink()
			default:
				sink = logsink.NewPostgresSink(s)
			}

			// Determine the invoker: remote (via Comet gRPC) or local.
			var invoker executor.Invoker
			var localExec *executor.Executor
			var be backend.Backend

			if cometAddr != "" {
				remote, err := executor.NewRemoteInvoker(cometAddr)
				if err != nil {
					return fmt.Errorf("create remote invoker: %w", err)
				}
				defer remote.Close()
				invoker = remote
				logging.Op().Info("using remote invoker via Comet gRPC", "addr", cometAddr)
			} else {
				// Fallback: local executor with its own backend/pool.
				var fcAdapter *firecracker.Adapter
				switch cfg.Firecracker.Backend {
				case "docker":
					dockerMgr, err := docker.NewManager(&cfg.Docker)
					if err != nil {
						return err
					}
					be = dockerMgr
				case "wasm":
					wasmMgr, err := wasm.NewManager(&cfg.Wasm)
					if err != nil {
						return err
					}
					be = wasmMgr
				case "kubernetes", "k8s":
					k8sMgr, err := kubernetes.NewManager(&cfg.Kubernetes)
					if err != nil {
						return err
					}
					be = k8sMgr
				case "libkrun":
					libkrunMgr, err := libkrun.NewManager(&cfg.LibKrun)
					if err != nil {
						return err
					}
					be = libkrunMgr
				default:
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
				defer be.Shutdown()

				execOpts := []executor.Option{
					executor.WithLogBatcherConfig(executor.LogBatcherConfig{
						BatchSize:     cfg.Executor.LogBatchSize,
						BufferSize:    cfg.Executor.LogBufferSize,
						FlushInterval: cfg.Executor.LogFlushInterval,
						Timeout:       cfg.Executor.LogTimeout,
					}),
					executor.WithLogSink(sink),
				}

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
						secretsResolver := secrets.NewResolver(secrets.NewStore(s, cipher))
						execOpts = append(execOpts, executor.WithSecretsResolver(secretsResolver))
					}
				}

				localExec = executor.New(s, p, execOpts...)
				invoker = localExec
				logging.Op().Info("using local executor (no --comet-grpc specified)")
			}

			// Workflow service and engine
			wfService := workflow.NewService(s)
			wfEngine := workflow.NewEngine(s, invoker, workflow.EngineConfig{})
			wfEngine.Start()
			defer wfEngine.Stop()

			// Async queue workers
			asyncWorkers := asyncqueue.New(s, invoker, asyncqueue.Config{
				Notifier: notifier,
			})
			asyncWorkers.Start()
			defer asyncWorkers.Stop()

			// Event bus workers
			eventWorkers := eventbus.New(s, invoker, wfService, eventbus.Config{
				Notifier: notifier,
			})
			eventWorkers.Start()
			defer eventWorkers.Stop()

			// Outbox relay
			outboxRelay := eventbus.NewOutboxRelay(s, eventbus.OutboxRelayConfig{
				Notifier: notifier,
			})
			outboxRelay.Start()
			defer outboxRelay.Stop()

			logging.Op().Info("Nebula event ingestion plane started")

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			logging.Op().Info("shutdown signal received")

			if localExec != nil {
				localExec.Shutdown(10 * time.Second)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")
	cmd.Flags().StringVar(&cometAddr, "comet-grpc", "", "Comet gRPC address for remote invocation (e.g. comet:9090)")

	return cmd
}
