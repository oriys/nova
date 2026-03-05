package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/slo"
	"github.com/oriys/nova/internal/store"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func daemonCmd() *cobra.Command {
	var (
		logLevel string
		grpcAddr string
	)

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run Aurora observability plane daemon",
		Long:  "Run Aurora as the observability service (SLO evaluation, Prometheus metrics export, output capture)",
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
				cfg.Observability.Tracing.ServiceName = "aurora"
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

			if cfg.Observability.OutputCapture.Enabled {
				if err := logging.InitOutputStore(
					cfg.Observability.OutputCapture.StorageDir,
					cfg.Observability.OutputCapture.MaxSize,
					cfg.Observability.OutputCapture.RetentionS,
				); err != nil {
					logging.Op().Warn("failed to init output capture", "error", err)
				}
			}

			pgStore, err := store.NewPostgresStore(context.Background(), cfg.Postgres.DSN, store.PoolOptions{MaxConns: cfg.Postgres.MaxConn, MinConns: cfg.Postgres.MinConn})
			if err != nil {
				return err
			}
			cachedStore := store.NewCachedMetadataStore(pgStore, store.DefaultCacheTTL)
			s := store.NewStore(cachedStore)
			defer s.Close()

			// SLO evaluation service
			sloService := slo.New(s, slo.Config{
				Enabled:           cfg.SLO.Enabled,
				Interval:          cfg.SLO.Interval,
				DefaultWindowS:    cfg.SLO.DefaultWindowS,
				DefaultMinSamples: cfg.SLO.DefaultMinSamples,
			})

			// Auto-heal: when a latency or cold-start SLO breach is detected,
			// automatically increase min_replicas to pre-warm more VMs.
			autoHealMax := cfg.SLO.AutoHealMaxReplicas
			if autoHealMax <= 0 {
				autoHealMax = 10
			}
			sloService.AutoHealCallback = func(ctx context.Context, fn *domain.Function, breaches []string) {
				current := fn.MinReplicas
				desired := current + 1
				if desired > autoHealMax {
					desired = autoHealMax
				}
				if desired <= current {
					return
				}
				logging.Op().Info("slo auto-heal: increasing min_replicas",
					"function", fn.Name,
					"from", current,
					"to", desired,
					"breaches", breaches)
				if _, err := s.UpdateFunction(ctx, fn.Name, &store.FunctionUpdate{
					MinReplicas: &desired,
				}); err != nil {
					logging.Op().Error("slo auto-heal: failed to update min_replicas",
						"function", fn.Name,
						"error", err)
				}
			}

			sloService.Start()
			defer sloService.Stop()

			// Log retention cleanup: periodically delete old invocation logs
			logCleanupCtx, logCleanupCancel := context.WithCancel(context.Background())
			defer logCleanupCancel()
			go func() {
				ticker := time.NewTicker(1 * time.Hour)
				defer ticker.Stop()
				for {
					select {
					case <-logCleanupCtx.Done():
						return
					case <-ticker.C:
						globalRetention := 30
						if config, err := s.GetConfig(logCleanupCtx); err == nil {
							if v, ok := config["log_retention_days"]; ok {
								if days, err := strconv.Atoi(v); err == nil && days > 0 {
									globalRetention = days
								}
							}
						}
						deleted, err := pgStore.CleanupExpiredLogs(logCleanupCtx, globalRetention)
						if err != nil {
							logging.Op().Error("log retention cleanup failed", "error", err)
						} else if deleted > 0 {
							logging.Op().Info("log retention cleanup completed", "deleted", deleted, "retention_days", globalRetention)
						}
					}
				}
			}()

			// gRPC endpoint for health checks
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
					logging.Op().Info("Aurora gRPC endpoint started", "addr", grpcAddr)
					if err := grpcServer.Serve(lis); err != nil {
						logging.Op().Error("aurora gRPC server error", "error", err)
					}
				}()
			}

			logging.Op().Info("Aurora observability plane started")

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
	cmd.Flags().StringVar(&grpcAddr, "grpc", ":9002", "gRPC listen address for health checks")

	return cmd
}
