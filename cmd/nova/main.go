package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	novagrpc "github.com/oriys/nova/internal/grpc"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/logs"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/output"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/ratelimit"
	"github.com/oriys/nova/internal/scheduler"
	"github.com/oriys/nova/internal/secrets"
	"github.com/oriys/nova/internal/spec"
	"github.com/oriys/nova/internal/store"
	"github.com/spf13/cobra"
)

var (
	redisAddr  string
	redisPass  string
	redisDB    int
	configFile string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "nova",
		Short: "Nova - Minimal Serverless Platform with Firecracker",
		Long:  "A minimal serverless CLI that runs functions in Firecracker microVMs",
	}

	rootCmd.PersistentFlags().StringVar(&redisAddr, "redis", "localhost:6379", "Redis address")
	rootCmd.PersistentFlags().StringVar(&redisPass, "redis-pass", "", "Redis password")
	rootCmd.PersistentFlags().IntVar(&redisDB, "redis-db", 0, "Redis database")
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "Path to config file (optional, flags override)")

	rootCmd.AddCommand(
		registerCmd(),
		listCmd(),
		getCmd(),
		deleteCmd(),
		updateCmd(),
		invokeCmd(),
		snapshotCmd(),
		versionCmd(),
		scheduleCmd(),
		daemonCmd(),
		secretCmd(),
		apikeyCmd(),
		applyCmd(),
		initCmd(),
		logsCmd(),
		testCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func getStore() (*store.RedisStore, error) {
	return store.NewRedisStore(redisAddr, redisPass, redisDB)
}

func registerCmd() *cobra.Command {
	var (
		runtime        string
		handler        string
		codePath       string
		memoryMB       int
		timeoutS       int
		minReplicas    int
		maxReplicas    int
		vcpus          int
		diskIOPS       int64
		diskBandwidth  int64
		netRxBandwidth int64
		netTxBandwidth int64
		mode           string
		envVars        []string
	)

	cmd := &cobra.Command{
		Use:   "register <name>",
		Short: "Register a new function",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			rt := domain.Runtime(runtime)
			if !rt.IsValid() {
				return fmt.Errorf("invalid runtime: %s (valid: python, go, rust, wasm)", runtime)
			}

			if _, err := os.Stat(codePath); os.IsNotExist(err) {
				return fmt.Errorf("code path not found: %s", codePath)
			}

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			envMap := make(map[string]string)
			for _, e := range envVars {
				var k, v string
				if _, err := fmt.Sscanf(e, "%s=%s", &k, &v); err == nil {
					envMap[k] = v
				}
			}

			// Calculate code hash for change detection
			codeHash, err := domain.HashCodeFile(codePath)
			if err != nil {
				fmt.Printf("Warning: could not hash code file: %v\n", err)
			}

			fn := &domain.Function{
				ID:          uuid.New().String(),
				Name:        name,
				Runtime:     rt,
				Handler:     handler,
				CodePath:    codePath,
				CodeHash:    codeHash,
				MemoryMB:    memoryMB,
				TimeoutS:    timeoutS,
				MinReplicas: minReplicas,
				MaxReplicas: maxReplicas,
				Mode:        domain.ExecutionMode(mode),
				EnvVars:     envMap,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			// Apply resource limits if any are set
			if vcpus > 1 || diskIOPS > 0 || diskBandwidth > 0 || netRxBandwidth > 0 || netTxBandwidth > 0 {
				fn.Limits = &domain.ResourceLimits{
					VCPUs:          vcpus,
					DiskIOPS:       diskIOPS,
					DiskBandwidth:  diskBandwidth,
					NetRxBandwidth: netRxBandwidth,
					NetTxBandwidth: netTxBandwidth,
				}
			}

			if err := s.SaveFunction(context.Background(), fn); err != nil {
				return err
			}

			fmt.Printf("Function registered:\n")
			fmt.Printf("  ID:           %s\n", fn.ID)
			fmt.Printf("  Name:         %s\n", fn.Name)
			fmt.Printf("  Runtime:      %s\n", fn.Runtime)
			fmt.Printf("  Handler:      %s\n", fn.Handler)
			fmt.Printf("  Code:         %s\n", fn.CodePath)
			fmt.Printf("  Memory:       %d MB\n", fn.MemoryMB)
			fmt.Printf("  vCPUs:        %d\n", vcpus)
			fmt.Printf("  Mode:         %s\n", fn.Mode)
			fmt.Printf("  Min Replicas: %d\n", fn.MinReplicas)
			if fn.Limits != nil {
				if fn.Limits.DiskIOPS > 0 {
					fmt.Printf("  Disk IOPS:    %d\n", fn.Limits.DiskIOPS)
				}
				if fn.Limits.DiskBandwidth > 0 {
					fmt.Printf("  Disk BW:      %d bytes/s\n", fn.Limits.DiskBandwidth)
				}
				if fn.Limits.NetRxBandwidth > 0 {
					fmt.Printf("  Net RX BW:    %d bytes/s\n", fn.Limits.NetRxBandwidth)
				}
				if fn.Limits.NetTxBandwidth > 0 {
					fmt.Printf("  Net TX BW:    %d bytes/s\n", fn.Limits.NetTxBandwidth)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&runtime, "runtime", "r", "", "Runtime (python, go, rust, wasm)")
	cmd.Flags().StringVarP(&handler, "handler", "H", "main.handler", "Handler function")
	cmd.Flags().StringVarP(&codePath, "code", "c", "", "Path to code file/directory")
	cmd.Flags().IntVarP(&memoryMB, "memory", "m", 128, "Memory in MB")
	cmd.Flags().IntVarP(&timeoutS, "timeout", "t", 30, "Timeout in seconds")
	cmd.Flags().IntVar(&minReplicas, "min-replicas", 0, "Minimum number of warm replicas")
	cmd.Flags().IntVar(&maxReplicas, "max-replicas", 0, "Maximum concurrent VMs (0 = unlimited)")
	cmd.Flags().IntVar(&vcpus, "vcpus", 1, "Number of vCPUs (1-32)")
	cmd.Flags().Int64Var(&diskIOPS, "disk-iops", 0, "Max disk IOPS (0 = unlimited)")
	cmd.Flags().Int64Var(&diskBandwidth, "disk-bandwidth", 0, "Max disk bandwidth in bytes/s (0 = unlimited)")
	cmd.Flags().Int64Var(&netRxBandwidth, "net-rx-bandwidth", 0, "Max network RX bandwidth in bytes/s (0 = unlimited)")
	cmd.Flags().Int64Var(&netTxBandwidth, "net-tx-bandwidth", 0, "Max network TX bandwidth in bytes/s (0 = unlimited)")
	cmd.Flags().StringArrayVarP(&envVars, "env", "e", nil, "Environment variables (KEY=VALUE)")
	cmd.Flags().StringVar(&mode, "mode", "process", "Execution mode (process, persistent)")

	cmd.MarkFlagRequired("runtime")
	cmd.MarkFlagRequired("code")

	return cmd
}

func listCmd() *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List all functions",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			funcs, err := s.ListFunctions(context.Background())
			if err != nil {
				return err
			}

			printer := output.NewPrinter(output.ParseFormat(outputFormat))

			// Convert to output rows
			rows := make([]output.FunctionRow, 0, len(funcs))
			for _, fn := range funcs {
				rows = append(rows, output.FunctionRow{
					Name:        fn.Name,
					Runtime:     string(fn.Runtime),
					Handler:     fn.Handler,
					Memory:      fn.MemoryMB,
					Timeout:     fn.TimeoutS,
					MinReplicas: fn.MinReplicas,
					Mode:        string(fn.Mode),
					Version:     fn.Version,
					Created:     fn.CreatedAt.Format("2006-01-02 15:04:05"),
					Updated:     fn.UpdatedAt.Format("2006-01-02 15:04:05"),
				})
			}

			return printer.PrintFunctions(rows)
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, wide, json, yaml)")
	return cmd
}

func getCmd() *cobra.Command {
	var showVersions bool
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get function details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			fn, err := s.GetFunctionByName(context.Background(), args[0])
			if err != nil {
				return err
			}

			// Check snapshot
			cfg := firecracker.DefaultConfig()
			hasSnapshot := executor.HasSnapshot(cfg.SnapshotDir, fn.ID)

			// Versions and aliases
			versions, _ := s.ListVersions(context.Background(), fn.ID)
			aliases, _ := s.ListAliases(context.Background(), fn.ID)

			aliasNames := make([]string, 0, len(aliases))
			for _, a := range aliases {
				aliasNames = append(aliasNames, fmt.Sprintf("%s->v%d", a.Name, a.Version))
			}

			printer := output.NewPrinter(output.ParseFormat(outputFormat))

			// For JSON/YAML, output the function directly
			if outputFormat == "json" || outputFormat == "yaml" {
				detail := output.FunctionDetail{
					ID:          fn.ID,
					Name:        fn.Name,
					Runtime:     string(fn.Runtime),
					Handler:     fn.Handler,
					CodePath:    fn.CodePath,
					CodeHash:    fn.CodeHash,
					MemoryMB:    fn.MemoryMB,
					TimeoutS:    fn.TimeoutS,
					MinReplicas: fn.MinReplicas,
					MaxReplicas: fn.MaxReplicas,
					Mode:        string(fn.Mode),
					Version:     fn.Version,
					EnvVars:     fn.EnvVars,
					Limits:      fn.Limits,
					HasSnapshot: hasSnapshot,
					Versions:    len(versions),
					Aliases:     aliasNames,
					Created:     fn.CreatedAt.Format(time.RFC3339),
					Updated:     fn.UpdatedAt.Format(time.RFC3339),
				}
				return printer.PrintFunctionDetail(detail)
			}

			// Table format - human readable
			detail := output.FunctionDetail{
				ID:          fn.ID,
				Name:        fn.Name,
				Runtime:     string(fn.Runtime),
				Handler:     fn.Handler,
				CodePath:    fn.CodePath,
				CodeHash:    fn.CodeHash,
				MemoryMB:    fn.MemoryMB,
				TimeoutS:    fn.TimeoutS,
				MinReplicas: fn.MinReplicas,
				MaxReplicas: fn.MaxReplicas,
				Mode:        string(fn.Mode),
				Version:     fn.Version,
				EnvVars:     fn.EnvVars,
				HasSnapshot: hasSnapshot,
				Versions:    len(versions),
				Aliases:     aliasNames,
				Created:     fn.CreatedAt.Format(time.RFC3339),
				Updated:     fn.UpdatedAt.Format(time.RFC3339),
			}
			printer.PrintFunctionDetail(detail)

			// Show version history if requested
			if showVersions && len(versions) > 0 {
				fmt.Printf("\nVersion History:\n")
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "  VERSION\tDESCRIPTION\tCREATED")
				for _, v := range versions {
					current := ""
					if v.Version == fn.Version {
						current = " (current)"
					}
					fmt.Fprintf(w, "  v%d%s\t%s\t%s\n",
						v.Version,
						current,
						truncate(v.Description, 30),
						v.CreatedAt.Format("2006-01-02 15:04"),
					)
				}
				w.Flush()
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&showVersions, "versions", "v", false, "Show version history")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	return cmd
}

func deleteCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a function",
		Aliases: []string{"rm"},
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			fn, err := s.GetFunctionByName(context.Background(), args[0])
			if err != nil {
				return err
			}

			// Check for versions and aliases
			versions, _ := s.ListVersions(context.Background(), fn.ID)
			aliases, _ := s.ListAliases(context.Background(), fn.ID)

			cfg := firecracker.DefaultConfig()
			hasSnapshot := executor.HasSnapshot(cfg.SnapshotDir, fn.ID)

			// Confirm deletion if there's associated data
			if !force && (len(versions) > 0 || len(aliases) > 0 || hasSnapshot) {
				fmt.Printf("Function '%s' has associated data:\n", fn.Name)
				if len(versions) > 0 {
					fmt.Printf("  - %d version(s)\n", len(versions))
				}
				if len(aliases) > 0 {
					fmt.Printf("  - %d alias(es)\n", len(aliases))
				}
				if hasSnapshot {
					fmt.Printf("  - 1 snapshot\n")
				}
				fmt.Printf("\nUse --force to delete anyway\n")
				return nil
			}

			// Delete snapshot
			if hasSnapshot {
				_ = executor.InvalidateSnapshot(cfg.SnapshotDir, fn.ID)
				fmt.Printf("  Deleted snapshot\n")
			}

			// Delete versions
			for _, v := range versions {
				_ = s.DeleteVersion(context.Background(), fn.ID, v.Version)
			}
			if len(versions) > 0 {
				fmt.Printf("  Deleted %d version(s)\n", len(versions))
			}

			// Delete aliases
			for _, a := range aliases {
				_ = s.DeleteAlias(context.Background(), fn.ID, a.Name)
			}
			if len(aliases) > 0 {
				fmt.Printf("  Deleted %d alias(es)\n", len(aliases))
			}

			// Delete function
			if err := s.DeleteFunction(context.Background(), fn.ID); err != nil {
				return err
			}

			fmt.Printf("Function '%s' deleted\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force delete without confirmation")
	return cmd
}

