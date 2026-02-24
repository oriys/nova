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

func setupScheduleHandler(t *testing.T, ms *mockMetadataStore, ss *mockScheduleStore) *http.ServeMux {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	if ss == nil {
		ss = &mockScheduleStore{}
	}
	s := newCompositeStore(ms, ss, nil)
	h := &ScheduleHandler{Store: s, Scheduler: nil}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	// Also register function routes so GetFunctionByName works through the same mux
	// We need to verify the function exists in CreateSchedule; register a dummy handler.
	return mux
}

func setupScheduleHandlerFull(t *testing.T, ms *mockMetadataStore, ss *mockScheduleStore) *http.ServeMux {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	if ss == nil {
		ss = &mockScheduleStore{}
	}
	s := newCompositeStore(ms, ss, nil)
	sh := &ScheduleHandler{Store: s, Scheduler: nil}
	mux := http.NewServeMux()
	sh.RegisterRoutes(mux)
	return mux
}

func TestCreateSchedule(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
		}
		ss := &mockScheduleStore{
			saveScheduleFn: func(_ context.Context, s *store.Schedule) error { return nil },
		}
		mux := setupScheduleHandlerFull(t, ms, ss)
		body := `{"cron_expression":"@every 5m"}`
		req := httptest.NewRequest("POST", "/functions/hello/schedules", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusCreated)
	})

	t.Run("function_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		mux := setupScheduleHandlerFull(t, ms, nil)
		body := `{"cron_expression":"@every 5m"}`
		req := httptest.NewRequest("POST", "/functions/nope/schedules", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})

	t.Run("bad_json", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
		}
		mux := setupScheduleHandlerFull(t, ms, nil)
		req := httptest.NewRequest("POST", "/functions/hello/schedules", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_cron", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
		}
		mux := setupScheduleHandlerFull(t, ms, nil)
		body := `{}`
		req := httptest.NewRequest("POST", "/functions/hello/schedules", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})
}

