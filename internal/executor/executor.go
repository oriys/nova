package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/secrets"
	"github.com/oriys/nova/internal/store"
	"go.opentelemetry.io/otel/attribute"
)

type Executor struct {
	store           *store.Store
	pool            *pool.Pool
	logger          *logging.Logger
	secretsResolver *secrets.Resolver
	logBatcher      *invocationLogBatcher
	logBatcherConfig LogBatcherConfig
	inflight        sync.WaitGroup
	closing         atomic.Bool
}

type Option func(*Executor)

// WithLogger sets the logger
func WithLogger(logger *logging.Logger) Option {
	return func(e *Executor) {
		e.logger = logger
	}
}

// WithSecretsResolver sets the secrets resolver for $SECRET: reference resolution
func WithSecretsResolver(resolver *secrets.Resolver) Option {
	return func(e *Executor) {
		e.secretsResolver = resolver
	}
}

// WithLogBatcherConfig sets the log batcher configuration
func WithLogBatcherConfig(cfg LogBatcherConfig) Option {
	return func(e *Executor) {
		e.logBatcherConfig = cfg
	}
}

func New(store *store.Store, pool *pool.Pool, opts ...Option) *Executor {
	e := &Executor{
		store:  store,
		pool:   pool,
		logger: logging.Default(),
	}
	for _, opt := range opts {
		opt(e)
	}
	e.logBatcher = newInvocationLogBatcher(store, e.logBatcherConfig)
	return e
}

func (e *Executor) Invoke(ctx context.Context, funcName string, payload json.RawMessage) (*domain.InvokeResponse, error) {
	// Check if executor is shutting down
	if e.closing.Load() {
		return nil, fmt.Errorf("executor is shutting down")
	}

	e.inflight.Add(1)
	defer e.inflight.Done()

	fn, err := e.store.GetFunctionByName(ctx, funcName)
	if err != nil {
		return nil, fmt.Errorf("get function: %w", err)
	}

	rtCfg, err := e.store.GetRuntime(ctx, string(fn.Runtime))
	if err != nil {
		if fn.Runtime != domain.RuntimeCustom && fn.Runtime != domain.RuntimeProvided {
			return nil, fmt.Errorf("get runtime config: %w", err)
		}
	} else {
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

	// Resolve $SECRET: references in env vars
	if e.secretsResolver != nil && len(fn.EnvVars) > 0 {
		resolved, err := e.secretsResolver.ResolveEnvVars(ctx, fn.EnvVars)
		if err != nil {
			return nil, fmt.Errorf("resolve secrets: %w", err)
		}
		fn.EnvVars = resolved
	}

	// Fetch code content from store
	codeRecord, err := e.store.GetFunctionCode(ctx, fn.ID)
	if err != nil {
		return nil, fmt.Errorf("get function code: %w", err)
	}
	if codeRecord == nil {
		return nil, fmt.Errorf("function code not found: %s", fn.Name)
	}

	// Check for multi-file function
	hasMultiFiles, err := e.store.HasFunctionFiles(ctx, fn.ID)
	if err != nil {
		return nil, fmt.Errorf("check function files: %w", err)
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
		metrics.Global().RecordInvocationWithDetails(fn.ID, fn.Name, string(fn.Runtime), durationMs, pvm.ColdStart, false)
		logEntry.Success = false
		logEntry.Error = err.Error()
		e.logger.Log(logEntry)
		observability.SetSpanError(span, err)

		// Async persist invocation log to database
		e.persistInvocationLog(reqID, fn, durationMs, pvm.ColdStart, false, err.Error(), len(payload), 0, payload, nil, "", "")

		return nil, fmt.Errorf("execute: %w", err)
	}

	// Record successful invocation
	success := resp.Error == ""
	metrics.Global().RecordInvocationWithDetails(fn.ID, fn.Name, string(fn.Runtime), durationMs, pvm.ColdStart, success)

	logEntry.Success = success
	logEntry.Error = resp.Error
	logEntry.OutputSize = len(resp.Output)
	e.logger.Log(logEntry)

	// Store captured output if available
	if resp.Stdout != "" || resp.Stderr != "" {
		if store := logging.GetOutputStore(); store != nil {
			store.Store(reqID, fn.ID, resp.Stdout, resp.Stderr)
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

// persistInvocationLog asynchronously saves an invocation log to Postgres
func (e *Executor) persistInvocationLog(reqID string, fn *domain.Function, durationMs int64, coldStart, success bool, errMsg string, inputSize, outputSize int, input, output json.RawMessage, stdout, stderr string) {
	e.logBatcher.Enqueue(&store.InvocationLog{
		ID:           reqID,
		FunctionID:   fn.ID,
		FunctionName: fn.Name,
		Runtime:      string(fn.Runtime),
		DurationMs:   durationMs,
		ColdStart:    coldStart,
		Success:      success,
		ErrorMessage: errMsg,
		InputSize:    inputSize,
		OutputSize:   outputSize,
		Input:        input,
		Output:       output,
		Stdout:       stdout,
		Stderr:       stderr,
		CreatedAt:    time.Now(),
	})
}

// InvalidateSnapshot removes the snapshot for a function (e.g., after code update)
func InvalidateSnapshot(snapshotDir, funcID string) error {
	if snapshotDir == "" {
		return nil
	}
	metaPath := filepath.Join(snapshotDir, funcID+".meta")
	if metaData, err := os.ReadFile(metaPath); err == nil {
		var meta struct {
			CodeDrive       string `json:"code_drive"`
			CodeDriveBackup string `json:"code_drive_backup"`
		}
		if json.Unmarshal(metaData, &meta) == nil {
			if meta.CodeDrive != "" {
				if err := os.Remove(meta.CodeDrive); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			if meta.CodeDriveBackup != "" {
				if err := os.Remove(meta.CodeDriveBackup); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
		}
	}

	paths := []string{
		filepath.Join(snapshotDir, funcID+".snap"),
		filepath.Join(snapshotDir, funcID+".mem"),
		metaPath,
	}

	var lastErr error
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			lastErr = err
		}
	}
	return lastErr
}

// HasSnapshot checks if a function has a valid snapshot
func HasSnapshot(snapshotDir, funcID string) bool {
	if snapshotDir == "" {
		return false
	}
	snapPath := filepath.Join(snapshotDir, funcID+".snap")
	memPath := filepath.Join(snapshotDir, funcID+".mem")

	if _, err := os.Stat(snapPath); err != nil {
		return false
	}
	if _, err := os.Stat(memPath); err != nil {
		return false
	}
	return true
}

// Shutdown gracefully shuts down the executor, waiting for in-flight requests
func (e *Executor) Shutdown(timeout time.Duration) {
	e.closing.Store(true)

	// Wait for in-flight requests with timeout
	done := make(chan struct{})
	go func() {
		e.inflight.Wait()
		close(done)
	}()

	select {
	case <-done:
		logging.Op().Info("all in-flight requests completed")
	case <-time.After(timeout):
		logging.Op().Warn("shutdown timeout waiting for in-flight requests", "timeout", timeout)
	}

	e.logBatcher.Shutdown(timeout)
	e.pool.Shutdown()
}
