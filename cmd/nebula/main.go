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
		Use:   "nebula",
		Short: "Nebula event ingestion plane service",
		Long:  "Run Nebula event bus, async queue, and workflow engine via the daemon command",
	}

	rootCmd.PersistentFlags().StringVar(&pgDSN, "pg-dsn", "", "Postgres DSN")
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "Path to config file")
	rootCmd.AddCommand(daemonCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