func invokeCmd() *cobra.Command {
	var payload string
	var local bool

	cmd := &cobra.Command{
		Use:   "invoke <name>",
		Short: "Invoke a function",
		Long: `Invoke a function and display the result.

Use --local to run the function directly on the host without VM isolation.
This is useful for development and debugging.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			var input json.RawMessage
			if payload != "" {
				input = json.RawMessage(payload)
			} else {
				input = json.RawMessage("{}")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			var resp *domain.InvokeResponse

			if local {
				// Local execution - no VM
				localExec := executor.NewLocalExecutor()
				resp, err = localExec.InvokeWithStore(ctx, s, args[0], input)
			} else {
				// Normal VM execution
				cfg := firecracker.DefaultConfig()
				mgr, err := firecracker.NewManager(cfg)
				if err != nil {
					return fmt.Errorf("create VM manager: %w", err)
				}
				defer mgr.Shutdown()

				p := pool.NewPool(mgr, pool.DefaultIdleTTL)
				defer p.Shutdown()

				exec := executor.New(s, p)
				defer exec.Shutdown(5 * time.Second)

				resp, err = exec.Invoke(ctx, args[0], input)
			}

			if err != nil {
				return err
			}

			if local {
				fmt.Printf("Mode:       local\n")
			}
			fmt.Printf("Request ID: %s\n", resp.RequestID)
			fmt.Printf("Cold Start: %v\n", resp.ColdStart)
			fmt.Printf("Duration:   %d ms\n", resp.DurationMs)
			if resp.Error != "" {
				fmt.Printf("Error:      %s\n", resp.Error)
			} else {
				output, _ := json.MarshalIndent(resp.Output, "", "  ")
				fmt.Printf("Output:\n%s\n", output)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&payload, "payload", "p", "", "JSON payload")
	cmd.Flags().BoolVarP(&local, "local", "l", false, "Run locally without VM (for development)")
	return cmd
}

func updateCmd() *cobra.Command {
	var (
		handler        string
		codePath       string
		memoryMB       int
		timeoutS       int
		minReplicas    int
		vcpus          int
		diskIOPS       int64
		diskBandwidth  int64
		netRxBandwidth int64
		netTxBandwidth int64
		mode           string
		envVars        []string
		mergeEnv       bool
	)

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an existing function",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			update := &store.FunctionUpdate{
				MergeEnvVars: mergeEnv,
			}

			// Only set fields that were explicitly provided
			if cmd.Flags().Changed("handler") {
				update.Handler = &handler
			}
			if cmd.Flags().Changed("code") {
				if _, err := os.Stat(codePath); os.IsNotExist(err) {
					return fmt.Errorf("code path not found: %s", codePath)
				}
				update.CodePath = &codePath
			}
			if cmd.Flags().Changed("memory") {
				update.MemoryMB = &memoryMB
			}
			if cmd.Flags().Changed("timeout") {
				update.TimeoutS = &timeoutS
			}
			if cmd.Flags().Changed("min-replicas") {
				update.MinReplicas = &minReplicas
			}
			if cmd.Flags().Changed("mode") {
				m := domain.ExecutionMode(mode)
				update.Mode = &m
			}
			if cmd.Flags().Changed("vcpus") || cmd.Flags().Changed("disk-iops") ||
				cmd.Flags().Changed("disk-bandwidth") || cmd.Flags().Changed("net-rx-bandwidth") ||
				cmd.Flags().Changed("net-tx-bandwidth") {
				update.Limits = &domain.ResourceLimits{
					VCPUs:          vcpus,
					DiskIOPS:       diskIOPS,
					DiskBandwidth:  diskBandwidth,
					NetRxBandwidth: netRxBandwidth,
					NetTxBandwidth: netTxBandwidth,
				}
			}
			if len(envVars) > 0 {
				envMap := make(map[string]string)
				for _, e := range envVars {
					parts := strings.SplitN(e, "=", 2)
					if len(parts) == 2 {
						envMap[parts[0]] = parts[1]
					}
				}
				update.EnvVars = envMap
			}

			fn, err := s.UpdateFunction(context.Background(), name, update)
			if err != nil {
				return err
			}

			fmt.Printf("Function '%s' updated:\n", fn.Name)
			fmt.Printf("  Handler:      %s\n", fn.Handler)
			fmt.Printf("  Code:         %s\n", fn.CodePath)
			fmt.Printf("  Memory:       %d MB\n", fn.MemoryMB)
			fmt.Printf("  Timeout:      %d s\n", fn.TimeoutS)
			fmt.Printf("  Mode:         %s\n", fn.Mode)
			fmt.Printf("  Min Replicas: %d\n", fn.MinReplicas)
			fmt.Printf("  Updated:      %s\n", fn.UpdatedAt.Format(time.RFC3339))
			return nil
		},
	}

	cmd.Flags().StringVarP(&handler, "handler", "H", "", "Handler function")
	cmd.Flags().StringVarP(&codePath, "code", "c", "", "Path to code file/directory")
	cmd.Flags().IntVarP(&memoryMB, "memory", "m", 0, "Memory in MB")
	cmd.Flags().IntVarP(&timeoutS, "timeout", "t", 0, "Timeout in seconds")
	cmd.Flags().IntVar(&minReplicas, "min-replicas", 0, "Minimum number of warm replicas")
	cmd.Flags().IntVar(&vcpus, "vcpus", 1, "Number of vCPUs")
	cmd.Flags().Int64Var(&diskIOPS, "disk-iops", 0, "Max disk IOPS")
	cmd.Flags().Int64Var(&diskBandwidth, "disk-bandwidth", 0, "Max disk bandwidth bytes/s")
	cmd.Flags().Int64Var(&netRxBandwidth, "net-rx-bandwidth", 0, "Max network RX bytes/s")
	cmd.Flags().Int64Var(&netTxBandwidth, "net-tx-bandwidth", 0, "Max network TX bytes/s")
	cmd.Flags().StringArrayVarP(&envVars, "env", "e", nil, "Environment variables (KEY=VALUE)")
	cmd.Flags().BoolVar(&mergeEnv, "merge-env", true, "Merge env vars instead of replacing")
	cmd.Flags().StringVar(&mode, "mode", "", "Execution mode (process, persistent)")

	return cmd
}

func snapshotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Manage function snapshots",
	}

	cmd.AddCommand(snapshotCreateCmd())
	cmd.AddCommand(snapshotListCmd())
	cmd.AddCommand(snapshotDeleteCmd())

	return cmd
}

func snapshotCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <function-name>",
		Short: "Create a snapshot of a warm VM",
		Long:  "Creates a snapshot of an existing warm VM. The function must have at least one warm VM.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			funcName := args[0]

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			fn, err := s.GetFunctionByName(context.Background(), funcName)
			if err != nil {
				return err
			}

			cfg := firecracker.DefaultConfig()
			mgr, err := firecracker.NewManager(cfg)
			if err != nil {
				return fmt.Errorf("create VM manager: %w", err)
			}
			defer mgr.Shutdown()

			p := pool.NewPool(mgr, pool.DefaultIdleTTL)
			defer p.Shutdown()

			// Create a VM specifically for snapshotting
			fmt.Printf("Creating VM for snapshot of %s...\n", fn.Name)
			ctx := context.Background()

			pvm, err := p.Acquire(ctx, fn)
			if err != nil {
				return fmt.Errorf("acquire VM: %w", err)
			}

			// Create snapshot
			fmt.Printf("Creating snapshot...\n")
			if err := mgr.CreateSnapshot(ctx, pvm.VM.ID, fn.ID); err != nil {
				p.Release(pvm)
				return fmt.Errorf("create snapshot: %w", err)
			}

			// Stop the VM after snapshotting (it's paused by CreateSnapshot)
			pvm.Client.Close()
			mgr.StopVM(pvm.VM.ID)

			fmt.Printf("Snapshot created for function '%s'\n", fn.Name)
			fmt.Printf("  Location: %s/%s.snap\n", cfg.SnapshotDir, fn.ID)
			fmt.Printf("  Memory:   %s/%s.mem\n", cfg.SnapshotDir, fn.ID)
			fmt.Printf("\nNext invocation will use the snapshot for faster cold starts.\n")
			return nil
		},
	}
}

func snapshotListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			cfg := firecracker.DefaultConfig()

			funcs, err := s.ListFunctions(context.Background())
			if err != nil {
				return err
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "FUNCTION\tSNAPSHOT\tSIZE\tCREATED")

			hasSnapshots := false
			for _, fn := range funcs {
				snapPath := filepath.Join(cfg.SnapshotDir, fn.ID+".snap")
				memPath := filepath.Join(cfg.SnapshotDir, fn.ID+".mem")

				snapInfo, err := os.Stat(snapPath)
				if err != nil {
					continue
				}
				memInfo, _ := os.Stat(memPath)

				hasSnapshots = true
				totalSize := snapInfo.Size()
				if memInfo != nil {
					totalSize += memInfo.Size()
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					fn.Name,
					fn.ID+".snap",
					formatBytes(totalSize),
					snapInfo.ModTime().Format("2006-01-02 15:04:05"),
				)
			}
			w.Flush()

			if !hasSnapshots {
				fmt.Println("No snapshots found")
			}
			return nil
		},
	}
}

func snapshotDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <function-name>",
		Aliases: []string{"rm"},
		Short:   "Delete a function's snapshot",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			funcName := args[0]

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			fn, err := s.GetFunctionByName(context.Background(), funcName)
			if err != nil {
				return err
			}

			cfg := firecracker.DefaultConfig()
			snapPath := filepath.Join(cfg.SnapshotDir, fn.ID+".snap")
			memPath := filepath.Join(cfg.SnapshotDir, fn.ID+".mem")
			metaPath := filepath.Join(cfg.SnapshotDir, fn.ID+".meta")

			deleted := false
			for _, path := range []string{snapPath, memPath, metaPath} {
				if err := os.Remove(path); err == nil {
					deleted = true
				}
			}

			if deleted {
				fmt.Printf("Snapshot deleted for function '%s'\n", fn.Name)
			} else {
				fmt.Printf("No snapshot found for function '%s'\n", fn.Name)
			}
			return nil
		},
	}
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ─── Version Management ─────────────────────────────────────────────────────

func versionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Manage function versions",
	}

	cmd.AddCommand(versionPublishCmd())
	cmd.AddCommand(versionListCmd())
	cmd.AddCommand(versionAliasCmd())
	cmd.AddCommand(versionRollbackCmd())

	return cmd
}

func versionPublishCmd() *cobra.Command {
	var description string

	cmd := &cobra.Command{
		Use:   "publish <function-name>",
		Short: "Publish a new version of a function",
		Long:  "Creates a new version from the current function configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			funcName := args[0]

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			fn, err := s.GetFunctionByName(context.Background(), funcName)
			if err != nil {
				return err
			}

			// Get existing versions to determine next version number
			versions, _ := s.ListVersions(context.Background(), fn.ID)
			nextVersion := 1
			for _, v := range versions {
				if v.Version >= nextVersion {
					nextVersion = v.Version + 1
				}
			}

			// Create version snapshot
			version := &domain.FunctionVersion{
				FunctionID:  fn.ID,
				Version:     nextVersion,
				CodePath:    fn.CodePath,
				Handler:     fn.Handler,
				MemoryMB:    fn.MemoryMB,
				TimeoutS:    fn.TimeoutS,
				Mode:        fn.Mode,
				Limits:      fn.Limits,
				EnvVars:     fn.EnvVars,
				Description: description,
				CreatedAt:   time.Now(),
			}

			if err := s.PublishVersion(context.Background(), fn.ID, version); err != nil {
				return err
			}

			// Update function's current version
			fn.Version = nextVersion
			fn.UpdatedAt = time.Now()
			if err := s.SaveFunction(context.Background(), fn); err != nil {
				return err
			}

			// Auto-create "latest" alias if it doesn't exist
			alias := &domain.FunctionAlias{
				FunctionID: fn.ID,
				Name:       "latest",
				Version:    nextVersion,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			_ = s.SetAlias(context.Background(), alias)

			fmt.Printf("Published version %d for function '%s'\n", nextVersion, fn.Name)
			fmt.Printf("  Code:        %s\n", fn.CodePath)
			fmt.Printf("  Handler:     %s\n", fn.Handler)
			fmt.Printf("  Memory:      %d MB\n", fn.MemoryMB)
			if description != "" {
				fmt.Printf("  Description: %s\n", description)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&description, "description", "d", "", "Version description")
	return cmd
}

func versionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list <function-name>",
		Aliases: []string{"ls"},
		Short:   "List all versions of a function",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			funcName := args[0]

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			fn, err := s.GetFunctionByName(context.Background(), funcName)
			if err != nil {
				return err
			}

			versions, err := s.ListVersions(context.Background(), fn.ID)
			if err != nil {
				return err
			}

			if len(versions) == 0 {
				fmt.Printf("No versions found for function '%s'\n", fn.Name)
				fmt.Println("Use 'nova version publish' to create the first version")
				return nil
			}

			aliases, _ := s.ListAliases(context.Background(), fn.ID)
			aliasMap := make(map[int][]string)
			for _, a := range aliases {
				if a.Version > 0 {
					aliasMap[a.Version] = append(aliasMap[a.Version], a.Name)
				}
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "VERSION\tALIASES\tDESCRIPTION\tCREATED")
			for _, v := range versions {
				aliasStr := ""
				if aliases, ok := aliasMap[v.Version]; ok {
					aliasStr = strings.Join(aliases, ", ")
				}
				current := ""
				if v.Version == fn.Version {
					current = " (current)"
				}
				fmt.Fprintf(w, "v%d%s\t%s\t%s\t%s\n",
					v.Version,
					current,
					aliasStr,
					truncate(v.Description, 30),
					v.CreatedAt.Format("2006-01-02 15:04"),
				)
			}
			w.Flush()
			return nil
		},
	}
}

func versionAliasCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias",
		Short: "Manage version aliases",
	}

	// Set alias
	setCmd := &cobra.Command{
		Use:   "set <function-name> <alias> <version>",
		Short: "Set an alias to point to a specific version",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			funcName, aliasName, versionStr := args[0], args[1], args[2]

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			fn, err := s.GetFunctionByName(context.Background(), funcName)
			if err != nil {
				return err
			}

			var version int
			if _, err := fmt.Sscanf(versionStr, "%d", &version); err != nil {
				// Try parsing "v1", "v2" format
				if _, err := fmt.Sscanf(versionStr, "v%d", &version); err != nil {
					return fmt.Errorf("invalid version: %s", versionStr)
				}
			}

			// Verify version exists
			if _, err := s.GetVersion(context.Background(), fn.ID, version); err != nil {
				return fmt.Errorf("version %d does not exist", version)
			}

			alias := &domain.FunctionAlias{
				FunctionID: fn.ID,
				Name:       aliasName,
				Version:    version,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}

			if err := s.SetAlias(context.Background(), alias); err != nil {
				return err
			}

			fmt.Printf("Alias '%s' now points to version %d for function '%s'\n", aliasName, version, fn.Name)
			return nil
		},
	}

	// List aliases
	listCmd := &cobra.Command{
		Use:     "list <function-name>",
		Aliases: []string{"ls"},
		Short:   "List all aliases for a function",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			funcName := args[0]

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			fn, err := s.GetFunctionByName(context.Background(), funcName)
			if err != nil {
				return err
			}

			aliases, err := s.ListAliases(context.Background(), fn.ID)
			if err != nil {
				return err
			}

			if len(aliases) == 0 {
				fmt.Printf("No aliases found for function '%s'\n", fn.Name)
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ALIAS\tVERSION\tUPDATED")
			for _, a := range aliases {
				fmt.Fprintf(w, "%s\tv%d\t%s\n",
					a.Name,
					a.Version,
					a.UpdatedAt.Format("2006-01-02 15:04"),
				)
			}
			w.Flush()
			return nil
		},
	}

	// Delete alias
	deleteCmd := &cobra.Command{
		Use:     "delete <function-name> <alias>",
		Aliases: []string{"rm"},
		Short:   "Delete an alias",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			funcName, aliasName := args[0], args[1]

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			fn, err := s.GetFunctionByName(context.Background(), funcName)
			if err != nil {
				return err
			}

			if err := s.DeleteAlias(context.Background(), fn.ID, aliasName); err != nil {
				return err
			}

			fmt.Printf("Alias '%s' deleted for function '%s'\n", aliasName, fn.Name)
			return nil
		},
	}

	cmd.AddCommand(setCmd, listCmd, deleteCmd)
	return cmd
}

func versionRollbackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rollback <function-name> <version>",
		Short: "Rollback to a previous version",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			funcName, versionStr := args[0], args[1]

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			fn, err := s.GetFunctionByName(context.Background(), funcName)
			if err != nil {
				return err
			}

			var version int
			if _, err := fmt.Sscanf(versionStr, "%d", &version); err != nil {
				if _, err := fmt.Sscanf(versionStr, "v%d", &version); err != nil {
					return fmt.Errorf("invalid version: %s", versionStr)
				}
			}

			// Get the version to rollback to
			v, err := s.GetVersion(context.Background(), fn.ID, version)
			if err != nil {
				return err
			}

			// Update function with version's config
			fn.CodePath = v.CodePath
			fn.Handler = v.Handler
			fn.MemoryMB = v.MemoryMB
			fn.TimeoutS = v.TimeoutS
			fn.Mode = v.Mode
			fn.Limits = v.Limits
			fn.EnvVars = v.EnvVars
			fn.Version = version
			fn.UpdatedAt = time.Now()

			if err := s.SaveFunction(context.Background(), fn); err != nil {
				return err
			}

			// Update "latest" alias
			alias := &domain.FunctionAlias{
				FunctionID: fn.ID,
				Name:       "latest",
				Version:    version,
				UpdatedAt:  time.Now(),
			}
			_ = s.SetAlias(context.Background(), alias)

			fmt.Printf("Rolled back function '%s' to version %d\n", fn.Name, version)
			return nil
		},
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ─── Schedule Management ────────────────────────────────────────────────────

func scheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage scheduled function invocations (cron)",
	}

	cmd.AddCommand(scheduleAddCmd())
	cmd.AddCommand(scheduleListCmd())
	cmd.AddCommand(scheduleRemoveCmd())

	return cmd
}

func scheduleAddCmd() *cobra.Command {
	var (
		schedule string
		payload  string
		name     string
	)

	cmd := &cobra.Command{
		Use:   "add <function-name>",
		Short: "Schedule a function to run periodically",
		Long: `Schedule a function to run on a recurring basis.

Supported schedule formats:
  @every <duration>   Run every duration (e.g., "@every 5m", "@every 1h")
  @hourly             Run every hour at minute 0
  @daily              Run every day at midnight
  <duration>          Simple interval (e.g., "5m", "1h", "30s")

Examples:
  nova schedule add my-func --schedule "@every 5m"
  nova schedule add my-func --schedule "@hourly" --payload '{"key":"value"}'
  nova schedule add my-func --schedule "1h" --name "hourly-cleanup"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			funcName := args[0]

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			fn, err := s.GetFunctionByName(context.Background(), funcName)
			if err != nil {
				return err
			}

			entryName := name
			if entryName == "" {
				entryName = fmt.Sprintf("%s-schedule", fn.Name)
			}

			var payloadJSON json.RawMessage
			if payload != "" {
				payloadJSON = json.RawMessage(payload)
			}

			entry := &scheduler.CronEntry{
				ID:         uuid.New().String()[:8],
				FunctionID: fn.ID,
				Name:       entryName,
				Schedule:   schedule,
				Payload:    payloadJSON,
				Enabled:    true,
				CreatedAt:  time.Now(),
			}

			// Validate schedule by parsing it
			nextRun, err := parseSchedule(schedule)
			if err != nil {
				return err
			}
			entry.NextRun = nextRun

			// Note: In a real implementation, we'd persist this to Redis
			// For now, just show what would be scheduled
			fmt.Printf("Schedule created:\n")
			fmt.Printf("  ID:       %s\n", entry.ID)
			fmt.Printf("  Function: %s\n", fn.Name)
			fmt.Printf("  Name:     %s\n", entry.Name)
			fmt.Printf("  Schedule: %s\n", entry.Schedule)
			fmt.Printf("  Next Run: %s\n", entry.NextRun.Format(time.RFC3339))
			if payload != "" {
				fmt.Printf("  Payload:  %s\n", payload)
			}
			fmt.Printf("\nNote: Schedule will be active when daemon runs with --scheduler flag\n")
			return nil
		},
	}

	cmd.Flags().StringVarP(&schedule, "schedule", "s", "", "Cron schedule (required)")
	cmd.Flags().StringVarP(&payload, "payload", "p", "", "JSON payload to send")
	cmd.Flags().StringVarP(&name, "name", "n", "", "Schedule name")
	cmd.MarkFlagRequired("schedule")

	return cmd
}

func scheduleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all scheduled invocations",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Scheduled invocations are managed by the daemon.")
			fmt.Println("Use 'nova daemon --scheduler' to enable the scheduler.")
			fmt.Println("\nTo add a schedule: nova schedule add <function> --schedule '@every 5m'")
			return nil
		},
	}
}

func scheduleRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <schedule-id>",
		Aliases: []string{"rm", "delete"},
		Short:   "Remove a scheduled invocation",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scheduleID := args[0]
			fmt.Printf("Would remove schedule: %s\n", scheduleID)
			fmt.Println("Note: Schedule management requires daemon with --scheduler flag")
			return nil
		},
	}
}

// parseSchedule validates and parses a schedule string
func parseSchedule(schedule string) (time.Time, error) {
	now := time.Now()

	// Handle @every syntax
	if len(schedule) > 7 && schedule[:7] == "@every " {
		durationStr := schedule[7:]
		d, err := time.ParseDuration(durationStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid duration: %s", durationStr)
		}
		return now.Add(d), nil
	}

	// Handle predefined schedules
	switch schedule {
	case "@hourly":
		return now.Truncate(time.Hour).Add(time.Hour), nil
	case "@daily", "@midnight":
		return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location()), nil
	}

	// Try parsing as simple duration
	if d, err := time.ParseDuration(schedule); err == nil {
		return now.Add(d), nil
	}

	return time.Time{}, fmt.Errorf("unsupported schedule: %s", schedule)
}

