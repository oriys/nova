package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/autoscaler"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/docker"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/kubernetes"
	"github.com/oriys/nova/internal/libkrun"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/scheduler"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/wasm"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

func daemonCmd() *cobra.Command {
	var (
		logLevel   string
		cometAddr  string
		listenAddr string
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
				defer be.Shutdown()
				invoker = executor.New(s, p, executor.WithPayloadPersistence(cfg.Observability.OutputCapture.Enabled))
				logging.Op().Info("using local executor (no --comet-grpc specified)")
			}

			// Start scheduler
			sched := scheduler.New(s, invoker)
			if err := sched.Start(); err != nil {
				logging.Op().Warn("failed to start scheduler", "error", err)
			}
			defer sched.Stop()

			// Start autoscaler (needs pool — only available in local mode)
			if cfg.AutoScale.Enabled && localPool != nil {
				as := autoscaler.New(localPool, s, cfg.AutoScale.Interval)
				as.Start()
				defer as.Stop()
				logging.Op().Info("autoscaler started (local mode)")
			}

			var httpServer *http.Server
			if listenAddr != "" {
				mux := http.NewServeMux()
				mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"status":"ok","service":"corona"}`))
				})
				if cfg.Observability.Metrics.Enabled {
					mux.Handle("/metrics", promhttp.Handler())
				}
				httpServer = &http.Server{
					Addr:    listenAddr,
					Handler: mux,
				}
				go func() {
					logging.Op().Info("Corona HTTP endpoint started", "addr", listenAddr)
					if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						logging.Op().Error("corona HTTP server error", "error", err)
					}
				}()
			}

			logging.Op().Info("Corona scheduler/placement plane started")

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			logging.Op().Info("shutdown signal received")

			if httpServer != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = httpServer.Shutdown(ctx)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")
	cmd.Flags().StringVar(&cometAddr, "comet-grpc", "", "Comet gRPC address for remote invocation (e.g. comet:9090)")
	cmd.Flags().StringVar(&listenAddr, "listen", "", "HTTP listen address for /health (optional)")

	return cmd
}
