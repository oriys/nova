package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
	"golang.org/x/sync/errgroup"
)

// InvokeStream executes a function in streaming mode, calling the callback for each chunk
func (e *Executor) InvokeStream(ctx context.Context, funcName string, payload json.RawMessage, callback func(chunk []byte, isLast bool, err error) error) error {
	// Check if executor is shutting down
	if e.closing.Load() {
		return fmt.Errorf("executor is shutting down")
	}

	e.inflight.Add(1)
	defer e.inflight.Done()

	// Resolve function and code (same as regular Invoke)
	fn, err := e.store.GetFunctionByName(ctx, funcName)
	if err != nil {
		return fmt.Errorf("get function: %w", err)
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
		return err
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
		resolved, err := e.secretsResolver.ResolveEnvVars(ctx, fn.EnvVars)
		if err != nil {
			return fmt.Errorf("resolve secrets: %w", err)
		}
		fn.EnvVars = resolved
	}

	// Apply resolved layer paths
	for _, l := range layers {
		fn.LayerPaths = append(fn.LayerPaths, l.ImagePath)
	}

	// Resolve volume mounts to host-side image paths
	fn.ResolvedMounts = resolveVolumeMounts(fn.Mounts, volumes)

	// For compiled languages, check compilation status
	if domain.NeedsCompilation(fn.Runtime) {
		switch codeRecord.CompileStatus {
		case domain.CompileStatusCompiling:
			return fmt.Errorf("function '%s' is still compiling", fn.Name)
		case domain.CompileStatusFailed:
			return fmt.Errorf("function '%s' compilation failed: %s", fn.Name, codeRecord.CompileError)
		case domain.CompileStatusPending:
			return fmt.Errorf("function '%s' compilation is pending", fn.Name)
		}
		if len(codeRecord.CompiledBinary) == 0 {
			return fmt.Errorf("function '%s' has no compiled binary", fn.Name)
		}
	}

	// Determine code content
	var codeContent []byte
	var files map[string][]byte

	if hasMultiFiles {
		files, err = e.store.GetFunctionFiles(ctx, fn.ID)
		if err != nil {
			return fmt.Errorf("get function files: %w", err)
		}

		if len(codeRecord.CompiledBinary) > 0 {
			files[fn.Handler] = codeRecord.CompiledBinary
		}

		if _, ok := files["handler"]; !ok {
			if entry, ok := files[fn.Handler]; ok {
				files["handler"] = entry
			}
		}
	} else {
		if len(codeRecord.CompiledBinary) > 0 {
			codeContent = codeRecord.CompiledBinary
		} else {
			codeContent = []byte(codeRecord.SourceCode)
		}
	}

	reqID := uuid.New().String()[:8]

	// Start tracing span
	ctx, span := observability.StartSpan(ctx, "nova.invoke.stream",
		observability.AttrFunctionName.String(fn.Name),
		observability.AttrFunctionID.String(fn.ID),
		observability.AttrRuntime.String(string(fn.Runtime)),
		observability.AttrRequestID.String(reqID),
	)
	defer span.End()

	metrics.IncActiveRequests()
	defer metrics.DecActiveRequests()

	traceID := observability.GetTraceID(ctx)
	spanID := observability.GetSpanID(ctx)

	start := time.Now()

	// Acquire VM
	var pvm *pool.PooledVM
	if files != nil && len(files) > 0 {
		pvm, err = e.pool.AcquireWithFiles(ctx, fn, files)
	} else {
		pvm, err = e.pool.Acquire(ctx, fn, codeContent)
	}
	if err != nil {
		observability.SetSpanError(span, err)
		return fmt.Errorf("acquire VM: %w", err)
	}
	defer e.pool.Release(pvm)

	span.SetAttributes(
		observability.AttrColdStart.Bool(pvm.ColdStart),
		observability.AttrVMID.String(pvm.VM.ID),
	)

	// Execute in streaming mode
	tc := observability.ExtractTraceContext(ctx)
	var execErr error
	err = pvm.Client.ExecuteStream(reqID, payload, fn.TimeoutS, tc.TraceParent, tc.TraceState, callback)
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
		execErr = err
		// Async: metrics recording
		safeGo(func() {
			metrics.Global().RecordInvocationWithDetails(fn.ID, fn.Name, string(fn.Runtime), durationMs, pvm.ColdStart, false)
		})

		logEntry.Success = false
		logEntry.Error = err.Error()

		// Async: request logging
		safeGo(func() { e.logger.Log(logEntry) })

		observability.SetSpanError(span, err)

		// Async persist invocation log
		e.persistInvocationLog(reqID, fn, durationMs, pvm.ColdStart, false, err.Error(), len(payload), 0, payload, nil, "", "")

		return fmt.Errorf("execute stream: %w", err)
	}

	// Async: record successful streaming invocation
	safeGo(func() {
		metrics.Global().RecordInvocationWithDetails(fn.ID, fn.Name, string(fn.Runtime), durationMs, pvm.ColdStart, true)
	})

	logEntry.Success = true

	// Async: request logging
	safeGo(func() { e.logger.Log(logEntry) })

	// Async persist invocation log (output is streamed, so we don't have it)
	e.persistInvocationLog(reqID, fn, durationMs, pvm.ColdStart, true, "", len(payload), 0, payload, nil, "", "")

	observability.SetSpanOK(span)
	return execErr
}