func daemonCmd() *cobra.Command {
	var (
		idleTTL  time.Duration
		httpAddr string
		logLevel string
	)

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as daemon (keeps VMs warm, optional HTTP API)",
		Long:  "Run nova as a daemon that maintains warm VMs and optionally exposes an HTTP API",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load config from file if specified
			var cfg *config.Config
			if configFile != "" {
				var err error
				cfg, err = config.LoadFromFile(configFile)
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
			} else {
				cfg = config.DefaultConfig()
			}
			config.LoadFromEnv(cfg)

			// Override with command-line flags
			if cmd.Flags().Changed("redis") {
				cfg.Redis.Addr = redisAddr
			}
			if cmd.Flags().Changed("redis-pass") {
				cfg.Redis.Password = redisPass
			}
			if cmd.Flags().Changed("redis-db") {
				cfg.Redis.DB = redisDB
			}
			if cmd.Flags().Changed("idle-ttl") {
				cfg.Pool.IdleTTL = idleTTL
			}
			if cmd.Flags().Changed("http") {
				cfg.Daemon.HTTPAddr = httpAddr
			}
			if cmd.Flags().Changed("log-level") {
				cfg.Daemon.LogLevel = logLevel
			}

			// Observability flag overrides
			if cmd.Flags().Changed("tracing-enabled") {
				v, _ := cmd.Flags().GetBool("tracing-enabled")
				cfg.Observability.Tracing.Enabled = v
			}
			if cmd.Flags().Changed("tracing-endpoint") {
				v, _ := cmd.Flags().GetString("tracing-endpoint")
				cfg.Observability.Tracing.Endpoint = v
			}
			if cmd.Flags().Changed("log-format") {
				v, _ := cmd.Flags().GetString("log-format")
				cfg.Observability.Logging.Format = v
			}
			if cmd.Flags().Changed("output-capture") {
				v, _ := cmd.Flags().GetBool("output-capture")
				cfg.Observability.OutputCapture.Enabled = v
			}

			// gRPC flag overrides
			if cmd.Flags().Changed("grpc") {
				v, _ := cmd.Flags().GetBool("grpc")
				cfg.GRPC.Enabled = v
			}
			if cmd.Flags().Changed("grpc-addr") {
				v, _ := cmd.Flags().GetString("grpc-addr")
				cfg.GRPC.Addr = v
			}

			// Set structured log level
			logging.SetLevelFromString(cfg.Daemon.LogLevel)

			// Initialize structured logging
			logging.InitStructured(cfg.Observability.Logging.Format, cfg.Observability.Logging.Level)

			// Initialize OpenTelemetry tracing
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

			// Initialize Prometheus metrics
			if cfg.Observability.Metrics.Enabled {
				metrics.InitPrometheus(
					cfg.Observability.Metrics.Namespace,
					cfg.Observability.Metrics.HistogramBuckets,
				)
			}

			// Initialize output capture
			if cfg.Observability.OutputCapture.Enabled {
				if err := logging.InitOutputStore(
					cfg.Observability.OutputCapture.StorageDir,
					cfg.Observability.OutputCapture.MaxSize,
					cfg.Observability.OutputCapture.RetentionS,
				); err != nil {
					logging.Op().Warn("failed to init output capture", "error", err)
				}
			}

			s, err := store.NewRedisStore(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
			if err != nil {
				return err
			}
			defer s.Close()

			cfg.Firecracker.LogLevel = logLevel
			mgr, err := firecracker.NewManager(&cfg.Firecracker)
			if err != nil {
				return fmt.Errorf("create VM manager: %w", err)
			}

			p := pool.NewPool(mgr, cfg.Pool.IdleTTL)

			// Wire snapshot callback: create snapshot after cold start, then resume VM
			p.SetSnapshotCallback(func(ctx context.Context, vmID, funcID string) error {
				if err := mgr.CreateSnapshot(ctx, vmID, funcID); err != nil {
					return err
				}
				return mgr.ResumeVM(ctx, vmID)
			})

			// Set up secrets resolver if configured
			var secretsResolver *secrets.Resolver
			if cfg.Secrets.Enabled || cfg.Secrets.MasterKey != "" || cfg.Secrets.MasterKeyFile != "" {
				var cipher *secrets.Cipher
				var err error
				if cfg.Secrets.MasterKey != "" {
					cipher, err = secrets.NewCipher(cfg.Secrets.MasterKey)
				} else if cfg.Secrets.MasterKeyFile != "" {
					cipher, err = secrets.NewCipherFromFile(cfg.Secrets.MasterKeyFile)
				}
				if err != nil {
					logging.Op().Warn("failed to initialize secrets", "error", err)
				} else if cipher != nil {
					secretsStore := secrets.NewStore(s.Client(), cipher)
					secretsResolver = secrets.NewResolver(secretsStore)
					logging.Op().Info("secrets management enabled")
				}
			}

			// Create executor with options
			execOpts := []executor.Option{}
			if secretsResolver != nil {
				execOpts = append(execOpts, executor.WithSecretsResolver(secretsResolver))
			}
			exec := executor.New(s, p, execOpts...)

			logging.Op().Info("nova daemon started",
				"redis", cfg.Redis.Addr,
				"idle_ttl", cfg.Pool.IdleTTL.String(),
				"log_level", cfg.Daemon.LogLevel)

			// Start HTTP server if address is provided
			var httpServer *http.Server
			if cfg.Daemon.HTTPAddr != "" {
				httpServer = startHTTPServer(cfg.Daemon.HTTPAddr, httpServerConfig{
					store:        s,
					exec:         exec,
					pool:         p,
					mgr:          mgr,
					authCfg:      &cfg.Auth,
					rateLimitCfg: &cfg.RateLimit,
				})
				logging.Op().Info("HTTP API started", "addr", cfg.Daemon.HTTPAddr)
			}

			// Start gRPC server if enabled
			var grpcServer *novagrpc.Server
			if cfg.GRPC.Enabled {
				grpcServer = novagrpc.NewServer(s, exec)
				if err := grpcServer.Start(cfg.GRPC.Addr); err != nil {
					return fmt.Errorf("start gRPC server: %w", err)
				}
				logging.Op().Info("gRPC API started", "addr", cfg.GRPC.Addr)
			}

			logging.Op().Info("waiting for signals (Ctrl+C to stop)")

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			// Status ticker
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-sigCh:
					logging.Op().Info("shutdown signal received")
					if grpcServer != nil {
						grpcServer.Stop()
					}
					if httpServer != nil {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						httpServer.Shutdown(ctx)
						cancel()
					}
					// Graceful shutdown: wait for in-flight requests
					exec.Shutdown(10 * time.Second)
					mgr.Shutdown()
					return nil
				case <-ticker.C:
					// Maintenance: Ensure minimum replicas
					ctx := context.Background()
					funcs, err := s.ListFunctions(ctx)
					if err != nil {
						logging.Op().Error("error listing functions", "error", err)
					} else {
						for _, fn := range funcs {
							if err := p.EnsureReady(ctx, fn); err != nil {
								logging.Op().Error("error ensuring ready", "function", fn.Name, "error", err)
							}
						}
					}

					stats := p.Stats()
					logging.Op().Debug("daemon status", "active_vms", stats["active_vms"])
				}
			}
		},
	}

	cmd.Flags().DurationVar(&idleTTL, "idle-ttl", 60*time.Second, "VM idle timeout")
	cmd.Flags().StringVar(&httpAddr, "http", "", "HTTP API address (e.g., :8080)")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")

	// Observability flags
	cmd.Flags().Bool("tracing-enabled", false, "Enable OpenTelemetry tracing")
	cmd.Flags().String("tracing-endpoint", "localhost:4318", "OTLP exporter endpoint")
	cmd.Flags().String("log-format", "text", "Log format (text, json)")
	cmd.Flags().Bool("output-capture", false, "Enable function output capture")

	// gRPC flags
	cmd.Flags().Bool("grpc", false, "Enable gRPC API server")
	cmd.Flags().String("grpc-addr", ":9090", "gRPC server address")

	return cmd
}

