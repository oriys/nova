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
	store           store.MetadataStore
	pool            *pool.Pool
	logger          *logging.Logger
	secretsResolver *secrets.Resolver
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

func New(store store.MetadataStore, pool *pool.Pool, opts ...Option) *Executor {
	e := &Executor{
		store:  store,
		pool:   pool,
		logger: logging.Default(),
	}
	for _, opt := range opts {
		opt(e)
	}
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

	// Resolve $SECRET: references in env vars
	if e.secretsResolver != nil && len(fn.EnvVars) > 0 {
		resolved, err := e.secretsResolver.ResolveEnvVars(ctx, fn.EnvVars)
		if err != nil {
			return nil, fmt.Errorf("resolve secrets: %w", err)
		}
		fn.EnvVars = resolved
	}

	// Refresh code hash for change detection
	e.refreshCodeHash(ctx, fn)

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

	pvm, err := e.pool.Acquire(ctx, fn)
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

// refreshCodeHash checks if code file has changed and updates the function's hash.
func (e *Executor) refreshCodeHash(ctx context.Context, fn *domain.Function) {
	currentHash, err := domain.HashCodeFile(fn.CodePath)
	if err != nil {
		return // Can't read file, skip check
	}

	if fn.CodeHash != "" && fn.CodeHash != currentHash {
		logging.Op().Info("code change detected", "function", fn.Name)
		fn.CodeHash = currentHash
		fn.UpdatedAt = time.Now()
		_ = e.store.SaveFunction(ctx, fn)
	} else if fn.CodeHash == "" {
		fn.CodeHash = currentHash
		_ = e.store.SaveFunction(ctx, fn)
	}
}

// InvalidateSnapshot removes the snapshot for a function (e.g., after code update)
func InvalidateSnapshot(snapshotDir, funcID string) error {
	paths := []string{
		filepath.Join(snapshotDir, funcID+".snap"),
		filepath.Join(snapshotDir, funcID+".mem"),
		filepath.Join(snapshotDir, funcID+".meta"),
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

	e.pool.Shutdown()
}
