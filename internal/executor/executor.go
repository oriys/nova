// Package executor orchestrates function invocations on behalf of the
// data-plane API and the remote gRPC endpoint.
//
// # Invocation pipeline
//
// Invoke is the single entry point for all synchronous function calls.
// The pipeline is:
//
//  1. Drain-check: reject if the executor is shutting down.
//  2. Parallel pre-fetch: runtime config, layers, volumes, code, and
//     multi-file flag are fetched concurrently via errgroup to minimise
//     round-trip latency.
//  3. Secret resolution: $SECRET: references in EnvVars are substituted
//     using the secrets resolver before the VM sees any environment.
//  4. Circuit-breaker check: if the per-function breaker is open the call
//     is rejected immediately without touching the pool.
//  5. Compilation guard: compiled runtimes (Go, Rust, Java, …) require a
//     successful CompileStatus before a VM is acquired; this prevents the
//     agent from receiving an incomplete binary.
//  6. VM acquisition: a warm VM is taken from the pool, or a cold start is
//     performed if none is available.
//  7. Execution: the payload is forwarded to the guest agent over vsock;
//     the response includes stdout/stderr and timing information.
//  8. Async side-effects: metrics recording, structured logging, output
//     capture, invocation-log persistence, and circuit-breaker bookkeeping
//     are all fire-and-forget to keep the critical path lean.
//
// # Concurrency
//
// Executor is safe for concurrent use. The inflight WaitGroup is used to
// drain in-flight calls during graceful shutdown (see GracefulShutdown).
// Each call increments the counter before any work begins and decrements it
// on return, so Shutdown blocks until all active invocations finish.
//
// # Side effects
//
// Every successful or failed invocation triggers the following side-effects
// (all asynchronous unless noted):
//   - Prometheus metrics update (RecordInvocationWithDetails)
//   - Structured request log via the Logger
//   - stdout/stderr capture via the output store
//   - Invocation log row persisted to Postgres via the log batcher
//   - Circuit-breaker success/failure counter update
//
// # Failure behaviour
//
// A VM that returns an execution error is immediately evicted from the pool
// (EvictVM) rather than returned to the warm set. This prevents a poisoned
// process from serving subsequent requests.
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/circuitbreaker"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/logsink"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/secrets"
	"github.com/oriys/nova/internal/store"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/errgroup"
)

// ErrCircuitOpen is returned when the circuit breaker is open for a function.
var ErrCircuitOpen = fmt.Errorf("circuit breaker is open")

// Executor orchestrates the full invocation pipeline for a single Nova node.
// It is the only component that may acquire VMs from the pool and send
// execution requests to the guest agent.
//
// The zero value is not usable; always construct via New.
type Executor struct {
	store            *store.Store
	pool             *pool.Pool
	logger           *logging.Logger
	secretsResolver  *secrets.Resolver
	transportCipher  *secrets.TransportCipher // encrypts secret values in vsock Init messages
	logSink          logsink.LogSink
	logBatcher       *invocationLogBatcher
	logBatcherConfig LogBatcherConfig
	persistPayloads  bool
	inflight         sync.WaitGroup // drained by GracefulShutdown
	closing          atomic.Bool    // set true before draining; rejects new calls
	breakers         *circuitbreaker.Registry
}

// New creates a ready-to-use Executor. The logSink defaults to a Postgres
// sink backed by store if not overridden via WithLogSink.
func New(store *store.Store, pool *pool.Pool, opts ...Option) *Executor {
	e := &Executor{
		store:           store,
		pool:            pool,
		logger:          logging.Default(),
		breakers:        circuitbreaker.NewRegistry(),
		persistPayloads: false,
	}
	for _, opt := range opts {
		opt(e)
	}
	sink := e.logSink
	if sink == nil {
		sink = logsink.NewPostgresSink(store)
	}
	e.logBatcher = newInvocationLogBatcher(store, sink, e.logBatcherConfig)
	return e
}

