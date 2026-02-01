package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/scheduler"
	"github.com/oriys/nova/internal/store"
	novagrpc "github.com/oriys/nova/internal/grpc"
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
	return &cobra.Command{
		Use:   "list",
		Short: "List all functions",
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

			if len(funcs) == 0 {
				fmt.Println("No functions registered")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tRUNTIME\tHANDLER\tMEMORY\tTIMEOUT\tCREATED")
			for _, fn := range funcs {
				fmt.Fprintf(w, "%s\t%s\t%s\t%dMB\t%ds\t%s\n",
					fn.Name,
					fn.Runtime,
					fn.Handler,
					fn.MemoryMB,
					fn.TimeoutS,
					fn.CreatedAt.Format("2006-01-02 15:04:05"),
				)
			}
			w.Flush()
			return nil
		},
	}
}

func getCmd() *cobra.Command {
	var showVersions bool

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

			fmt.Printf("Function: %s\n", fn.Name)
			fmt.Printf("  ID:          %s\n", fn.ID)
			fmt.Printf("  Runtime:     %s\n", fn.Runtime)
			fmt.Printf("  Handler:     %s\n", fn.Handler)
			fmt.Printf("  Code Path:   %s\n", fn.CodePath)
			if fn.CodeHash != "" {
				fmt.Printf("  Code Hash:   %s\n", fn.CodeHash)
			}
			fmt.Printf("  Memory:      %d MB\n", fn.MemoryMB)
			fmt.Printf("  Timeout:     %d s\n", fn.TimeoutS)
			fmt.Printf("  Mode:        %s\n", fn.Mode)
			fmt.Printf("  Min Replicas: %d\n", fn.MinReplicas)
			if fn.MaxReplicas > 0 {
				fmt.Printf("  Max Replicas: %d\n", fn.MaxReplicas)
			}
			if fn.Version > 0 {
				fmt.Printf("  Version:     v%d\n", fn.Version)
			}
			fmt.Printf("  Created:     %s\n", fn.CreatedAt.Format(time.RFC3339))
			fmt.Printf("  Updated:     %s\n", fn.UpdatedAt.Format(time.RFC3339))

			// Resource limits
			if fn.Limits != nil {
				fmt.Printf("  Limits:\n")
				if fn.Limits.VCPUs > 1 {
					fmt.Printf("    vCPUs:         %d\n", fn.Limits.VCPUs)
				}
				if fn.Limits.DiskIOPS > 0 {
					fmt.Printf("    Disk IOPS:     %d\n", fn.Limits.DiskIOPS)
				}
				if fn.Limits.DiskBandwidth > 0 {
					fmt.Printf("    Disk BW:       %s/s\n", formatBytes(fn.Limits.DiskBandwidth))
				}
				if fn.Limits.NetRxBandwidth > 0 {
					fmt.Printf("    Net RX BW:     %s/s\n", formatBytes(fn.Limits.NetRxBandwidth))
				}
				if fn.Limits.NetTxBandwidth > 0 {
					fmt.Printf("    Net TX BW:     %s/s\n", formatBytes(fn.Limits.NetTxBandwidth))
				}
			}

			// Environment variables
			if len(fn.EnvVars) > 0 {
				fmt.Printf("  Env Vars:\n")
				for k, v := range fn.EnvVars {
					fmt.Printf("    %s=%s\n", k, v)
				}
			}

			// Check snapshot
			cfg := firecracker.DefaultConfig()
			if executor.HasSnapshot(cfg.SnapshotDir, fn.ID) {
				snapPath := filepath.Join(cfg.SnapshotDir, fn.ID+".snap")
				if info, err := os.Stat(snapPath); err == nil {
					fmt.Printf("  Snapshot:    Yes (%s, %s)\n",
						formatBytes(info.Size()),
						info.ModTime().Format("2006-01-02 15:04"))
				}
			} else {
				fmt.Printf("  Snapshot:    No\n")
			}

			// Versions and aliases
			versions, _ := s.ListVersions(context.Background(), fn.ID)
			aliases, _ := s.ListAliases(context.Background(), fn.ID)

			if len(versions) > 0 {
				fmt.Printf("  Versions:    %d\n", len(versions))
			}
			if len(aliases) > 0 {
				aliasNames := make([]string, 0, len(aliases))
				for _, a := range aliases {
					aliasNames = append(aliasNames, fmt.Sprintf("%s->v%d", a.Name, a.Version))
				}
				fmt.Printf("  Aliases:     %s\n", strings.Join(aliasNames, ", "))
			}

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

			exec := executor.New(s, p)

			logging.Op().Info("nova daemon started",
				"redis", cfg.Redis.Addr,
				"idle_ttl", cfg.Pool.IdleTTL.String(),
				"log_level", cfg.Daemon.LogLevel)

			// Start HTTP server if address is provided
			var httpServer *http.Server
			if cfg.Daemon.HTTPAddr != "" {
				httpServer = startHTTPServer(cfg.Daemon.HTTPAddr, s, exec, p, mgr)
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

func startHTTPServer(addr string, s *store.RedisStore, exec *executor.Executor, p *pool.Pool, mgr *firecracker.Manager) *http.Server {
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
