package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/api"
	"github.com/oriys/nova/internal/asyncqueue"
	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/autoscaler"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/compiler"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/docker"
	"github.com/oriys/nova/internal/eventbus"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	novagrpc "github.com/oriys/nova/internal/grpc"
	"github.com/oriys/nova/internal/kubernetes"
	"github.com/oriys/nova/internal/layer"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/scheduler"
	"github.com/oriys/nova/internal/secrets"
	"github.com/oriys/nova/internal/service"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/wasm"
	"github.com/oriys/nova/internal/workflow"
	"github.com/spf13/cobra"
)

func daemonCmd() *cobra.Command {
	var (
		idleTTL  time.Duration
		httpAddr string
		logLevel string
	)

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run Nova control plane daemon",
		Long:  "Run Nova as a control plane daemon with management APIs and workers",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.DefaultConfig()
			if configFile != "" {
				var err error
				cfg, err = config.LoadFromFile(configFile)
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
			}
			config.LoadFromEnv(cfg)

			if cmd.Flags().Changed("pg-dsn") {
				cfg.Postgres.DSN = pgDSN
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

			logging.SetLevelFromString(cfg.Daemon.LogLevel)
			logging.InitStructured(cfg.Observability.Logging.Format, cfg.Observability.Logging.Level)

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

			if cfg.Observability.Metrics.Enabled {
				metrics.InitPrometheus(
					cfg.Observability.Metrics.Namespace,
					cfg.Observability.Metrics.HistogramBuckets,
				)
			}

			if cfg.Observability.OutputCapture.Enabled {
				if err := logging.InitOutputStore(
					cfg.Observability.OutputCapture.StorageDir,
					cfg.Observability.OutputCapture.MaxSize,
					cfg.Observability.OutputCapture.RetentionS,
				); err != nil {
					logging.Op().Warn("failed to init output capture", "error", err)
				}
			}

			pgStore, err := store.NewPostgresStore(context.Background(), cfg.Postgres.DSN)
			if err != nil {
				return err
			}
			s := store.NewStore(pgStore)
			defer s.Close()

			var be backend.Backend
			var fcAdapter *firecracker.Adapter

			switch cfg.Firecracker.Backend {
			case "docker":
				logging.Op().Info("using Docker backend")
				dockerMgr, err := docker.NewManager(&cfg.Docker)
				if err != nil {
					return err
				}
				be = dockerMgr
			case "wasm":
				logging.Op().Info("using WASM backend")
				wasmMgr, err := wasm.NewManager(&cfg.Wasm)
				if err != nil {
					return err
				}
				be = wasmMgr
			case "kubernetes", "k8s":
				logging.Op().Info("using Kubernetes backend",
					"namespace", cfg.Kubernetes.Namespace,
					"runtime_class", cfg.Kubernetes.RuntimeClassName,
					"scale_to_zero_grace", cfg.Kubernetes.ScaleToZeroGracePeriod,
				)
				k8sMgr, err := kubernetes.NewManager(&cfg.Kubernetes)
				if err != nil {
					return err
				}
				be = k8sMgr
			default:
				logging.Op().Info("using Firecracker backend")
				adapter, err := firecracker.NewAdapter(&cfg.Firecracker)
				if err != nil {
					return err
				}
				fcAdapter = adapter
				be = adapter
			}

			p := pool.NewPool(be, pool.PoolConfig{
				IdleTTL:             cfg.Pool.IdleTTL,
				CleanupInterval:     cfg.Pool.CleanupInterval,
				HealthCheckInterval: cfg.Pool.HealthCheckInterval,
				MaxPreWarmWorkers:   cfg.Pool.MaxPreWarmWorkers,
			})
			if fcAdapter != nil {
				mgr := fcAdapter.Manager()
				p.SetSnapshotCallback(func(ctx context.Context, vmID, funcID string) error {
					if err := mgr.CreateSnapshot(ctx, vmID, funcID); err != nil {
						return err
					}
					return mgr.ResumeVM(ctx, vmID)
				})
			}

			if cfg.AutoScale.Enabled {
				as := autoscaler.New(p, s, cfg.AutoScale.Interval)
				as.Start()
				defer as.Stop()
			}

			var layerManager *layer.Manager
			if cfg.Layers.Enabled {
				layerManager = layer.New(s, cfg.Layers.StorageDir, cfg.Layers.MaxPerFunc)
				logging.Op().Info("shared dependency layers enabled", "storage_dir", cfg.Layers.StorageDir)
			}

			var secretsResolver *secrets.Resolver
			var secretsStore *secrets.Store
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
					secretsStore = secrets.NewStore(s, cipher)
					secretsResolver = secrets.NewResolver(secretsStore)
					logging.Op().Info("secrets management enabled")
				}
			}

			// Create API Key manager
			apiKeyAdapter := &apiKeyStoreAdapterDaemon{s: s}
			apiKeyManager := auth.NewAPIKeyManager(apiKeyAdapter)

			execOpts := []executor.Option{}
			if secretsResolver != nil {
				execOpts = append(execOpts, executor.WithSecretsResolver(secretsResolver))
			}
			execOpts = append(execOpts, executor.WithLogBatcherConfig(executor.LogBatcherConfig{
				BatchSize:     cfg.Executor.LogBatchSize,
				BufferSize:    cfg.Executor.LogBufferSize,
				FlushInterval: cfg.Executor.LogFlushInterval,
				Timeout:       cfg.Executor.LogTimeout,
			}))
			exec := executor.New(s, p, execOpts...)

			// Create workflow service and engine
			wfService := workflow.NewService(s)
			wfEngine := workflow.NewEngine(s, exec, workflow.EngineConfig{})
			wfEngine.Start()

			// Create and start scheduler
			sched := scheduler.New(s, exec)
			if err := sched.Start(); err != nil {
				logging.Op().Warn("failed to start scheduler", "error", err)
			}
			asyncWorkers := asyncqueue.New(s, exec, asyncqueue.Config{})
			asyncWorkers.Start()
			eventWorkers := eventbus.New(s, exec, wfService, eventbus.Config{})
			eventWorkers.Start()
			outboxRelay := eventbus.NewOutboxRelay(s, eventbus.OutboxRelayConfig{})
			outboxRelay.Start()

			var httpServer *http.Server
			if cfg.Daemon.HTTPAddr != "" {
				httpServer = api.StartHTTPServer(cfg.Daemon.HTTPAddr, api.ServerConfig{
					Store:           s,
					Exec:            exec,
					Pool:            p,
					Backend:         be,
					FCAdapter:       fcAdapter,
					AuthCfg:         &cfg.Auth,
					RateLimitCfg:    &cfg.RateLimit,
					GatewayCfg:      &cfg.Gateway,
					WorkflowService: wfService,
					APIKeyManager:   apiKeyManager,
					SecretsStore:    secretsStore,
					Scheduler:       sched,
					RootfsDir:       cfg.Firecracker.RootfsDir,
					LayerManager:    layerManager,
					PlaneMode:       api.PlaneModeControlPlane,
				})
				logging.Op().Info("HTTP API started", "addr", cfg.Daemon.HTTPAddr)
			}

			var grpcUnifiedServer *novagrpc.UnifiedServer
			if cfg.GRPC.Enabled {
				// Create compiler for control plane
				comp := compiler.New(s)
				funcService := service.NewFunctionService(s, comp)

				grpcUnifiedServer, err = novagrpc.NewUnifiedServer(&novagrpc.Config{
					Address:         cfg.GRPC.Addr,
					Store:           s,
					Executor:        exec,
					Pool:            p,
					FunctionService: funcService,
					Compiler:        comp,
				})
				if err != nil {
					return fmt.Errorf("create gRPC server: %w", err)
				}

				if err := grpcUnifiedServer.Start(cfg.GRPC.Addr); err != nil {
					return fmt.Errorf("start gRPC server: %w", err)
				}
				logging.Op().Info("unified gRPC API started", "addr", cfg.GRPC.Addr, "mode", cfg.GRPC.Mode)
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-sigCh:
					logging.Op().Info("shutdown signal received")
					if grpcUnifiedServer != nil {
						grpcUnifiedServer.Stop()
					}
					if httpServer != nil {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						httpServer.Shutdown(ctx)
						cancel()
					}
					sched.Stop()
					asyncWorkers.Stop()
					eventWorkers.Stop()
					outboxRelay.Stop()
					wfEngine.Stop()
					exec.Shutdown(10 * time.Second)
					be.Shutdown()
					return nil
				case <-ticker.C:
					ctx := context.Background()
					funcs, err := s.ListFunctions(ctx, 0, 0)
					if err != nil {
						logging.Op().Error("error listing functions", "error", err)
					} else {
						for _, fn := range funcs {
							// Fetch code content for pre-warming
							codeRecord, err := s.GetFunctionCode(ctx, fn.ID)
							if err != nil || codeRecord == nil {
								logging.Op().Debug("skipping function with no code", "function", fn.Name)
								continue
							}
							var codeContent []byte
							if len(codeRecord.CompiledBinary) > 0 {
								codeContent = codeRecord.CompiledBinary
							} else {
								codeContent = []byte(codeRecord.SourceCode)
							}
							if err := p.EnsureReady(ctx, fn, codeContent); err != nil {
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
	cmd.Flags().StringVar(&httpAddr, "http", "", "HTTP API address")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")

	return cmd
}

// apiKeyStoreAdapterDaemon adapts store.Store to auth.APIKeyStore for use in daemon.
type apiKeyStoreAdapterDaemon struct {
	s *store.Store
}

func (a *apiKeyStoreAdapterDaemon) SaveAPIKey(ctx context.Context, key *auth.APIKey) error {
	return a.s.SaveAPIKey(ctx, &store.APIKeyRecord{
		Name: key.Name, KeyHash: key.KeyHash, Tier: key.Tier,
		Enabled: key.Enabled, ExpiresAt: key.ExpiresAt,
		CreatedAt: key.CreatedAt, UpdatedAt: key.UpdatedAt,
	})
}

func (a *apiKeyStoreAdapterDaemon) GetAPIKeyByHash(ctx context.Context, keyHash string) (*auth.APIKey, error) {
	rec, err := a.s.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	return &auth.APIKey{
		Name: rec.Name, KeyHash: rec.KeyHash, Tier: rec.Tier,
		Enabled: rec.Enabled, ExpiresAt: rec.ExpiresAt,
		CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
	}, nil
}

func (a *apiKeyStoreAdapterDaemon) GetAPIKeyByName(ctx context.Context, name string) (*auth.APIKey, error) {
	rec, err := a.s.GetAPIKeyByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return &auth.APIKey{
		Name: rec.Name, KeyHash: rec.KeyHash, Tier: rec.Tier,
		Enabled: rec.Enabled, ExpiresAt: rec.ExpiresAt,
		CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
	}, nil
}

func (a *apiKeyStoreAdapterDaemon) ListAPIKeys(ctx context.Context) ([]*auth.APIKey, error) {
	recs, err := a.s.ListAPIKeys(ctx, 0, 0)
	if err != nil {
		return nil, err
	}
	keys := make([]*auth.APIKey, len(recs))
	for i, rec := range recs {
		keys[i] = &auth.APIKey{
			Name: rec.Name, KeyHash: rec.KeyHash, Tier: rec.Tier,
			Enabled: rec.Enabled, ExpiresAt: rec.ExpiresAt,
			CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
		}
	}
	return keys, nil
}

func (a *apiKeyStoreAdapterDaemon) DeleteAPIKey(ctx context.Context, name string) error {
	return a.s.DeleteAPIKey(ctx, name)
}
