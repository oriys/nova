package executor

import (
	"context"
	"encoding/json"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
)

// mockMetaStore embeds MetadataStore so unimplemented methods panic with a nil
// receiver (acceptable in tests). Override only the methods exercised by Invoke.
type mockMetaStore struct {
	store.MetadataStore

	fnByName    map[string]*domain.Function
	runtimes    map[string]*store.RuntimeRecord
	codes       map[string]*domain.FunctionCode
	multiFiles  map[string]bool
	files       map[string]map[string][]byte
	layers      map[string][]*domain.Layer
	volumes     map[string][]*domain.Volume
}

func newMockMetaStore() *mockMetaStore {
	return &mockMetaStore{
		fnByName:   make(map[string]*domain.Function),
		runtimes:   make(map[string]*store.RuntimeRecord),
		codes:      make(map[string]*domain.FunctionCode),
		multiFiles: make(map[string]bool),
		files:      make(map[string]map[string][]byte),
		layers:     make(map[string][]*domain.Layer),
		volumes:    make(map[string][]*domain.Volume),
	}
}

func (m *mockMetaStore) Close() error                     { return nil }
func (m *mockMetaStore) Ping(_ context.Context) error     { return nil }

func (m *mockMetaStore) GetFunctionByName(_ context.Context, name string) (*domain.Function, error) {
	fn, ok := m.fnByName[name]
	if !ok {
		return nil, context.DeadlineExceeded // stand-in for "not found"
	}
	// Return a copy to avoid mutation across calls
	cp := *fn
	return &cp, nil
}

func (m *mockMetaStore) GetRuntime(_ context.Context, id string) (*store.RuntimeRecord, error) {
	rt, ok := m.runtimes[id]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	return rt, nil
}

func (m *mockMetaStore) GetFunctionCode(_ context.Context, funcID string) (*domain.FunctionCode, error) {
	code, ok := m.codes[funcID]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	return code, nil
}

func (m *mockMetaStore) HasFunctionFiles(_ context.Context, funcID string) (bool, error) {
	return m.multiFiles[funcID], nil
}

func (m *mockMetaStore) GetFunctionFiles(_ context.Context, funcID string) (map[string][]byte, error) {
	f, ok := m.files[funcID]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	return f, nil
}

func (m *mockMetaStore) GetFunctionLayers(_ context.Context, funcID string) ([]*domain.Layer, error) {
	return m.layers[funcID], nil
}

func (m *mockMetaStore) GetFunctionVolumes(_ context.Context, funcID string) ([]*domain.Volume, error) {
	return m.volumes[funcID], nil
}

// mockBackend implements backend.Backend with in-memory VMs.
type mockBackend struct {
	client backend.Client
}

func (b *mockBackend) CreateVM(_ context.Context, fn *domain.Function, _ []byte) (*backend.VM, error) {
	return &backend.VM{ID: "mock-vm-" + fn.ID, Runtime: fn.Runtime}, nil
}

func (b *mockBackend) CreateVMWithFiles(_ context.Context, fn *domain.Function, _ map[string][]byte) (*backend.VM, error) {
	return &backend.VM{ID: "mock-vm-" + fn.ID, Runtime: fn.Runtime}, nil
}

func (b *mockBackend) StopVM(_ string) error    { return nil }
func (b *mockBackend) Shutdown()                  {}
func (b *mockBackend) SnapshotDir() string        { return "" }

func (b *mockBackend) NewClient(_ *backend.VM) (backend.Client, error) {
	return b.client, nil
}

// mockClient implements backend.Client.
type mockClient struct {
	execResp      *backend.RespPayload
	execErr       error
	streamChunks  [][]byte
	streamErr     error
}

func (c *mockClient) Init(_ *domain.Function) error { return nil }

func (c *mockClient) Execute(_ string, _ json.RawMessage, _ int) (*backend.RespPayload, error) {
	return c.execResp, c.execErr
}

func (c *mockClient) ExecuteWithTrace(_ string, _ json.RawMessage, _ int, _, _ string) (*backend.RespPayload, error) {
	return c.execResp, c.execErr
}

func (c *mockClient) ExecuteStream(_ string, _ json.RawMessage, _ int, _, _ string, callback func(chunk []byte, isLast bool, err error) error) error {
	if c.streamErr != nil {
		return c.streamErr
	}
	for i, chunk := range c.streamChunks {
		isLast := i == len(c.streamChunks)-1
		if err := callback(chunk, isLast, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *mockClient) Reload(_ map[string][]byte) error { return nil }
func (c *mockClient) Ping() error                       { return nil }
func (c *mockClient) Close() error                       { return nil }

// newTestPool creates a pool.Pool backed by a mock backend.
func newTestPool(client backend.Client) *pool.Pool {
	b := &mockBackend{client: client}
	return pool.NewPool(b, pool.PoolConfig{})
}

// mockSecretsBackend implements secrets.Backend for testing.
type mockSecretsBackend struct {
	data map[string]string
}

func (m *mockSecretsBackend) SaveSecret(_ context.Context, name, value string) error {
	m.data[name] = value
	return nil
}

func (m *mockSecretsBackend) GetSecret(_ context.Context, name string) (string, error) {
	v, ok := m.data[name]
	if !ok {
		return "", context.DeadlineExceeded
	}
	return v, nil
}

func (m *mockSecretsBackend) DeleteSecret(_ context.Context, _ string) error { return nil }

func (m *mockSecretsBackend) ListSecrets(_ context.Context) (map[string]string, error) {
	return m.data, nil
}

func (m *mockSecretsBackend) SecretExists(_ context.Context, name string) (bool, error) {
	_, ok := m.data[name]
	return ok, nil
}
