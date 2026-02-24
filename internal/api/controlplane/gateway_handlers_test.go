package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

func setupGatewayTestHandler(t *testing.T, ms *mockMetadataStore) (*Handler, *http.ServeMux) {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	s := store.NewStore(ms)
	h := &Handler{Store: s, GatewayEnabled: true}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return h, mux
}

func TestGateway_CreateRoute(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
			saveGatewayRouteFn: func(_ context.Context, route *domain.GatewayRoute) error {
				return nil
			},
			getConfigFn: func(_ context.Context) (map[string]string, error) {
				return map[string]string{}, nil
			},
		}
		_, mux := setupGatewayTestHandler(t, ms)
		body := `{"path":"/api/hello","function_name":"hello"}`
		req := httptest.NewRequest("POST", "/gateway/routes", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupGatewayTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/gateway/routes", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_path", func(t *testing.T) {
		_, mux := setupGatewayTestHandler(t, nil)
		body := `{"function_name":"hello"}`
		req := httptest.NewRequest("POST", "/gateway/routes", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_function_name", func(t *testing.T) {
		_, mux := setupGatewayTestHandler(t, nil)
		body := `{"path":"/api/hello"}`
		req := httptest.NewRequest("POST", "/gateway/routes", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("function_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupGatewayTestHandler(t, ms)
		body := `{"path":"/api/hello","function_name":"nope"}`
		req := httptest.NewRequest("POST", "/gateway/routes", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("store_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
			saveGatewayRouteFn: func(_ context.Context, route *domain.GatewayRoute) error {
				return fmt.Errorf("db error")
			},
			getConfigFn: func(_ context.Context) (map[string]string, error) {
				return map[string]string{}, nil
			},
		}
		_, mux := setupGatewayTestHandler(t, ms)
		body := `{"path":"/api/hello","function_name":"hello"}`
		req := httptest.NewRequest("POST", "/gateway/routes", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestGateway_ListRoutes(t *testing.T) {
	ms := &mockMetadataStore{
		listGatewayRoutesFn: func(_ context.Context, limit, offset int) ([]*domain.GatewayRoute, error) {
			return []*domain.GatewayRoute{{ID: "r1", Path: "/api/hello"}}, nil
		},
	}
	_, mux := setupGatewayTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/gateway/routes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGateway_ListRoutes_ByDomain(t *testing.T) {
	ms := &mockMetadataStore{
		listRoutesByDomainFn: func(_ context.Context, d string, limit, offset int) ([]*domain.GatewayRoute, error) {
			return []*domain.GatewayRoute{{ID: "r1", Domain: d}}, nil
		},
	}
	_, mux := setupGatewayTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/gateway/routes?domain=example.com", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGateway_GetRoute(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getGatewayRouteFn: func(_ context.Context, id string) (*domain.GatewayRoute, error) {
				return &domain.GatewayRoute{ID: id, Path: "/test"}, nil
			},
		}
		_, mux := setupGatewayTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/gateway/routes/r1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getGatewayRouteFn: func(_ context.Context, id string) (*domain.GatewayRoute, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupGatewayTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/gateway/routes/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestGateway_UpdateRoute(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getGatewayRouteFn: func(_ context.Context, id string) (*domain.GatewayRoute, error) {
				return &domain.GatewayRoute{ID: id, Path: "/old"}, nil
			},
			updateGatewayRouteFn: func(_ context.Context, id string, route *domain.GatewayRoute) error {
				return nil
			},
		}
		_, mux := setupGatewayTestHandler(t, ms)
		body := `{"path":"/new"}`
		req := httptest.NewRequest("PATCH", "/gateway/routes/r1", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getGatewayRouteFn: func(_ context.Context, id string) (*domain.GatewayRoute, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupGatewayTestHandler(t, ms)
		req := httptest.NewRequest("PATCH", "/gateway/routes/nope", strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		ms := &mockMetadataStore{
			getGatewayRouteFn: func(_ context.Context, id string) (*domain.GatewayRoute, error) {
				return &domain.GatewayRoute{ID: id}, nil
			},
		}
		_, mux := setupGatewayTestHandler(t, ms)
		req := httptest.NewRequest("PATCH", "/gateway/routes/r1", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestGateway_DeleteRoute(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteGatewayRouteFn: func(_ context.Context, id string) error { return nil },
		}
		_, mux := setupGatewayTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/gateway/routes/r1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("error", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteGatewayRouteFn: func(_ context.Context, id string) error { return fmt.Errorf("not found") },
		}
		_, mux := setupGatewayTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/gateway/routes/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestGateway_RateLimitTemplate(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		ms := &mockMetadataStore{
			getConfigFn: func(_ context.Context) (map[string]string, error) {
				return map[string]string{
					"gateway.default_rate_limit_enabled": "true",
					"gateway.default_rate_limit_rps":     "100",
					"gateway.default_rate_limit_burst":   "200",
				}, nil
			},
		}
		_, mux := setupGatewayTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/gateway/rate-limit-template", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp gatewayRateLimitTemplate
		json.NewDecoder(w.Body).Decode(&resp)
		if !resp.Enabled || resp.RequestsPerSecond != 100 || resp.BurstSize != 200 {
			t.Fatalf("unexpected: %+v", resp)
		}
	})

	t.Run("update", func(t *testing.T) {
		setKeys := map[string]string{}
		ms := &mockMetadataStore{
			getConfigFn: func(_ context.Context) (map[string]string, error) {
				return map[string]string{}, nil
			},
			setConfigFn: func(_ context.Context, key, value string) error {
				setKeys[key] = value
				return nil
			},
		}
		_, mux := setupGatewayTestHandler(t, ms)
		body := `{"enabled":true,"requests_per_second":50,"burst_size":100}`
		req := httptest.NewRequest("PUT", "/gateway/rate-limit-template", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("update_bad_json", func(t *testing.T) {
		_, mux := setupGatewayTestHandler(t, nil)
		req := httptest.NewRequest("PUT", "/gateway/rate-limit-template", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("update_negative_rps", func(t *testing.T) {
		ms := &mockMetadataStore{
			getConfigFn: func(_ context.Context) (map[string]string, error) {
				return map[string]string{}, nil
			},
		}
		_, mux := setupGatewayTestHandler(t, ms)
		body := `{"requests_per_second":-1}`
		req := httptest.NewRequest("PUT", "/gateway/rate-limit-template", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestGateway_Disabled(t *testing.T) {
	ms := &mockMetadataStore{}
	s := store.NewStore(ms)
	h := &Handler{Store: s, GatewayEnabled: false}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/gateway/routes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// When gateway is disabled, routes are not registered, so we get 405 or 404
	if w.Code == http.StatusOK {
		t.Fatalf("expected non-200 when gateway disabled, got %d", w.Code)
	}
}

func TestGateway_UpdateRoute_AllFields(t *testing.T) {
	ms := &mockMetadataStore{
		getGatewayRouteFn: func(_ context.Context, id string) (*domain.GatewayRoute, error) {
			return &domain.GatewayRoute{ID: id, Path: "/old", FunctionName: "hello"}, nil
		},
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		updateGatewayRouteFn: func(_ context.Context, id string, route *domain.GatewayRoute) error { return nil },
	}
	h, mux := setupGatewayTestHandler(t, ms)
	_ = h
	body := `{"domain":"example.com","path":"/new","methods":["GET","POST"],"function_name":"world","auth_strategy":"jwt","auth_config":{"key":"val"},"request_schema":{"type":"object"},"rate_limit":{"requests_per_second":100,"burst_size":200},"enabled":false}`
	req := httptest.NewRequest("PATCH", "/gateway/routes/r1", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestGateway_UpdateRoute_FunctionNotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getGatewayRouteFn: func(_ context.Context, id string) (*domain.GatewayRoute, error) {
			return &domain.GatewayRoute{ID: id, Path: "/old"}, nil
		},
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	h, mux := setupGatewayTestHandler(t, ms)
	_ = h
	body := `{"function_name":"nope"}`
	req := httptest.NewRequest("PATCH", "/gateway/routes/r1", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestGateway_UpdateRoute_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		getGatewayRouteFn: func(_ context.Context, id string) (*domain.GatewayRoute, error) {
			return &domain.GatewayRoute{ID: id}, nil
		},
		updateGatewayRouteFn: func(_ context.Context, id string, route *domain.GatewayRoute) error {
			return fmt.Errorf("db error")
		},
	}
	h, mux := setupGatewayTestHandler(t, ms)
	_ = h
	body := `{"path":"/new"}`
	req := httptest.NewRequest("PATCH", "/gateway/routes/r1", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestGateway_GetRateLimitTemplate_Error(t *testing.T) {
	ms := &mockMetadataStore{
		getConfigFn: func(_ context.Context) (map[string]string, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h, mux := setupGatewayTestHandler(t, ms)
	_ = h
	req := httptest.NewRequest("GET", "/gateway/rate-limit-template", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestGateway_UpdateRateLimitTemplate_SetConfigError(t *testing.T) {
	callCount := 0
	ms := &mockMetadataStore{
		getConfigFn: func(_ context.Context) (map[string]string, error) {
			return map[string]string{}, nil
		},
		setConfigFn: func(_ context.Context, key, value string) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("db error on second setconfig")
			}
			return nil
		},
	}
	h, mux := setupGatewayTestHandler(t, ms)
	_ = h
	body := `{"enabled":true,"requests_per_second":100,"burst_size":200}`
	req := httptest.NewRequest("PUT", "/gateway/rate-limit-template", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestGateway_UpdateRateLimitTemplate_LoadError(t *testing.T) {
	ms := &mockMetadataStore{
		getConfigFn: func(_ context.Context) (map[string]string, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h, mux := setupGatewayTestHandler(t, ms)
	_ = h
	body := `{"enabled":true}`
	req := httptest.NewRequest("PUT", "/gateway/rate-limit-template", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestGateway_CreateRoute_WithExplicitEnabled(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		saveGatewayRouteFn: func(_ context.Context, route *domain.GatewayRoute) error { return nil },
		getConfigFn: func(_ context.Context) (map[string]string, error) {
			return map[string]string{}, nil
		},
	}
	h, mux := setupGatewayTestHandler(t, ms)
	_ = h
	body := `{"path":"/api/hello","function_name":"hello","enabled":false,"auth_strategy":"jwt","rate_limit":{"requests_per_second":50,"burst_size":100}}`
	req := httptest.NewRequest("POST", "/gateway/routes", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusCreated)
}

func TestGateway_ListRoutes_NilResult(t *testing.T) {
	ms := &mockMetadataStore{
		listGatewayRoutesFn: func(_ context.Context, limit, offset int) ([]*domain.GatewayRoute, error) {
			return nil, nil
		},
	}
	h, mux := setupGatewayTestHandler(t, ms)
	_ = h
	req := httptest.NewRequest("GET", "/gateway/routes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}
