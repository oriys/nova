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
		Use:   "corona",
		Short: "Corona scheduler / placement plane service",
		Long:  "Run Corona scheduler and autoscaler via the daemon command",
	}

	rootCmd.PersistentFlags().StringVar(&pgDSN, "pg-dsn", "", "Postgres DSN")
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "Path to config file")
	rootCmd.AddCommand(daemonCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
