package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/oriys/nova/internal/autoscaler"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/cluster"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/docker"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/kubernetes"
	"github.com/oriys/nova/internal/applevz"
	"github.com/oriys/nova/internal/libkrun"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/scheduler"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/wasm"
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
		Short: "Run Corona scheduler / placement plane daemon",
		Long:  "Run Corona as the scheduler and autoscaler service (cron, placement, scaling decisions)",
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
				cfg.Observability.Tracing.ServiceName = "corona"
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

			// Determine the invoker: remote (via Comet gRPC) or local.
			var invoker executor.Invoker
			var localPool *pool.Pool
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
				var be backend.Backend
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
				localPool = p
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
				invoker = executor.New(s, p, executor.WithPayloadPersistence(cfg.Observability.OutputCapture.Enabled))
				logging.Op().Info("using local executor (no --comet-grpc specified)")
			}

			// Start scheduler
			sched := scheduler.New(s, invoker)

			// Leader election — only one Corona instance runs the scheduler.
			nodeID := os.Getenv("NOVA_CLUSTER_NODE_ID")
			if nodeID == "" {
				nodeID = "corona-local"
			}
			leaderElector := cluster.NewLeaderElector(pgStore.Pool(), cluster.LeaderConfig{
				LockKey: cluster.WellKnownLockKeys.Corona,
				NodeID:  nodeID,
				OnElected: func() {
					if err := sched.Start(); err != nil {
						logging.Op().Warn("failed to start scheduler after election", "error", err)
					} else {
						logging.Op().Info("scheduler started (leader elected)")
					}
				},
				OnRevoked: func() {
					sched.Stop()
					logging.Op().Info("scheduler stopped (leadership revoked)")
				},
			})
			stopElection := leaderElector.Start(context.Background())
			defer stopElection()
			defer sched.Stop()

			// Start autoscaler (needs pool — only available in local mode)
			if cfg.AutoScale.Enabled && localPool != nil {
				as := autoscaler.New(localPool, s, cfg.AutoScale.Interval)
				as.Start()
				defer as.Stop()
				logging.Op().Info("autoscaler started (local mode)")
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
					logging.Op().Info("Corona gRPC endpoint started", "addr", grpcAddr)
					if err := grpcServer.Serve(lis); err != nil {
						logging.Op().Error("corona gRPC server error", "error", err)
					}
				}()
			}

			logging.Op().Info("Corona scheduler/placement plane started")

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			logging.Op().Info("shutdown signal received")

			if grpcServer != nil {
				grpcServer.GracefulStop()
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")
	cmd.Flags().StringVar(&cometAddr, "comet-grpc", "", "Comet gRPC address for remote invocation (e.g. comet:9090)")
	cmd.Flags().StringVar(&grpcAddr, "grpc", "", "gRPC listen address for health checks (optional)")

	return cmd
}
