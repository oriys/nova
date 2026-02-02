package main

import (
	"context"
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
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
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
		Short: "Run as daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.DefaultConfig()
			config.LoadFromEnv(cfg)

			if pgDSN != "" {
				cfg.Postgres.DSN = pgDSN
			}

			logging.SetLevelFromString(logLevel)
			logging.InitStructured(cfg.Observability.Logging.Format, cfg.Observability.Logging.Level)

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
				dockerMgr, err := docker.NewManager(&cfg.Docker)
				if err != nil {
					return err
				}
				be = dockerMgr
			default:
				adapter, err := firecracker.NewAdapter(&cfg.Firecracker)
				if err != nil {
					return err
				}
				fcAdapter = adapter
				be = adapter
			}

			p := pool.NewPool(be, cfg.Pool.IdleTTL)
			exec := executor.New(s, p)

			var httpServer *http.Server
			if httpAddr != "" {
				httpServer = api.StartHTTPServer(httpAddr, api.ServerConfig{
					Store:     s,
					Exec:      exec,
					Pool:      p,
					Backend:   be,
					FCAdapter: fcAdapter,
				})
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			<-sigCh
			if httpServer != nil {
				httpServer.Shutdown(context.Background())
			}
			exec.Shutdown(10 * time.Second)
			be.Shutdown()
			return nil
		},
	}

	cmd.Flags().DurationVar(&idleTTL, "idle-ttl", 60*time.Second, "Idle timeout")
	cmd.Flags().StringVar(&httpAddr, "http", "", "HTTP address")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")

	return cmd
}
