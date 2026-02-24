package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/store"
)

func TestRegisterClusterNode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			upsertClusterNodeFn: func(_ context.Context, node *store.ClusterNodeRecord) error { return nil },
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"id":"node-1","name":"worker-1","address":"10.0.0.1:9090"}`
		req := httptest.NewRequest("POST", "/cluster/nodes", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusCreated)
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/cluster/nodes", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_id", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"name":"worker-1"}`
		req := httptest.NewRequest("POST", "/cluster/nodes", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("store_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			upsertClusterNodeFn: func(_ context.Context, node *store.ClusterNodeRecord) error {
				return fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"id":"node-1"}`
		req := httptest.NewRequest("POST", "/cluster/nodes", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusInternalServerError)
	})
}

func TestHeartbeatClusterNode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			updateClusterNodeHeartbeatFn: func(_ context.Context, id string, activeVMs, queueDepth int) error { return nil },
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"active_vms":5,"queue_depth":3}`
		req := httptest.NewRequest("POST", "/cluster/nodes/node-1/heartbeat", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNoContent)
	})

	t.Run("error", func(t *testing.T) {
		ms := &mockMetadataStore{
			updateClusterNodeHeartbeatFn: func(_ context.Context, id string, activeVMs, queueDepth int) error {
				return fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"active_vms":0}`
		req := httptest.NewRequest("POST", "/cluster/nodes/nope/heartbeat", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})
}

func TestListClusterNodes(t *testing.T) {
	ms := &mockMetadataStore{
		listClusterNodesFn: func(_ context.Context, limit, offset int) ([]*store.ClusterNodeRecord, error) {
			return []*store.ClusterNodeRecord{{ID: "node-1"}}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/cluster/nodes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestListHealthyClusterNodes(t *testing.T) {
	ms := &mockMetadataStore{
		listActiveClusterNodesFn: func(_ context.Context) ([]*store.ClusterNodeRecord, error) {
			return []*store.ClusterNodeRecord{}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/cluster/nodes/healthy", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestGetClusterNode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getClusterNodeFn: func(_ context.Context, id string) (*store.ClusterNodeRecord, error) {
				return &store.ClusterNodeRecord{ID: id}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/cluster/nodes/node-1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getClusterNodeFn: func(_ context.Context, id string) (*store.ClusterNodeRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/cluster/nodes/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})
}

func TestDeleteClusterNode(t *testing.T) {
	ms := &mockMetadataStore{
		deleteClusterNodeFn: func(_ context.Context, id string) error { return nil },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/cluster/nodes/node-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNoContent)
}

func TestDeleteClusterNode_Error(t *testing.T) {
	ms := &mockMetadataStore{
		deleteClusterNodeFn: func(_ context.Context, id string) error { return fmt.Errorf("err") },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/cluster/nodes/node-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestListClusterNodes_Error(t *testing.T) {
	ms := &mockMetadataStore{
		listClusterNodesFn: func(_ context.Context, limit, offset int) ([]*store.ClusterNodeRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/cluster/nodes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestListHealthyClusterNodes_Error(t *testing.T) {
	ms := &mockMetadataStore{
		listActiveClusterNodesFn: func(_ context.Context) ([]*store.ClusterNodeRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/cluster/nodes/healthy", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestHeartbeatClusterNode_BadJSON(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	req := httptest.NewRequest("POST", "/cluster/nodes/node-1/heartbeat", strings.NewReader("{bad"))
	req.Header.Set("Content-Length", "4")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestHeartbeatClusterNode_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		updateClusterNodeHeartbeatFn: func(_ context.Context, id string, activeVMs, queueDepth int) error {
			return fmt.Errorf("node not found")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"active_vms":5,"queue_depth":2}`
	req := httptest.NewRequest("POST", "/cluster/nodes/node-1/heartbeat", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestHeartbeatClusterNode_Success(t *testing.T) {
	ms := &mockMetadataStore{
		updateClusterNodeHeartbeatFn: func(_ context.Context, id string, activeVMs, queueDepth int) error {
			return nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"active_vms":5,"queue_depth":2}`
	req := httptest.NewRequest("POST", "/cluster/nodes/node-1/heartbeat", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNoContent)
}

func TestHeartbeatClusterNode_EmptyBody(t *testing.T) {
	ms := &mockMetadataStore{
		updateClusterNodeHeartbeatFn: func(_ context.Context, id string, activeVMs, queueDepth int) error {
			return nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("POST", "/cluster/nodes/node-1/heartbeat", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNoContent)
}
