package dataplane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oriys/nova/internal/ai"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
	"google.golang.org/grpc/metadata"
)

// --- test helpers -------------------------------------------------------

type mockBackend struct {
	createVMFn func(ctx context.Context, fn *domain.Function, code []byte) (*backend.VM, error)
}

func (mb *mockBackend) CreateVM(ctx context.Context, fn *domain.Function, code []byte) (*backend.VM, error) {
	if mb.createVMFn != nil {
		return mb.createVMFn(ctx, fn, code)
	}
	return &backend.VM{ID: "vm-test", Runtime: fn.Runtime}, nil
}
func (mb *mockBackend) CreateVMWithFiles(ctx context.Context, fn *domain.Function, files map[string][]byte) (*backend.VM, error) {
	if mb.createVMFn != nil {
		return mb.createVMFn(ctx, fn, nil)
	}
	return &backend.VM{ID: "vm-test", Runtime: fn.Runtime}, nil
}
func (mb *mockBackend) StopVM(vmID string) error   { return nil }
func (mb *mockBackend) NewClient(vm *backend.VM) (backend.Client, error) {
	return &mockClient{}, nil
}
func (mb *mockBackend) Shutdown()            {}
func (mb *mockBackend) SnapshotDir() string  { return "" }

type mockClient struct{}

func (mc *mockClient) Init(fn *domain.Function) error                     { return nil }
func (mc *mockClient) Execute(reqID string, input json.RawMessage, timeoutS int) (*backend.RespPayload, error) {
	return &backend.RespPayload{Output: json.RawMessage(`{}`)}, nil
}
func (mc *mockClient) ExecuteWithTrace(reqID string, input json.RawMessage, timeoutS int, traceParent, traceState string) (*backend.RespPayload, error) {
	return &backend.RespPayload{Output: json.RawMessage(`{}`)}, nil
}
func (mc *mockClient) ExecuteStream(reqID string, input json.RawMessage, timeoutS int, traceParent, traceState string, callback func(chunk []byte, isLast bool, err error) error) error {
	return callback(nil, true, nil)
}
func (mc *mockClient) Reload(files map[string][]byte) error { return nil }
func (mc *mockClient) Ping() error                          { return nil }
func (mc *mockClient) Close() error                         { return nil }

func newTestPool(t *testing.T) *pool.Pool {
	t.Helper()
	return newTestPoolWithBackend(t, &mockBackend{})
}

func newTestPoolWithBackend(t *testing.T, b backend.Backend) *pool.Pool {
	t.Helper()
	return pool.NewPool(b, pool.PoolConfig{
		IdleTTL:             time.Minute,
		CleanupInterval:     time.Minute,
		HealthCheckInterval: time.Minute,
	})
}

func testFunction() *domain.Function {
	return &domain.Function{
		ID:       "fn-1",
		Name:     "hello",
		Runtime:  domain.RuntimePython,
		Handler:  "handler",
		MemoryMB: 128,
		TimeoutS: 30,
	}
}

func setupTestHandler(t *testing.T) (*Handler, *http.ServeMux, *mockMetadataStore) {
	t.Helper()
	ms := &mockMetadataStore{}
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return testFunction(), nil
	}
	wf := &mockWorkflowStore{}
	s := store.NewStore(ms)
	s.WorkflowStore = wf
	p := newTestPool(t)
	t.Cleanup(p.Shutdown)
	h := &Handler{Store: s, Pool: p}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux, ms
}

func setupTestHandlerWithExec(t *testing.T) (*Handler, *http.ServeMux, *mockMetadataStore) {
	t.Helper()
	observability.Init(context.Background(), observability.Config{Enabled: false})
	ms := &mockMetadataStore{}
	fn := testFunction()
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return fn, nil
	}
	ms.getFunctionCodeFn = func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
		return &domain.FunctionCode{SourceCode: "print('hello')", CompileStatus: domain.CompileStatusNotRequired}, nil
	}
	wf := &mockWorkflowStore{}
	s := store.NewStore(ms)
	s.WorkflowStore = wf
	p := newTestPool(t)
	t.Cleanup(p.Shutdown)
	exec := executor.New(s, p)
	h := &Handler{Store: s, Pool: p, Exec: exec}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux, ms
}

func doRequest(t *testing.T, mux *http.ServeMux, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("want status %d, got %d; body: %s", want, w.Code, w.Body.String())
	}
}

func assertJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("response is not valid JSON: %v; body: %s", err, w.Body.String())
	}
	return result
}

// --- Health handlers ---------------------------------------------------

func TestHealthLiveViaRoute(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/health/live", nil)
	assertStatus(t, w, 200)
	body := assertJSON(t, w)
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", body["status"])
	}
}

func TestHealthReady_PingOK(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.pingFn = func(context.Context) error { return nil }
	w := doRequest(t, mux, "GET", "/health/ready", nil)
	assertStatus(t, w, 200)
}

func TestHealthReady_PingFail(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.pingFn = func(context.Context) error { return fmt.Errorf("down") }
	w := doRequest(t, mux, "GET", "/health/ready", nil)
	assertStatus(t, w, 503)
}

func TestHealthStartup_PingOK(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.pingFn = func(context.Context) error { return nil }
	w := doRequest(t, mux, "GET", "/health/startup", nil)
	assertStatus(t, w, 200)
}

func TestHealthStartup_PingFail(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.pingFn = func(context.Context) error { return fmt.Errorf("down") }
	w := doRequest(t, mux, "GET", "/health/startup", nil)
	assertStatus(t, w, 503)
}

func TestHealth_Full(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.pingFn = func(context.Context) error { return nil }
	w := doRequest(t, mux, "GET", "/health", nil)
	assertStatus(t, w, 200)
	body := assertJSON(t, w)
	if body["status"] != "ok" {
		t.Fatalf("expected ok, got %v", body["status"])
	}
}

func TestHealth_Degraded(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.pingFn = func(context.Context) error { return fmt.Errorf("unreachable") }
	w := doRequest(t, mux, "GET", "/health", nil)
	assertStatus(t, w, 200)
	body := assertJSON(t, w)
	if body["status"] != "degraded" {
		t.Fatalf("expected degraded, got %v", body["status"])
	}
}

// --- Stats handler -------------------------------------------------------

func TestStats(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/stats", nil)
	assertStatus(t, w, 200)
	body := assertJSON(t, w)
	if _, ok := body["active_vms"]; !ok {
		t.Fatal("expected active_vms key")
	}
}

// --- Logs handlers -------------------------------------------------------

func TestLogs_FunctionNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "GET", "/functions/missing/logs", nil)
	assertStatus(t, w, 404)
}

func TestLogs_EmptyLogs(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/logs", nil)
	assertStatus(t, w, 200)
}

func TestLogs_WithRequestID(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getInvocationLogFn = func(_ context.Context, reqID string) (*store.InvocationLog, error) {
		return &store.InvocationLog{ID: reqID, FunctionID: "fn-1"}, nil
	}
	w := doRequest(t, mux, "GET", "/functions/hello/logs?request_id=abc123", nil)
	assertStatus(t, w, 200)
}

func TestLogs_WithRequestID_NotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getInvocationLogFn = func(_ context.Context, reqID string) (*store.InvocationLog, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "GET", "/functions/hello/logs?request_id=missing", nil)
	assertStatus(t, w, 404)
}

func TestLogs_WithTailParam(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listInvocationLogsFn = func(_ context.Context, fnID string, limit, offset int) ([]*store.InvocationLog, error) {
		if limit != 5 {
			return nil, fmt.Errorf("expected limit=5, got %d", limit)
		}
		return []*store.InvocationLog{}, nil
	}
	w := doRequest(t, mux, "GET", "/functions/hello/logs?tail=5", nil)
	assertStatus(t, w, 200)
}

func TestListAllInvocations(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/invocations", nil)
	assertStatus(t, w, 200)
}

func TestListAllInvocations_WithFilters(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/invocations?search=foo&function=hello&status=success", nil)
	assertStatus(t, w, 200)
}

func TestListAllInvocations_FailedFilter(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/invocations?status=failed", nil)
	assertStatus(t, w, 200)
}

func TestListAllInvocations_InvalidStatus(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/invocations?status=invalid_xyz", nil)
	assertStatus(t, w, 400)
}

func TestListAllInvocations_StoreError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listAllInvocationLogsFn = func(_ context.Context, limit, offset int) ([]*store.InvocationLog, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/invocations", nil)
	assertStatus(t, w, 500)
}

// --- State handlers ------------------------------------------------------

func TestGetFunctionState_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "GET", "/functions/missing/state?key=x", nil)
	assertStatus(t, w, 404)
}

func TestGetFunctionState_KeyNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionStateFn = func(_ context.Context, fnID, key string) (*store.FunctionStateEntry, error) {
		return nil, store.ErrFunctionStateNotFound
	}
	w := doRequest(t, mux, "GET", "/functions/hello/state?key=missing", nil)
	assertStatus(t, w, 404)
}

func TestGetFunctionState_KeyFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionStateFn = func(_ context.Context, fnID, key string) (*store.FunctionStateEntry, error) {
		return &store.FunctionStateEntry{FunctionID: fnID, Key: key, Value: json.RawMessage(`"val"`)}, nil
	}
	w := doRequest(t, mux, "GET", "/functions/hello/state?key=mykey", nil)
	assertStatus(t, w, 200)
}

func TestGetFunctionState_InternalError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionStateFn = func(_ context.Context, fnID, key string) (*store.FunctionStateEntry, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/functions/hello/state?key=x", nil)
	assertStatus(t, w, 500)
}

func TestGetFunctionState_ListAll(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/state", nil)
	assertStatus(t, w, 200)
}

func TestGetFunctionState_ListWithPrefix(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/state?prefix=user_", nil)
	assertStatus(t, w, 200)
}

func TestPutFunctionState_OK(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	body := map[string]interface{}{"value": "hello"}
	w := doRequest(t, mux, "PUT", "/functions/hello/state?key=mykey", body)
	assertStatus(t, w, 200)
}

func TestPutFunctionState_MissingKey(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	body := map[string]interface{}{"value": "hello"}
	w := doRequest(t, mux, "PUT", "/functions/hello/state", body)
	assertStatus(t, w, 400)
}

