package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "zenith",
		Short: "Zenith gateway service",
		Long:  "Run Zenith gateway for UI/MCP/CLI traffic",
	}

	rootCmd.AddCommand(serveCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