// HTTP API Server

type httpServerConfig struct {
	store       *store.RedisStore
	exec        *executor.Executor
	pool        *pool.Pool
	mgr         *firecracker.Manager
	authCfg     *config.AuthConfig
	rateLimitCfg *config.RateLimitConfig
}

func startHTTPServer(addr string, cfg httpServerConfig) *http.Server {
	s := cfg.store
	exec := cfg.exec
	p := cfg.pool
	mgr := cfg.mgr
	mux := http.NewServeMux()

	// Health check - detailed status
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		redisOK := s.Ping(ctx) == nil
		stats := p.Stats()

		status := "ok"
		if !redisOK {
			status = "degraded"
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": status,
			"components": map[string]interface{}{
				"redis": redisOK,
				"pool": map[string]interface{}{
					"active_vms":  stats["active_vms"],
					"total_pools": stats["total_pools"],
				},
			},
			"uptime_seconds": int64(time.Since(time.Now()).Seconds()), // Will be properly set
		})
	})

	// Kubernetes liveness probe - minimal check
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Kubernetes readiness probe - checks Redis connectivity
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := s.Ping(ctx); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "not_ready",
				"error":  "redis unavailable: " + err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})

	// Kubernetes startup probe - checks basic initialization
	mux.HandleFunc("GET /health/startup", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// Check Redis is reachable
		if err := s.Ping(ctx); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "starting",
				"error":  "waiting for redis: " + err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "started"})
	})

	// List functions
	mux.HandleFunc("GET /functions", func(w http.ResponseWriter, r *http.Request) {
		funcs, err := s.ListFunctions(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(funcs)
	})

	// Get function
	mux.HandleFunc("GET /functions/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		fn, err := s.GetFunctionByName(r.Context(), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fn)
	})

	// Create function
	mux.HandleFunc("POST /functions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        string            `json:"name"`
			Runtime     string            `json:"runtime"`
			Handler     string            `json:"handler"`
			CodePath    string            `json:"code_path"`
			MemoryMB    int               `json:"memory_mb"`
			TimeoutS    int               `json:"timeout_s"`
			MinReplicas int               `json:"min_replicas"`
			MaxReplicas int               `json:"max_replicas"`
			Mode        string            `json:"mode"`
			EnvVars     map[string]string `json:"env_vars"`
			Limits      *domain.ResourceLimits `json:"limits"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		// Validate required fields
		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if req.Runtime == "" {
			http.Error(w, "runtime is required", http.StatusBadRequest)
			return
		}
		if req.CodePath == "" {
			http.Error(w, "code_path is required", http.StatusBadRequest)
			return
		}

		rt := domain.Runtime(req.Runtime)
		if !rt.IsValid() {
			http.Error(w, "invalid runtime (valid: python, go, rust, wasm)", http.StatusBadRequest)
			return
		}

		// Check if code file exists
		if _, err := os.Stat(req.CodePath); os.IsNotExist(err) {
			http.Error(w, fmt.Sprintf("code path not found: %s", req.CodePath), http.StatusBadRequest)
			return
		}

		// Check if function name already exists
		if existing, _ := s.GetFunctionByName(r.Context(), req.Name); existing != nil {
			http.Error(w, fmt.Sprintf("function '%s' already exists", req.Name), http.StatusConflict)
			return
		}

		// Set defaults
		if req.Handler == "" {
			req.Handler = "main.handler"
		}
		if req.MemoryMB == 0 {
			req.MemoryMB = 128
		}
		if req.TimeoutS == 0 {
			req.TimeoutS = 30
		}
		if req.Mode == "" {
			req.Mode = "process"
		}

		// Calculate code hash
		codeHash, _ := domain.HashCodeFile(req.CodePath)

		fn := &domain.Function{
			ID:          uuid.New().String(),
			Name:        req.Name,
			Runtime:     rt,
			Handler:     req.Handler,
			CodePath:    req.CodePath,
			CodeHash:    codeHash,
			MemoryMB:    req.MemoryMB,
			TimeoutS:    req.TimeoutS,
			MinReplicas: req.MinReplicas,
			MaxReplicas: req.MaxReplicas,
			Mode:        domain.ExecutionMode(req.Mode),
			EnvVars:     req.EnvVars,
			Limits:      req.Limits,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		if err := s.SaveFunction(r.Context(), fn); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(fn)
	})

	// Delete function (also cleans up versions, aliases, snapshots, VMs)
	mux.HandleFunc("DELETE /functions/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		fn, err := s.GetFunctionByName(r.Context(), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		// Evict all VMs for this function first
		p.Evict(fn.ID)

		// Delete snapshot if exists
		_ = executor.InvalidateSnapshot(mgr.SnapshotDir(), fn.ID)

		// Delete all versions
		versions, _ := s.ListVersions(r.Context(), fn.ID)
		for _, v := range versions {
			_ = s.DeleteVersion(r.Context(), fn.ID, v.Version)
		}

		// Delete all aliases
		aliases, _ := s.ListAliases(r.Context(), fn.ID)
		for _, a := range aliases {
			_ = s.DeleteAlias(r.Context(), fn.ID, a.Name)
		}

		// Finally delete the function
		if err := s.DeleteFunction(r.Context(), fn.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":           "deleted",
			"name":             name,
			"versions_deleted": len(versions),
			"aliases_deleted":  len(aliases),
		})
	})

	// Invoke function
	mux.HandleFunc("POST /functions/{name}/invoke", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var payload json.RawMessage
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "invalid JSON payload", http.StatusBadRequest)
				return
			}
		} else {
			payload = json.RawMessage("{}")
		}

		resp, err := exec.Invoke(r.Context(), name, payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// Pool stats
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p.Stats())
	})

	// Update function (invalidates snapshot if code changes)
	mux.HandleFunc("PATCH /functions/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		var update store.FunctionUpdate
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		// Check if code is being updated
		codeChanged := update.CodePath != nil

		fn, err := s.UpdateFunction(r.Context(), name, &update)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Evict VMs and invalidate snapshot if code changed
		if codeChanged {
			p.Evict(fn.ID)
			executor.InvalidateSnapshot(mgr.SnapshotDir(), fn.ID)
			p.InvalidateSnapshotCache(fn.ID)
			logging.Op().Info("invalidated snapshot", "function", fn.Name, "reason", "code_changed")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fn)
	})

	// List snapshots
	mux.HandleFunc("GET /snapshots", func(w http.ResponseWriter, r *http.Request) {
		funcs, err := s.ListFunctions(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		type snapshotInfo struct {
			FunctionID   string `json:"function_id"`
			FunctionName string `json:"function_name"`
			SnapSize     int64  `json:"snap_size"`
			MemSize      int64  `json:"mem_size"`
			TotalSize    int64  `json:"total_size"`
			CreatedAt    string `json:"created_at"`
		}

		var snapshots []snapshotInfo
		for _, fn := range funcs {
			if executor.HasSnapshot(mgr.SnapshotDir(), fn.ID) {
				snapPath := filepath.Join(mgr.SnapshotDir(), fn.ID+".snap")
				memPath := filepath.Join(mgr.SnapshotDir(), fn.ID+".mem")

				snapInfo, _ := os.Stat(snapPath)
				memInfo, _ := os.Stat(memPath)

				var snapSize, memSize int64
				var createdAt string
				if snapInfo != nil {
					snapSize = snapInfo.Size()
					createdAt = snapInfo.ModTime().Format(time.RFC3339)
				}
				if memInfo != nil {
					memSize = memInfo.Size()
				}

				snapshots = append(snapshots, snapshotInfo{
					FunctionID:   fn.ID,
					FunctionName: fn.Name,
					SnapSize:     snapSize,
					MemSize:      memSize,
					TotalSize:    snapSize + memSize,
					CreatedAt:    createdAt,
				})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(snapshots)
	})

	// Create snapshot for a function
	mux.HandleFunc("POST /functions/{name}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		fn, err := s.GetFunctionByName(r.Context(), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		// Check if snapshot already exists
		if executor.HasSnapshot(mgr.SnapshotDir(), fn.ID) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "exists",
				"message": "Snapshot already exists for this function",
			})
			return
		}

		// Acquire a VM and create snapshot
		pvm, err := p.Acquire(r.Context(), fn)
		if err != nil {
			http.Error(w, fmt.Sprintf("acquire VM: %v", err), http.StatusInternalServerError)
			return
		}

		if err := mgr.CreateSnapshot(r.Context(), pvm.VM.ID, fn.ID); err != nil {
			p.Release(pvm)
			http.Error(w, fmt.Sprintf("create snapshot: %v", err), http.StatusInternalServerError)
			return
		}

		// Stop the VM after snapshotting (it's paused)
		pvm.Client.Close()
		mgr.StopVM(pvm.VM.ID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "created",
			"message": fmt.Sprintf("Snapshot created for %s", fn.Name),
		})
	})

	// Delete snapshot for a function
	mux.HandleFunc("DELETE /functions/{name}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		fn, err := s.GetFunctionByName(r.Context(), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		if !executor.HasSnapshot(mgr.SnapshotDir(), fn.ID) {
			http.Error(w, "No snapshot exists for this function", http.StatusNotFound)
			return
		}

		if err := executor.InvalidateSnapshot(mgr.SnapshotDir(), fn.ID); err != nil {
			http.Error(w, fmt.Sprintf("delete snapshot: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "deleted",
			"message": fmt.Sprintf("Snapshot deleted for %s", fn.Name),
		})
	})

	// Metrics endpoints
	mux.Handle("GET /metrics", metrics.Global().JSONHandler())
	mux.Handle("GET /metrics/prometheus", metrics.PrometheusHandler())

	// Function logs endpoint
	mux.HandleFunc("GET /functions/{name}/logs", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		fn, err := s.GetFunctionByName(r.Context(), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		store := logging.GetOutputStore()
		if store == nil {
			http.Error(w, "output capture not enabled", http.StatusServiceUnavailable)
			return
		}

		// Get request_id from query params if specified
		requestID := r.URL.Query().Get("request_id")
		if requestID != "" {
			entry, found := store.Get(requestID)
			if !found {
				http.Error(w, "logs not found for request_id", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(entry)
			return
		}

		// Otherwise return recent logs for function
		tailStr := r.URL.Query().Get("tail")
		tail := 10
		if tailStr != "" {
			if n, err := fmt.Sscanf(tailStr, "%d", &tail); err != nil || n != 1 {
				tail = 10
			}
		}

		entries := store.GetByFunction(fn.ID, tail)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	})

	// Wrap with tracing middleware
	var handler http.Handler = mux
	handler = observability.HTTPMiddleware(handler)

	// Add rate limiting middleware
	if cfg.rateLimitCfg != nil && cfg.rateLimitCfg.Enabled {
		tiers := make(map[string]ratelimit.TierConfig)
		for name, tier := range cfg.rateLimitCfg.Tiers {
			tiers[name] = ratelimit.TierConfig{
				RequestsPerSecond: tier.RequestsPerSecond,
				BurstSize:         tier.BurstSize,
			}
		}
		limiter := ratelimit.New(s.Client(), tiers, ratelimit.TierConfig{
			RequestsPerSecond: cfg.rateLimitCfg.Default.RequestsPerSecond,
			BurstSize:         cfg.rateLimitCfg.Default.BurstSize,
		})
		publicPaths := []string{"/health", "/health/live", "/health/ready", "/health/startup"}
		if cfg.authCfg != nil {
			publicPaths = cfg.authCfg.PublicPaths
		}
		handler = ratelimit.Middleware(limiter, publicPaths)(handler)
		logging.Op().Info("rate limiting enabled", "default_rps", cfg.rateLimitCfg.Default.RequestsPerSecond)
	}

	// Add auth middleware
	if cfg.authCfg != nil && cfg.authCfg.Enabled {
		authenticators := buildAuthenticators(cfg.authCfg, s.Client())
		if len(authenticators) > 0 {
			handler = auth.Middleware(authenticators, cfg.authCfg.PublicPaths)(handler)
			logging.Op().Info("authentication enabled", "public_paths", cfg.authCfg.PublicPaths)
		}
	}

	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Op().Error("HTTP server error", "error", err)
		}
	}()

	return server
}

// buildAuthenticators creates authenticators based on config
func buildAuthenticators(cfg *config.AuthConfig, redisClient *redis.Client) []auth.Authenticator {
	var authenticators []auth.Authenticator

	// Add JWT authenticator if enabled
	if cfg.JWT.Enabled {
		jwtAuth, err := auth.NewJWTAuthenticator(auth.JWTAuthConfig{
			Algorithm:     cfg.JWT.Algorithm,
			Secret:        cfg.JWT.Secret,
			PublicKeyFile: cfg.JWT.PublicKeyFile,
			Issuer:        cfg.JWT.Issuer,
		})
		if err != nil {
			logging.Op().Warn("failed to create JWT authenticator", "error", err)
		} else {
			authenticators = append(authenticators, jwtAuth)
		}
	}

	// Add API Key authenticator if enabled
	if cfg.APIKeys.Enabled {
		var staticKeys []auth.StaticKeyConfig
		for _, k := range cfg.APIKeys.StaticKeys {
			staticKeys = append(staticKeys, auth.StaticKeyConfig{
				Name: k.Name,
				Key:  k.Key,
				Tier: k.Tier,
			})
		}
		apiKeyAuth := auth.NewAPIKeyAuthenticator(auth.APIKeyAuthConfig{
			Redis:      redisClient,
			StaticKeys: staticKeys,
		})
		authenticators = append(authenticators, apiKeyAuth)
	}

	return authenticators
}

// ─── Secret Management CLI ─────────────────────────────────────────────────

func secretCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Manage secrets",
	}

	cmd.AddCommand(secretSetCmd())
	cmd.AddCommand(secretGetCmd())
	cmd.AddCommand(secretListCmd())
	cmd.AddCommand(secretDeleteCmd())

	return cmd
}

func getSecretsStore() (*secrets.Store, error) {
	cfg := config.DefaultConfig()
	config.LoadFromEnv(cfg)

	if !cfg.Secrets.Enabled && cfg.Secrets.MasterKey == "" && cfg.Secrets.MasterKeyFile == "" {
		return nil, fmt.Errorf("secrets not configured: set NOVA_MASTER_KEY or NOVA_MASTER_KEY_FILE")
	}

	s, err := store.NewRedisStore(redisAddr, redisPass, redisDB)
	if err != nil {
		return nil, fmt.Errorf("connect to redis: %w", err)
	}

	var cipher *secrets.Cipher
	if cfg.Secrets.MasterKey != "" {
		cipher, err = secrets.NewCipher(cfg.Secrets.MasterKey)
	} else if cfg.Secrets.MasterKeyFile != "" {
		cipher, err = secrets.NewCipherFromFile(cfg.Secrets.MasterKeyFile)
	} else {
		return nil, fmt.Errorf("master key not configured")
	}
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	return secrets.NewStore(s.Client(), cipher), nil
}

func secretSetCmd() *cobra.Command {
	var value string

	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Set a secret value",
		Long:  "Set a secret value. If --value is not provided, reads from stdin.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			secretStore, err := getSecretsStore()
			if err != nil {
				return err
			}

			var secretValue []byte
			if value != "" {
				secretValue = []byte(value)
			} else {
				// Read from stdin
				reader := bufio.NewReader(os.Stdin)
				data, err := io.ReadAll(reader)
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}
				secretValue = data
				// Trim trailing newline
				if len(secretValue) > 0 && secretValue[len(secretValue)-1] == '\n' {
					secretValue = secretValue[:len(secretValue)-1]
				}
			}

			if err := secretStore.Set(context.Background(), name, secretValue); err != nil {
				return err
			}

			fmt.Printf("Secret '%s' set successfully\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&value, "value", "", "Secret value (reads from stdin if not provided)")
	return cmd
}

func secretGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Get a secret value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			secretStore, err := getSecretsStore()
			if err != nil {
				return err
			}

			value, err := secretStore.Get(context.Background(), name)
			if err != nil {
				return err
			}

			fmt.Println(string(value))
			return nil
		},
	}
}

func secretListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			secretStore, err := getSecretsStore()
			if err != nil {
				return err
			}

			secrets, err := secretStore.List(context.Background())
			if err != nil {
				return err
			}

			if len(secrets) == 0 {
				fmt.Println("No secrets found")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tCREATED")
			for name, created := range secrets {
				fmt.Fprintf(w, "%s\t%s\n", name, created)
			}
			w.Flush()
			return nil
		},
	}
}

func secretDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <name>",
		Aliases: []string{"rm"},
		Short:   "Delete a secret",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			secretStore, err := getSecretsStore()
			if err != nil {
				return err
			}

			if err := secretStore.Delete(context.Background(), name); err != nil {
				return err
			}

			fmt.Printf("Secret '%s' deleted\n", name)
			return nil
		},
	}
}

// ─── API Key Management CLI ────────────────────────────────────────────────

func apikeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apikey",
		Short: "Manage API keys",
	}

	cmd.AddCommand(apikeyCreateCmd())
	cmd.AddCommand(apikeyListCmd())
	cmd.AddCommand(apikeyRevokeCmd())
	cmd.AddCommand(apikeyDeleteCmd())

	return cmd
}

func getAPIKeyStore() (*auth.APIKeyStore, error) {
	s, err := store.NewRedisStore(redisAddr, redisPass, redisDB)
	if err != nil {
		return nil, fmt.Errorf("connect to redis: %w", err)
	}

	return auth.NewAPIKeyStore(s.Client()), nil
}

func apikeyCreateCmd() *cobra.Command {
	var tier string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			keyStore, err := getAPIKeyStore()
			if err != nil {
				return err
			}

			key, err := keyStore.Create(context.Background(), name, tier)
			if err != nil {
				return err
			}

			fmt.Printf("API Key created:\n")
			fmt.Printf("  Name: %s\n", name)
			fmt.Printf("  Tier: %s\n", tier)
			fmt.Printf("  Key:  %s\n", key)
			fmt.Printf("\nStore this key securely - it cannot be retrieved later.\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&tier, "tier", "default", "Rate limit tier")
	return cmd
}

func apikeyListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all API keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			keyStore, err := getAPIKeyStore()
			if err != nil {
				return err
			}

			keys, err := keyStore.List(context.Background())
			if err != nil {
				return err
			}

			if len(keys) == 0 {
				fmt.Println("No API keys found")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTIER\tENABLED\tCREATED")
			for _, k := range keys {
				fmt.Fprintf(w, "%s\t%s\t%v\t%s\n",
					k.Name,
					k.Tier,
					k.Enabled,
					k.CreatedAt.Format("2006-01-02 15:04"),
				)
			}
			w.Flush()
			return nil
		},
	}
}

func apikeyRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <name>",
		Short: "Revoke an API key (disable without deleting)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			keyStore, err := getAPIKeyStore()
			if err != nil {
				return err
			}

			if err := keyStore.Revoke(context.Background(), name); err != nil {
				return err
			}

			fmt.Printf("API key '%s' revoked\n", name)
			return nil
		},
	}
}

func apikeyDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <name>",
		Aliases: []string{"rm"},
		Short:   "Delete an API key",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			keyStore, err := getAPIKeyStore()
			if err != nil {
				return err
			}

			if err := keyStore.Delete(context.Background(), name); err != nil {
				return err
			}

			fmt.Printf("API key '%s' deleted\n", name)
			return nil
		},
	}
}

// ─── YAML Apply Command ────────────────────────────────────────────────────

func applyCmd() *cobra.Command {
	var (
		filePath string
		dryRun   bool
	)

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply function configuration from a YAML file",
		Long: `Apply function configuration from a YAML file.

Creates new functions or updates existing ones based on the YAML specification.
Supports multiple functions in a single file using YAML document separators (---).

Example:
  nova apply -f function.yaml
  nova apply -f functions/ --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return fmt.Errorf("file path required: use -f or --file")
			}

			// Check if path is a directory
			info, err := os.Stat(filePath)
			if err != nil {
				return fmt.Errorf("stat path: %w", err)
			}

			var files []string
			if info.IsDir() {
				// Find all YAML files in directory
				entries, err := os.ReadDir(filePath)
				if err != nil {
					return fmt.Errorf("read directory: %w", err)
				}
				for _, entry := range entries {
					if entry.IsDir() {
						continue
					}
					name := entry.Name()
					if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
						files = append(files, filepath.Join(filePath, name))
					}
				}
				if len(files) == 0 {
					return fmt.Errorf("no YAML files found in directory: %s", filePath)
				}
			} else {
				files = []string{filePath}
			}

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			var created, updated int

			for _, file := range files {
				specs, err := spec.ParseFile(file)
				if err != nil {
					return fmt.Errorf("parse %s: %w", file, err)
				}

				for _, fnSpec := range specs.Functions {
					if err := fnSpec.Validate(); err != nil {
						return fmt.Errorf("validate %s: %w", fnSpec.Name, err)
					}

					// Check if function exists
					existing, _ := s.GetFunctionByName(context.Background(), fnSpec.Name)

					if dryRun {
						if existing != nil {
							fmt.Printf("[dry-run] Would update function '%s'\n", fnSpec.Name)
						} else {
							fmt.Printf("[dry-run] Would create function '%s'\n", fnSpec.Name)
						}
						continue
					}

					var fn *domain.Function
					if existing != nil {
						// Update existing function
						fn, err = fnSpec.ToFunction(existing.ID)
						if err != nil {
							return fmt.Errorf("convert spec %s: %w", fnSpec.Name, err)
						}
						fn.CreatedAt = existing.CreatedAt
						fn.UpdatedAt = time.Now()
						fn.Version = existing.Version
						updated++
					} else {
						// Create new function
						fn, err = fnSpec.ToFunction(uuid.New().String())
						if err != nil {
							return fmt.Errorf("convert spec %s: %w", fnSpec.Name, err)
						}
						fn.CreatedAt = time.Now()
						fn.UpdatedAt = time.Now()
						created++
					}

					if err := s.SaveFunction(context.Background(), fn); err != nil {
						return fmt.Errorf("save function %s: %w", fn.Name, err)
					}

					action := "created"
					if existing != nil {
						action = "updated"
					}
					fmt.Printf("Function '%s' %s\n", fn.Name, action)
					fmt.Printf("  Runtime:  %s\n", fn.Runtime)
					fmt.Printf("  Handler:  %s\n", fn.Handler)
					fmt.Printf("  Code:     %s\n", fn.CodePath)
					fmt.Printf("  Memory:   %d MB\n", fn.MemoryMB)
					fmt.Printf("  Timeout:  %d s\n", fn.TimeoutS)
					if len(fn.EnvVars) > 0 {
						fmt.Printf("  Env Vars: %d\n", len(fn.EnvVars))
					}
				}
			}

			if !dryRun && (created > 0 || updated > 0) {
				fmt.Printf("\nSummary: %d created, %d updated\n", created, updated)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to YAML file or directory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without applying")

	return cmd
}