// Invoke executes the named function with the supplied JSON payload and
// returns a structured response.
//
// # Contract
//
// Preconditions:
//   - funcName must identify an existing, deployed function in the store.
//   - payload must be valid JSON or nil (nil is passed as "null" to the handler).
//
// Postconditions:
//   - On success, InvokeResponse.RequestID is a unique 8-hex-char correlation
//     ID that appears in metrics, structured logs, and the invocation log.
//   - On error, any VM that was acquired is evicted to prevent reuse of a
//     potentially broken process.
//
// # Idempotency
//
// Not idempotent. Each call creates a new invocation record with a fresh
// RequestID. Callers that need at-most-once or exactly-once semantics must
// implement deduplication upstream (e.g. via the async-queue idempotency key).
//
// # Side effects
//
// Regardless of success or failure, Invoke writes asynchronously to:
//   - Prometheus metrics (invocation count, latency, cold-start ratio)
//   - Structured request log
//   - Invocation log table in Postgres
//   - Output capture store (stdout/stderr) when available
//
// # Concurrency
//
// Safe for concurrent use. The method increments the inflight counter before
// any work begins so GracefulShutdown can drain all in-flight calls.
func (e *Executor) Invoke(ctx context.Context, funcName string, payload json.RawMessage) (*domain.InvokeResponse, error) {
	// Reject early during shutdown so we don't start work that cannot finish.
	if e.closing.Load() {
		return nil, fmt.Errorf("executor is shutting down")
	}

	e.inflight.Add(1)
	defer e.inflight.Done()

	fn, err := e.store.GetFunctionByName(ctx, funcName)
	if err != nil {
		return nil, fmt.Errorf("get function: %w", err)
	}
	requestedFunction := fn.Name
	fn = e.selectRolloutTarget(ctx, fn)
	if fn.Name != requestedFunction {
		logging.Op().Debug(
			"rollout canary selected",
			"requested_function", requestedFunction,
			"target_function", fn.Name,
		)
	}

	// Parallel pre-execution queries: runtime config, layers, code, and multi-file check
	var (
		rtCfg         *store.RuntimeRecord
		layers        []*domain.Layer
		volumes       []*domain.Volume
		codeRecord    *domain.FunctionCode
		hasMultiFiles bool
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		rtCfg, err = e.store.GetRuntime(gctx, string(fn.Runtime))
		if err != nil {
			if fn.Runtime != domain.RuntimeCustom && fn.Runtime != domain.RuntimeProvided {
				return fmt.Errorf("get runtime config: %w", err)
			}
		}
		return nil
	})

	if len(fn.Layers) > 0 {
		g.Go(func() error {
			var err error
			layers, err = e.store.GetFunctionLayers(gctx, fn.ID)
			if err != nil {
				logging.Op().Warn("failed to resolve layers", "function", fn.Name, "error", err)
			}
			return nil
		})
	}

	if len(fn.Mounts) > 0 {
		g.Go(func() error {
			var err error
			volumes, err = e.store.GetFunctionVolumes(gctx, fn.ID)
			if err != nil {
				logging.Op().Warn("failed to resolve volumes", "function", fn.Name, "error", err)
			}
			return nil
		})
	}

	g.Go(func() error {
		var err error
		codeRecord, err = e.store.GetFunctionCode(gctx, fn.ID)
		if err != nil {
			return fmt.Errorf("get function code: %w", err)
		}
		if codeRecord == nil {
			return fmt.Errorf("function code not found: %s", fn.Name)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		hasMultiFiles, err = e.store.HasFunctionFiles(gctx, fn.ID)
		if err != nil {
			return fmt.Errorf("check function files: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Apply runtime config
	if rtCfg != nil {
		fn.RuntimeCommand = append([]string(nil), rtCfg.Entrypoint...)
		fn.RuntimeExtension = rtCfg.FileExtension
		fn.RuntimeImageName = rtCfg.ImageName
		if fn.EnvVars == nil {
			fn.EnvVars = map[string]string{}
		}
		for k, v := range rtCfg.EnvVars {
			if _, ok := fn.EnvVars[k]; !ok {
				fn.EnvVars[k] = v
			}
		}
	}

	// Resolve $SECRET: references in env vars (depends on runtime config merge above)
	if e.secretsResolver != nil && len(fn.EnvVars) > 0 {
		// Track which env var keys hold secret values so we can encrypt
		// them before sending over vsock to prevent plaintext leakage.
		secretKeys := make(map[string]bool)
		for k, v := range fn.EnvVars {
			if secrets.IsSecretRef(v) {
				secretKeys[k] = true
			}
		}

		resolved, err := e.secretsResolver.ResolveEnvVars(ctx, fn.EnvVars)
		if err != nil {
			return nil, fmt.Errorf("resolve secrets: %w", err)
		}

		// Encrypt resolved secret values for safe transport over vsock.
		// The agent decrypts them using the shared transport key.
		if e.transportCipher != nil && len(secretKeys) > 0 {
			resolved, err = e.transportCipher.EncryptEnvVars(resolved, secretKeys)
			if err != nil {
				return nil, fmt.Errorf("encrypt secrets for transport: %w", err)
			}
		}

		fn.EnvVars = resolved
	}

	// Apply resolved layer paths
	for _, l := range layers {
		fn.LayerPaths = append(fn.LayerPaths, l.ImagePath)
	}

	// Resolve volume mounts to host-side image paths
	fn.ResolvedMounts = resolveVolumeMounts(fn.Mounts, volumes)

	// Circuit breaker check
	breaker := e.getBreakerForFunction(fn)
	if breaker != nil && !breaker.Allow() {
		metrics.RecordShed(fn.Name, "circuit_breaker_open")
		return nil, ErrCircuitOpen
	}

	// For compiled languages, check compilation status before proceeding
	if domain.NeedsCompilation(fn.Runtime) {
		switch codeRecord.CompileStatus {
		case domain.CompileStatusCompiling:
			return nil, fmt.Errorf("function '%s' is still compiling", fn.Name)
		case domain.CompileStatusFailed:
			return nil, fmt.Errorf("function '%s' compilation failed: %s", fn.Name, codeRecord.CompileError)
		case domain.CompileStatusPending:
			return nil, fmt.Errorf("function '%s' compilation is pending", fn.Name)
		}
		if len(codeRecord.CompiledBinary) == 0 {
			return nil, fmt.Errorf("function '%s' has no compiled binary", fn.Name)
		}
	}

	// Determine code content
	var codeContent []byte
	var files map[string][]byte

	if hasMultiFiles {
		// Fetch all files for multi-file function
		files, err = e.store.GetFunctionFiles(ctx, fn.ID)
		if err != nil {
			return nil, fmt.Errorf("get function files: %w", err)
		}

		// For compiled languages with multi-file, use the compiled binary
		if len(codeRecord.CompiledBinary) > 0 {
			// Replace the entry point file with compiled binary
			files[fn.Handler] = codeRecord.CompiledBinary
		}

		// Agent always executes/loads /code/handler.
		// Keep a canonical alias even when entry point is a different file name.
		if _, ok := files["handler"]; !ok {
			if entry, ok := files[fn.Handler]; ok {
				files["handler"] = entry
			}
		}
	} else {
		// Single file function
		if len(codeRecord.CompiledBinary) > 0 {
			codeContent = codeRecord.CompiledBinary
		} else {
			codeContent = []byte(codeRecord.SourceCode)
		}
	}

	reqID := uuid.New().String()[:8]

	// Start tracing span
	ctx, span := observability.StartSpan(ctx, "nova.invoke",
		observability.AttrFunctionName.String(fn.Name),
		observability.AttrFunctionID.String(fn.ID),
		observability.AttrRuntime.String(string(fn.Runtime)),
		observability.AttrRequestID.String(reqID),
	)
	defer span.End()

	// Track active requests
	metrics.IncActiveRequests()
	defer metrics.DecActiveRequests()

	traceID := observability.GetTraceID(ctx)
	spanID := observability.GetSpanID(ctx)

	start := time.Now()

	var pvm *pool.PooledVM
	if files != nil && len(files) > 0 {
		pvm, err = e.pool.AcquireWithFiles(ctx, fn, files)
	} else {
		pvm, err = e.pool.Acquire(ctx, fn, codeContent)
	}
	if err != nil {
		observability.SetSpanError(span, err)
		return nil, fmt.Errorf("acquire VM: %w", err)
	}
	defer e.pool.Release(pvm)

	span.SetAttributes(
		observability.AttrColdStart.Bool(pvm.ColdStart),
		observability.AttrVMID.String(pvm.VM.ID),
	)

	// Propagate trace context over vsock
	tc := observability.ExtractTraceContext(ctx)
	resp, err := pvm.Client.ExecuteWithTrace(reqID, payload, fn.TimeoutS, tc.TraceParent, tc.TraceState)
	durationMs := time.Since(start).Milliseconds()

	span.SetAttributes(observability.AttrDurationMs.Int64(durationMs))

	// Log the request
	logEntry := &logging.RequestLog{
		RequestID:  reqID,
		TraceID:    traceID,
		SpanID:     spanID,
		Function:   fn.Name,
		FunctionID: fn.ID,
		Runtime:    string(fn.Runtime),
		DurationMs: durationMs,
		ColdStart:  pvm.ColdStart,
		InputSize:  len(payload),
	}

	if err != nil {
		e.pool.EvictVM(fn.ID, pvm)

		// Async: metrics recording
		safeGo(func() {
			metrics.Global().RecordInvocationWithDetails(fn.ID, fn.Name, string(fn.Runtime), durationMs, pvm.ColdStart, false)
		})

		logEntry.Success = false
		logEntry.Error = err.Error()

		// Async: request logging
		safeGo(func() { e.logger.Log(logEntry) })

		observability.SetSpanError(span, err)

		// Record circuit breaker failure
		if breaker != nil {
			breaker.RecordFailure()
		}

		// Async persist invocation log to database
		e.persistInvocationLog(reqID, fn, durationMs, pvm.ColdStart, false, err.Error(), len(payload), 0, payload, nil, "", "")

		return nil, fmt.Errorf("execute: %w", err)
	}

	// Record successful invocation
	success := resp.Error == ""

	// Async: metrics recording
	safeGo(func() {
		metrics.Global().RecordInvocationWithDetails(fn.ID, fn.Name, string(fn.Runtime), durationMs, pvm.ColdStart, success)
	})

	// Record circuit breaker outcome
	if breaker != nil {
		if success {
			breaker.RecordSuccess()
		} else {
			breaker.RecordFailure()
		}
	}

	logEntry.Success = success
	logEntry.Error = resp.Error
	logEntry.OutputSize = len(resp.Output)

	// Async: request logging
	safeGo(func() { e.logger.Log(logEntry) })

	// Async: store captured output if available
	if resp.Stdout != "" || resp.Stderr != "" {
		if outStore := logging.GetOutputStore(); outStore != nil {
			safeGo(func() { outStore.Store(reqID, fn.ID, resp.Stdout, resp.Stderr) })
		}
	}

	// Async persist invocation log to database
	e.persistInvocationLog(reqID, fn, durationMs, pvm.ColdStart, success, resp.Error, len(payload), len(resp.Output), payload, resp.Output, resp.Stdout, resp.Stderr)

	if success {
		observability.SetSpanOK(span)
	} else {
		span.SetAttributes(attribute.String("nova.error", resp.Error))
	}

	return &domain.InvokeResponse{
		RequestID:  reqID,
		Output:     resp.Output,
		Error:      resp.Error,
		DurationMs: durationMs,
		ColdStart:  pvm.ColdStart,
	}, nil
}