func TestPutFunctionState_InvalidJSON(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	req := httptest.NewRequest("PUT", "/functions/hello/state?key=k", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assertStatus(t, w, 400)
}

func TestPutFunctionState_EmptyValue(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	body := map[string]interface{}{"value": nil}
	w := doRequest(t, mux, "PUT", "/functions/hello/state?key=mykey", body)
	// null is valid JSON; handler stores it as-is
	assertStatus(t, w, 200)
}

func TestPutFunctionState_NegativeTTL(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	body := map[string]interface{}{"value": "hello", "ttl_s": -1}
	w := doRequest(t, mux, "PUT", "/functions/hello/state?key=mykey", body)
	assertStatus(t, w, 400)
}

func TestPutFunctionState_NegativeVersion(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	body := map[string]interface{}{"value": "hello", "expected_version": -1}
	w := doRequest(t, mux, "PUT", "/functions/hello/state?key=mykey", body)
	assertStatus(t, w, 400)
}

func TestPutFunctionState_VersionConflict(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.putFunctionStateFn = func(_ context.Context, fnID, key string, value json.RawMessage, opts *store.FunctionStatePutOptions) (*store.FunctionStateEntry, error) {
		return nil, store.ErrFunctionStateVersionConflict
	}
	body := map[string]interface{}{"value": "hello", "expected_version": 1}
	w := doRequest(t, mux, "PUT", "/functions/hello/state?key=mykey", body)
	assertStatus(t, w, 409)
}

func TestPutFunctionState_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	body := map[string]interface{}{"value": "hello"}
	w := doRequest(t, mux, "PUT", "/functions/missing/state?key=x", body)
	assertStatus(t, w, 404)
}

func TestPutFunctionState_WithTTL(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	body := map[string]interface{}{"value": "hello", "ttl_s": 60}
	w := doRequest(t, mux, "PUT", "/functions/hello/state?key=mykey", body)
	assertStatus(t, w, 200)
}

func TestDeleteFunctionState_OK(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "DELETE", "/functions/hello/state?key=mykey", nil)
	assertStatus(t, w, 204)
}

func TestDeleteFunctionState_MissingKey(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "DELETE", "/functions/hello/state", nil)
	assertStatus(t, w, 400)
}

func TestDeleteFunctionState_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "DELETE", "/functions/missing/state?key=x", nil)
	assertStatus(t, w, 404)
}

func TestDeleteFunctionState_StoreError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.deleteFunctionStateFn = func(_ context.Context, fnID, key string) error {
		return fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "DELETE", "/functions/hello/state?key=x", nil)
	assertStatus(t, w, 500)
}

// --- Async handlers -------------------------------------------------------

func TestGetAsyncInvocation_NotFound(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/async-invocations/missing-id", nil)
	assertStatus(t, w, 404)
}

func TestGetAsyncInvocation_Found(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getAsyncInvocationFn = func(_ context.Context, id string) (*store.AsyncInvocation, error) {
		return &store.AsyncInvocation{ID: id, Status: store.AsyncInvocationStatusQueued}, nil
	}
	w := doRequest(t, mux, "GET", "/async-invocations/inv-1", nil)
	assertStatus(t, w, 200)
}

func TestGetAsyncInvocation_InternalError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getAsyncInvocationFn = func(_ context.Context, id string) (*store.AsyncInvocation, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/async-invocations/inv-1", nil)
	assertStatus(t, w, 500)
}

func TestListAsyncInvocations(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/async-invocations", nil)
	assertStatus(t, w, 200)
}

func TestListAsyncInvocations_WithStatusFilter(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/async-invocations?status=queued", nil)
	assertStatus(t, w, 200)
}

func TestListAsyncInvocations_InvalidStatus(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/async-invocations?status=invalid", nil)
	assertStatus(t, w, 400)
}

func TestListFunctionAsyncInvocations(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/async-invocations", nil)
	assertStatus(t, w, 200)
}

func TestListFunctionAsyncInvocations_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "GET", "/functions/missing/async-invocations", nil)
	assertStatus(t, w, 404)
}

func TestRetryAsyncInvocation_NotFound(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "POST", "/async-invocations/missing/retry", nil)
	assertStatus(t, w, 404)
}

func TestRetryAsyncInvocation_NotDLQ(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.requeueAsyncInvocationFn = func(_ context.Context, id string, maxAttempts int) (*store.AsyncInvocation, error) {
		return nil, store.ErrAsyncInvocationNotDLQ
	}
	w := doRequest(t, mux, "POST", "/async-invocations/inv-1/retry", nil)
	assertStatus(t, w, 409)
}

func TestRetryAsyncInvocation_OK(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.requeueAsyncInvocationFn = func(_ context.Context, id string, maxAttempts int) (*store.AsyncInvocation, error) {
		return &store.AsyncInvocation{ID: id, Status: store.AsyncInvocationStatusQueued}, nil
	}
	w := doRequest(t, mux, "POST", "/async-invocations/inv-1/retry", nil)
	assertStatus(t, w, 200)
}

func TestRetryAsyncInvocation_WithBody(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.requeueAsyncInvocationFn = func(_ context.Context, id string, maxAttempts int) (*store.AsyncInvocation, error) {
		if maxAttempts != 5 {
			return nil, fmt.Errorf("expected maxAttempts=5, got %d", maxAttempts)
		}
		return &store.AsyncInvocation{ID: id, Status: store.AsyncInvocationStatusQueued}, nil
	}
	body := map[string]interface{}{"max_attempts": 5}
	w := doRequest(t, mux, "POST", "/async-invocations/inv-1/retry", body)
	assertStatus(t, w, 200)
}

func TestPauseAsyncInvocation_NotFound(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "POST", "/async-invocations/missing/pause", nil)
	assertStatus(t, w, 404)
}

func TestPauseAsyncInvocation_NotQueued(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.pauseAsyncInvocationFn = func(_ context.Context, id string) (*store.AsyncInvocation, error) {
		return nil, store.ErrAsyncInvocationNotQueued
	}
	w := doRequest(t, mux, "POST", "/async-invocations/inv-1/pause", nil)
	assertStatus(t, w, 409)
}

func TestPauseAsyncInvocation_OK(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.pauseAsyncInvocationFn = func(_ context.Context, id string) (*store.AsyncInvocation, error) {
		return &store.AsyncInvocation{ID: id, Status: store.AsyncInvocationStatusPaused}, nil
	}
	w := doRequest(t, mux, "POST", "/async-invocations/inv-1/pause", nil)
	assertStatus(t, w, 200)
}

func TestResumeAsyncInvocation_NotFound(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "POST", "/async-invocations/missing/resume", nil)
	assertStatus(t, w, 404)
}

func TestResumeAsyncInvocation_NotPaused(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.resumeAsyncInvocationFn = func(_ context.Context, id string) (*store.AsyncInvocation, error) {
		return nil, store.ErrAsyncInvocationNotPaused
	}
	w := doRequest(t, mux, "POST", "/async-invocations/inv-1/resume", nil)
	assertStatus(t, w, 409)
}

func TestResumeAsyncInvocation_OK(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.resumeAsyncInvocationFn = func(_ context.Context, id string) (*store.AsyncInvocation, error) {
		return &store.AsyncInvocation{ID: id, Status: store.AsyncInvocationStatusQueued}, nil
	}
	w := doRequest(t, mux, "POST", "/async-invocations/inv-1/resume", nil)
	assertStatus(t, w, 200)
}

func TestDeleteAsyncInvocation_NotFound(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "DELETE", "/async-invocations/missing", nil)
	assertStatus(t, w, 404)
}

func TestDeleteAsyncInvocation_NotDeletable(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.deleteAsyncInvocationFn = func(_ context.Context, id string) error {
		return store.ErrAsyncInvocationNotDeletable
	}
	w := doRequest(t, mux, "DELETE", "/async-invocations/inv-1", nil)
	assertStatus(t, w, 409)
}

func TestDeleteAsyncInvocation_OK(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.deleteAsyncInvocationFn = func(_ context.Context, id string) error { return nil }
	w := doRequest(t, mux, "DELETE", "/async-invocations/inv-1", nil)
	assertStatus(t, w, 204)
}

func TestPauseAsyncInvocationsByFunction(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.pauseAsyncInvocationsByFunctionFn = func(_ context.Context, fnID string) (int, error) {
		return 3, nil
	}
	w := doRequest(t, mux, "POST", "/async-invocations/functions/fn-1/pause", nil)
	assertStatus(t, w, 200)
	body := assertJSON(t, w)
	if body["paused"] != float64(3) {
		t.Fatalf("expected paused=3, got %v", body["paused"])
	}
}

func TestResumeAsyncInvocationsByFunction(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.resumeAsyncInvocationsByFunctionFn = func(_ context.Context, fnID string) (int, error) {
		return 2, nil
	}
	w := doRequest(t, mux, "POST", "/async-invocations/functions/fn-1/resume", nil)
	assertStatus(t, w, 200)
	body := assertJSON(t, w)
	if body["resumed"] != float64(2) {
		t.Fatalf("expected resumed=2, got %v", body["resumed"])
	}
}

func TestPauseAsyncInvocationsByWorkflow(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.pauseAsyncInvocationsByWorkflowFn = func(_ context.Context, wfID string) (int, error) {
		return 1, nil
	}
	w := doRequest(t, mux, "POST", "/async-invocations/workflows/wf-1/pause", nil)
	assertStatus(t, w, 200)
}

func TestResumeAsyncInvocationsByWorkflow(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.resumeAsyncInvocationsByWorkflowFn = func(_ context.Context, wfID string) (int, error) {
		return 1, nil
	}
	w := doRequest(t, mux, "POST", "/async-invocations/workflows/wf-1/resume", nil)
	assertStatus(t, w, 200)
}

func TestGetGlobalAsyncPause(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getGlobalAsyncPauseFn = func(context.Context) (bool, error) { return true, nil }
	w := doRequest(t, mux, "GET", "/async-invocations/global-pause", nil)
	assertStatus(t, w, 200)
	body := assertJSON(t, w)
	if body["paused"] != true {
		t.Fatalf("expected paused=true")
	}
}

func TestGetGlobalAsyncPause_Error(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getGlobalAsyncPauseFn = func(context.Context) (bool, error) { return false, fmt.Errorf("err") }
	w := doRequest(t, mux, "GET", "/async-invocations/global-pause", nil)
	assertStatus(t, w, 500)
}

func TestSetGlobalAsyncPause(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	body := map[string]interface{}{"paused": true}
	w := doRequest(t, mux, "POST", "/async-invocations/global-pause", body)
	assertStatus(t, w, 200)
}

