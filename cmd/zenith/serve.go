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
		listenAddr     string
		novaGRPCAddr   string
		cometGRPCAddr  string
		coronaGRPCAddr string
		nebulaGRPCAddr string
		auroraGRPCAddr string
		timeout        time.Duration
		maxTimeout     time.Duration
		grpcTLSCert    string
		grpcTLSKey     string
		grpcTLSCA      string
		logLevel       string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run Zenith gateway",
		Long:  "Run Zenith gateway service to route UI/MCP/CLI calls to Nova and Comet",
		RunE: func(cmd *cobra.Command, args []string) error {
			logging.SetLevelFromString(logLevel)
			logging.InitStructured("text", logLevel)

			handler, err := zenith.New(zenith.Config{
				NovaGRPCAddr:      novaGRPCAddr,
				CometGRPCAddr:     cometGRPCAddr,
				CoronaGRPCAddr:    coronaGRPCAddr,
				NebulaGRPCAddr:    nebulaGRPCAddr,
				AuroraGRPCAddr:    auroraGRPCAddr,
				Timeout:           timeout,
				MaxTimeout:        maxTimeout,
				CometServiceToken: os.Getenv("NOVA_GRPC_SERVICE_TOKEN"),
				GRPCTLSCertFile:   grpcTLSCert,
				GRPCTLSKeyFile:    grpcTLSKey,
				GRPCTLSCAFile:     grpcTLSCA,
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
				logging.Op().Info(
					"Zenith gateway started",
					"addr", listenAddr,
					"nova_grpc", novaGRPCAddr,
					"comet_grpc", cometGRPCAddr,
					"corona_grpc", coronaGRPCAddr,
					"nebula_grpc", nebulaGRPCAddr,
					"aurora_grpc", auroraGRPCAddr,
				)
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
	cmd.Flags().StringVar(&novaGRPCAddr, "nova-grpc", "127.0.0.1:8081", "Nova control plane gRPC address")
	cmd.Flags().StringVar(&cometGRPCAddr, "comet-grpc", "127.0.0.1:9090", "Comet gRPC address")
	cmd.Flags().StringVar(&coronaGRPCAddr, "corona-grpc", "", "Corona scheduler gRPC address (optional, used for health aggregation)")
	cmd.Flags().StringVar(&nebulaGRPCAddr, "nebula-grpc", "", "Nebula event bus gRPC address (optional, used for health aggregation)")
	cmd.Flags().StringVar(&auroraGRPCAddr, "aurora-grpc", "", "Aurora observability gRPC address (optional, used for health aggregation)")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "Upstream timeout")
	cmd.Flags().DurationVar(&maxTimeout, "max-timeout", 300*time.Second, "Global maximum timeout for gRPC calls")
	cmd.Flags().StringVar(&grpcTLSCert, "grpc-tls-cert", "", "TLS client certificate for gRPC connections")
	cmd.Flags().StringVar(&grpcTLSKey, "grpc-tls-key", "", "TLS client key for gRPC connections (mTLS)")
	cmd.Flags().StringVar(&grpcTLSCA, "grpc-tls-ca", "", "CA certificate for verifying gRPC server certs")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")

	return cmd
}
