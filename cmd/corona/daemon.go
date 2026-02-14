package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/docker"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/kata"
	"github.com/oriys/nova/internal/kubernetes"
	"github.com/oriys/nova/internal/libkrun"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/scheduler"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/wasm"
	"github.com/spf13/cobra"
)

func daemonCmd() *cobra.Command {
	var (
		logLevel  string
		cometAddr string
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

			pgStore, err := store.NewPostgresStore(context.Background(), cfg.Postgres.DSN)
			if err != nil {
				return err
			}
			cachedStore := store.NewCachedMetadataStore(pgStore, store.DefaultCacheTTL)
			s := store.NewStore(cachedStore)
			defer s.Close()

			// Determine the invoker: remote (via Comet gRPC) or local.
			var invoker executor.Invoker
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
				case "kata":
					kataMgr, err := kata.NewManager(&cfg.Kata)
					if err != nil {
						return err
					}
					be = kataMgr
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
				invoker = executor.New(s, p)
				logging.Op().Info("using local executor (no --comet-grpc specified)")
			}

			// Start scheduler
			sched := scheduler.New(s, invoker)
			if err := sched.Start(); err != nil {
				logging.Op().Warn("failed to start scheduler", "error", err)
			}
			defer sched.Stop()

			// Start autoscaler (needs pool — only available in local mode)
			if cfg.AutoScale.Enabled && cometAddr == "" {
				// Autoscaler requires direct pool access; only in local mode.
				logging.Op().Info("autoscaler enabled (local mode)")
			}

			logging.Op().Info("Corona scheduler/placement plane started")

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			logging.Op().Info("shutdown signal received")
			return nil
		},
	}

	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")
	cmd.Flags().StringVar(&cometAddr, "comet-grpc", "", "Comet gRPC address for remote invocation (e.g. comet:9090)")

	return cmd
}
