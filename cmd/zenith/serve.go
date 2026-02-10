package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/zenith"
	"github.com/spf13/cobra"
)

func serveCmd() *cobra.Command {
	var (
		listenAddr    string
		novaURL       string
		cometGRPCAddr string
		timeout       time.Duration
		logLevel      string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run Zenith gateway",
		Long:  "Run Zenith gateway service to route UI/MCP/CLI calls to Nova and Comet",
		RunE: func(cmd *cobra.Command, args []string) error {
			logging.SetLevelFromString(logLevel)
			logging.InitStructured("text", logLevel)

			handler, err := zenith.New(zenith.Config{
				NovaURL:       novaURL,
				CometGRPCAddr: cometGRPCAddr,
				Timeout:       timeout,
			})
			if err != nil {
				return err
			}
			defer handler.Close()

			httpServer := &http.Server{
				Addr:    listenAddr,
				Handler: handler,
			}

			errCh := make(chan error, 1)
			go func() {
				logging.Op().Info("Zenith gateway started", "addr", listenAddr, "nova", novaURL, "comet_grpc", cometGRPCAddr)
				if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
			}()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			select {
			case sig := <-sigCh:
				logging.Op().Info("shutdown signal received", "signal", sig.String())
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := httpServer.Shutdown(ctx); err != nil {
					return fmt.Errorf("shutdown zenith: %w", err)
				}
				return nil
			case err := <-errCh:
				return fmt.Errorf("zenith server error: %w", err)
			}
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", ":8080", "Zenith listen address")
	cmd.Flags().StringVar(&novaURL, "nova-url", "http://127.0.0.1:8081", "Nova control plane base URL")
	cmd.Flags().StringVar(&cometGRPCAddr, "comet-grpc", "127.0.0.1:9090", "Comet gRPC address")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "Upstream timeout")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")

	return cmd
}