func TestListSchedules(t *testing.T) {
	ss := &mockScheduleStore{
		listSchedulesByFunctionFn: func(_ context.Context, fnName string, limit, offset int) ([]*store.Schedule, error) {
			return []*store.Schedule{{ID: "s1", FunctionName: fnName}}, nil
		},
	}
	mux := setupScheduleHandlerFull(t, nil, ss)
	req := httptest.NewRequest("GET", "/functions/hello/schedules", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestDeleteSchedule(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ss := &mockScheduleStore{
			getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
				return &store.Schedule{ID: id, FunctionName: "hello"}, nil
			},
			deleteScheduleFn: func(_ context.Context, id string) error { return nil },
		}
		mux := setupScheduleHandlerFull(t, nil, ss)
		req := httptest.NewRequest("DELETE", "/functions/hello/schedules/s1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("not_found", func(t *testing.T) {
		ss := &mockScheduleStore{
			getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		mux := setupScheduleHandlerFull(t, nil, ss)
		req := httptest.NewRequest("DELETE", "/functions/hello/schedules/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})

	t.Run("wrong_function", func(t *testing.T) {
		ss := &mockScheduleStore{
			getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
				return &store.Schedule{ID: id, FunctionName: "other"}, nil
			},
		}
		mux := setupScheduleHandlerFull(t, nil, ss)
		req := httptest.NewRequest("DELETE", "/functions/hello/schedules/s1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})

	t.Run("delete_store_error", func(t *testing.T) {
		ss := &mockScheduleStore{
			getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
				return &store.Schedule{ID: id, FunctionName: "hello"}, nil
			},
			deleteScheduleFn: func(_ context.Context, id string) error { return fmt.Errorf("db error") },
		}
		mux := setupScheduleHandlerFull(t, nil, ss)
		req := httptest.NewRequest("DELETE", "/functions/hello/schedules/s1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusInternalServerError)
	})
}

func TestListSchedules_Error(t *testing.T) {
	ss := &mockScheduleStore{
		listSchedulesByFunctionFn: func(_ context.Context, fnName string, limit, offset int) ([]*store.Schedule, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	mux := setupScheduleHandlerFull(t, nil, ss)
	req := httptest.NewRequest("GET", "/functions/hello/schedules", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestCreateSchedule_FnNotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	mux := setupScheduleHandlerFull(t, ms, nil)
	body := `{"cron_expression":"@every 10m"}`
	req := httptest.NewRequest("POST", "/functions/nope/schedules", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestCreateSchedule_BadJSON(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	mux := setupScheduleHandlerFull(t, ms, nil)
	req := httptest.NewRequest("POST", "/functions/hello/schedules", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestCreateSchedule_MissingCron(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	mux := setupScheduleHandlerFull(t, ms, nil)
	body := `{"input":{}}`
	req := httptest.NewRequest("POST", "/functions/hello/schedules", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestToggleSchedule(t *testing.T) {
	t.Run("enable", func(t *testing.T) {
		ss := &mockScheduleStore{
			getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
				return &store.Schedule{ID: id, FunctionName: "hello", Enabled: false}, nil
			},
			updateScheduleEnabledFn: func(_ context.Context, id string, enabled bool) error { return nil },
		}
		mux := setupScheduleHandlerFull(t, nil, ss)
		body := `{"enabled":true}`
		req := httptest.NewRequest("PATCH", "/functions/hello/schedules/s1", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("update_cron", func(t *testing.T) {
		ss := &mockScheduleStore{
			getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
				return &store.Schedule{ID: id, FunctionName: "hello", Enabled: true}, nil
			},
			updateScheduleCronFn: func(_ context.Context, id, cronExpr string) error { return nil },
		}
		mux := setupScheduleHandlerFull(t, nil, ss)
		body := `{"cron_expression":"@every 10m"}`
		req := httptest.NewRequest("PATCH", "/functions/hello/schedules/s1", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("bad_json", func(t *testing.T) {
		ss := &mockScheduleStore{
			getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
				return &store.Schedule{ID: id, FunctionName: "hello"}, nil
			},
		}
		mux := setupScheduleHandlerFull(t, nil, ss)
		req := httptest.NewRequest("PATCH", "/functions/hello/schedules/s1", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("not_found", func(t *testing.T) {
		ss := &mockScheduleStore{
			getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		mux := setupScheduleHandlerFull(t, nil, ss)
		body := `{"enabled":true}`
		req := httptest.NewRequest("PATCH", "/functions/hello/schedules/s1", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})

	t.Run("wrong_function", func(t *testing.T) {
		ss := &mockScheduleStore{
			getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
				return &store.Schedule{ID: id, FunctionName: "other"}, nil
			},
		}
		mux := setupScheduleHandlerFull(t, nil, ss)
		body := `{"enabled":true}`
		req := httptest.NewRequest("PATCH", "/functions/hello/schedules/s1", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})

	t.Run("enable_error", func(t *testing.T) {
		ss := &mockScheduleStore{
			getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
				return &store.Schedule{ID: id, FunctionName: "hello"}, nil
			},
			updateScheduleEnabledFn: func(_ context.Context, id string, enabled bool) error {
				return fmt.Errorf("db error")
			},
		}
		mux := setupScheduleHandlerFull(t, nil, ss)
		body := `{"enabled":true}`
		req := httptest.NewRequest("PATCH", "/functions/hello/schedules/s1", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusInternalServerError)
	})

	t.Run("cron_error", func(t *testing.T) {
		ss := &mockScheduleStore{
			getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
				return &store.Schedule{ID: id, FunctionName: "hello"}, nil
			},
			updateScheduleCronFn: func(_ context.Context, id, cronExpr string) error {
				return fmt.Errorf("db error")
			},
		}
		mux := setupScheduleHandlerFull(t, nil, ss)
		body := `{"cron_expression":"@every 10m"}`
		req := httptest.NewRequest("PATCH", "/functions/hello/schedules/s1", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusInternalServerError)
	})
}
