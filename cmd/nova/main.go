package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	pgDSN      string
	configFile string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "nova",
		Short: "Nova backend service",
		Long:  "Run Nova backend services (HTTP/gRPC/control plane workers) via the daemon command",
	}

	rootCmd.PersistentFlags().StringVar(&pgDSN, "pg-dsn", "", "Postgres DSN")
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "Path to config file")

	rootCmd.AddCommand(
		daemonCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