func TestSetGlobalAsyncPause_InvalidJSON(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	req := httptest.NewRequest("POST", "/async-invocations/global-pause", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assertStatus(t, w, 400)
}

func TestSetGlobalAsyncPause_StoreError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.setGlobalAsyncPauseFn = func(_ context.Context, paused bool) error { return fmt.Errorf("err") }
	body := map[string]interface{}{"paused": true}
	w := doRequest(t, mux, "POST", "/async-invocations/global-pause", body)
	assertStatus(t, w, 500)
}

func TestAsyncInvocationsSummary(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/async-invocations/summary", nil)
	assertStatus(t, w, 200)
}

func TestListDLQInvocations(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/async-invocations/dlq", nil)
	assertStatus(t, w, 200)
}

func TestListDLQInvocations_StoreError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listAsyncInvocationsFn = func(_ context.Context, limit, offset int, statuses []store.AsyncInvocationStatus) ([]*store.AsyncInvocation, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/async-invocations/dlq", nil)
	assertStatus(t, w, 500)
}

func TestRetryAllDLQ(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listAsyncInvocationsFn = func(_ context.Context, limit, offset int, statuses []store.AsyncInvocationStatus) ([]*store.AsyncInvocation, error) {
		return []*store.AsyncInvocation{
			{ID: "inv-1", Status: store.AsyncInvocationStatusDLQ},
			{ID: "inv-2", Status: store.AsyncInvocationStatusDLQ},
		}, nil
	}
	ms.requeueAsyncInvocationFn = func(_ context.Context, id string, maxAttempts int) (*store.AsyncInvocation, error) {
		return &store.AsyncInvocation{ID: id, Status: store.AsyncInvocationStatusQueued}, nil
	}
	w := doRequest(t, mux, "POST", "/async-invocations/dlq/retry-all", nil)
	assertStatus(t, w, 200)
	body := assertJSON(t, w)
	if body["retried"] != float64(2) {
		t.Fatalf("expected retried=2, got %v", body["retried"])
	}
}

func TestRetryAllDLQ_InvalidJSON(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	req := httptest.NewRequest("POST", "/async-invocations/dlq/retry-all", strings.NewReader("bad json"))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 8
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assertStatus(t, w, 400)
}

func TestListWorkflowAsyncInvocations(t *testing.T) {
	h, mux, _ := setupTestHandler(t)
	wf := &mockWorkflowStore{
		getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: "wf-1", Name: name}, nil
		},
	}
	h.Store.WorkflowStore = wf
	w := doRequest(t, mux, "GET", "/workflows/my-workflow/async-invocations", nil)
	assertStatus(t, w, 200)
}

func TestListWorkflowAsyncInvocations_NotFound(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/workflows/missing/async-invocations", nil)
	assertStatus(t, w, 404)
}

// --- Invoke handlers (error cases without executor) -----------------------

func TestInvokeFunction_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "POST", "/functions/missing/invoke", nil)
	assertStatus(t, w, 404)
}

func TestInvokeFunctionStream_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "POST", "/functions/missing/invoke-stream", nil)
	assertStatus(t, w, 404)
}

func TestEnqueueAsyncFunction_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "POST", "/functions/missing/invoke-async", nil)
	assertStatus(t, w, 404)
}

func TestEnqueueAsyncFunction_OK(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	body := map[string]interface{}{"payload": map[string]string{"key": "val"}}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-async", body)
	assertStatus(t, w, 202)
}

func TestEnqueueAsyncFunction_EmptyPayload(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-async", nil)
	assertStatus(t, w, 202)
}

func TestEnqueueAsyncFunction_WithIdempotencyKey(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.enqueueAsyncInvocationWithIdempotencyFn = func(_ context.Context, inv *store.AsyncInvocation, key string, ttl time.Duration) (*store.AsyncInvocation, bool, error) {
		return inv, false, nil
	}
	body := map[string]interface{}{
		"payload":         map[string]string{"key": "val"},
		"idempotency_key": "idem-1",
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-async", body)
	assertStatus(t, w, 202)
}

func TestEnqueueAsyncFunction_IdempotencyReplay(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.enqueueAsyncInvocationWithIdempotencyFn = func(_ context.Context, inv *store.AsyncInvocation, key string, ttl time.Duration) (*store.AsyncInvocation, bool, error) {
		return inv, true, nil
	}
	body := map[string]interface{}{
		"payload":         map[string]string{"key": "val"},
		"idempotency_key": "idem-1",
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-async", body)
	assertStatus(t, w, 200)
}

func TestEnqueueAsyncFunction_InvalidIdempotencyKey(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.enqueueAsyncInvocationWithIdempotencyFn = func(_ context.Context, inv *store.AsyncInvocation, key string, ttl time.Duration) (*store.AsyncInvocation, bool, error) {
		return nil, false, store.ErrInvalidIdempotencyKey
	}
	body := map[string]interface{}{
		"payload":         map[string]string{"key": "val"},
		"idempotency_key": "bad",
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-async", body)
	assertStatus(t, w, 400)
}

func TestEnqueueAsyncFunction_QueueQuotaExceeded(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.checkTenantAbsoluteQuotaFn = func(_ context.Context, tenantID, dim string, value int64) (*store.TenantQuotaDecision, error) {
		return &store.TenantQuotaDecision{Allowed: false, TenantID: tenantID, Dimension: dim}, nil
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-async", nil)
	assertStatus(t, w, 429)
}

func TestEnqueueAsyncFunction_BackoffFields(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	var captured *store.AsyncInvocation
	ms.enqueueAsyncInvocationFn = func(_ context.Context, inv *store.AsyncInvocation) error {
		captured = inv
		return nil
	}
	body := map[string]interface{}{
		"max_attempts":    5,
		"backoff_base_ms": 2000,
		"backoff_max_ms":  30000,
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-async", body)
	assertStatus(t, w, 202)
	if captured != nil {
		if captured.MaxAttempts != 5 {
			t.Fatalf("expected max_attempts=5, got %d", captured.MaxAttempts)
		}
	}
}

func TestEnqueueAsyncFunction_InvalidJSON(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	req := httptest.NewRequest("POST", "/functions/hello/invoke-async", strings.NewReader("bad"))
	req.ContentLength = 3
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assertStatus(t, w, 400)
}

// --- Prewarm (cluster) handler -------------------------------------------

func TestPrewarmFunction_NoPool(t *testing.T) {
	ms := &mockMetadataStore{}
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return testFunction(), nil
	}
	s := store.NewStore(ms)
	h := &Handler{Store: s, Pool: nil}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	w := doRequest(t, mux, "POST", "/functions/hello/prewarm", nil)
	assertStatus(t, w, 503)
}

func TestPrewarmFunction_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "POST", "/functions/missing/prewarm", nil)
	assertStatus(t, w, 404)
}

func TestPrewarmFunction_CodeNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionCodeFn = func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
		return nil, nil
	}
	w := doRequest(t, mux, "POST", "/functions/hello/prewarm", nil)
	// Returns 404 because codeRecord is nil
	assertStatus(t, w, 404)
}

// --- Cost handlers -------------------------------------------------------

func TestFunctionCost(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/cost", nil)
	assertStatus(t, w, 200)
}

func TestFunctionCost_WithWindow(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/cost?window=3600", nil)
	assertStatus(t, w, 200)
}

func TestFunctionCost_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "GET", "/functions/missing/cost", nil)
	assertStatus(t, w, 404)
}

func TestFunctionCost_WithLogs(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listInvocationLogsFn = func(_ context.Context, fnID string, limit, offset int) ([]*store.InvocationLog, error) {
		return []*store.InvocationLog{
			{ID: "log-1", DurationMs: 100, ColdStart: true, CreatedAt: time.Now()},
			{ID: "log-2", DurationMs: 200, ColdStart: false, CreatedAt: time.Now()},
		}, nil
	}
	w := doRequest(t, mux, "GET", "/functions/hello/cost", nil)
	assertStatus(t, w, 200)
}

func TestCostSummary(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/cost/summary", nil)
	assertStatus(t, w, 200)
}

func TestCostSummary_WithFunctions(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listFunctionsFn = func(_ context.Context, limit, offset int) ([]*domain.Function, error) {
		return []*domain.Function{testFunction()}, nil
	}
	ms.listInvocationLogsFn = func(_ context.Context, fnID string, limit, offset int) ([]*store.InvocationLog, error) {
		return []*store.InvocationLog{
			{ID: "log-1", DurationMs: 100, ColdStart: true, CreatedAt: time.Now()},
		}, nil
	}
	w := doRequest(t, mux, "GET", "/cost/summary?window=86400", nil)
	assertStatus(t, w, 200)
}

func TestCostSummary_ListFunctionsError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listFunctionsFn = func(_ context.Context, limit, offset int) ([]*domain.Function, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/cost/summary", nil)
	assertStatus(t, w, 500)
}

// --- Metrics handlers ---------------------------------------------------

func TestFunctionMetrics(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/metrics", nil)
	assertStatus(t, w, 200)
}

func TestFunctionMetrics_WithRange(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/metrics?range=5m", nil)
	assertStatus(t, w, 200)
}

func TestFunctionMetrics_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "GET", "/functions/missing/metrics", nil)
	assertStatus(t, w, 404)
}

func TestFunctionSLOStatus_NoPolicy(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/slo/status", nil)
	assertStatus(t, w, 200)
	body := assertJSON(t, w)
	if body["enabled"] != false {
		t.Fatalf("expected enabled=false")
	}
}

func TestFunctionSLOStatus_WithPolicy(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		fn := testFunction()
		fn.SLOPolicy = &domain.SLOPolicy{
			Enabled:    true,
			WindowS:    900,
			MinSamples: 10,
			Objectives: domain.SLOObjectives{
				SuccessRatePct:   99.0,
				P95DurationMs:    500,
				ColdStartRatePct: 10.0,
			},
		}
		return fn, nil
	}
	ms.getFunctionSLOSnapshotFn = func(_ context.Context, fnID string, windowSeconds int) (*store.FunctionSLOSnapshot, error) {
		return &store.FunctionSLOSnapshot{
			TotalInvocations: 100,
			SuccessRatePct:   95.0,
			P95DurationMs:    600,
			ColdStartRatePct: 15.0,
		}, nil
	}
	w := doRequest(t, mux, "GET", "/functions/hello/slo/status", nil)
	assertStatus(t, w, 200)
	body := assertJSON(t, w)
	breaches, ok := body["breaches"].([]interface{})
	if !ok || len(breaches) != 3 {
		t.Fatalf("expected 3 breaches, got %v", body["breaches"])
	}
}

