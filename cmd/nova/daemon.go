package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/oriys/nova/api/proto/novapb"
	"github.com/oriys/nova/internal/ai"
	"github.com/oriys/nova/internal/api"
	"github.com/oriys/nova/internal/applevz"
	"github.com/oriys/nova/internal/asyncqueue"
	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/autoscaler"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/compiler"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/docker"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/eventbus"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	novagrpc "github.com/oriys/nova/internal/grpc"
	"github.com/oriys/nova/internal/kubernetes"
	"github.com/oriys/nova/internal/layer"
	"github.com/oriys/nova/internal/libkrun"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/sandbox"
	"github.com/oriys/nova/internal/scheduler"
	"github.com/oriys/nova/internal/secrets"
	"github.com/oriys/nova/internal/slo"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/volume"
	"github.com/oriys/nova/internal/wasm"
	"github.com/oriys/nova/internal/workflow"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func daemonCmd() *cobra.Command {
	var (
		idleTTL      time.Duration
		grpcAddr     string
		logLevel     string
		cometGRPAddr string
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
			if cmd.Flags().Changed("grpc") {
				cfg.GRPC.Addr = grpcAddr
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

			pgStore, err := store.NewPostgresStore(context.Background(), cfg.Postgres.DSN, store.PoolOptions{MaxConns: cfg.Postgres.MaxConn, MinConns: cfg.Postgres.MinConn})
			if err != nil {
				return err
			}
			cachedStore := store.NewCachedMetadataStore(pgStore, store.DefaultCacheTTL)
			s := store.NewStore(cachedStore)
			defer s.Close()

			sloService := slo.New(s, slo.Config{
				Enabled:           cfg.SLO.Enabled,
				Interval:          cfg.SLO.Interval,
				DefaultWindowS:    cfg.SLO.DefaultWindowS,
				DefaultMinSamples: cfg.SLO.DefaultMinSamples,
			})
			sloService.Start()
			defer sloService.Stop()

			var be backend.Backend
			var fcAdapter *firecracker.Adapter
			var vzManager *applevz.Manager

			defaultBackend := domain.BackendType(cfg.Firecracker.Backend)
			if defaultBackend == "" || defaultBackend == domain.BackendAuto {
				detected := backend.DetectDefaultBackend()
				defaultBackend = detected
				logging.Op().Info("auto-detected backend", "backend", defaultBackend)
			}
			if defaultBackend == domain.BackendType("k8s") {
				defaultBackend = domain.BackendKubernetes
			}

			factories := map[domain.BackendType]backend.BackendFactory{
				domain.BackendDocker: func() (backend.Backend, error) {
					logging.Op().Info("initializing Docker backend")
					return docker.NewManager(&cfg.Docker)
				},
				domain.BackendWasm: func() (backend.Backend, error) {
					logging.Op().Info("initializing WASM backend")
					return wasm.NewManager(&cfg.Wasm)
				},
				domain.BackendKubernetes: func() (backend.Backend, error) {
					logging.Op().Info("initializing Kubernetes backend",
						"namespace", cfg.Kubernetes.Namespace,
						"runtime_class", cfg.Kubernetes.RuntimeClassName,
						"scale_to_zero_grace", cfg.Kubernetes.ScaleToZeroGracePeriod,
					)
					return kubernetes.NewManager(&cfg.Kubernetes)
				},
				domain.BackendLibKrun: func() (backend.Backend, error) {
					logging.Op().Info("initializing libkrun backend")
					return libkrun.NewManager(&cfg.LibKrun)
				},
				domain.BackendAppleVZ: func() (backend.Backend, error) {
					logging.Op().Info("initializing Apple VZ backend")
					mgr, err := applevz.NewManager(&cfg.AppleVZ)
					if err != nil {
						return nil, err
					}
					vzManager = mgr
					return mgr, nil
				},
				domain.BackendFirecracker: func() (backend.Backend, error) {
					logging.Op().Info("initializing Firecracker backend")
					if runtime.GOOS == "linux" {
						if err := compiler.EnsureDockerToolchainReady(context.Background()); err != nil {
							return nil, fmt.Errorf("ensure compiler docker availability: %w", err)
						}
					}
					adapter, err := firecracker.NewAdapter(&cfg.Firecracker)
					if err != nil {
						return nil, err
					}
					fcAdapter = adapter
					return adapter, nil
				},
			}
			switch defaultBackend {
			case domain.BackendDocker, domain.BackendWasm, domain.BackendKubernetes, domain.BackendLibKrun, domain.BackendFirecracker, domain.BackendAppleVZ:
			default:
				return fmt.Errorf("unsupported default backend for nova: %s", defaultBackend)
			}

			router, err := backend.NewRouter(defaultBackend, factories)
			if err != nil {
				return err
			}
			if err := router.EnsureReady(defaultBackend); err != nil {
				return err
			}
			be = router
			logging.Op().Info("using backend router", "default_backend", defaultBackend)

			p := pool.NewPool(be, pool.PoolConfig{
				IdleTTL:             cfg.Pool.IdleTTL,
				CleanupInterval:     cfg.Pool.CleanupInterval,
				HealthCheckInterval: cfg.Pool.HealthCheckInterval,
				MaxPreWarmWorkers:   cfg.Pool.MaxPreWarmWorkers,
			})
			if defaultBackend == domain.BackendFirecracker && fcAdapter != nil {
				mgr := fcAdapter.Manager()
				p.SetSnapshotCallback(func(ctx context.Context, vmID, funcID string) error {
					if err := mgr.CreateSnapshot(ctx, vmID, funcID); err != nil {
						return err
					}
					return mgr.ResumeVM(ctx, vmID)
				})
			}
			if defaultBackend == domain.BackendAppleVZ && vzManager != nil && vzManager.SnapshotDir() != "" {
				p.SetSnapshotCallback(func(ctx context.Context, vmID, funcID string) error {
					if err := vzManager.CreateSnapshot(ctx, vmID, funcID); err != nil {
						return err
					}
					return vzManager.ResumeVM(ctx, vmID)
				})
			}

			// Load max_global_vms from config store
			if sysConfig, err := s.GetConfig(context.Background()); err == nil {
				config.ApplyStoreOverrides(cfg, sysConfig)
				if v, ok := sysConfig["max_global_vms"]; ok {
					if n, err := strconv.Atoi(v); err == nil && n >= 0 {
						p.SetMaxGlobalVMs(n)
						logging.Op().Info("loaded max_global_vms from config", "value", n)
					}
				}
			}

			// Initialize runtime template pool for reduced cold-start latency
			if cfg.RuntimePool.Enabled && len(cfg.RuntimePool.Runtimes) > 0 {
				refillInterval := 30 * time.Second
				if cfg.RuntimePool.RefillInterval != "" {
					if d, err := time.ParseDuration(cfg.RuntimePool.RefillInterval); err == nil {
						refillInterval = d
					}
				}
				tp := pool.NewRuntimeTemplatePool(be, pool.RuntimePoolConfig{
					Enabled:        true,
					PoolSize:       cfg.RuntimePool.PoolSize,
					RefillInterval: refillInterval,
					Runtimes:       cfg.RuntimePool.Runtimes,
				})
				p.SetTemplatePool(tp)
				logging.Op().Info("runtime template pool enabled",
					"runtimes", cfg.RuntimePool.Runtimes,
					"pool_size", cfg.RuntimePool.PoolSize)
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

			var volumeManager *volume.Manager
			if cfg.Volumes.Enabled {
				volumeManager, err = volume.NewManager(s, &volume.Config{VolumeDir: cfg.Volumes.StorageDir})
				if err != nil {
					logging.Op().Warn("failed to initialize volume manager", "error", err)
				} else {
					logging.Op().Info("persistent volumes enabled", "storage_dir", cfg.Volumes.StorageDir)
				}
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
			execOpts = append(execOpts, executor.WithPayloadPersistence(cfg.Observability.OutputCapture.Enabled))
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

			// Create AI service (always created so config endpoints work)
			aiService := ai.NewService(cfg.AI)
			if aiService.Enabled() {
				logging.Op().Info("AI service enabled", "model", cfg.AI.Model)
			}

			sandboxMgr := sandbox.NewManager(be)
			defer sandboxMgr.Shutdown()

			// Connect to Comet gRPC for pool eviction in split deployments
			var cometClient novapb.NovaServiceClient
			if cometGRPAddr != "" {
				cometConn, err := grpc.NewClient(cometGRPAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					logging.Op().Warn("failed to connect to Comet gRPC", "addr", cometGRPAddr, "error", err)
				} else {
					cometClient = novapb.NewNovaServiceClient(cometConn)
					defer cometConn.Close()
					logging.Op().Info("connected to Comet gRPC for pool eviction", "addr", cometGRPAddr)
				}
			}

			// Build full HTTP handler for ProxyHTTP routing (no HTTP listener)
			httpHandler := api.BuildHTTPHandler(api.ServerConfig{
				Store:                 s,
				Exec:                  exec,
				Pool:                  p,
				Backend:               be,
				FCAdapter:             fcAdapter,
				AuthCfg:               &cfg.Auth,
				RateLimitCfg:          &cfg.RateLimit,
				GatewayCfg:            &cfg.Gateway,
				WorkflowService:       wfService,
				APIKeyManager:         apiKeyManager,
				SecretsStore:          secretsStore,
				Scheduler:             sched,
				RootfsDir:             cfg.Firecracker.RootfsDir,
				LayerManager:          layerManager,
				VolumeManager:         volumeManager,
				AIService:             aiService,
				SandboxManager:        sandboxMgr,
				PlaneMode:             api.PlaneModeControlPlane,
				LocalNodeID:           strings.TrimSpace(os.Getenv("NOVA_CLUSTER_NODE_ID")),
				ClusterForwardTimeout: 3 * time.Second,
				CometClient:           cometClient,
			})
			logging.Op().Info("HTTP handler built for gRPC ProxyHTTP routing")

			// Start gRPC server with ProxyHTTP wired to full HTTP handler
			if cfg.GRPC.Addr == "" {
				cfg.GRPC.Addr = ":8081"
			}
			grpcServer := novagrpc.NewServerWithRouter(s, exec, httpHandler, cfg.GRPC.ServiceToken)
			if err := grpcServer.Start(cfg.GRPC.Addr, cfg.GRPC.CertFile, cfg.GRPC.KeyFile); err != nil {
				return fmt.Errorf("start gRPC server: %w", err)
			}
			logging.Op().Info("Nova gRPC API started", "addr", cfg.GRPC.Addr)

			// Pool metrics recorder: write periodic snapshots for dashboard charts.
			go func() {
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()
				snap := store.PoolMetricsSnapshot{
					ActiveVMs:  p.TotalVMCount(),
					TotalPools: p.PoolCount(),
					VMsCreated: metrics.Global().VMsCreated.Load(),
					VMsStopped: metrics.Global().VMsStopped.Load(),
					VMsCrashed: metrics.Global().VMsCrashed.Load(),
				}
				if err := s.RecordPoolMetrics(context.Background(), snap); err != nil {
					logging.Op().Warn("failed to record pool metrics", "error", err)
				}
				for {
					select {
					case <-ticker.C:
						snap := store.PoolMetricsSnapshot{
							ActiveVMs:  p.TotalVMCount(),
							TotalPools: p.PoolCount(),
							VMsCreated: metrics.Global().VMsCreated.Load(),
							VMsStopped: metrics.Global().VMsStopped.Load(),
							VMsCrashed: metrics.Global().VMsCrashed.Load(),
						}
						if err := s.RecordPoolMetrics(context.Background(), snap); err != nil {
							logging.Op().Warn("failed to record pool metrics", "error", err)
						}
						if err := s.PrunePoolMetrics(context.Background(), 7*24*3600); err != nil {
							logging.Op().Warn("failed to prune pool metrics", "error", err)
						}
					case <-p.Done():
						return
					}
				}
			}()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-sigCh:
					logging.Op().Info("shutdown signal received")
					grpcServer.Stop()
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
	cmd.Flags().StringVar(&grpcAddr, "grpc", ":8081", "gRPC API address")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")
	cmd.Flags().StringVar(&cometGRPAddr, "comet-grpc", "", "Comet gRPC address for pool eviction in split deployments (e.g. 127.0.0.1:9090)")

	return cmd
}

// apiKeyStoreAdapterDaemon adapts store.Store to auth.APIKeyStore for use in daemon.
type apiKeyStoreAdapterDaemon struct {
	s *store.Store
}

func (a *apiKeyStoreAdapterDaemon) SaveAPIKey(ctx context.Context, key *auth.APIKey) error {
	permissions, _ := auth.MarshalPolicies(key.Policies)
	return a.s.SaveAPIKey(ctx, &store.APIKeyRecord{
		Name: key.Name, KeyHash: key.KeyHash, Tier: key.Tier,
		Enabled: key.Enabled, ExpiresAt: key.ExpiresAt,
		Permissions: permissions,
		CreatedAt:   key.CreatedAt, UpdatedAt: key.UpdatedAt,
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
	policies, _ := auth.UnmarshalPolicies(rec.Permissions)
	return &auth.APIKey{
		Name: rec.Name, KeyHash: rec.KeyHash, Tier: rec.Tier,
		Enabled: rec.Enabled, ExpiresAt: rec.ExpiresAt,
		TenantID: rec.TenantID, Namespace: rec.Namespace,
		Policies:  policies,
		CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
	}, nil
}

func (a *apiKeyStoreAdapterDaemon) GetAPIKeyByName(ctx context.Context, name string) (*auth.APIKey, error) {
	rec, err := a.s.GetAPIKeyByName(ctx, name)
	if err != nil {
		return nil, err
	}
	policies, _ := auth.UnmarshalPolicies(rec.Permissions)
	return &auth.APIKey{
		Name: rec.Name, KeyHash: rec.KeyHash, Tier: rec.Tier,
		Enabled: rec.Enabled, ExpiresAt: rec.ExpiresAt,
		TenantID: rec.TenantID, Namespace: rec.Namespace,
		Policies:  policies,
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
		policies, _ := auth.UnmarshalPolicies(rec.Permissions)
		keys[i] = &auth.APIKey{
			Name: rec.Name, KeyHash: rec.KeyHash, Tier: rec.Tier,
			Enabled: rec.Enabled, ExpiresAt: rec.ExpiresAt,
			TenantID: rec.TenantID, Namespace: rec.Namespace,
			Policies:  policies,
			CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
		}
	}
	return keys, nil
}

func (a *apiKeyStoreAdapterDaemon) DeleteAPIKey(ctx context.Context, name string) error {
	return a.s.DeleteAPIKey(ctx, name)
}
