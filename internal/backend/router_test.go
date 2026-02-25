package backend

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/oriys/nova/internal/domain"
)

type mockBackend struct {
	snapshotDir string
	createVMs   int
	stopVMs     []string
	newClients  int
}

func (m *mockBackend) CreateVM(ctx context.Context, fn *domain.Function, codeContent []byte) (*VM, error) {
	m.createVMs++
	vmID := "vm-default"
	if fn != nil && fn.Name != "" {
		vmID = "vm-" + fn.Name
	}
	return &VM{ID: vmID}, nil
}

func (m *mockBackend) CreateVMWithFiles(ctx context.Context, fn *domain.Function, files map[string][]byte) (*VM, error) {
	return m.CreateVM(ctx, fn, nil)
}

func (m *mockBackend) StopVM(vmID string) error {
	m.stopVMs = append(m.stopVMs, vmID)
	return nil
}

func (m *mockBackend) NewClient(vm *VM) (Client, error) {
	m.newClients++
	return &mockClient{}, nil
}

func (m *mockBackend) Shutdown() {}

func (m *mockBackend) SnapshotDir() string { return m.snapshotDir }

type mockClient struct{}

func (m *mockClient) Init(fn *domain.Function) error { return nil }

func (m *mockClient) Execute(reqID string, input json.RawMessage, timeoutS int) (*RespPayload, error) {
	return &RespPayload{}, nil
}

func (m *mockClient) ExecuteWithTrace(reqID string, input json.RawMessage, timeoutS int, traceParent, traceState string) (*RespPayload, error) {
	return &RespPayload{}, nil
}

func (m *mockClient) ExecuteStream(reqID string, input json.RawMessage, timeoutS int, traceParent, traceState string, callback func(chunk []byte, isLast bool, err error) error) error {
	return nil
}

func (m *mockClient) Reload(files map[string][]byte) error { return nil }

func (m *mockClient) Ping() error { return nil }

func (m *mockClient) Close() error { return nil }

func TestRouter_RoutesByFunctionBackend(t *testing.T) {
	fc := &mockBackend{snapshotDir: "/tmp/snapshots"}
	docker := &mockBackend{}

	router, err := NewRouter(domain.BackendFirecracker, map[domain.BackendType]BackendFactory{
		domain.BackendFirecracker: func() (Backend, error) { return fc, nil },
		domain.BackendDocker:      func() (Backend, error) { return docker, nil },
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	// Empty backend should route to default (firecracker).
	vmDefault, err := router.CreateVM(context.Background(), &domain.Function{Name: "default"}, nil)
	if err != nil {
		t.Fatalf("create vm default: %v", err)
	}
	if fc.createVMs != 1 {
		t.Fatalf("firecracker create calls = %d, want 1", fc.createVMs)
	}
	if docker.createVMs != 0 {
		t.Fatalf("docker create calls = %d, want 0", docker.createVMs)
	}

	// Explicit docker should route to docker backend.
	vmDocker, err := router.CreateVM(context.Background(), &domain.Function{Name: "docker", Backend: domain.BackendDocker}, nil)
	if err != nil {
		t.Fatalf("create vm docker: %v", err)
	}
	if docker.createVMs != 1 {
		t.Fatalf("docker create calls = %d, want 1", docker.createVMs)
	}

	if _, err := router.NewClient(vmDocker); err != nil {
		t.Fatalf("new client docker vm: %v", err)
	}
	if docker.newClients != 1 {
		t.Fatalf("docker new client calls = %d, want 1", docker.newClients)
	}

	if err := router.StopVM(vmDocker.ID); err != nil {
		t.Fatalf("stop docker vm: %v", err)
	}
	if len(docker.stopVMs) != 1 || docker.stopVMs[0] != vmDocker.ID {
		t.Fatalf("docker stop calls = %v, want [%s]", docker.stopVMs, vmDocker.ID)
	}

	if err := router.StopVM(vmDefault.ID); err != nil {
		t.Fatalf("stop default vm: %v", err)
	}
	if len(fc.stopVMs) != 1 || fc.stopVMs[0] != vmDefault.ID {
		t.Fatalf("firecracker stop calls = %v, want [%s]", fc.stopVMs, vmDefault.ID)
	}

	if got := router.SnapshotDir(); got != "/tmp/snapshots" {
		t.Fatalf("snapshot dir = %q, want %q", got, "/tmp/snapshots")
	}
}

func TestRouter_UnsupportedBackend(t *testing.T) {
	fc := &mockBackend{}
	router, err := NewRouter(domain.BackendFirecracker, map[domain.BackendType]BackendFactory{
		domain.BackendFirecracker: func() (Backend, error) { return fc, nil },
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	_, err = router.CreateVM(context.Background(), &domain.Function{
		Name:    "x",
		Backend: domain.BackendDocker,
	}, nil)
	if err == nil {
		t.Fatal("expected unsupported backend error")
	}
}

func TestRouter_EnsureReadyUsesFactory(t *testing.T) {
	calls := 0
	router, err := NewRouter(domain.BackendDocker, map[domain.BackendType]BackendFactory{
		domain.BackendDocker: func() (Backend, error) {
			calls++
			return &mockBackend{}, nil
		},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	if err := router.EnsureReady(domain.BackendAuto); err != nil {
		t.Fatalf("ensure ready: %v", err)
	}
	if err := router.EnsureReady(domain.BackendDocker); err != nil {
		t.Fatalf("ensure ready second call: %v", err)
	}
	if calls != 1 {
		t.Fatalf("factory calls = %d, want 1", calls)
	}
}

func TestRouter_EnsureReadyFactoryError(t *testing.T) {
	wantErr := errors.New("boom")
	router, err := NewRouter(domain.BackendDocker, map[domain.BackendType]BackendFactory{
		domain.BackendDocker: func() (Backend, error) { return nil, wantErr },
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	err = router.EnsureReady(domain.BackendDocker)
	if err == nil {
		t.Fatal("expected error")
	}
}