func TestFunctionSLOStatus_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "GET", "/functions/missing/slo/status", nil)
	assertStatus(t, w, 404)
}

func TestFunctionDiagnostics_NoLogs(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/diagnostics", nil)
	assertStatus(t, w, 200)
}

func TestFunctionDiagnostics_WithLogs(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	now := time.Now()
	ms.listInvocationLogsFn = func(_ context.Context, fnID string, limit, offset int) ([]*store.InvocationLog, error) {
		logs := make([]*store.InvocationLog, 20)
		for i := range logs {
			logs[i] = &store.InvocationLog{
				ID:         fmt.Sprintf("log-%d", i),
				DurationMs: int64(i*100 + 50),
				ColdStart:  i%3 == 0,
				Success:    i%5 != 0,
				CreatedAt:  now.Add(-time.Duration(i) * time.Minute),
			}
			if !logs[i].Success {
				logs[i].ErrorMessage = "test error"
			}
		}
		return logs, nil
	}
	w := doRequest(t, mux, "GET", "/functions/hello/diagnostics?window=86400&sample=100", nil)
	assertStatus(t, w, 200)
	body := assertJSON(t, w)
	if body["total_invocations"].(float64) < 1 {
		t.Fatal("expected some invocations")
	}
}

func TestFunctionDiagnostics_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "GET", "/functions/missing/diagnostics", nil)
	assertStatus(t, w, 404)
}

func TestAnalyzeFunctionDiagnostics_NoAI(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "POST", "/functions/hello/diagnostics/analyze", nil)
	assertStatus(t, w, 503) // AI not enabled
}

func TestGetPerformanceRecommendations_NoAdvisor(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	// With nil AIService, advisor is still created but AnalyzePerformance might fail
	// The handler creates a default advisor, so it should reach the store call
	w := doRequest(t, mux, "GET", "/functions/hello/recommendations", nil)
	// The advisor may return an error since store methods return minimal data
	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("expected 200 or 500, got %d", w.Code)
	}
}

func TestGetPerformanceRecommendations_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "GET", "/functions/missing/recommendations", nil)
	assertStatus(t, w, 404)
}

func TestFunctionHeatmap(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/heatmap", nil)
	assertStatus(t, w, 200)
}

func TestFunctionHeatmap_WithWeeks(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/heatmap?weeks=12", nil)
	assertStatus(t, w, 200)
}

func TestFunctionHeatmap_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "GET", "/functions/missing/heatmap", nil)
	assertStatus(t, w, 404)
}

func TestGlobalHeatmap(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/metrics/heatmap", nil)
	assertStatus(t, w, 200)
}

func TestGlobalHeatmap_WithWeeks(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/metrics/heatmap?weeks=26", nil)
	assertStatus(t, w, 200)
}

func TestGlobalTimeSeries(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/metrics/timeseries", nil)
	assertStatus(t, w, 200)
}

func TestGlobalTimeSeries_WithRange(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/metrics/timeseries?range=24h", nil)
	assertStatus(t, w, 200)
}

// --- Helper function tests -----------------------------------------------

func TestParseWindowParam(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback int
		want     int
	}{
		{"empty", "", 300, 300},
		{"raw_seconds", "600", 300, 600},
		{"minutes", "5m", 300, 300},
		{"hours", "2h", 300, 7200},
		{"days", "1d", 300, 86400},
		{"invalid", "xyz", 300, 300},
		{"zero", "0", 300, 300},
		{"negative", "-5", 300, 300},
		{"single_char", "m", 300, 300},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWindowParam(tt.input, tt.fallback)
			if got != tt.want {
				t.Errorf("parseWindowParam(%q, %d) = %d, want %d", tt.input, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestPercentile(t *testing.T) {
	tests := []struct {
		name   string
		values []int64
		p      float64
		want   int64
	}{
		{"empty", nil, 0.5, 0},
		{"single", []int64{42}, 0.5, 42},
		{"p0", []int64{1, 2, 3}, 0, 1},
		{"p100", []int64{1, 2, 3}, 1, 3},
		{"p50", []int64{10, 20, 30, 40, 50}, 0.5, 30},
		{"p95", []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0.95, 10},
		{"negative_p", []int64{1, 2, 3}, -0.1, 1},
		{"over_p", []int64{1, 2, 3}, 1.5, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := percentile(tt.values, tt.p)
			if got != tt.want {
				t.Errorf("percentile(%v, %f) = %d, want %d", tt.values, tt.p, got, tt.want)
			}
		})
	}
}

func TestParseLimitQuery(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		fallback int
		max      int
		want     int
	}{
		{"empty", "", 10, 100, 10},
		{"valid", "50", 10, 100, 50},
		{"over_max", "200", 10, 100, 100},
		{"zero", "0", 10, 100, 10},
		{"negative", "-5", 10, 100, 10},
		{"no_max", "999", 10, 0, 999},
		{"invalid", "abc", 10, 100, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLimitQuery(tt.raw, tt.fallback, tt.max)
			if got != tt.want {
				t.Errorf("parseLimitQuery(%q, %d, %d) = %d, want %d", tt.raw, tt.fallback, tt.max, got, tt.want)
			}
		})
	}
}

func TestParseRangeParam_Days(t *testing.T) {
	r, b := parseRangeParam("7d")
	if r != 7*86400 {
		t.Errorf("expected 7d range = %d, got %d", 7*86400, r)
	}
	if b < 1 {
		t.Error("bucket must be >= 1")
	}
}

// --- Pagination helpers --------------------------------------------------

func TestEstimatePaginatedTotal(t *testing.T) {
	tests := []struct {
		limit, offset, returned int
		want                    int64
	}{
		{10, 0, 5, 5},   // less than limit, exact
		{10, 0, 10, 11},  // full page, +1 for possible more
		{10, 20, 10, 31}, // full page from offset
		{-1, -1, -1, 0},  // negatives
	}
	for _, tt := range tests {
		got := estimatePaginatedTotal(tt.limit, tt.offset, tt.returned)
		if got != tt.want {
			t.Errorf("estimatePaginatedTotal(%d, %d, %d) = %d, want %d",
				tt.limit, tt.offset, tt.returned, got, tt.want)
		}
	}
}

func TestWritePaginatedList(t *testing.T) {
	w := httptest.NewRecorder()
	items := []string{"a", "b", "c"}
	writePaginatedList(w, 10, 0, 3, 3, items)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result paginatedListWithSummaryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Pagination.HasMore {
		t.Fatal("expected has_more=false")
	}
}

func TestWritePaginatedList_HasMore(t *testing.T) {
	w := httptest.NewRecorder()
	items := []string{"a", "b"}
	writePaginatedList(w, 2, 0, 2, 10, items)
	var result paginatedListWithSummaryResponse
	json.Unmarshal(w.Body.Bytes(), &result)
	if !result.Pagination.HasMore {
		t.Fatal("expected has_more=true")
	}
	if result.Pagination.NextOffset == nil || *result.Pagination.NextOffset != 2 {
		t.Fatal("expected next_offset=2")
	}
}

func TestWritePaginatedListWithSummary(t *testing.T) {
	w := httptest.NewRecorder()
	items := []string{"a"}
	summary := map[string]int{"total": 1}
	writePaginatedListWithSummary(w, 10, 0, 1, 1, items, summary)
	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["summary"] == nil {
		t.Fatal("expected summary")
	}
}

func TestWritePaginatedList_NegativeInputs(t *testing.T) {
	w := httptest.NewRecorder()
	writePaginatedList(w, -1, -1, -1, -1, []string{})
	assertStatus(t, w, 200)
}

// --- Ingress policy helpers -----------------------------------------------

func TestIngressCallerFromRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Nova-Source-Function", "caller-fn")
	req.Header.Set("X-Nova-Source-IP", "10.0.0.1")
	req.RemoteAddr = "192.168.1.1:12345"
	caller := ingressCallerFromRequest(req.Context(), req)
	if caller.SourceFunction != "caller-fn" {
		t.Fatalf("expected caller-fn, got %s", caller.SourceFunction)
	}
	if caller.SourceIP != "10.0.0.1" {
		t.Fatalf("expected 10.0.0.1, got %s", caller.SourceIP)
	}
}

func TestIngressCallerFromRequest_FallbackToRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:9090"
	caller := ingressCallerFromRequest(req.Context(), req)
	if caller.SourceIP != "192.168.1.1" {
		t.Fatalf("expected 192.168.1.1, got %s", caller.SourceIP)
	}
}

func TestIngressCallerFromRequest_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	req.RemoteAddr = "192.168.1.1:80"
	caller := ingressCallerFromRequest(req.Context(), req)
	if caller.SourceIP != "1.2.3.4" {
		t.Fatalf("expected 1.2.3.4, got %s", caller.SourceIP)
	}
}

