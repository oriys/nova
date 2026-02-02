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
		Short: "Nova - Minimal Serverless Platform with Firecracker",
		Long:  "A minimal serverless CLI that runs functions in Firecracker microVMs",
	}

	rootCmd.PersistentFlags().StringVar(&pgDSN, "pg-dsn", "", "Postgres DSN")
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "Path to config file")

	rootCmd.AddCommand(
		registerCmd(),
		listCmd(),
		getCmd(),
		deleteCmd(),
		updateCmd(),
		invokeCmd(),
		daemonCmd(),
		// snapshotCmd(),
		// versionCmd(),
		// scheduleCmd(),
		// secretCmd(),
		// apikeyCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}