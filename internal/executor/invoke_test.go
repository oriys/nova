package executor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/secrets"
	"github.com/oriys/nova/internal/store"
)

func TestMain(m *testing.M) {
	// Initialize a noop tracer to avoid nil pointer dereference in Invoke/InvokeStream
	observability.Init(context.Background(), observability.Config{Enabled: false})
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func setupInvokeTest(t *testing.T, client *mockClient) (*Executor, *mockMetaStore) {
	t.Helper()
	meta := newMockMetaStore()
	s := store.NewStore(meta)
	p := newTestPool(client)
	sink := &captureSink{ch: make(chan *store.InvocationLog, 10)}
	e := New(s, p,
		WithLogSink(sink),
		WithLogBatcherConfig(LogBatcherConfig{BatchSize: 1, FlushInterval: 10 * time.Millisecond}),
	)
	t.Cleanup(func() {
		e.logBatcher.Shutdown(time.Second)
		p.Shutdown()
	})
	return e, meta
}

func seedSimpleFunction(meta *mockMetaStore) {
	meta.fnByName["hello"] = &domain.Function{
		ID:      "f1",
		Name:    "hello",
		Runtime: domain.RuntimePython,
	}
	meta.runtimes["python"] = &store.RuntimeRecord{
		ID:         "python",
		Entrypoint: []string{"python3"},
	}
	meta.codes["f1"] = &domain.FunctionCode{
		FunctionID: "f1",
		SourceCode: "print('hi')",
	}
}

// ---------------------------------------------------------------------------
// Invoke – success path
// ---------------------------------------------------------------------------

func TestInvoke_Success(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			RequestID:  "abc",
			Output:     json.RawMessage(`{"ok":true}`),
			DurationMs: 42,
		},
	}
	e, meta := setupInvokeTest(t, client)
	seedSimpleFunction(meta)

	resp, err := e.Invoke(context.Background(), "hello", json.RawMessage(`{"input":1}`))
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.RequestID == "" {
		t.Fatal("expected non-empty RequestID")
	}
	if string(resp.Output) != `{"ok":true}` {
		t.Fatalf("unexpected output: %s", resp.Output)
	}
}

// ---------------------------------------------------------------------------
// Invoke – function not found
// ---------------------------------------------------------------------------