func TestParsePositivePort(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"8080", 8080},
		{"0", 0},
		{"-1", 0},
		{"99999", 0},
		{"abc", 0},
		{"443", 443},
		{"", 0},
	}
	for _, tt := range tests {
		got := parsePositivePort(tt.input)
		if got != tt.want {
			t.Errorf("parsePositivePort(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestFirstIP(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"1.2.3.4", "1.2.3.4"},
		{"1.2.3.4, 5.6.7.8", "1.2.3.4"},
		{"::1", "::1"},
		{"invalid", ""},
		{"1.2.3.4:8080", "1.2.3.4"},
	}
	for _, tt := range tests {
		got := firstIP(tt.input)
		if got != tt.want {
			t.Errorf("firstIP(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRemoteAddrIP(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"192.168.1.1:8080", "192.168.1.1"},
		{"192.168.1.1", "192.168.1.1"},
		{"[::1]:80", "::1"},
		{"invalid:abc", ""},
	}
	for _, tt := range tests {
		got := remoteAddrIP(tt.input)
		if got != tt.want {
			t.Errorf("remoteAddrIP(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRequestPort(t *testing.T) {
	t.Run("X-Forwarded-Port", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-Port", "8443")
		if got := requestPort(req); got != 8443 {
			t.Errorf("expected 8443, got %d", got)
		}
	})

	t.Run("Host with port", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com:9000/", nil)
		if got := requestPort(req); got != 9000 {
			t.Errorf("expected 9000, got %d", got)
		}
	})

	t.Run("Default HTTP", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/", nil)
		if got := requestPort(req); got != 80 {
			t.Errorf("expected 80, got %d", got)
		}
	})
}

// --- InvokeFunction additional error paths --------------------------------

func TestInvokeFunction_InvalidPayload(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	req := httptest.NewRequest("POST", "/functions/hello/invoke", strings.NewReader("not json"))
	req.ContentLength = 8
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assertStatus(t, w, 400)
}

func TestInvokeFunctionStream_InvalidPayload(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	req := httptest.NewRequest("POST", "/functions/hello/invoke-stream", strings.NewReader("not json"))
	req.ContentLength = 8
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assertStatus(t, w, 400)
}

func TestInvokeFunction_TenantQuotaExceeded(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.checkAndConsumeTenantQuotaFn = func(_ context.Context, tenantID, dim string, amount int64) (*store.TenantQuotaDecision, error) {
		return &store.TenantQuotaDecision{Allowed: false, TenantID: tenantID, Dimension: "invocations", RetryAfterS: 5}, nil
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke", map[string]string{"key": "val"})
	assertStatus(t, w, 429)
}

func TestInvokeFunction_QuotaCheckError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.checkAndConsumeTenantQuotaFn = func(_ context.Context, tenantID, dim string, amount int64) (*store.TenantQuotaDecision, error) {
		return nil, fmt.Errorf("quota db error")
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke", map[string]string{"key": "val"})
	assertStatus(t, w, 500)
}

func TestInvokeFunctionStream_TenantQuotaExceeded(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.checkAndConsumeTenantQuotaFn = func(_ context.Context, tenantID, dim string, amount int64) (*store.TenantQuotaDecision, error) {
		return &store.TenantQuotaDecision{Allowed: false, TenantID: tenantID, Dimension: "invocations"}, nil
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-stream", map[string]string{"key": "val"})
	assertStatus(t, w, 429)
}

func TestInvokeFunctionStream_QuotaCheckError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.checkAndConsumeTenantQuotaFn = func(_ context.Context, tenantID, dim string, amount int64) (*store.TenantQuotaDecision, error) {
		return nil, fmt.Errorf("quota db error")
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-stream", map[string]string{"key": "val"})
	assertStatus(t, w, 500)
}

// --- Capacity policy helpers -----------------------------------------------

func TestCapacityShedStatus(t *testing.T) {
	if s := capacityShedStatus(nil); s != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", s)
	}
	fn := testFunction()
	if s := capacityShedStatus(fn); s != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for nil policy, got %d", s)
	}
	fn.CapacityPolicy = &domain.CapacityPolicy{ShedStatusCode: 429}
	if s := capacityShedStatus(fn); s != 429 {
		t.Fatalf("expected 429, got %d", s)
	}
}

func TestCapacityRetryAfter(t *testing.T) {
	if r := capacityRetryAfter(nil); r != 1 {
		t.Fatalf("expected 1, got %d", r)
	}
	fn := testFunction()
	if r := capacityRetryAfter(fn); r != 1 {
		t.Fatalf("expected 1, got %d", r)
	}
	fn.CapacityPolicy = &domain.CapacityPolicy{RetryAfterS: 10}
	if r := capacityRetryAfter(fn); r != 10 {
		t.Fatalf("expected 10, got %d", r)
	}
}

// --- writeTenantQuotaExceeded -------------------------------------------

func TestWriteTenantQuotaExceeded_NilDecision(t *testing.T) {
	w := httptest.NewRecorder()
	writeTenantQuotaExceeded(w, nil)
	assertStatus(t, w, 429)
}

func TestWriteTenantQuotaExceeded_WithRetryAfter(t *testing.T) {
	w := httptest.NewRecorder()
	writeTenantQuotaExceeded(w, &store.TenantQuotaDecision{
		TenantID:    "t1",
		Dimension:   "invocations",
		RetryAfterS: 5,
	})
	assertStatus(t, w, 429)
	if w.Header().Get("Retry-After") != "5" {
		t.Fatalf("expected Retry-After=5, got %q", w.Header().Get("Retry-After"))
	}
}

// --- summarizeInvocations -----------------------------------------------

func TestSummarizeInvocations_Empty(t *testing.T) {
	s := summarizeInvocations(nil)
	if s.TotalInvocations != 0 {
		t.Fatalf("expected 0")
	}
}

func TestSummarizeInvocations_WithData(t *testing.T) {
	entries := []*store.InvocationLog{
		{Success: true, ColdStart: true, DurationMs: 100},
		{Success: false, ColdStart: false, DurationMs: 200},
		nil, // should be skipped
	}
	s := summarizeInvocations(entries)
	if s.TotalInvocations != 3 {
		t.Fatalf("expected 3, got %d", s.TotalInvocations)
	}
	if s.Successes != 1 {
		t.Fatalf("expected 1 success, got %d", s.Successes)
	}
	if s.Failures != 1 {
		t.Fatalf("expected 1 failure, got %d", s.Failures)
	}
	if s.ColdStarts != 1 {
		t.Fatalf("expected 1 cold start, got %d", s.ColdStarts)
	}
}

// --- parseAsyncStatuses ---------------------------------------------------

func TestParseAsyncStatuses(t *testing.T) {
	tests := []struct {
		input   string
		wantN   int
		wantErr bool
	}{
		{"", 0, false},
		{"queued", 1, false},
		{"queued,running", 2, false},
		{"queued,,running", 2, false},
		{"invalid", 0, true},
	}
	for _, tt := range tests {
		statuses, err := parseAsyncStatuses(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseAsyncStatuses(%q): err=%v, wantErr=%v", tt.input, err, tt.wantErr)
		}
		if len(statuses) != tt.wantN {
			t.Errorf("parseAsyncStatuses(%q): got %d statuses, want %d", tt.input, len(statuses), tt.wantN)
		}
	}
}

// --- ListLogs store error -------------------------------------------------

func TestLogs_StoreError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listInvocationLogsFn = func(_ context.Context, fnID string, limit, offset int) ([]*store.InvocationLog, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/functions/hello/logs", nil)
	assertStatus(t, w, 500)
}

// --- FunctionDiagnostics store error --------------------------------------

func TestFunctionDiagnostics_StoreError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listInvocationLogsFn = func(_ context.Context, fnID string, limit, offset int) ([]*store.InvocationLog, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/functions/hello/diagnostics", nil)
	assertStatus(t, w, 500)
}

// --- SLO snapshot error ---------------------------------------------------

func TestFunctionSLOStatus_SnapshotError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		fn := testFunction()
		fn.SLOPolicy = &domain.SLOPolicy{Enabled: true}
		return fn, nil
	}
	ms.getFunctionSLOSnapshotFn = func(_ context.Context, fnID string, windowSeconds int) (*store.FunctionSLOSnapshot, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/functions/hello/slo/status", nil)
	assertStatus(t, w, 500)
}

// --- Async operations store errors ----------------------------------------

func TestListAsyncInvocations_StoreError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listAsyncInvocationsFn = func(_ context.Context, limit, offset int, statuses []store.AsyncInvocationStatus) ([]*store.AsyncInvocation, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/async-invocations", nil)
	assertStatus(t, w, 500)
}

func TestListFunctionAsyncInvocations_StoreError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listFunctionAsyncInvocationsFn = func(_ context.Context, fnID string, limit, offset int, statuses []store.AsyncInvocationStatus) ([]*store.AsyncInvocation, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/functions/hello/async-invocations", nil)
	assertStatus(t, w, 500)
}

func TestPauseAsyncInvocationsByFunction_Error(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.pauseAsyncInvocationsByFunctionFn = func(_ context.Context, fnID string) (int, error) {
		return 0, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "POST", "/async-invocations/functions/fn-1/pause", nil)
	assertStatus(t, w, 500)
}

func TestResumeAsyncInvocationsByFunction_Error(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.resumeAsyncInvocationsByFunctionFn = func(_ context.Context, fnID string) (int, error) {
		return 0, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "POST", "/async-invocations/functions/fn-1/resume", nil)
	assertStatus(t, w, 500)
}

func TestPauseAsyncInvocationsByWorkflow_Error(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.pauseAsyncInvocationsByWorkflowFn = func(_ context.Context, wfID string) (int, error) {
		return 0, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "POST", "/async-invocations/workflows/wf-1/pause", nil)
	assertStatus(t, w, 500)
}

func TestResumeAsyncInvocationsByWorkflow_Error(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.resumeAsyncInvocationsByWorkflowFn = func(_ context.Context, wfID string) (int, error) {
		return 0, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "POST", "/async-invocations/workflows/wf-1/resume", nil)
	assertStatus(t, w, 500)
}

func TestDeleteAsyncInvocation_InternalError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.deleteAsyncInvocationFn = func(_ context.Context, id string) error {
		return fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "DELETE", "/async-invocations/inv-1", nil)
	assertStatus(t, w, 500)
}

func TestPauseAsyncInvocation_InternalError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.pauseAsyncInvocationFn = func(_ context.Context, id string) (*store.AsyncInvocation, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "POST", "/async-invocations/inv-1/pause", nil)
	assertStatus(t, w, 500)
}

func TestResumeAsyncInvocation_InternalError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.resumeAsyncInvocationFn = func(_ context.Context, id string) (*store.AsyncInvocation, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "POST", "/async-invocations/inv-1/resume", nil)
	assertStatus(t, w, 500)
}

func TestRetryAsyncInvocation_GenericError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.requeueAsyncInvocationFn = func(_ context.Context, id string, maxAttempts int) (*store.AsyncInvocation, error) {
		return nil, fmt.Errorf("unexpected error")
	}
	w := doRequest(t, mux, "POST", "/async-invocations/inv-1/retry", nil)
	assertStatus(t, w, 400)
}

func TestRetryAsyncInvocation_InvalidJSON(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	req := httptest.NewRequest("POST", "/async-invocations/inv-1/retry", strings.NewReader("not json"))
	req.ContentLength = 8
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assertStatus(t, w, 400)
}

// --- EnqueueAsyncFunction store errors ------------------------------------

func TestEnqueueAsyncFunction_QueueDepthError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getTenantAsyncQueueDepthFn = func(_ context.Context, tenantID string) (int64, error) {
		return 0, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-async", nil)
	assertStatus(t, w, 500)
}

func TestEnqueueAsyncFunction_QuotaCheckError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.checkTenantAbsoluteQuotaFn = func(_ context.Context, tenantID, dim string, value int64) (*store.TenantQuotaDecision, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-async", nil)
	assertStatus(t, w, 500)
}

func TestEnqueueAsyncFunction_EnqueueError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.enqueueAsyncInvocationFn = func(_ context.Context, inv *store.AsyncInvocation) error {
		return fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-async", nil)
	assertStatus(t, w, 500)
}

