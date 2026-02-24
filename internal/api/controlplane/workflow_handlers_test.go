package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/workflow"
)

func setupWorkflowHandler(t *testing.T, ms *mockMetadataStore, ws *mockWorkflowStore) (*Handler, *http.ServeMux) {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	if ws == nil {
		ws = &mockWorkflowStore{}
	}
	s := newCompositeStore(ms, nil, ws)
	h := &Handler{Store: s, WorkflowService: workflow.NewService(s)}
	mux := http.NewServeMux()
	h.RegisterWorkflowRoutes(mux)
	return h, mux
}

func TestCreateWorkflow(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ws := &mockWorkflowStore{
			getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
				return nil, fmt.Errorf("not found")
			},
			createWorkflowFn: func(_ context.Context, w *domain.Workflow) error { return nil },
		}
		_, mux := setupWorkflowHandler(t, nil, ws)
		body := `{"name":"my-wf","description":"test"}`
		req := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusCreated)
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupWorkflowHandler(t, nil, nil)
		req := httptest.NewRequest("POST", "/workflows", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_name", func(t *testing.T) {
		_, mux := setupWorkflowHandler(t, nil, nil)
		body := `{"description":"test"}`
		req := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})
}

func TestListWorkflows(t *testing.T) {
	ws := &mockWorkflowStore{
		listWorkflowsFn: func(_ context.Context, limit, offset int) ([]*domain.Workflow, error) {
			return []*domain.Workflow{{ID: "wf-1", Name: "test"}}, nil
		},
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	req := httptest.NewRequest("GET", "/workflows", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestGetWorkflow(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ws := &mockWorkflowStore{
			getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Name: name}, nil
			},
		}
		_, mux := setupWorkflowHandler(t, nil, ws)
		req := httptest.NewRequest("GET", "/workflows/my-wf", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("not_found", func(t *testing.T) {
		ws := &mockWorkflowStore{
			getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupWorkflowHandler(t, nil, ws)
		req := httptest.NewRequest("GET", "/workflows/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})
}

func TestDeleteWorkflow(t *testing.T) {
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: "wf-1", Name: name}, nil
		},
		deleteWorkflowFn: func(_ context.Context, id string) error { return nil },
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	req := httptest.NewRequest("DELETE", "/workflows/my-wf", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestListWorkflowVersions(t *testing.T) {
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: "wf-1", Name: name}, nil
		},
		listWorkflowVersionsFn: func(_ context.Context, workflowID string, limit, offset int) ([]*domain.WorkflowVersion, error) {
			return []*domain.WorkflowVersion{}, nil
		},
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	req := httptest.NewRequest("GET", "/workflows/my-wf/versions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestGetWorkflowVersion(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ws := &mockWorkflowStore{
			getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: "wf-1", Name: name}, nil
			},
			getWorkflowVersionByNumberFn: func(_ context.Context, workflowID string, version int) (*domain.WorkflowVersion, error) {
				return &domain.WorkflowVersion{ID: "wv-1", Version: version}, nil
			},
			getWorkflowNodesFn: func(_ context.Context, versionID string) ([]domain.WorkflowNode, error) {
				return nil, nil
			},
			getWorkflowEdgesFn: func(_ context.Context, versionID string) ([]domain.WorkflowEdge, error) {
				return nil, nil
			},
		}
		_, mux := setupWorkflowHandler(t, nil, ws)
		req := httptest.NewRequest("GET", "/workflows/my-wf/versions/1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("invalid_version", func(t *testing.T) {
		_, mux := setupWorkflowHandler(t, nil, nil)
		req := httptest.NewRequest("GET", "/workflows/my-wf/versions/abc", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})
}

func TestTriggerWorkflowRun(t *testing.T) {
	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupWorkflowHandler(t, nil, nil)
		req := httptest.NewRequest("POST", "/workflows/my-wf/runs", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("workflow_not_found", func(t *testing.T) {
		ws := &mockWorkflowStore{
			getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupWorkflowHandler(t, nil, ws)
		body := `{"input":{}}`
		req := httptest.NewRequest("POST", "/workflows/nope/runs", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == http.StatusCreated {
			t.Fatal("expected error for missing workflow")
		}
	})
}

func TestListWorkflowRuns(t *testing.T) {
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: "wf-1", Name: name}, nil
		},
		listRunsFn: func(_ context.Context, workflowID string, limit, offset int) ([]*domain.WorkflowRun, error) {
			return []*domain.WorkflowRun{}, nil
		},
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	req := httptest.NewRequest("GET", "/workflows/my-wf/runs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestGetWorkflowRun(t *testing.T) {
	ws := &mockWorkflowStore{
		getRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, WorkflowName: "my-wf"}, nil
		},
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	req := httptest.NewRequest("GET", "/workflows/my-wf/runs/run-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestGetWorkflowRun_WrongWorkflow(t *testing.T) {
	ws := &mockWorkflowStore{
		getRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: id, WorkflowName: "other-wf"}, nil
		},
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	req := httptest.NewRequest("GET", "/workflows/my-wf/runs/run-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestInvokeWorkflowAsync(t *testing.T) {
	ms := &mockMetadataStore{
		enqueueAsyncInvocationFn: func(_ context.Context, inv *store.AsyncInvocation) error { return nil },
	}
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: "wf-1", Name: name}, nil
		},
	}
	_, mux := setupWorkflowHandler(t, ms, ws)
	body := `{"input":{"key":"value"}}`
	req := httptest.NewRequest("POST", "/workflows/my-wf/invoke-async", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusCreated)
}

func TestInvokeWorkflowAsync_BadJSON(t *testing.T) {
	_, mux := setupWorkflowHandler(t, nil, nil)
	req := httptest.NewRequest("POST", "/workflows/my-wf/invoke-async", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestInvokeWorkflowAsync_WorkflowNotFound(t *testing.T) {
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	body := `{"input":{}}`
	req := httptest.NewRequest("POST", "/workflows/nope/invoke-async", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestPublishWorkflowVersion_BadJSON(t *testing.T) {
	_, mux := setupWorkflowHandler(t, nil, nil)
	req := httptest.NewRequest("POST", "/workflows/my-wf/versions", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestPublishWorkflowVersion_ValidationError(t *testing.T) {
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: "wf-1", Name: name, CurrentVersion: 0}, nil
		},
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	// Empty definition should fail validation
	body := `{"nodes":[],"edges":[]}`
	req := httptest.NewRequest("POST", "/workflows/my-wf/versions", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Should be 400 or 500 depending on validation
	if w.Code == http.StatusCreated {
		t.Fatal("expected error for empty definition")
	}
}

func TestIsValidationError(t *testing.T) {
	if isValidationError(fmt.Errorf("some random error")) {
		t.Fatal("should not be validation error")
	}
	if !isValidationError(fmt.Errorf("invalid DAG: cycle detected")) {
		t.Fatal("should be validation error")
	}
	if isValidationError(fmt.Errorf("short")) {
		t.Fatal("should not be validation error")
	}
}

func TestListWorkflows_Error(t *testing.T) {
	ws := &mockWorkflowStore{
		listWorkflowsFn: func(_ context.Context, limit, offset int) ([]*domain.Workflow, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	req := httptest.NewRequest("GET", "/workflows", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestDeleteWorkflow_NotFound(t *testing.T) {
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	req := httptest.NewRequest("DELETE", "/workflows/nope", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 404 or 500, got %d", w.Code)
	}
}

func TestGetWorkflowRun_NotFound(t *testing.T) {
	ws := &mockWorkflowStore{
		getRunFn: func(_ context.Context, id string) (*domain.WorkflowRun, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	req := httptest.NewRequest("GET", "/workflows/my-wf/runs/nope", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestListWorkflowRuns_Error(t *testing.T) {
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(_ context.Context, name string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: "wf-1", Name: name}, nil
		},
		listRunsFn: func(_ context.Context, workflowID string, limit, offset int) ([]*domain.WorkflowRun, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	req := httptest.NewRequest("GET", "/workflows/my-wf/runs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}