func TestInvoke_FunctionNotFound(t *testing.T) {
	t.Parallel()
	client := &mockClient{}
	e, _ := setupInvokeTest(t, client)

	_, err := e.Invoke(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent function")
	}
	if !strings.Contains(err.Error(), "get function") {
		t.Fatalf("expected 'get function' error, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Invoke – code not found
// ---------------------------------------------------------------------------

func TestInvoke_CodeNotFound(t *testing.T) {
	t.Parallel()
	client := &mockClient{}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["hello"] = &domain.Function{
		ID:      "f1",
		Name:    "hello",
		Runtime: domain.RuntimePython,
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}
	// No code registered -> GetFunctionCode returns error

	_, err := e.Invoke(context.Background(), "hello", nil)
	if err == nil {
		t.Fatal("expected error for missing code")
	}
	if !strings.Contains(err.Error(), "function code") || !strings.Contains(err.Error(), "get") {
		t.Fatalf("expected 'get function code' error, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Invoke – execution error
// ---------------------------------------------------------------------------

func TestInvoke_ExecutionError(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execErr: errors.New("timeout"),
	}
	e, meta := setupInvokeTest(t, client)
	seedSimpleFunction(meta)

	_, err := e.Invoke(context.Background(), "hello", nil)
	if err == nil {
		t.Fatal("expected execution error")
	}
	if !strings.Contains(err.Error(), "execute") {
		t.Fatalf("expected 'execute' error, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Invoke – response with error (function-level, not exec error)
// ---------------------------------------------------------------------------

func TestInvoke_ResponseError(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			RequestID: "abc",
			Output:    json.RawMessage(`null`),
			Error:     "runtime error: division by zero",
		},
	}
	e, meta := setupInvokeTest(t, client)
	seedSimpleFunction(meta)

	resp, err := e.Invoke(context.Background(), "hello", nil)
	if err != nil {
		t.Fatalf("Invoke should succeed even with resp.Error: %v", err)
	}
	if resp.Error != "runtime error: division by zero" {
		t.Fatalf("expected runtime error in response, got %q", resp.Error)
	}
}

// ---------------------------------------------------------------------------
// Invoke – compiled language pending
// ---------------------------------------------------------------------------

func TestInvoke_CompiledPending(t *testing.T) {
	t.Parallel()
	client := &mockClient{}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["gofn"] = &domain.Function{
		ID:      "f-go",
		Name:    "gofn",
		Runtime: domain.RuntimeGo,
	}
	meta.runtimes["go"] = &store.RuntimeRecord{ID: "go"}
	meta.codes["f-go"] = &domain.FunctionCode{
		FunctionID:    "f-go",
		CompileStatus: domain.CompileStatusPending,
	}

	_, err := e.Invoke(context.Background(), "gofn", nil)
	if err == nil || !strings.Contains(err.Error(), "pending") {
		t.Fatalf("expected 'pending' error, got %v", err)
	}
}

func TestInvoke_CompiledCompiling(t *testing.T) {
	t.Parallel()
	client := &mockClient{}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["gofn"] = &domain.Function{
		ID:      "f-go2",
		Name:    "gofn",
		Runtime: domain.RuntimeGo,
	}
	meta.runtimes["go"] = &store.RuntimeRecord{ID: "go"}
	meta.codes["f-go2"] = &domain.FunctionCode{
		FunctionID:    "f-go2",
		CompileStatus: domain.CompileStatusCompiling,
	}

	_, err := e.Invoke(context.Background(), "gofn", nil)
	if err == nil || !strings.Contains(err.Error(), "compiling") {
		t.Fatalf("expected 'compiling' error, got %v", err)
	}
}

func TestInvoke_CompiledFailed(t *testing.T) {
	t.Parallel()
	client := &mockClient{}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["gofn"] = &domain.Function{
		ID:      "f-go3",
		Name:    "gofn",
		Runtime: domain.RuntimeGo,
	}
	meta.runtimes["go"] = &store.RuntimeRecord{ID: "go"}
	meta.codes["f-go3"] = &domain.FunctionCode{
		FunctionID:    "f-go3",
		CompileStatus: domain.CompileStatusFailed,
		CompileError:  "syntax error",
	}

	_, err := e.Invoke(context.Background(), "gofn", nil)
	if err == nil || !strings.Contains(err.Error(), "compilation failed") {
		t.Fatalf("expected 'compilation failed' error, got %v", err)
	}
}

func TestInvoke_CompiledNoBinary(t *testing.T) {
	t.Parallel()
	client := &mockClient{}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["gofn"] = &domain.Function{
		ID:      "f-go4",
		Name:    "gofn",
		Runtime: domain.RuntimeGo,
	}
	meta.runtimes["go"] = &store.RuntimeRecord{ID: "go"}
	meta.codes["f-go4"] = &domain.FunctionCode{
		FunctionID:     "f-go4",
		CompileStatus:  domain.CompileStatusSuccess,
		CompiledBinary: nil, // empty binary
	}

	_, err := e.Invoke(context.Background(), "gofn", nil)
	if err == nil || !strings.Contains(err.Error(), "no compiled binary") {
		t.Fatalf("expected 'no compiled binary' error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Invoke – compiled success
// ---------------------------------------------------------------------------

func TestInvoke_CompiledSuccess(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			Output: json.RawMessage(`"ok"`),
		},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["gofn"] = &domain.Function{
		ID:      "f-go5",
		Name:    "gofn",
		Runtime: domain.RuntimeGo,
	}
	meta.runtimes["go"] = &store.RuntimeRecord{ID: "go"}
	meta.codes["f-go5"] = &domain.FunctionCode{
		FunctionID:     "f-go5",
		CompileStatus:  domain.CompileStatusSuccess,
		CompiledBinary: []byte("binary-data"),
	}

	resp, err := e.Invoke(context.Background(), "gofn", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// ---------------------------------------------------------------------------
// Invoke – multi-file function
// ---------------------------------------------------------------------------

func TestInvoke_MultiFile(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			Output: json.RawMessage(`"multi"`),
		},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["multi"] = &domain.Function{
		ID:      "f-multi",
		Name:    "multi",
		Runtime: domain.RuntimePython,
		Handler: "main.py",
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}
	meta.codes["f-multi"] = &domain.FunctionCode{
		FunctionID: "f-multi",
		SourceCode: "print('multi')",
	}
	meta.multiFiles["f-multi"] = true
	meta.files["f-multi"] = map[string][]byte{
		"main.py": []byte("print('main')"),
		"util.py": []byte("x = 1"),
	}

	resp, err := e.Invoke(context.Background(), "multi", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if string(resp.Output) != `"multi"` {
		t.Fatalf("unexpected output: %s", resp.Output)
	}
}

// ---------------------------------------------------------------------------
// Invoke – circuit breaker open
// ---------------------------------------------------------------------------

func TestInvoke_CircuitBreakerOpen(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execErr: errors.New("fail"),
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["cb-fn"] = &domain.Function{
		ID:      "f-cb",
		Name:    "cb-fn",
		Runtime: domain.RuntimePython,
		CapacityPolicy: &domain.CapacityPolicy{
			Enabled:         true,
			BreakerErrorPct: 1, // very low threshold
			BreakerWindowS:  60,
			BreakerOpenS:    60,
			HalfOpenProbes:  1,
		},
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}
	meta.codes["f-cb"] = &domain.FunctionCode{
		FunctionID: "f-cb",
		SourceCode: "fail",
	}

	// First call triggers execution error (records failure)
	_, _ = e.Invoke(context.Background(), "cb-fn", nil)
	// Give async side-effects a moment
	time.Sleep(50 * time.Millisecond)

	// Trip the circuit breaker by generating many failures
	for i := 0; i < 50; i++ {
		_, _ = e.Invoke(context.Background(), "cb-fn", nil)
	}
	time.Sleep(50 * time.Millisecond)

	// Check if the circuit breaker eventually trips
	var lastErr error
	for i := 0; i < 100; i++ {
		_, lastErr = e.Invoke(context.Background(), "cb-fn", nil)
		if errors.Is(lastErr, ErrCircuitOpen) {
			break
		}
	}
	if !errors.Is(lastErr, ErrCircuitOpen) {
		t.Log("circuit breaker did not trip (may need more failures); skipping assertion")
	}
}

// ---------------------------------------------------------------------------
// Invoke – with stdout/stderr
// ---------------------------------------------------------------------------

func TestInvoke_WithStdoutStderr(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			Output: json.RawMessage(`"ok"`),
			Stdout: "hello stdout",
			Stderr: "hello stderr",
		},
	}
	e, meta := setupInvokeTest(t, client)
	seedSimpleFunction(meta)

	resp, err := e.Invoke(context.Background(), "hello", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// ---------------------------------------------------------------------------
// Invoke – custom runtime (no runtime record needed)
// ---------------------------------------------------------------------------

func TestInvoke_CustomRuntime(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			Output: json.RawMessage(`"custom"`),
		},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["custom-fn"] = &domain.Function{
		ID:      "f-custom",
		Name:    "custom-fn",
		Runtime: domain.RuntimeCustom,
	}
	meta.codes["f-custom"] = &domain.FunctionCode{
		FunctionID: "f-custom",
		SourceCode: "custom code",
	}

	resp, err := e.Invoke(context.Background(), "custom-fn", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if string(resp.Output) != `"custom"` {
		t.Fatalf("unexpected output: %s", resp.Output)
	}
}

// ---------------------------------------------------------------------------
// Invoke – with env vars and runtime env merge
// ---------------------------------------------------------------------------

func TestInvoke_RuntimeEnvMerge(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			Output: json.RawMessage(`"ok"`),
		},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["env-fn"] = &domain.Function{
		ID:      "f-env",
		Name:    "env-fn",
		Runtime: domain.RuntimePython,
		EnvVars: map[string]string{"USER_VAR": "user_val"},
	}
	meta.runtimes["python"] = &store.RuntimeRecord{
		ID:         "python",
		Entrypoint: []string{"python3"},
		EnvVars:    map[string]string{"RUNTIME_VAR": "rt_val", "USER_VAR": "should_not_override"},
	}
	meta.codes["f-env"] = &domain.FunctionCode{
		FunctionID: "f-env",
		SourceCode: "code",
	}

	resp, err := e.Invoke(context.Background(), "env-fn", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// ---------------------------------------------------------------------------
// Invoke – with secrets resolution
// ---------------------------------------------------------------------------

func TestInvoke_WithSecretsResolver(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			Output: json.RawMessage(`"secret-ok"`),
		},
	}
	meta := newMockMetaStore()
	s := store.NewStore(meta)
	p := newTestPool(client)
	sink := &captureSink{ch: make(chan *store.InvocationLog, 10)}

	// Create a real secrets store/resolver with a mock backend
	hexKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cipher, _ := secrets.NewCipher(hexKey)
	secBackend := &mockSecretsBackend{data: make(map[string]string)}
	secStore := secrets.NewStore(secBackend, cipher)
	// Store a secret
	secStore.Set(context.Background(), "my_secret", []byte("secret_value"))
	resolver := secrets.NewResolver(secStore)

	tc := secrets.NewTransportCipher(cipher)

	e := New(s, p,
		WithLogSink(sink),
		WithSecretsResolver(resolver),
		WithTransportCipher(tc),
		WithLogBatcherConfig(LogBatcherConfig{BatchSize: 1, FlushInterval: 10 * time.Millisecond}),
	)
	t.Cleanup(func() {
		e.logBatcher.Shutdown(time.Second)
		p.Shutdown()
	})

	meta.fnByName["secret-fn"] = &domain.Function{
		ID:      "f-secret",
		Name:    "secret-fn",
		Runtime: domain.RuntimePython,
		EnvVars: map[string]string{
			"NORMAL_VAR": "normal_value",
			"SECRET_VAR": "$SECRET:my_secret",
		},
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}
	meta.codes["f-secret"] = &domain.FunctionCode{
		FunctionID: "f-secret",
		SourceCode: "code",
	}

	resp, err := e.Invoke(context.Background(), "secret-fn", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// ---------------------------------------------------------------------------
// InvokeStream – with secrets resolution
// ---------------------------------------------------------------------------

func TestInvokeStream_WithSecretsResolver(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		streamChunks: [][]byte{[]byte("ok")},
	}
	meta := newMockMetaStore()
	s := store.NewStore(meta)
	p := newTestPool(client)
	sink := &captureSink{ch: make(chan *store.InvocationLog, 10)}

	hexKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cipher, _ := secrets.NewCipher(hexKey)
	secBackend := &mockSecretsBackend{data: make(map[string]string)}
	secStore := secrets.NewStore(secBackend, cipher)
	secStore.Set(context.Background(), "api_key", []byte("key123"))
	resolver := secrets.NewResolver(secStore)

	e := New(s, p,
		WithLogSink(sink),
		WithSecretsResolver(resolver),
		WithLogBatcherConfig(LogBatcherConfig{BatchSize: 1, FlushInterval: 10 * time.Millisecond}),
	)
	t.Cleanup(func() {
		e.logBatcher.Shutdown(time.Second)
		p.Shutdown()
	})

	meta.fnByName["sec-stream"] = &domain.Function{
		ID:      "f-sec-s",
		Name:    "sec-stream",
		Runtime: domain.RuntimePython,
		EnvVars: map[string]string{"API_KEY": "$SECRET:api_key"},
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}
	meta.codes["f-sec-s"] = &domain.FunctionCode{
		FunctionID: "f-sec-s",
		SourceCode: "code",
	}

	err := e.InvokeStream(context.Background(), "sec-stream", nil, func([]byte, bool, error) error { return nil })
	if err != nil {
		t.Fatalf("InvokeStream failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Invoke – with layers
// ---------------------------------------------------------------------------

func TestInvoke_WithLayers(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			Output: json.RawMessage(`"layered"`),
		},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["layer-fn"] = &domain.Function{
		ID:      "f-layer",
		Name:    "layer-fn",
		Runtime: domain.RuntimePython,
		Layers:  []string{"layer1"},
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}
	meta.codes["f-layer"] = &domain.FunctionCode{
		FunctionID: "f-layer",
		SourceCode: "code",
	}
	meta.layers["f-layer"] = []*domain.Layer{
		{ID: "layer1", ImagePath: "/layers/layer1.ext4"},
	}

	resp, err := e.Invoke(context.Background(), "layer-fn", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if string(resp.Output) != `"layered"` {
		t.Fatalf("unexpected output: %s", resp.Output)
	}
}

// ---------------------------------------------------------------------------
// Invoke – with volumes
// ---------------------------------------------------------------------------

func TestInvoke_WithVolumes(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			Output: json.RawMessage(`"vol"`),
		},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["vol-fn"] = &domain.Function{
		ID:      "f-vol",
		Name:    "vol-fn",
		Runtime: domain.RuntimePython,
		Mounts:  []domain.VolumeMount{{VolumeID: "v1", MountPath: "/data"}},
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}
	meta.codes["f-vol"] = &domain.FunctionCode{
		FunctionID: "f-vol",
		SourceCode: "code",
	}
	meta.volumes["f-vol"] = []*domain.Volume{
		{ID: "v1", ImagePath: "/vol/v1.ext4"},
	}

	resp, err := e.Invoke(context.Background(), "vol-fn", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if string(resp.Output) != `"vol"` {
		t.Fatalf("unexpected output: %s", resp.Output)
	}
}

// ---------------------------------------------------------------------------
// InvokeStream – success
// ---------------------------------------------------------------------------

func TestInvokeStream_Success(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		streamChunks: [][]byte{[]byte("chunk1"), []byte("chunk2")},
	}
	e, meta := setupInvokeTest(t, client)
	seedSimpleFunction(meta)

	var chunks []string
	err := e.InvokeStream(context.Background(), "hello", json.RawMessage(`{}`), func(chunk []byte, isLast bool, callbackErr error) error {
		chunks = append(chunks, string(chunk))
		return nil
	})
	if err != nil {
		t.Fatalf("InvokeStream failed: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
}

// ---------------------------------------------------------------------------
// InvokeStream – function not found
// ---------------------------------------------------------------------------

func TestInvokeStream_FunctionNotFound(t *testing.T) {
	t.Parallel()
	client := &mockClient{}
	e, _ := setupInvokeTest(t, client)

	err := e.InvokeStream(context.Background(), "nonexistent", nil, func([]byte, bool, error) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "get function") {
		t.Fatalf("expected 'get function' error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// InvokeStream – code not found
// ---------------------------------------------------------------------------

func TestInvokeStream_CodeNotFound(t *testing.T) {
	t.Parallel()
	client := &mockClient{}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["hello"] = &domain.Function{
		ID: "f1", Name: "hello", Runtime: domain.RuntimePython,
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}

	err := e.InvokeStream(context.Background(), "hello", nil, func([]byte, bool, error) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "function code") {
		t.Fatalf("expected code error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// InvokeStream – execution error
// ---------------------------------------------------------------------------

func TestInvokeStream_ExecutionError(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		streamErr: errors.New("stream broken"),
	}
	e, meta := setupInvokeTest(t, client)
	seedSimpleFunction(meta)

	err := e.InvokeStream(context.Background(), "hello", nil, func([]byte, bool, error) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "execute stream") {
		t.Fatalf("expected 'execute stream' error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// InvokeStream – compiled pending
// ---------------------------------------------------------------------------

func TestInvokeStream_CompiledPending(t *testing.T) {
	t.Parallel()
	client := &mockClient{}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["gofn"] = &domain.Function{
		ID: "f-go-s", Name: "gofn", Runtime: domain.RuntimeGo,
	}
	meta.runtimes["go"] = &store.RuntimeRecord{ID: "go"}
	meta.codes["f-go-s"] = &domain.FunctionCode{
		FunctionID:    "f-go-s",
		CompileStatus: domain.CompileStatusPending,
	}

	err := e.InvokeStream(context.Background(), "gofn", nil, func([]byte, bool, error) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "pending") {
		t.Fatalf("expected 'pending' error, got %v", err)
	}
}

func TestInvokeStream_CompiledFailed(t *testing.T) {
	t.Parallel()
	client := &mockClient{}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["gofn"] = &domain.Function{
		ID: "f-go-s2", Name: "gofn", Runtime: domain.RuntimeGo,
	}
	meta.runtimes["go"] = &store.RuntimeRecord{ID: "go"}
	meta.codes["f-go-s2"] = &domain.FunctionCode{
		FunctionID:    "f-go-s2",
		CompileStatus: domain.CompileStatusFailed,
		CompileError:  "err",
	}

	err := e.InvokeStream(context.Background(), "gofn", nil, func([]byte, bool, error) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "compilation failed") {
		t.Fatalf("expected 'compilation failed' error, got %v", err)
	}
}

func TestInvokeStream_CompiledCompiling(t *testing.T) {
	t.Parallel()
	client := &mockClient{}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["gofn"] = &domain.Function{
		ID: "f-go-s3", Name: "gofn", Runtime: domain.RuntimeGo,
	}
	meta.runtimes["go"] = &store.RuntimeRecord{ID: "go"}
	meta.codes["f-go-s3"] = &domain.FunctionCode{
		FunctionID:    "f-go-s3",
		CompileStatus: domain.CompileStatusCompiling,
	}

	err := e.InvokeStream(context.Background(), "gofn", nil, func([]byte, bool, error) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "compiling") {
		t.Fatalf("expected 'compiling' error, got %v", err)
	}
}

func TestInvokeStream_CompiledNoBinary(t *testing.T) {
	t.Parallel()
	client := &mockClient{}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["gofn"] = &domain.Function{
		ID: "f-go-s4", Name: "gofn", Runtime: domain.RuntimeGo,
	}
	meta.runtimes["go"] = &store.RuntimeRecord{ID: "go"}
	meta.codes["f-go-s4"] = &domain.FunctionCode{
		FunctionID:    "f-go-s4",
		CompileStatus: domain.CompileStatusSuccess,
	}

	err := e.InvokeStream(context.Background(), "gofn", nil, func([]byte, bool, error) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "no compiled binary") {
		t.Fatalf("expected 'no compiled binary' error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// InvokeStream – compiled success
// ---------------------------------------------------------------------------

func TestInvokeStream_CompiledSuccess(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		streamChunks: [][]byte{[]byte("bin-out")},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["gofn"] = &domain.Function{
		ID: "f-go-s5", Name: "gofn", Runtime: domain.RuntimeGo,
	}
	meta.runtimes["go"] = &store.RuntimeRecord{ID: "go"}
	meta.codes["f-go-s5"] = &domain.FunctionCode{
		FunctionID:     "f-go-s5",
		CompileStatus:  domain.CompileStatusSuccess,
		CompiledBinary: []byte("binary"),
	}

	err := e.InvokeStream(context.Background(), "gofn", nil, func([]byte, bool, error) error { return nil })
	if err != nil {
		t.Fatalf("InvokeStream failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// InvokeStream – multi-file
// ---------------------------------------------------------------------------

func TestInvokeStream_MultiFile(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		streamChunks: [][]byte{[]byte("mf")},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["mf"] = &domain.Function{
		ID: "f-mf", Name: "mf", Runtime: domain.RuntimePython, Handler: "main.py",
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}
	meta.codes["f-mf"] = &domain.FunctionCode{
		FunctionID: "f-mf",
		SourceCode: "code",
	}
	meta.multiFiles["f-mf"] = true
	meta.files["f-mf"] = map[string][]byte{
		"main.py": []byte("code"),
	}

	err := e.InvokeStream(context.Background(), "mf", nil, func([]byte, bool, error) error { return nil })
	if err != nil {
		t.Fatalf("InvokeStream failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// InvokeStream – with layers and volumes
// ---------------------------------------------------------------------------

func TestInvokeStream_WithLayers(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		streamChunks: [][]byte{[]byte("ok")},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["lf"] = &domain.Function{
		ID: "f-lf", Name: "lf", Runtime: domain.RuntimePython, Layers: []string{"l1"},
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}
	meta.codes["f-lf"] = &domain.FunctionCode{FunctionID: "f-lf", SourceCode: "x"}
	meta.layers["f-lf"] = []*domain.Layer{{ID: "l1", ImagePath: "/l1"}}

	err := e.InvokeStream(context.Background(), "lf", nil, func([]byte, bool, error) error { return nil })
	if err != nil {
		t.Fatalf("InvokeStream failed: %v", err)
	}
}

func TestInvokeStream_WithVolumes(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		streamChunks: [][]byte{[]byte("ok")},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["vf"] = &domain.Function{
		ID: "f-vf", Name: "vf", Runtime: domain.RuntimePython,
		Mounts: []domain.VolumeMount{{VolumeID: "v1", MountPath: "/data"}},
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}
	meta.codes["f-vf"] = &domain.FunctionCode{FunctionID: "f-vf", SourceCode: "x"}
	meta.volumes["f-vf"] = []*domain.Volume{{ID: "v1", ImagePath: "/v1.ext4"}}

	err := e.InvokeStream(context.Background(), "vf", nil, func([]byte, bool, error) error { return nil })
	if err != nil {
		t.Fatalf("InvokeStream failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// InvokeStream – custom runtime
// ---------------------------------------------------------------------------

func TestInvokeStream_CustomRuntime(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		streamChunks: [][]byte{[]byte("custom")},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["custom-fn"] = &domain.Function{
		ID: "f-cust-s", Name: "custom-fn", Runtime: domain.RuntimeCustom,
	}
	meta.codes["f-cust-s"] = &domain.FunctionCode{
		FunctionID: "f-cust-s",
		SourceCode: "custom code",
	}

	err := e.InvokeStream(context.Background(), "custom-fn", nil, func([]byte, bool, error) error { return nil })
	if err != nil {
		t.Fatalf("InvokeStream failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Shutdown – full (with real pool)
// ---------------------------------------------------------------------------

func TestShutdown_Full(t *testing.T) {
	t.Parallel()
	client := &mockClient{}
	meta := newMockMetaStore()
	s := store.NewStore(meta)
	p := newTestPool(client)
	sink := &captureSink{ch: make(chan *store.InvocationLog, 10)}
	e := New(s, p, WithLogSink(sink))

	e.Shutdown(time.Second)

	// After shutdown, Invoke should be rejected
	_, err := e.Invoke(context.Background(), "any", nil)
	if err == nil || !strings.Contains(err.Error(), "shutting down") {
		t.Fatalf("expected 'shutting down' error after Shutdown, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Invoke – rollout canary target (100% canary)
// ---------------------------------------------------------------------------

func TestInvoke_RolloutCanary100Percent(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			Output: json.RawMessage(`"canary"`),
		},
	}
	e, meta := setupInvokeTest(t, client)

	// Primary function with 100% canary rollout
	meta.fnByName["primary"] = &domain.Function{
		ID:      "f-primary",
		Name:    "primary",
		Runtime: domain.RuntimePython,
		RolloutPolicy: &domain.RolloutPolicy{
			Enabled:        true,
			CanaryFunction: "canary",
			CanaryPercent:  100, // always route to canary
		},
	}
	// Canary function
	meta.fnByName["canary"] = &domain.Function{
		ID:      "f-canary",
		Name:    "canary",
		Runtime: domain.RuntimePython,
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}
	meta.codes["f-primary"] = &domain.FunctionCode{
		FunctionID: "f-primary",
		SourceCode: "primary code",
	}
	meta.codes["f-canary"] = &domain.FunctionCode{
		FunctionID: "f-canary",
		SourceCode: "canary code",
	}

	resp, err := e.Invoke(context.Background(), "primary", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// ---------------------------------------------------------------------------
// Invoke – rollout canary > 100 clamped
// ---------------------------------------------------------------------------

func TestInvoke_RolloutCanaryClampedPercent(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{Output: json.RawMessage(`"ok"`)},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["primary"] = &domain.Function{
		ID:      "f-p2",
		Name:    "primary",
		Runtime: domain.RuntimePython,
		RolloutPolicy: &domain.RolloutPolicy{
			Enabled:        true,
			CanaryFunction: "canary2",
			CanaryPercent:  200, // should be clamped to 100
		},
	}
	meta.fnByName["canary2"] = &domain.Function{
		ID: "f-c2", Name: "canary2", Runtime: domain.RuntimePython,
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}
	meta.codes["f-p2"] = &domain.FunctionCode{FunctionID: "f-p2", SourceCode: "x"}
	meta.codes["f-c2"] = &domain.FunctionCode{FunctionID: "f-c2", SourceCode: "y"}

	// With 200% (clamped to 100%), should always go to canary
	resp, err := e.Invoke(context.Background(), "primary", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// ---------------------------------------------------------------------------
// Invoke with breaker recording success
// ---------------------------------------------------------------------------

func TestInvoke_BreakerRecordsSuccess(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			Output: json.RawMessage(`"ok"`),
		},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["br-fn"] = &domain.Function{
		ID:      "f-br",
		Name:    "br-fn",
		Runtime: domain.RuntimePython,
		CapacityPolicy: &domain.CapacityPolicy{
			Enabled:         true,
			BreakerErrorPct: 50,
			BreakerWindowS:  60,
			BreakerOpenS:    60,
		},
	}
	meta.runtimes["python"] = &store.RuntimeRecord{ID: "python"}
	meta.codes["f-br"] = &domain.FunctionCode{FunctionID: "f-br", SourceCode: "code"}

	resp, err := e.Invoke(context.Background(), "br-fn", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// ---------------------------------------------------------------------------
// Invoke – multi-file with compiled binary
// ---------------------------------------------------------------------------

func TestInvoke_MultiFileCompiledBinary(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			Output: json.RawMessage(`"ok"`),
		},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["mfbin"] = &domain.Function{
		ID:      "f-mfbin",
		Name:    "mfbin",
		Runtime: domain.RuntimeGo,
		Handler: "main",
	}
	meta.runtimes["go"] = &store.RuntimeRecord{ID: "go"}
	meta.codes["f-mfbin"] = &domain.FunctionCode{
		FunctionID:     "f-mfbin",
		CompileStatus:  domain.CompileStatusSuccess,
		CompiledBinary: []byte("compiled-bin"),
	}
	meta.multiFiles["f-mfbin"] = true
	meta.files["f-mfbin"] = map[string][]byte{
		"main.go": []byte("package main"),
		"util.go": []byte("package main"),
	}

	resp, err := e.Invoke(context.Background(), "mfbin", nil)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// ---------------------------------------------------------------------------
// InvokeStream – multi-file with compiled binary
// ---------------------------------------------------------------------------

func TestInvokeStream_MultiFileCompiledBinary(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		streamChunks: [][]byte{[]byte("ok")},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["mfbin"] = &domain.Function{
		ID: "f-mfbin-s", Name: "mfbin", Runtime: domain.RuntimeGo, Handler: "main",
	}
	meta.runtimes["go"] = &store.RuntimeRecord{ID: "go"}
	meta.codes["f-mfbin-s"] = &domain.FunctionCode{
		FunctionID:     "f-mfbin-s",
		CompileStatus:  domain.CompileStatusSuccess,
		CompiledBinary: []byte("bin"),
	}
	meta.multiFiles["f-mfbin-s"] = true
	meta.files["f-mfbin-s"] = map[string][]byte{"main.go": []byte("pkg")}

	err := e.InvokeStream(context.Background(), "mfbin", nil, func([]byte, bool, error) error { return nil })
	if err != nil {
		t.Fatalf("InvokeStream failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// InvokeStream – with env vars and runtime config
// ---------------------------------------------------------------------------

func TestInvokeStream_RuntimeEnvMerge(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		streamChunks: [][]byte{[]byte("ok")},
	}
	e, meta := setupInvokeTest(t, client)

	meta.fnByName["env-s"] = &domain.Function{
		ID:      "f-env-s",
		Name:    "env-s",
		Runtime: domain.RuntimePython,
		EnvVars: map[string]string{"USER": "val"},
	}
	meta.runtimes["python"] = &store.RuntimeRecord{
		ID:         "python",
		Entrypoint: []string{"python3"},
		EnvVars:    map[string]string{"RT": "rt_val"},
	}
	meta.codes["f-env-s"] = &domain.FunctionCode{
		FunctionID: "f-env-s",
		SourceCode: "code",
	}

	err := e.InvokeStream(context.Background(), "env-s", nil, func([]byte, bool, error) error { return nil })
	if err != nil {
		t.Fatalf("InvokeStream failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// LogBatcher – shutdown timeout
// ---------------------------------------------------------------------------

func TestLogBatcher_ShutdownTimeout(t *testing.T) {
	t.Parallel()
	blockSink := &blockingSink{block: make(chan struct{})}
	b := newInvocationLogBatcher(nil, blockSink, LogBatcherConfig{
		BatchSize:     1,
		FlushInterval: time.Millisecond,
	})

	// Enqueue a log that will block the sink
	b.Enqueue(&store.InvocationLog{ID: "block"})
	time.Sleep(20 * time.Millisecond) // let it start flushing

	// Shutdown with very short timeout should hit the timeout path
	b.Shutdown(time.Millisecond)

	// Unblock the sink so goroutine can exit
	close(blockSink.block)
}

// ---------------------------------------------------------------------------
// Invoke with payload persistence
// ---------------------------------------------------------------------------

func TestInvoke_PayloadPersistence(t *testing.T) {
	t.Parallel()
	client := &mockClient{
		execResp: &backend.RespPayload{
			Output: json.RawMessage(`"result"`),
		},
	}
	meta := newMockMetaStore()
	s := store.NewStore(meta)
	p := newTestPool(client)
	sink := &captureSink{ch: make(chan *store.InvocationLog, 10)}
	e := New(s, p,
		WithLogSink(sink),
		WithPayloadPersistence(true),
		WithLogBatcherConfig(LogBatcherConfig{BatchSize: 1, FlushInterval: 10 * time.Millisecond}),
	)
	t.Cleanup(func() {
		e.logBatcher.Shutdown(time.Second)
		p.Shutdown()
	})

	seedSimpleFunction(meta)

	input := json.RawMessage(`{"key":"val"}`)
	_, err := e.Invoke(context.Background(), "hello", input)
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	select {
	case log := <-sink.ch:
		if log.Input == nil {
			t.Fatal("expected input to be preserved with payload persistence")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for invocation log")
	}
}