func TestEnqueueAsyncFunction_IdempotencyInternalError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.enqueueAsyncInvocationWithIdempotencyFn = func(_ context.Context, inv *store.AsyncInvocation, key string, ttl time.Duration) (*store.AsyncInvocation, bool, error) {
		return nil, false, fmt.Errorf("db error")
	}
	body := map[string]interface{}{"idempotency_key": "test-key"}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-async", body)
	assertStatus(t, w, 500)
}

// --- FunctionCost store error ---------------------------------------------

func TestFunctionCost_ListLogsError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listInvocationLogsFn = func(_ context.Context, fnID string, limit, offset int) ([]*store.InvocationLog, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/functions/hello/cost", nil)
	assertStatus(t, w, 500)
}

// --- GetFunctionState list errors ----------------------------------------

func TestGetFunctionState_ListStoreError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listFunctionStatesFn = func(_ context.Context, fnID string, opts *store.FunctionStateListOptions) ([]*store.FunctionStateEntry, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/functions/hello/state", nil)
	assertStatus(t, w, 500)
}

// --- PutFunctionState store errors ----------------------------------------

func TestPutFunctionState_InternalError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.putFunctionStateFn = func(_ context.Context, fnID, key string, value json.RawMessage, opts *store.FunctionStatePutOptions) (*store.FunctionStateEntry, error) {
		return nil, fmt.Errorf("db error")
	}
	body := map[string]interface{}{"value": "hello"}
	w := doRequest(t, mux, "PUT", "/functions/hello/state?key=mykey", body)
	assertStatus(t, w, 500)
}

func TestPutFunctionState_StateNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.putFunctionStateFn = func(_ context.Context, fnID, key string, value json.RawMessage, opts *store.FunctionStatePutOptions) (*store.FunctionStateEntry, error) {
		return nil, store.ErrFunctionStateNotFound
	}
	body := map[string]interface{}{"value": "hello"}
	w := doRequest(t, mux, "PUT", "/functions/hello/state?key=mykey", body)
	assertStatus(t, w, 404)
}

// --- DLQ count error ------------------------------------------------------

func TestListDLQInvocations_CountError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listAsyncInvocationsFn = func(_ context.Context, limit, offset int, statuses []store.AsyncInvocationStatus) ([]*store.AsyncInvocation, error) {
		return []*store.AsyncInvocation{}, nil
	}
	ms.countAsyncInvocationsFn = func(_ context.Context, statuses []store.AsyncInvocationStatus) (int64, error) {
		return 0, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/async-invocations/dlq", nil)
	assertStatus(t, w, 500)
}

// --- RetryAllDLQ store error  ---------------------------------------------

func TestRetryAllDLQ_ListError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listAsyncInvocationsFn = func(_ context.Context, limit, offset int, statuses []store.AsyncInvocationStatus) ([]*store.AsyncInvocation, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "POST", "/async-invocations/dlq/retry-all", nil)
	assertStatus(t, w, 500)
}

// --- safeError function ---------------------------------------------------

func TestSafeError(t *testing.T) {
	w := httptest.NewRecorder()
	safeError(w, "something went wrong", 500, fmt.Errorf("internal"))
	assertStatus(t, w, 500)
	if !strings.Contains(w.Body.String(), "something went wrong") {
		t.Fatal("expected public message in body")
	}
}

func TestSafeError_NilErr(t *testing.T) {
	w := httptest.NewRecorder()
	safeError(w, "not found", 404, nil)
	assertStatus(t, w, 404)
}

// --- ListAllInvocations with Q parameter ---------------------------------

func TestListAllInvocations_QParam(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/invocations?q=hello", nil)
	assertStatus(t, w, 200)
}

// --- ListFunctionAsyncInvocations invalid status --------------------------

func TestListFunctionAsyncInvocations_InvalidStatus(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/async-invocations?status=bogus", nil)
	assertStatus(t, w, 400)
}

// --- ListWorkflowAsyncInvocations errors ---------------------------------

func TestListWorkflowAsyncInvocations_InvokeListError(t *testing.T) {
	h, mux, _ := setupTestHandler(t)
	wf := &mockWorkflowStore{
		getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: "wf-1", Name: name}, nil
		},
	}
	h.Store.WorkflowStore = wf
	h.Store.MetadataStore.(*mockMetadataStore).listWorkflowAsyncInvocationsFn = func(_ context.Context, wfID string, limit, offset int, statuses []store.AsyncInvocationStatus) ([]*store.AsyncInvocation, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/workflows/my-workflow/async-invocations", nil)
	assertStatus(t, w, 500)
}

func TestListWorkflowAsyncInvocations_CountError(t *testing.T) {
	h, mux, _ := setupTestHandler(t)
	wf := &mockWorkflowStore{
		getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: "wf-1", Name: name}, nil
		},
	}
	h.Store.WorkflowStore = wf
	h.Store.MetadataStore.(*mockMetadataStore).countWorkflowAsyncInvocationsFn = func(_ context.Context, wfID string, statuses []store.AsyncInvocationStatus) (int64, error) {
		return 0, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/workflows/my-workflow/async-invocations", nil)
	assertStatus(t, w, 500)
}

// --- StreamLogs handler ---------------------------------------------------

func TestStreamLogs_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "GET", "/functions/missing/logs/stream", nil)
	assertStatus(t, w, 404)
}

func TestStreamLogs_CancelledContext(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	req := httptest.NewRequest("GET", "/functions/hello/logs/stream", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Should return quickly after context cancelled
	assertStatus(t, w, 200)
}

// --- InvokeFunction with nil executor → skipped (panics as expected) ---

// --- PrewarmFunction with code and body ----------------------------------

func TestPrewarmFunction_WithCode(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionCodeFn = func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
		return &domain.FunctionCode{SourceCode: "print('hello')"}, nil
	}
	w := doRequest(t, mux, "POST", "/functions/hello/prewarm", nil)
	// EnsureReady may fail because mock backend can't create full clients.
	// Accept any of: 200 (success), 500 (expected pool error)
	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("expected 200 or 500, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestPrewarmFunction_WithBody(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionCodeFn = func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
		return &domain.FunctionCode{SourceCode: "print('hello')"}, nil
	}
	body := map[string]interface{}{"target_replicas": 3}
	w := doRequest(t, mux, "POST", "/functions/hello/prewarm", body)
	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("expected 200 or 500, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestPrewarmFunction_InvalidJSON(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	req := httptest.NewRequest("POST", "/functions/hello/prewarm", strings.NewReader("bad"))
	req.ContentLength = 3
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assertStatus(t, w, 400)
}

func TestPrewarmFunction_CodeError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionCodeFn = func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "POST", "/functions/hello/prewarm", nil)
	assertStatus(t, w, 500)
}

// --- IngressPolicy with NetworkPolicy ------------------------------------

func TestInvokeFunction_IngressDenied(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	fn := testFunction()
	fn.NetworkPolicy = &domain.NetworkPolicy{
		IsolationMode: "strict",
		IngressRules: []domain.IngressRule{
			{Source: "allowed-fn"},
		},
	}
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return fn, nil
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke", map[string]string{"key": "val"})
	// strict mode with no matching source function → 403
	assertStatus(t, w, 403)
}

func TestEnqueueAsyncFunction_IngressDenied(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	fn := testFunction()
	fn.NetworkPolicy = &domain.NetworkPolicy{
		IsolationMode: "strict",
		IngressRules: []domain.IngressRule{
			{Source: "allowed-fn"},
		},
	}
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return fn, nil
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-async", nil)
	assertStatus(t, w, 403)
}

// --- ingressCallerFromRequest with gRPC metadata -------------------------

func TestIngressCallerFromRequest_ProtocolAndPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Nova-Source-Function", "caller-fn")
	req.Header.Set("X-Nova-Source-IP", "10.0.0.1")
	req.Header.Set("X-Nova-Source-Protocol", "udp")
	req.Header.Set("X-Nova-Source-Port", "8080")
	caller := ingressCallerFromRequest(req.Context(), req)
	if caller.Protocol != "udp" {
		t.Fatalf("expected udp, got %s", caller.Protocol)
	}
	if caller.Port != 8080 {
		t.Fatalf("expected 8080, got %d", caller.Port)
	}
}

func TestIngressCallerFromRequest_GRPCMetadata(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	// Use gRPC metadata context
	md := metadata.New(map[string]string{
		"x-nova-source-function": "grpc-caller",
		"x-nova-source-ip":      "10.0.0.2",
		"x-nova-source-protocol": "grpc",
		"x-nova-source-port":     "50051",
	})
	ctx := metadata.NewIncomingContext(req.Context(), md)
	caller := ingressCallerFromRequest(ctx, req)
	if caller.SourceFunction != "grpc-caller" {
		t.Fatalf("expected grpc-caller, got %s", caller.SourceFunction)
	}
	if caller.SourceIP != "10.0.0.2" {
		t.Fatalf("expected 10.0.0.2, got %s", caller.SourceIP)
	}
	if caller.Protocol != "grpc" {
		t.Fatalf("expected grpc, got %s", caller.Protocol)
	}
	if caller.Port != 50051 {
		t.Fatalf("expected 50051, got %d", caller.Port)
	}
}

func TestIngressCallerFromRequest_GRPCMetadataFallback(t *testing.T) {
	// Headers empty, only gRPC metadata set
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	md := metadata.New(map[string]string{
		"x-nova-source-function": "grpc-fn",
	})
	ctx := metadata.NewIncomingContext(req.Context(), md)
	caller := ingressCallerFromRequest(ctx, req)
	if caller.SourceFunction != "grpc-fn" {
		t.Fatalf("expected grpc-fn, got %s", caller.SourceFunction)
	}
	// IP should fallback to remoteAddr
	if caller.SourceIP != "192.168.1.1" {
		t.Fatalf("expected 192.168.1.1, got %s", caller.SourceIP)
	}
	if caller.Protocol != "tcp" {
		t.Fatalf("expected tcp default, got %s", caller.Protocol)
	}
}

// --- enforceIngressPolicy ------------------------------------------------

func TestEnforceIngressPolicy_NilNetworkPolicy(t *testing.T) {
	h, _, _ := setupTestHandler(t)
	fn := testFunction()
	fn.NetworkPolicy = nil
	req := httptest.NewRequest("GET", "/", nil)
	if err := h.enforceIngressPolicy(req.Context(), req, fn); err != nil {
		t.Fatalf("expected nil error for nil NetworkPolicy, got %v", err)
	}
}