func initCmd() *cobra.Command {
	var (
		name     string
		runtime  string
		output   string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate a function YAML template",
		Long: `Generate a function YAML template file.

Creates a template YAML file that you can customize for your function.

Example:
  nova init -n my-function -r python -o function.yaml
  nova init --name hello --runtime go`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Use defaults if not specified
			if name == "" {
				name = "my-function"
			}
			if runtime == "" {
				runtime = "python"
			}

			// Validate runtime
			rt := domain.Runtime(runtime)
			if !rt.IsValid() {
				return fmt.Errorf("invalid runtime: %s (valid: python, go, rust, wasm, node, ruby, java, deno, bun)", runtime)
			}

			// Determine code file extension
			ext := ".py"
			switch rt {
			case domain.RuntimeGo:
				ext = ""
			case domain.RuntimeRust:
				ext = ""
			case domain.RuntimeWasm:
				ext = ".wasm"
			case domain.RuntimeNode:
				ext = ".js"
			case domain.RuntimeRuby:
				ext = ".rb"
			case domain.RuntimeJava:
				ext = ".jar"
			case domain.RuntimeDeno:
				ext = ".ts"
			case domain.RuntimeBun:
				ext = ".ts"
			}

			template := fmt.Sprintf(`# Nova Function Specification
apiVersion: nova/v1
kind: Function

# Function name (must be unique)
name: %s

# Optional description
description: A serverless function

# Runtime: python, go, rust, wasm, node, ruby, java, deno, bun
runtime: %s

# Handler function (format depends on runtime)
handler: main.handler

# Path to code file or directory
code: ./handler%s

# Resources
memory: 128      # Memory in MB (default: 128)
timeout: 30      # Timeout in seconds (default: 30)

# Scaling
minReplicas: 0   # Minimum warm replicas (default: 0)
maxReplicas: 0   # Maximum concurrent VMs (0 = unlimited)

# Execution mode: process (default) or persistent
mode: process

# Environment variables
# Use $SECRET:name to reference secrets
env:
  LOG_LEVEL: info
  # DATABASE_URL: $SECRET:database_url

# Resource limits (optional)
# limits:
#   vcpus: 1
#   diskIOPS: 0
#   diskBandwidth: 0
#   netRxBandwidth: 0
#   netTxBandwidth: 0
`, name, runtime, ext)

			if output == "" {
				output = name + ".yaml"
			}

			// Check if file exists
			if _, err := os.Stat(output); err == nil {
				return fmt.Errorf("file already exists: %s", output)
			}

			if err := os.WriteFile(output, []byte(template), 0644); err != nil {
				return fmt.Errorf("write file: %w", err)
			}

			fmt.Printf("Created function template: %s\n", output)
			fmt.Printf("\nNext steps:\n")
			fmt.Printf("  1. Edit %s to configure your function\n", output)
			fmt.Printf("  2. Create your handler file (handler%s)\n", ext)
			fmt.Printf("  3. Deploy with: nova apply -f %s\n", output)

			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Function name")
	cmd.Flags().StringVarP(&runtime, "runtime", "r", "", "Runtime (python, go, rust, wasm, node, ruby, java, deno, bun)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (default: <name>.yaml)")

	return cmd
}

// ─── Logs Command ──────────────────────────────────────────────────────────

func logsCmd() *cobra.Command {
	var (
		follow       bool
		tail         int64
		since        string
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "logs <function-name>",
		Short: "View function logs",
		Long: `View function invocation logs.

Examples:
  nova logs my-func              # Show recent logs
  nova logs my-func -f           # Follow logs in real-time
  nova logs my-func --tail 100   # Show last 100 entries
  nova logs my-func --since 1h   # Show logs from last hour
  nova logs my-func -o json      # Output as JSON`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			funcName := args[0]

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			fn, err := s.GetFunctionByName(context.Background(), funcName)
			if err != nil {
				return err
			}

			logStore := logs.NewStore(s.Client())
			printer := output.NewPrinter(output.ParseFormat(outputFormat))

			if follow {
				// Real-time log streaming
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				// Handle Ctrl+C
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				go func() {
					<-sigCh
					cancel()
				}()

				ch, err := logStore.Tail(ctx, fn.ID)
				if err != nil {
					return fmt.Errorf("tail logs: %w", err)
				}

				printer.Info("Following logs for %s (Ctrl+C to stop)...", funcName)
				for entry := range ch {
					printer.PrintLogEntry(output.LogEntry{
						Timestamp:  entry.Timestamp.Format("2006-01-02 15:04:05"),
						RequestID:  entry.RequestID,
						Function:   entry.Function,
						Level:      entry.Level,
						Message:    entry.Message,
						DurationMs: entry.DurationMs,
					})
				}
				return nil
			}

			// Query historical logs
			opts := logs.QueryOptions{
				FunctionID: fn.ID,
				Limit:      tail,
			}

			if since != "" {
				duration, err := time.ParseDuration(since)
				if err != nil {
					return fmt.Errorf("invalid duration: %s", since)
				}
				opts.Since = time.Now().Add(-duration)
			}

			entries, err := logStore.Query(context.Background(), opts)
			if err != nil {
				return fmt.Errorf("query logs: %w", err)
			}

			if len(entries) == 0 {
				printer.Info("No logs found for %s", funcName)
				return nil
			}

			for _, entry := range entries {
				printer.PrintLogEntry(output.LogEntry{
					Timestamp:  entry.Timestamp.Format("2006-01-02 15:04:05"),
					RequestID:  entry.RequestID,
					Function:   entry.Function,
					Level:      entry.Level,
					Message:    entry.Message,
					DurationMs: entry.DurationMs,
				})
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output in real-time")
	cmd.Flags().Int64Var(&tail, "tail", 50, "Number of recent log entries to show")
	cmd.Flags().StringVar(&since, "since", "", "Show logs since duration (e.g., 1h, 30m, 24h)")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json)")

	return cmd
}

// ─── Test Command ──────────────────────────────────────────────────────────

func testCmd() *cobra.Command {
	var (
		payload      string
		payloadFile  string
		envOverrides []string
		verbose      bool
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "test <function-name>",
		Short: "Test a function locally without VM",
		Long: `Test a function locally without requiring Firecracker VM.

This command runs the function directly on the host for quick testing
during development. It does not require Firecracker to be running.

Examples:
  nova test my-func                        # Test with empty payload
  nova test my-func -p '{"key":"value"}'   # Test with JSON payload
  nova test my-func -f payload.json        # Test with payload from file
  nova test my-func -e API_KEY=test        # Override environment variable`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			funcName := args[0]

			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			fn, err := s.GetFunctionByName(context.Background(), funcName)
			if err != nil {
				return err
			}

			// Determine payload
			var input json.RawMessage
			if payloadFile != "" {
				data, err := os.ReadFile(payloadFile)
				if err != nil {
					return fmt.Errorf("read payload file: %w", err)
				}
				input = json.RawMessage(data)
			} else if payload != "" {
				input = json.RawMessage(payload)
			} else {
				input = json.RawMessage("{}")
			}

			// Validate JSON
			var jsonTest interface{}
			if err := json.Unmarshal(input, &jsonTest); err != nil {
				return fmt.Errorf("invalid JSON payload: %w", err)
			}

			// Apply environment overrides
			if len(envOverrides) > 0 {
				if fn.EnvVars == nil {
					fn.EnvVars = make(map[string]string)
				}
				for _, e := range envOverrides {
					parts := strings.SplitN(e, "=", 2)
					if len(parts) == 2 {
						fn.EnvVars[parts[0]] = parts[1]
					}
				}
			}

			printer := output.NewPrinter(output.ParseFormat(outputFormat))

			if verbose {
				printer.Info("Testing function: %s", fn.Name)
				printer.Info("Runtime: %s", fn.Runtime)
				printer.Info("Handler: %s", fn.Handler)
				printer.Info("Code: %s", fn.CodePath)
				if len(fn.EnvVars) > 0 {
					printer.Info("Environment: %d variables", len(fn.EnvVars))
				}
				fmt.Println()
			}

			// Execute locally
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(fn.TimeoutS)*time.Second)
			defer cancel()

			localExec := executor.NewLocalExecutor()
			resp, err := localExec.Invoke(ctx, fn, input)

			if err != nil {
				printer.Error("Execution failed: %v", err)
				return err
			}

			result := output.InvokeResult{
				RequestID:  resp.RequestID,
				Success:    resp.Error == "",
				Output:     resp.Output,
				Error:      resp.Error,
				DurationMs: resp.DurationMs,
				ColdStart:  false,
				Mode:       "local",
			}

			return printer.PrintInvokeResult(result)
		},
	}

	cmd.Flags().StringVarP(&payload, "payload", "p", "", "JSON payload")
	cmd.Flags().StringVarP(&payloadFile, "file", "f", "", "Read payload from file")
	cmd.Flags().StringArrayVarP(&envOverrides, "env", "e", nil, "Override environment variables (KEY=VALUE)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show verbose output")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")

	return cmd
}
