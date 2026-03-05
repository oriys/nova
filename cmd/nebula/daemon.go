package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/asyncqueue"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/cluster"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/docker"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/eventbus"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/kubernetes"
	"github.com/oriys/nova/internal/applevz"
	"github.com/oriys/nova/internal/libkrun"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/logsink"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/queue"
	"github.com/oriys/nova/internal/secrets"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/triggers"
	"github.com/oriys/nova/internal/wasm"
	"github.com/oriys/nova/internal/workflow"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func daemonCmd() *cobra.Command {
	var (
		logLevel   string
		cometAddr  string
		grpcAddr   string
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

			pgStore, err := store.NewPostgresStore(context.Background(), cfg.Postgres.DSN, store.PoolOptions{MaxConns: cfg.Postgres.MaxConn, MinConns: cfg.Postgres.MinConn})
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
			case "redis", "redis-list":
				redisClient := redis.NewClient(&redis.Options{
					Addr: cfg.Queue.RedisAddr,
					DB:   cfg.Queue.RedisDB,
				})
				if err := redisClient.Ping(context.Background()).Err(); err != nil {
					return fmt.Errorf("connect to redis for queue notifier: %w", err)
				}
				if cfg.Queue.NotifierType == "redis-list" {
					notifier = queue.NewRedisListNotifier(redisClient)
					logging.Op().Info("using Redis list queue notifier (push-pull)", "addr", cfg.Queue.RedisAddr)
				} else {
					notifier = queue.NewRedisNotifier(redisClient)
					logging.Op().Info("using Redis queue notifier", "addr", cfg.Queue.RedisAddr)
				}
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
				var vzManager *applevz.Manager
				defaultBackend := domain.BackendType(cfg.Firecracker.Backend)
				if defaultBackend == "" || defaultBackend == domain.BackendAuto {
					detected := backend.DetectDefaultBackend()
					defaultBackend = detected
					logging.Op().Info("auto-detected backend", "backend", defaultBackend)
				}
				if defaultBackend == domain.BackendType("k8s") {
					defaultBackend = domain.BackendKubernetes
				}
				factories := map[domain.BackendType]backend.BackendFactory{
					domain.BackendDocker: func() (backend.Backend, error) { return docker.NewManager(&cfg.Docker) },
					domain.BackendWasm:   func() (backend.Backend, error) { return wasm.NewManager(&cfg.Wasm) },
					domain.BackendKubernetes: func() (backend.Backend, error) {
						return kubernetes.NewManager(&cfg.Kubernetes)
					},
					domain.BackendLibKrun: func() (backend.Backend, error) { return libkrun.NewManager(&cfg.LibKrun) },
					domain.BackendAppleVZ: func() (backend.Backend, error) {
						mgr, err := applevz.NewManager(&cfg.AppleVZ)
						if err != nil {
							return nil, err
						}
						vzManager = mgr
						return mgr, nil
					},
					domain.BackendFirecracker: func() (backend.Backend, error) {
						adapter, err := firecracker.NewAdapter(&cfg.Firecracker)
						if err != nil {
							return nil, err
						}
						fcAdapter = adapter
						return adapter, nil
					},
				}
				router, err := backend.NewRouter(defaultBackend, factories)
				if err != nil {
					return err
				}
				if err := router.EnsureReady(defaultBackend); err != nil {
					return err
				}
				be = router
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
				if vzManager != nil && vzManager.SnapshotDir() != "" {
					p.SetSnapshotCallback(func(ctx context.Context, vmID, funcID string) error {
						if err := vzManager.CreateSnapshot(ctx, vmID, funcID); err != nil {
							return err
						}
						return vzManager.ResumeVM(ctx, vmID)
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
					executor.WithPayloadPersistence(cfg.Observability.OutputCapture.Enabled),
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

			// Async queue workers
			asyncWorkers := asyncqueue.New(s, invoker, asyncqueue.Config{
				Workers:      cfg.Queue.Workers,
				PollInterval: cfg.Queue.PollInterval,
				BatchSize:    cfg.Queue.BatchSize,
				Notifier:     notifier,
				Adaptive: asyncqueue.AdaptiveConfig{
					Enabled:         cfg.Queue.AdaptiveEnabled,
					MinWorkers:      cfg.Queue.AdaptiveMinWorkers,
					MaxWorkers:      cfg.Queue.AdaptiveMaxWorkers,
					MinPollInterval: cfg.Queue.AdaptiveMinPoll,
					MaxPollInterval: cfg.Queue.AdaptiveMaxPoll,
					ProbeInterval:   cfg.Queue.AdaptiveProbeInterval,
				},
			})

			// Event bus workers
			eventWorkers := eventbus.New(s, invoker, wfService, eventbus.Config{
				Notifier: notifier,
			})

			// Outbox relay
			outboxRelay := eventbus.NewOutboxRelay(s, eventbus.OutboxRelayConfig{
				Notifier: notifier,
			})

			// Leader election — only one Nebula instance runs workers to
			// prevent duplicate event/async processing in multi-instance deployments.
			nebulaNodeID := os.Getenv("NOVA_CLUSTER_NODE_ID")
			if nebulaNodeID == "" {
				nebulaNodeID = "nebula-local"
			}
			leaderElector := cluster.NewLeaderElector(pgStore.Pool(), cluster.LeaderConfig{
				LockKey: cluster.WellKnownLockKeys.Nebula,
				NodeID:  nebulaNodeID,
				OnElected: func() {
					wfEngine.Start()
					asyncWorkers.Start()
					eventWorkers.Start()
					outboxRelay.Start()
					logging.Op().Info("nebula workers started (leader elected)")
				},
				OnRevoked: func() {
					outboxRelay.Stop()
					eventWorkers.Stop()
					asyncWorkers.Stop()
					wfEngine.Stop()
					logging.Op().Info("nebula workers stopped (leadership revoked)")
				},
			})
			stopElection := leaderElector.Start(context.Background())
			defer stopElection()
			defer func() {
				outboxRelay.Stop()
				eventWorkers.Stop()
				asyncWorkers.Stop()
				wfEngine.Stop()
			}()

			// Trigger manager: load persisted triggers and start connectors.
			// Requires a local Executor (not just Invoker) because the trigger
			// manager dispatches via executor.Invoke directly.
			if localExec != nil {
				triggerMgr := triggers.NewManager(s, localExec)
				storedTriggers, err := s.ListTriggers(context.Background(), 1000, 0)
				if err != nil {
					logging.Op().Warn("failed to load triggers from store", "error", err)
				} else {
					for _, rec := range storedTriggers {
						t := &triggers.Trigger{
							ID:           rec.ID,
							TenantID:     rec.TenantID,
							Namespace:    rec.Namespace,
							Name:         rec.Name,
							Type:         triggers.TriggerType(rec.Type),
							FunctionID:   rec.FunctionID,
							FunctionName: rec.FunctionName,
							Enabled:      rec.Enabled,
							Config:       rec.Config,
							CreatedAt:    rec.CreatedAt,
							UpdatedAt:    rec.UpdatedAt,
						}
						if err := triggerMgr.RegisterTrigger(t); err != nil {
							logging.Op().Warn("failed to register trigger", "trigger", t.Name, "error", err)
						}
					}
					if len(storedTriggers) > 0 {
						logging.Op().Info("loaded triggers from store", "count", len(storedTriggers))
					}
				}
				defer triggerMgr.Shutdown()
			}

			// Start gRPC server with health service
			var grpcServer *grpc.Server
			if grpcAddr != "" {
				grpcServer = grpc.NewServer()
				healthSrv := health.NewServer()
				grpc_health_v1.RegisterHealthServer(grpcServer, healthSrv)
				healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

				lis, err := net.Listen("tcp", grpcAddr)
				if err != nil {
					return fmt.Errorf("listen gRPC %s: %w", grpcAddr, err)
				}
				go func() {
					logging.Op().Info("Nebula gRPC endpoint started", "addr", grpcAddr)
					if err := grpcServer.Serve(lis); err != nil {
						logging.Op().Error("nebula gRPC server error", "error", err)
					}
				}()
			}

			logging.Op().Info("Nebula event ingestion plane started")

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			logging.Op().Info("shutdown signal received")

			if grpcServer != nil {
				grpcServer.GracefulStop()
			}

			if localExec != nil {
				localExec.Shutdown(10 * time.Second)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")
	cmd.Flags().StringVar(&cometAddr, "comet-grpc", "", "Comet gRPC address for remote invocation (e.g. comet:9090)")
	cmd.Flags().StringVar(&grpcAddr, "grpc", "", "gRPC listen address for health checks (optional)")

	return cmd
}
