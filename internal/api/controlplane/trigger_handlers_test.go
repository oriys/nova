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
)

func TestCreateTrigger(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
			createTriggerFn: func(_ context.Context, tr *store.TriggerRecord) error { return nil },
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"tr1","type":"http","function_name":"hello","enabled":true}`
		req := httptest.NewRequest("POST", "/triggers", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusCreated)
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/triggers", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_name", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"type":"http","function_name":"hello"}`
		req := httptest.NewRequest("POST", "/triggers", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_type", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"name":"tr1","function_name":"hello"}`
		req := httptest.NewRequest("POST", "/triggers", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_function_name", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"name":"tr1","type":"http"}`
		req := httptest.NewRequest("POST", "/triggers", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("function_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"tr1","type":"http","function_name":"nope"}`
		req := httptest.NewRequest("POST", "/triggers", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})
}

func TestListTriggers(t *testing.T) {
	ms := &mockMetadataStore{
		listTriggersFn: func(_ context.Context, limit, offset int) ([]*store.TriggerRecord, error) {
			return []*store.TriggerRecord{{ID: "t1", Name: "tr1"}}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/triggers", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestGetTrigger(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getTriggerFn: func(_ context.Context, id string) (*store.TriggerRecord, error) {
				return &store.TriggerRecord{ID: id, Name: "tr1"}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/triggers/t1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getTriggerFn: func(_ context.Context, id string) (*store.TriggerRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/triggers/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})
}

func TestUpdateTrigger(t *testing.T) {
	ms := &mockMetadataStore{
		updateTriggerFn: func(_ context.Context, id string, update *store.TriggerUpdate) (*store.TriggerRecord, error) {
			return &store.TriggerRecord{ID: id, Name: "updated"}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"enabled":false}`
	req := httptest.NewRequest("PATCH", "/triggers/t1", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestDeleteTrigger(t *testing.T) {
	ms := &mockMetadataStore{
		deleteTriggerFn: func(_ context.Context, id string) error { return nil },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/triggers/t1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNoContent)
}

func TestDeleteTrigger_Error(t *testing.T) {
	ms := &mockMetadataStore{
		deleteTriggerFn: func(_ context.Context, id string) error { return fmt.Errorf("err") },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/triggers/t1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestUpdateTrigger_BadJSON(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	req := httptest.NewRequest("PATCH", "/triggers/t1", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestUpdateTrigger_Error(t *testing.T) {
	ms := &mockMetadataStore{
		updateTriggerFn: func(_ context.Context, id string, update *store.TriggerUpdate) (*store.TriggerRecord, error) {
			return nil, fmt.Errorf("err")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"enabled":false}`
	req := httptest.NewRequest("PATCH", "/triggers/t1", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestListTriggers_Error(t *testing.T) {
	ms := &mockMetadataStore{
		listTriggersFn: func(_ context.Context, limit, offset int) ([]*store.TriggerRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/triggers", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestCreateTrigger_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		createTriggerFn: func(_ context.Context, tr *store.TriggerRecord) error { return fmt.Errorf("err") },
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"name":"tr1","type":"http","function_name":"hello","enabled":true}`
	req := httptest.NewRequest("POST", "/triggers", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}
