package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/slo"
	"github.com/oriys/nova/internal/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

func daemonCmd() *cobra.Command {
	var (
		logLevel   string
		listenAddr string
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

			pgStore, err := store.NewPostgresStore(context.Background(), cfg.Postgres.DSN)
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
			sloService.AutoHealCallback = func(ctx context.Context, fn *domain.Function, breaches []string) {
				current := fn.MinReplicas
				desired := current + 1
				if desired > 10 {
					desired = 10 // cap to prevent runaway scaling
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

			// HTTP endpoint for Prometheus scraping and health checks
			var httpServer *http.Server
			if listenAddr != "" {
				mux := http.NewServeMux()
				mux.Handle("/metrics", promhttp.Handler())
				mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"status":"ok","service":"aurora"}`))
				})
				httpServer = &http.Server{
					Addr:    listenAddr,
					Handler: mux,
				}
				go func() {
					logging.Op().Info("Aurora HTTP endpoint started", "addr", listenAddr)
					if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						logging.Op().Error("aurora HTTP server error", "error", err)
					}
				}()
			}

			logging.Op().Info("Aurora observability plane started")

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			logging.Op().Info("shutdown signal received")

			if httpServer != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				httpServer.Shutdown(ctx)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")
	cmd.Flags().StringVar(&listenAddr, "listen", ":9002", "HTTP listen address for /metrics and /health")

	return cmd
}