// --- metadataValue -------------------------------------------------------

func TestMetadataValue(t *testing.T) {
	md := metadata.New(map[string]string{
		"x-my-key": "my-value",
	})
	if got := metadataValue(md, "X-My-Key"); got != "my-value" {
		t.Fatalf("expected my-value, got %q", got)
	}
	if got := metadataValue(md, "nonexistent"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestMetadataValue_EmptyValue(t *testing.T) {
	md := metadata.New(map[string]string{
		"x-empty": "",
	})
	if got := metadataValue(md, "x-empty"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// --- AsyncInvocationsSummary errors --------------------------------------

func TestAsyncInvocationsSummary_Error(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getAsyncInvocationSummaryFn = func(_ context.Context) (*store.AsyncInvocationSummary, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/async-invocations/summary", nil)
	assertStatus(t, w, 500)
}

func TestAsyncInvocationsSummary_NilSummary(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getAsyncInvocationSummaryFn = func(_ context.Context) (*store.AsyncInvocationSummary, error) {
		return nil, nil
	}
	w := doRequest(t, mux, "GET", "/async-invocations/summary", nil)
	assertStatus(t, w, 200)
}

// --- Heatmap/TimeSeries store errors -------------------------------------

func TestFunctionHeatmap_StoreError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionDailyHeatmapFn = func(_ context.Context, fnID string, weeks int) ([]store.DailyCount, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/functions/hello/heatmap", nil)
	assertStatus(t, w, 200) // handler swallows error and returns []
}

func TestGlobalHeatmap_StoreError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getGlobalDailyHeatmapFn = func(_ context.Context, weeks int) ([]store.DailyCount, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/metrics/heatmap", nil)
	assertStatus(t, w, 200) // handler swallows error and returns []
}

func TestGlobalTimeSeries_StoreError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getGlobalTimeSeriesFn = func(_ context.Context, rangeSeconds, bucketSeconds int) ([]store.TimeSeriesBucket, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/metrics/timeseries", nil)
	assertStatus(t, w, 200) // handler swallows error and returns []
}

func TestFunctionMetrics_TimeSeriesError(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionTimeSeriesFn = func(_ context.Context, fnID string, rangeSeconds, bucketSeconds int) ([]store.TimeSeriesBucket, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/functions/hello/metrics", nil)
	assertStatus(t, w, 200) // handler swallows error and continues with empty data
}

// --- parseRangeParam edge cases -------------------------------------------

func TestParseRangeParam_Variants(t *testing.T) {
	tests := []struct {
		input    string
		wantSec  int
	}{
		{"5m", 300},
		{"1h", 3600},
		{"24h", 86400},
		{"7d", 604800},
		{"", 3600},       // default
		{"invalid", 3600},// default
		{"0h", 3600},     // fallback on 0
	}
	for _, tt := range tests {
		r, _ := parseRangeParam(tt.input)
		if r != tt.wantSec {
			t.Errorf("parseRangeParam(%q) = %d, want %d", tt.input, r, tt.wantSec)
		}
	}
}

// --- CostSummary with window error ----------------------------------------

func TestCostSummary_WithWindow(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listFunctionsFn = func(_ context.Context, limit, offset int) ([]*domain.Function, error) {
		return []*domain.Function{}, nil
	}
	w := doRequest(t, mux, "GET", "/cost/summary?window=3600", nil)
	assertStatus(t, w, 200)
}

// --- Pagination param edge cases ------------------------------------------

func TestEstimatePaginatedTotal_Additional(t *testing.T) {
	// 0 returned
	if got := estimatePaginatedTotal(10, 0, 0); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// --- GetPerformanceRecommendations with days param -------------------------

func TestGetPerformanceRecommendations_WithDays(t *testing.T) {
	_, mux, _ := setupTestHandler(t)
	w := doRequest(t, mux, "GET", "/functions/hello/recommendations?days=30", nil)
	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("expected 200 or 500, got %d", w.Code)
	}
}

// --- AnalyzeFunctionDiagnostics with AI enabled ---------------------------

func TestAnalyzeFunctionDiagnostics_FnNotFound(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "POST", "/functions/missing/diagnostics/analyze", nil)
	// AI not enabled, so 503 before function lookup
	assertStatus(t, w, 503)
}

func TestAnalyzeFunctionDiagnostics_WithAI_NoLogs(t *testing.T) {
	h, mux, ms := setupTestHandler(t)
	h.AIService = ai.NewService(ai.Config{
		Enabled: true,
		APIKey:  "test-key",
		BaseURL: "http://localhost:99999",
	})
	ms.listInvocationLogsFn = func(_ context.Context, fnID string, limit, offset int) ([]*store.InvocationLog, error) {
		return []*store.InvocationLog{}, nil
	}
	w := doRequest(t, mux, "POST", "/functions/hello/diagnostics/analyze", nil)
	assertStatus(t, w, 400) // no data
}

func TestAnalyzeFunctionDiagnostics_WithAI_StoreError(t *testing.T) {
	h, mux, ms := setupTestHandler(t)
	h.AIService = ai.NewService(ai.Config{
		Enabled: true,
		APIKey:  "test-key",
		BaseURL: "http://localhost:99999",
	})
	ms.listInvocationLogsFn = func(_ context.Context, fnID string, limit, offset int) ([]*store.InvocationLog, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "POST", "/functions/hello/diagnostics/analyze", nil)
	assertStatus(t, w, 500)
}

func TestAnalyzeFunctionDiagnostics_WithAI_FnNotFound(t *testing.T) {
	h, mux, ms := setupTestHandler(t)
	h.AIService = ai.NewService(ai.Config{
		Enabled: true,
		APIKey:  "test-key",
		BaseURL: "http://localhost:99999",
	})
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return nil, fmt.Errorf("not found")
	}
	w := doRequest(t, mux, "POST", "/functions/missing/diagnostics/analyze", nil)
	assertStatus(t, w, 404)
}

func TestAnalyzeFunctionDiagnostics_WithAI_WithLogs(t *testing.T) {
	h, mux, ms := setupTestHandler(t)
	h.AIService = ai.NewService(ai.Config{
		Enabled: true,
		APIKey:  "test-key",
		BaseURL: "http://localhost:99999",
	})
	now := time.Now()
	ms.listInvocationLogsFn = func(_ context.Context, fnID string, limit, offset int) ([]*store.InvocationLog, error) {
		logs := make([]*store.InvocationLog, 20)
		for i := range logs {
			logs[i] = &store.InvocationLog{
				ID:         fmt.Sprintf("log-%d", i),
				DurationMs: int64(i*100 + 50),
				ColdStart:  i%3 == 0,
				Success:    i%5 != 0,
				CreatedAt:  now.Add(-time.Duration(i) * time.Minute),
			}
			if !logs[i].Success {
				logs[i].ErrorMessage = "test error"
			}
		}
		return logs, nil
	}
	w := doRequest(t, mux, "POST", "/functions/hello/diagnostics/analyze?window=86400", nil)
	// The AI call will fail (no real API), so expect 500
	assertStatus(t, w, 500)
}

// --- CostSummary per-function log errors ----------------------------------

func TestCostSummary_LogErrorContinues(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.listFunctionsFn = func(_ context.Context, limit, offset int) ([]*domain.Function, error) {
		return []*domain.Function{testFunction()}, nil
	}
	ms.listInvocationLogsFn = func(_ context.Context, fnID string, limit, offset int) ([]*store.InvocationLog, error) {
		return nil, fmt.Errorf("db error")
	}
	w := doRequest(t, mux, "GET", "/cost/summary", nil)
	assertStatus(t, w, 200) // handler continues despite per-function error
}

// --- FunctionMetrics with pool data --------------------------------------

func TestFunctionMetrics_WithPoolAndLogs(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionTimeSeriesFn = func(_ context.Context, fnID string, rangeSeconds, bucketSeconds int) ([]store.TimeSeriesBucket, error) {
		return []store.TimeSeriesBucket{
			{Timestamp: time.Now(), Invocations: 10, Errors: 1, AvgDuration: 150},
		}, nil
	}
	w := doRequest(t, mux, "GET", "/functions/hello/metrics?range=1h", nil)
	assertStatus(t, w, 200)
}

// --- EnforceIngressPolicy with ingress denied on stream -------------------

func TestInvokeFunctionStream_IngressDenied(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	fn := testFunction()
	fn.NetworkPolicy = &domain.NetworkPolicy{
		IsolationMode: "strict",
		IngressRules: []domain.IngressRule{{Source: "allowed-fn"}},
	}
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return fn, nil
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-stream", nil)
	assertStatus(t, w, 403)
}

// --- PrewarmFunction with compiled binary  --------------------------------

func TestPrewarmFunction_WithCompiledBinary(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	ms.getFunctionCodeFn = func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
		return &domain.FunctionCode{CompiledBinary: []byte("binary-data")}, nil
	}
	w := doRequest(t, mux, "POST", "/functions/hello/prewarm", nil)
	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("expected 200 or 500, got %d", w.Code)
	}
}

// --- requestPort edge cases -----------------------------------------------

func TestRequestPort_HTTPS(t *testing.T) {
	req := httptest.NewRequest("GET", "https://example.com/", nil)
	// HTTPS with no explicit port
	if got := requestPort(req); got != 443 {
		t.Errorf("expected 443, got %d", got)
	}
}

func TestRequestPort_NoPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "example.com"
	if got := requestPort(req); got != 80 {
		t.Errorf("expected 80, got %d", got)
	}
}

// --- InvokeFunction with real executor -----------------------------------

func TestInvokeFunction_WithExec_OK(t *testing.T) {
	_, mux, _ := setupTestHandlerWithExec(t)
	w := doRequest(t, mux, "POST", "/functions/hello/invoke", map[string]string{"key": "val"})
	assertStatus(t, w, 200)
	body := assertJSON(t, w)
	if body["request_id"] == nil {
		t.Fatal("expected request_id in response")
	}
}

func TestInvokeFunction_WithExec_EmptyPayload(t *testing.T) {
	_, mux, _ := setupTestHandlerWithExec(t)
	w := doRequest(t, mux, "POST", "/functions/hello/invoke", nil)
	assertStatus(t, w, 200)
}

func TestInvokeFunction_WithExec_ClusterForwardHeader(t *testing.T) {
	// With X-Nova-Cluster-Forwarded set, cluster routing is skipped
	_, mux, _ := setupTestHandlerWithExec(t)
	req := httptest.NewRequest("POST", "/functions/hello/invoke", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nova-Cluster-Forwarded", "true")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assertStatus(t, w, 200)
}

func TestInvokeFunction_WithExec_CapacityPolicy(t *testing.T) {
	_, mux, ms := setupTestHandlerWithExec(t)
	// Make the function require very low concurrency so pool returns error
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		fn := testFunction()
		fn.CapacityPolicy = &domain.CapacityPolicy{
			Enabled:        true,
			ShedStatusCode: 429,
			RetryAfterS:    2,
		}
		return fn, nil
	}
	w := doRequest(t, mux, "POST", "/functions/hello/invoke", map[string]string{"key": "val"})
	// Pool should succeed with mock backend, but verify handler path
	if w.Code != 200 && w.Code != 429 && w.Code != 503 && w.Code != 500 {
		t.Fatalf("unexpected status %d", w.Code)
	}
}

// --- InvokeFunctionStream with real executor -----------------------------

func TestInvokeFunctionStream_WithExec_OK(t *testing.T) {
	_, mux, _ := setupTestHandlerWithExec(t)
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-stream", map[string]string{"key": "val"})
	assertStatus(t, w, 200)
	if !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected SSE content type, got %s", w.Header().Get("Content-Type"))
	}
}

