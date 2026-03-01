package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/logsink"
	"github.com/oriys/nova/internal/observability"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
)

var gatewayInvokeObservabilityOnce sync.Once

type gatewayInvokeStore struct {
	store.MetadataStore

	routes   []*domain.GatewayRoute
	fnByName map[string]*domain.Function
	runtimes map[string]*store.RuntimeRecord
	codes    map[string]*domain.FunctionCode
}

func newGatewayInvokeStore(route *domain.GatewayRoute) *gatewayInvokeStore {
	return &gatewayInvokeStore{
		routes: []*domain.GatewayRoute{route},
		fnByName: map[string]*domain.Function{
			"hello": {
				ID:      "fn-1",
				Name:    "hello",
				Runtime: domain.RuntimePython,
			},
		},
		runtimes: map[string]*store.RuntimeRecord{
			"python": {
				ID:         "python",
				Entrypoint: []string{"python3"},
			},
		},
		codes: map[string]*domain.FunctionCode{
			"fn-1": {
				FunctionID: "fn-1",
				SourceCode: "print('hello')",
			},
		},
	}
}

func (m *gatewayInvokeStore) Close() error                 { return nil }
func (m *gatewayInvokeStore) Ping(_ context.Context) error { return nil }

func (m *gatewayInvokeStore) GetFunctionByName(_ context.Context, name string) (*domain.Function, error) {
	fn, ok := m.fnByName[name]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	cp := *fn
	return &cp, nil
}

func (m *gatewayInvokeStore) GetRuntime(_ context.Context, id string) (*store.RuntimeRecord, error) {
	rt, ok := m.runtimes[id]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	return rt, nil
}

func (m *gatewayInvokeStore) GetFunctionCode(_ context.Context, funcID string) (*domain.FunctionCode, error) {
	code, ok := m.codes[funcID]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	return code, nil
}

func (m *gatewayInvokeStore) HasFunctionFiles(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *gatewayInvokeStore) GetFunctionLayers(_ context.Context, _ string) ([]*domain.Layer, error) {
	return nil, nil
}

func (m *gatewayInvokeStore) GetFunctionVolumes(_ context.Context, _ string) ([]*domain.Volume, error) {
	return nil, nil
}

func (m *gatewayInvokeStore) ListGatewayRoutes(_ context.Context, _, _ int) ([]*domain.GatewayRoute, error) {
	routes := make([]*domain.GatewayRoute, len(m.routes))
	copy(routes, m.routes)
	return routes, nil
}

func (m *gatewayInvokeStore) GetRouteByDomainPath(_ context.Context, domainName, path string) (*domain.GatewayRoute, error) {
	for _, route := range m.routes {
		if route.Domain == domainName && route.Path == path {
			cp := *route
			return &cp, nil
		}
	}
	return nil, context.DeadlineExceeded
}

type gatewayInvokeBackend struct {
	client *gatewayInvokeClient
}

func (b *gatewayInvokeBackend) CreateVM(_ context.Context, fn *domain.Function, _ []byte) (*backend.VM, error) {
	return &backend.VM{ID: "vm-" + fn.ID, Runtime: fn.Runtime}, nil
}

func (b *gatewayInvokeBackend) CreateVMWithFiles(_ context.Context, fn *domain.Function, _ map[string][]byte) (*backend.VM, error) {
	return &backend.VM{ID: "vm-" + fn.ID, Runtime: fn.Runtime}, nil
}

func (b *gatewayInvokeBackend) StopVM(_ string) error { return nil }
func (b *gatewayInvokeBackend) Shutdown()             {}
func (b *gatewayInvokeBackend) SnapshotDir() string   { return "" }

func (b *gatewayInvokeBackend) NewClient(_ *backend.VM) (backend.Client, error) {
	return b.client, nil
}

type gatewayInvokeClient struct {
	resp        *backend.RespPayload
	lastReqID   string
	lastPayload json.RawMessage
}

func (c *gatewayInvokeClient) Init(_ *domain.Function) error { return nil }

func (c *gatewayInvokeClient) Execute(reqID string, input json.RawMessage, _ int) (*backend.RespPayload, error) {
	c.lastReqID = reqID
	c.lastPayload = append(c.lastPayload[:0], input...)
	return c.resp, nil
}

func (c *gatewayInvokeClient) ExecuteWithTrace(reqID string, input json.RawMessage, _ int, _, _ string) (*backend.RespPayload, error) {
	c.lastReqID = reqID
	c.lastPayload = append(c.lastPayload[:0], input...)
	return c.resp, nil
}

func (c *gatewayInvokeClient) ExecuteStream(_ string, _ json.RawMessage, _ int, _, _ string, callback func([]byte, bool, error) error) error {
	return callback(nil, true, nil)
}

func (c *gatewayInvokeClient) Reload(_ map[string][]byte) error { return nil }
func (c *gatewayInvokeClient) Ping() error                      { return nil }
func (c *gatewayInvokeClient) Close() error                     { return nil }

func initGatewayInvokeObservability(t *testing.T) {
	t.Helper()
	gatewayInvokeObservabilityOnce.Do(func() {
		if err := observability.Init(context.Background(), observability.Config{Enabled: false}); err != nil {
			t.Fatalf("init observability: %v", err)
		}
	})
}

func TestServeHTTP_FunctionRouteInvokesExecutor(t *testing.T) {
	initGatewayInvokeObservability(t)

	route := &domain.GatewayRoute{
		ID:           "route-1",
		Domain:       "gw.local",
		Path:         "/v1/hello",
		Methods:      []string{http.MethodPost},
		FunctionName: "hello",
		AuthStrategy: "none",
		Enabled:      true,
	}
	meta := newGatewayInvokeStore(route)
	client := &gatewayInvokeClient{
		resp: &backend.RespPayload{
			RequestID:  "upstream-1",
			Output:     json.RawMessage(`{"ok":true}`),
			DurationMs: 7,
		},
	}
	p := pool.NewPool(&gatewayInvokeBackend{client: client}, pool.PoolConfig{})
	exec := executor.New(
		store.NewStore(meta),
		p,
		executor.WithLogSink(logsink.NewNoopSink()),
	)
	t.Cleanup(func() {
		exec.Shutdown(time.Second)
		observability.Shutdown(context.Background())
	})

	gw := New(meta, exec, nil)
	if err := gw.ReloadRoutes(context.Background()); err != nil {
		t.Fatalf("reload routes: %v", err)
	}

	reqBody := `{"name":"nova"}`
	req := httptest.NewRequest(http.MethodPost, "http://gw.local/v1/hello", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if client.lastReqID == "" {
		t.Fatal("expected gateway invocation to pass a request id to executor client")
	}
	if got := string(client.lastPayload); got != reqBody {
		t.Fatalf("payload = %s, want %s", got, reqBody)
	}

	var resp domain.InvokeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if string(resp.Output) != `{"ok":true}` {
		t.Fatalf("output = %s, want {\"ok\":true}", resp.Output)
	}
}
