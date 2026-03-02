package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/asyncqueue"
	"github.com/oriys/nova/internal/autoscaler"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/cluster"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/docker"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/firecracker"
	novagrpc "github.com/oriys/nova/internal/grpc"
	"github.com/oriys/nova/internal/kata"
	"github.com/oriys/nova/internal/kubernetes"
	"github.com/oriys/nova/internal/applevz"
	"github.com/oriys/nova/internal/libkrun"
	"github.com/oriys/nova/internal/wasm"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/logsink"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/queue"
	"github.com/oriys/nova/internal/secrets"
	"github.com/oriys/nova/internal/stateproxy"
	"github.com/oriys/nova/internal/store"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
)

func daemonCmd() *cobra.Command {
	var (
		idleTTL  time.Duration
		grpcAddr string
		logLevel string
	)

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run Comet data plane daemon",
		Long:  "Run comet as a data plane daemon with executor, pool, and gRPC invocation API",
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

			cfg.GRPC.Enabled = true
			if cfg.GRPC.Addr == "" {
				cfg.GRPC.Addr = ":9090"
			}
			if cfg.Observability.Tracing.ServiceName == "" || cfg.Observability.Tracing.ServiceName == "nova" {
				cfg.Observability.Tracing.ServiceName = "comet"
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
				metrics.InitPrometheus(cfg.Observability.Metrics.Namespace, cfg.Observability.Metrics.HistogramBuckets)
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
			cachedStore := store.NewCachedMetadataStore(pgStore, store.DefaultCacheTTL)
			s := store.NewStore(cachedStore)
			defer s.Close()

			// Initialize queue notifier for push-based task triggering
			var notifier queue.Notifier
			switch cfg.Queue.NotifierType {
			case "channel":
				notifier = queue.NewChannelNotifier()
			case "redis", "redis-list":
				redisClient := redis.NewClient(&redis.Options{
					Addr: cfg.Queue.RedisAddr,
					DB:   cfg.Queue.RedisDB,
				})
				if err := redisClient.Ping(context.Background()).Err(); err != nil {
					return fmt.Errorf("connect to redis for queue notifier: %w", err)
				}
				if cfg.Queue.NotifierType == "redis-list" {
					notifier = queue.NewRedisListNotifier(redisClient)
					logging.Op().Info("using Redis list queue notifier (push-pull)", "addr", cfg.Queue.RedisAddr)
				} else {
					notifier = queue.NewRedisNotifier(redisClient)
					logging.Op().Info("using Redis queue notifier", "addr", cfg.Queue.RedisAddr)
				}
			default:
				notifier = queue.NewNoopNotifier()
			}
			defer notifier.Close()

			// Initialize log sink
			var sink logsink.LogSink
			switch cfg.LogSink.Type {
			case "noop":
				sink = logsink.NewNoopSink()
			default:
				sink = logsink.NewPostgresSink(s)
			}

			var be backend.Backend
			var fcAdapter *firecracker.Adapter
			var vzManager *applevz.Manager

			defaultBackend := domain.BackendType(cfg.Firecracker.Backend)
			if defaultBackend == "" || defaultBackend == domain.BackendAuto {
				detected := backend.DetectDefaultBackend()
				defaultBackend = detected
				logging.Op().Info("auto-detected backend",
					"backend", defaultBackend,
					"os", runtime.GOOS,
					"arch", runtime.GOARCH,
				)
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
						"image_prefix", cfg.Kubernetes.ImagePrefix,
					)
					return kubernetes.NewManager(&cfg.Kubernetes)
				},
				domain.BackendKata: func() (backend.Backend, error) {
					logging.Op().Info("initializing Kata Containers backend")
					return kata.NewManager(&cfg.Kata)
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
					adapter, err := firecracker.NewAdapter(&cfg.Firecracker)
					if err != nil {
						return nil, err
					}
					fcAdapter = adapter
					return adapter, nil
				},
			}
			switch defaultBackend {
			case domain.BackendDocker, domain.BackendWasm, domain.BackendKubernetes, domain.BackendKata, domain.BackendLibKrun, domain.BackendFirecracker, domain.BackendAppleVZ:
			default:
				return fmt.Errorf("unsupported default backend for comet: %s", defaultBackend)
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

			var secretsResolver *secrets.Resolver
			if cfg.Secrets.Enabled || cfg.Secrets.MasterKey != "" || cfg.Secrets.MasterKeyFile != "" {
				var cipher *secrets.Cipher
				if cfg.Secrets.MasterKey != "" {
					cipher, err = secrets.NewCipher(cfg.Secrets.MasterKey)
				} else if cfg.Secrets.MasterKeyFile != "" {
					cipher, err = secrets.NewCipherFromFile(cfg.Secrets.MasterKeyFile)
				}
				if err != nil {
					logging.Op().Warn("failed to initialize secrets", "error", err)
				} else if cipher != nil {
					secretsResolver = secrets.NewResolver(secrets.NewStore(s, cipher))
					logging.Op().Info("secrets management enabled")
				}
			}

			execOpts := []executor.Option{
				executor.WithLogBatcherConfig(executor.LogBatcherConfig{
					BatchSize:     cfg.Executor.LogBatchSize,
					BufferSize:    cfg.Executor.LogBufferSize,
					FlushInterval: cfg.Executor.LogFlushInterval,
					Timeout:       cfg.Executor.LogTimeout,
				}),
				executor.WithLogSink(sink),
				executor.WithPayloadPersistence(cfg.Observability.OutputCapture.Enabled),
			}
			if secretsResolver != nil {
				execOpts = append(execOpts, executor.WithSecretsResolver(secretsResolver))
			}
			exec := executor.New(s, p, execOpts...)

			asyncWorkers := asyncqueue.New(s, exec, asyncqueue.Config{
				Notifier: notifier,
			})
			asyncWorkers.Start()

			grpcServer := novagrpc.NewServer(s, exec, p, cfg.GRPC.ServiceToken)
			if err := grpcServer.Start(cfg.GRPC.Addr, cfg.GRPC.CertFile, cfg.GRPC.KeyFile); err != nil {
				return fmt.Errorf("start Comet gRPC server: %w", err)
			}
			logging.Op().Info("Comet gRPC API started", "addr", cfg.GRPC.Addr)

			// Self-register this node in the cluster registry so the
			// dashboard /cluster page always shows at least the local node.
			clusterReg := cluster.NewRegistry(s, cluster.DefaultConfig(nodeID()))
			selfNode := &cluster.Node{
				ID:      nodeID(),
				Name:    nodeName(),
				Address: cfg.GRPC.Addr,
				State:   cluster.NodeStateActive,
				MaxVMs:  64,
			}
			if err := clusterReg.RegisterNode(context.Background(), selfNode); err != nil {
				logging.Op().Warn("failed to self-register cluster node", "error", err)
			}

			// State proxy listener for in-VM state access
			stateProxy, err := stateproxy.Start(":9998", s)
			if err != nil {
				logging.Op().Warn("failed to start state proxy", "error", err)
			} else {
				defer stateProxy.Close()
				logging.Op().Info("State proxy started", "addr", ":9998")
			}

			// Pool metrics recorder: write periodic snapshots for dashboard charts
			// and send cluster heartbeats with live pool data.
			poolMetricsDone := make(chan struct{})
			go func() {
				defer close(poolMetricsDone)
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()
				// Record an initial snapshot immediately.
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
				_ = clusterReg.UpdateHeartbeat(context.Background(), nodeID(), &cluster.NodeMetrics{
					ActiveVMs:  p.TotalVMCount(),
					QueueDepth: 0,
				})
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
						_ = clusterReg.UpdateHeartbeat(context.Background(), nodeID(), &cluster.NodeMetrics{
							ActiveVMs:  p.TotalVMCount(),
							QueueDepth: 0,
						})
						// Prune old records (keep 7 days)
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
					asyncWorkers.Stop()
					exec.Shutdown(10 * time.Second)
					be.Shutdown()
					return nil
				case <-ticker.C:
					ctx := context.Background()
					funcs, err := s.ListFunctions(ctx, 0, 0)
					if err != nil {
						logging.Op().Error("error listing functions", "error", err)
						continue
					}
					for _, fn := range funcs {
						codeRecord, err := s.GetFunctionCode(ctx, fn.ID)
						if err != nil || codeRecord == nil {
							logging.Op().Debug("skipping function with no code", "function", fn.Name)
							continue
						}
						codeContent := codeRecord.CompiledBinary
						if len(codeContent) == 0 {
							codeContent = []byte(codeRecord.SourceCode)
						}
						if err := p.EnsureReady(ctx, fn, codeContent); err != nil {
							logging.Op().Error("error ensuring ready", "function", fn.Name, "error", err)
						}
					}
				}
			}
		},
	}

	cmd.Flags().DurationVar(&idleTTL, "idle-ttl", 60*time.Second, "Worker idle timeout")
	cmd.Flags().StringVar(&grpcAddr, "grpc", ":9090", "Comet gRPC address")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level")

	return cmd
}

// nodeID returns a stable identifier for this comet instance.
func nodeID() string {
	if id := os.Getenv("NOVA_NODE_ID"); id != "" {
		return id
	}
	if h, err := os.Hostname(); err == nil {
		return "comet-" + h
	}
	return "comet-local"
}

// nodeName returns a human-readable name for this comet instance.
func nodeName() string {
	if name := os.Getenv("NOVA_NODE_NAME"); name != "" {
		return name
	}
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "comet-local"
}