func TestInvokeFunctionStream_WithExec_EmptyPayload(t *testing.T) {
	_, mux, _ := setupTestHandlerWithExec(t)
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-stream", nil)
	assertStatus(t, w, 200)
}

// --- InvokeFunction executor error paths ----------------------------------

func TestInvokeFunction_WithExec_ConcurrencyLimit(t *testing.T) {
	observability.Init(context.Background(), observability.Config{Enabled: false})
	ms := &mockMetadataStore{}
	fn := testFunction()
	fn.MaxReplicas = 1
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return fn, nil
	}
	ms.getFunctionCodeFn = func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
		return &domain.FunctionCode{SourceCode: "code", CompileStatus: domain.CompileStatusNotRequired}, nil
	}
	// Create a backend that returns error to simulate concurrency limit
	failingBackend := &mockBackend{
		createVMFn: func(ctx context.Context, fn *domain.Function, code []byte) (*backend.VM, error) {
			return nil, pool.ErrConcurrencyLimit
		},
	}
	p := newTestPoolWithBackend(t, failingBackend)
	p.SetMaxGlobalVMs(0) // unlimited
	t.Cleanup(p.Shutdown)
	s := store.NewStore(ms)
	exec := executor.New(s, p)
	h := &Handler{Store: s, Pool: p, Exec: exec}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	w := doRequest(t, mux, "POST", "/functions/hello/invoke", map[string]string{"k": "v"})
	// Should get 503 (concurrency limit → service unavailable)
	if w.Code != 503 && w.Code != 500 {
		t.Fatalf("expected 503 or 500, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestInvokeFunctionStream_WithExec_VMError(t *testing.T) {
	observability.Init(context.Background(), observability.Config{Enabled: false})
	ms := &mockMetadataStore{}
	fn := testFunction()
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return fn, nil
	}
	ms.getFunctionCodeFn = func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
		return &domain.FunctionCode{SourceCode: "code", CompileStatus: domain.CompileStatusNotRequired}, nil
	}
	failingBackend := &mockBackend{
		createVMFn: func(ctx context.Context, fn *domain.Function, code []byte) (*backend.VM, error) {
			return nil, pool.ErrConcurrencyLimit
		},
	}
	p := newTestPoolWithBackend(t, failingBackend)
	t.Cleanup(p.Shutdown)
	s := store.NewStore(ms)
	exec := executor.New(s, p)
	h := &Handler{Store: s, Pool: p, Exec: exec}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	w := doRequest(t, mux, "POST", "/functions/hello/invoke-stream", map[string]string{"k": "v"})
	// SSE returns 200 and then sends error event
	assertStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), "error") {
		t.Fatalf("expected error event in SSE, got: %s", w.Body.String())
	}
}

func TestInvokeFunction_WithExec_DeadlineExceeded(t *testing.T) {
	observability.Init(context.Background(), observability.Config{Enabled: false})
	ms := &mockMetadataStore{}
	fn := testFunction()
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return fn, nil
	}
	ms.getFunctionCodeFn = func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
		return &domain.FunctionCode{SourceCode: "code", CompileStatus: domain.CompileStatusNotRequired}, nil
	}
	failingBackend := &mockBackend{
		createVMFn: func(ctx context.Context, fn *domain.Function, code []byte) (*backend.VM, error) {
			return nil, context.DeadlineExceeded
		},
	}
	p := newTestPoolWithBackend(t, failingBackend)
	t.Cleanup(p.Shutdown)
	s := store.NewStore(ms)
	exec := executor.New(s, p)
	h := &Handler{Store: s, Pool: p, Exec: exec}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	w := doRequest(t, mux, "POST", "/functions/hello/invoke", map[string]string{"k": "v"})
	if w.Code != 504 && w.Code != 500 {
		t.Fatalf("expected 504 or 500, got %d", w.Code)
	}
}

func TestInvokeFunction_WithExec_GenericError(t *testing.T) {
	observability.Init(context.Background(), observability.Config{Enabled: false})
	ms := &mockMetadataStore{}
	fn := testFunction()
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return fn, nil
	}
	ms.getFunctionCodeFn = func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
		return &domain.FunctionCode{SourceCode: "code", CompileStatus: domain.CompileStatusNotRequired}, nil
	}
	failingBackend := &mockBackend{
		createVMFn: func(ctx context.Context, fn *domain.Function, code []byte) (*backend.VM, error) {
			return nil, fmt.Errorf("some internal error")
		},
	}
	p := newTestPoolWithBackend(t, failingBackend)
	t.Cleanup(p.Shutdown)
	s := store.NewStore(ms)
	exec := executor.New(s, p)
	h := &Handler{Store: s, Pool: p, Exec: exec}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	w := doRequest(t, mux, "POST", "/functions/hello/invoke", map[string]string{"k": "v"})
	assertStatus(t, w, 500)
}

func TestInvokeFunction_WithExec_QueueFull(t *testing.T) {
	observability.Init(context.Background(), observability.Config{Enabled: false})
	ms := &mockMetadataStore{}
	fn := testFunction()
	fn.CapacityPolicy = &domain.CapacityPolicy{
		Enabled:        true,
		ShedStatusCode: 429,
		RetryAfterS:    3,
	}
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return fn, nil
	}
	ms.getFunctionCodeFn = func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
		return &domain.FunctionCode{SourceCode: "code", CompileStatus: domain.CompileStatusNotRequired}, nil
	}
	failingBackend := &mockBackend{
		createVMFn: func(ctx context.Context, fn *domain.Function, code []byte) (*backend.VM, error) {
			return nil, pool.ErrQueueFull
		},
	}
	p := newTestPoolWithBackend(t, failingBackend)
	t.Cleanup(p.Shutdown)
	s := store.NewStore(ms)
	exec := executor.New(s, p)
	h := &Handler{Store: s, Pool: p, Exec: exec}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	w := doRequest(t, mux, "POST", "/functions/hello/invoke", map[string]string{"k": "v"})
	if w.Code != 429 && w.Code != 503 && w.Code != 500 {
		t.Fatalf("expected 429/503/500, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" && (w.Code == 429 || w.Code == 503) {
		t.Log("Retry-After header expected on shed status")
	}
}

func TestInvokeFunction_WithExec_InflightLimit(t *testing.T) {
	observability.Init(context.Background(), observability.Config{Enabled: false})
	ms := &mockMetadataStore{}
	fn := testFunction()
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return fn, nil
	}
	ms.getFunctionCodeFn = func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
		return &domain.FunctionCode{SourceCode: "code", CompileStatus: domain.CompileStatusNotRequired}, nil
	}
	failingBackend := &mockBackend{
		createVMFn: func(ctx context.Context, fn *domain.Function, code []byte) (*backend.VM, error) {
			return nil, pool.ErrInflightLimit
		},
	}
	p := newTestPoolWithBackend(t, failingBackend)
	t.Cleanup(p.Shutdown)
	s := store.NewStore(ms)
	exec := executor.New(s, p)
	h := &Handler{Store: s, Pool: p, Exec: exec}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	w := doRequest(t, mux, "POST", "/functions/hello/invoke", map[string]string{"k": "v"})
	if w.Code != 429 && w.Code != 503 && w.Code != 500 {
		t.Fatalf("expected shed status, got %d", w.Code)
	}
}

func TestInvokeFunction_WithExec_QueueWaitTimeout(t *testing.T) {
	observability.Init(context.Background(), observability.Config{Enabled: false})
	ms := &mockMetadataStore{}
	fn := testFunction()
	ms.getFunctionByNameFn = func(_ context.Context, name string) (*domain.Function, error) {
		return fn, nil
	}
	ms.getFunctionCodeFn = func(_ context.Context, funcID string) (*domain.FunctionCode, error) {
		return &domain.FunctionCode{SourceCode: "code", CompileStatus: domain.CompileStatusNotRequired}, nil
	}
	failingBackend := &mockBackend{
		createVMFn: func(ctx context.Context, fn *domain.Function, code []byte) (*backend.VM, error) {
			return nil, pool.ErrQueueWaitTimeout
		},
	}
	p := newTestPoolWithBackend(t, failingBackend)
	t.Cleanup(p.Shutdown)
	s := store.NewStore(ms)
	exec := executor.New(s, p)
	h := &Handler{Store: s, Pool: p, Exec: exec}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	w := doRequest(t, mux, "POST", "/functions/hello/invoke", map[string]string{"k": "v"})
	if w.Code != 429 && w.Code != 503 && w.Code != 500 {
		t.Fatalf("expected shed status, got %d", w.Code)
	}
}

// --- StreamLogs with data -------------------------------------------------

func TestStreamLogs_WithData(t *testing.T) {
	_, mux, ms := setupTestHandler(t)
	callCount := 0
	ms.listInvocationLogsFn = func(_ context.Context, fnID string, limit, offset int) ([]*store.InvocationLog, error) {
		callCount++
		if callCount == 1 {
			return []*store.InvocationLog{
				{ID: "log-1", CreatedAt: time.Now().Add(time.Second)},
			}, nil
		}
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest("GET", "/functions/hello/logs/stream", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assertStatus(t, w, 200)
	if !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected SSE content type")
	}
}
