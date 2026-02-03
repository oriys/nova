package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// LocalExecutor runs functions directly on the host without VMs
type LocalExecutor struct {
	store *store.Store
}

// NewLocalExecutor creates a new local executor
func NewLocalExecutor(s *store.Store) *LocalExecutor {
	return &LocalExecutor{store: s}
}

// Invoke executes a function locally without VM isolation
func (e *LocalExecutor) Invoke(ctx context.Context, fn *domain.Function, payload json.RawMessage) (*domain.InvokeResponse, error) {
	reqID := uuid.New().String()[:8]
	start := time.Now()

	// Fetch code content from store
	codeRecord, err := e.store.GetFunctionCode(ctx, fn.ID)
	if err != nil {
		return nil, fmt.Errorf("get function code: %w", err)
	}
	if codeRecord == nil {
		return nil, fmt.Errorf("function code not found: %s", fn.Name)
	}

	// Use compiled binary if available, otherwise use source code
	var codeContent []byte
	if len(codeRecord.CompiledBinary) > 0 {
		codeContent = codeRecord.CompiledBinary
	} else {
		codeContent = []byte(codeRecord.SourceCode)
	}

	// Write code to temp file
	codeFile, err := os.CreateTemp("", "nova-code-*")
	if err != nil {
		return nil, fmt.Errorf("create code file: %w", err)
	}
	codePath := codeFile.Name()
	defer os.Remove(codePath)

	if _, err := codeFile.Write(codeContent); err != nil {
		codeFile.Close()
		return nil, fmt.Errorf("write code: %w", err)
	}
	if err := codeFile.Chmod(0755); err != nil {
		codeFile.Close()
		return nil, fmt.Errorf("chmod code: %w", err)
	}
	codeFile.Close()

	// Write input to temp file
	inputFile, err := os.CreateTemp("", "nova-input-*.json")
	if err != nil {
		return nil, fmt.Errorf("create input file: %w", err)
	}
	defer os.Remove(inputFile.Name())

	if _, err := inputFile.Write(payload); err != nil {
		inputFile.Close()
		return nil, fmt.Errorf("write input: %w", err)
	}
	inputFile.Close()

	// Build command based on runtime
	var cmd *exec.Cmd

	switch fn.Runtime {
	case domain.RuntimePython:
		cmd = exec.CommandContext(ctx, "python3", codePath, inputFile.Name())
	case domain.RuntimeGo:
		// For Go, the code path should be a compiled binary
		cmd = exec.CommandContext(ctx, codePath, inputFile.Name())
	case domain.RuntimeRust:
		// For Rust, the code path should be a compiled binary
		cmd = exec.CommandContext(ctx, codePath, inputFile.Name())
	case domain.RuntimeWasm:
		cmd = exec.CommandContext(ctx, "wasmtime", codePath, "--", inputFile.Name())
	default:
		return nil, fmt.Errorf("unsupported runtime for local execution: %s", fn.Runtime)
	}

	// Set environment
	cmd.Env = append(os.Environ(),
		"NOVA_LOCAL=true",
		"NOVA_FUNCTION_NAME="+fn.Name,
		"NOVA_REQUEST_ID="+reqID,
		"NOVA_CODE_DIR="+filepath.Dir(codePath),
	)
	for k, v := range fn.EnvVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute with timeout
	if fn.TimeoutS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(fn.TimeoutS)*time.Second)
		defer cancel()
		// Recreate command with timeout context, preserving env
		env := cmd.Env
		cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Env = env
	}

	err = cmd.Run()
	durationMs := time.Since(start).Milliseconds()

	resp := &domain.InvokeResponse{
		RequestID:  reqID,
		DurationMs: durationMs,
		ColdStart:  false, // Local execution is never a "cold start"
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			resp.Error = "timeout"
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			resp.Error = fmt.Sprintf("exit %d: %s", exitErr.ExitCode(), stderr.String())
		} else {
			resp.Error = err.Error()
		}
		return resp, nil
	}

	// Parse output
	output := stdout.Bytes()
	if json.Valid(output) {
		resp.Output = output
	} else {
		// Wrap non-JSON output as string
		resp.Output, _ = json.Marshal(string(output))
	}

	return resp, nil
}

// InvokeWithStore looks up function by name and invokes it locally
func (e *LocalExecutor) InvokeWithStore(ctx context.Context, funcName string, payload json.RawMessage) (*domain.InvokeResponse, error) {
	fn, err := e.store.GetFunctionByName(ctx, funcName)
	if err != nil {
		return nil, fmt.Errorf("get function: %w", err)
	}
	return e.Invoke(ctx, fn, payload)
}
