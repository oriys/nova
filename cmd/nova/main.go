package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	redisAddr string
	redisPass string
	redisDB   int
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

	rootCmd.AddCommand(
		registerCmd(),
		listCmd(),
		getCmd(),
		deleteCmd(),
		invokeCmd(),
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
		vcpus          int
		diskIOPS       int64
		diskBandwidth  int64
		netRxBandwidth int64
		netTxBandwidth int64
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

			fn := &domain.Function{
				ID:          uuid.New().String(),
				Name:        name,
				Runtime:     rt,
				Handler:     handler,
				CodePath:    codePath,
				MemoryMB:    memoryMB,
				TimeoutS:    timeoutS,
				MinReplicas: minReplicas,
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
	cmd.Flags().IntVar(&vcpus, "vcpus", 1, "Number of vCPUs (1-32)")
	cmd.Flags().Int64Var(&diskIOPS, "disk-iops", 0, "Max disk IOPS (0 = unlimited)")
	cmd.Flags().Int64Var(&diskBandwidth, "disk-bandwidth", 0, "Max disk bandwidth in bytes/s (0 = unlimited)")
	cmd.Flags().Int64Var(&netRxBandwidth, "net-rx-bandwidth", 0, "Max network RX bandwidth in bytes/s (0 = unlimited)")
	cmd.Flags().Int64Var(&netTxBandwidth, "net-tx-bandwidth", 0, "Max network TX bandwidth in bytes/s (0 = unlimited)")
	cmd.Flags().StringArrayVarP(&envVars, "env", "e", nil, "Environment variables (KEY=VALUE)")

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
	return &cobra.Command{
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
			fmt.Printf("  ID:        %s\n", fn.ID)
			fmt.Printf("  Runtime:   %s\n", fn.Runtime)
			fmt.Printf("  Handler:   %s\n", fn.Handler)
			fmt.Printf("  Code Path: %s\n", fn.CodePath)
			fmt.Printf("  Memory:    %d MB\n", fn.MemoryMB)
			fmt.Printf("  Timeout:   %d s\n", fn.TimeoutS)
			fmt.Printf("  Created:   %s\n", fn.CreatedAt.Format(time.RFC3339))
			fmt.Printf("  Updated:   %s\n", fn.UpdatedAt.Format(time.RFC3339))
			if len(fn.EnvVars) > 0 {
				fmt.Printf("  Env Vars:\n")
				for k, v := range fn.EnvVars {
					fmt.Printf("    %s=%s\n", k, v)
				}
			}
			return nil
		},
	}
}

func deleteCmd() *cobra.Command {
	return &cobra.Command{
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

			if err := s.DeleteFunction(context.Background(), fn.ID); err != nil {
				return err
			}

			fmt.Printf("Function '%s' deleted\n", args[0])
			return nil
		},
	}
}

func invokeCmd() *cobra.Command {
	var payload string

	cmd := &cobra.Command{
		Use:   "invoke <name>",
		Short: "Invoke a function",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			cfg := firecracker.DefaultConfig()
			mgr, err := firecracker.NewManager(cfg)
			if err != nil {
				return fmt.Errorf("create VM manager: %w", err)
			}
			defer mgr.Shutdown()

			p := pool.NewPool(mgr, pool.DefaultIdleTTL)
			defer p.Shutdown()

			exec := executor.New(s, p)
			defer exec.Shutdown()

			var input json.RawMessage
			if payload != "" {
				input = json.RawMessage(payload)
			} else {
				input = json.RawMessage("{}")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			resp, err := exec.Invoke(ctx, args[0], input)
			if err != nil {
				return err
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
	return cmd
}

func daemonCmd() *cobra.Command {
	var idleTTL time.Duration

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as daemon (keeps VMs warm)",
		Long:  "Run nova as a daemon that maintains warm VMs and handles invocations",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			cfg := firecracker.DefaultConfig()
			mgr, err := firecracker.NewManager(cfg)
			if err != nil {
				return fmt.Errorf("create VM manager: %w", err)
			}

			p := pool.NewPool(mgr, idleTTL)

			fmt.Printf("Nova daemon started\n")
			fmt.Printf("  Redis:    %s\n", redisAddr)
			fmt.Printf("  Idle TTL: %s\n", idleTTL)
			fmt.Printf("\nWaiting for signals (Ctrl+C to stop)...\n")

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			// Status ticker
			ticker := time.NewTicker(10 * time.Second) // Check more frequently for scaling
			defer ticker.Stop()

			for {
				select {
				case <-sigCh:
					fmt.Println("\nShutting down...")
					p.Shutdown()
					mgr.Shutdown()
					return nil
				case <-ticker.C:
					// Maintenance: Ensure minimum replicas
					ctx := context.Background()
					funcs, err := s.ListFunctions(ctx)
					if err != nil {
						fmt.Printf("[daemon] Error listing functions: %v\n", err)
					} else {
						for _, fn := range funcs {
							if err := p.EnsureReady(ctx, fn); err != nil {
								fmt.Printf("[daemon] Error ensuring ready for %s: %v\n", fn.Name, err)
							}
						}
					}

					stats := p.Stats()
					fmt.Printf("[daemon] Active VMs: %d\n", stats["active_vms"])
				}
			}
		},
	}

	cmd.Flags().DurationVar(&idleTTL, "idle-ttl", 60*time.Second, "VM idle timeout")
	return cmd
}
