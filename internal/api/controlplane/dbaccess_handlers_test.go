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

func TestCreateDbResource(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			createDbResourceFn: func(_ context.Context, rec *store.DbResourceRecord) (*store.DbResourceRecord, error) {
				rec.ID = "dbr-1"
				return rec, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"mydb","type":"postgres","endpoint":"localhost:5432"}`
		req := httptest.NewRequest("POST", "/db-resources", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusCreated)
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/db-resources", strings.NewReader("{bad"))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_name", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"type":"postgres","endpoint":"localhost"}`
		req := httptest.NewRequest("POST", "/db-resources", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid_type", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"name":"mydb","type":"invalid","endpoint":"localhost"}`
		req := httptest.NewRequest("POST", "/db-resources", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})

	t.Run("missing_endpoint", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"name":"mydb","type":"postgres"}`
		req := httptest.NewRequest("POST", "/db-resources", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})
}

func TestListDbResources(t *testing.T) {
	ms := &mockMetadataStore{
		listDbResourcesFn: func(_ context.Context, limit, offset int) ([]*store.DbResourceRecord, error) {
			return []*store.DbResourceRecord{{ID: "dbr-1", Name: "mydb"}}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/db-resources", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestGetDbResource(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
				return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/db-resources/mydb", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/db-resources/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusNotFound)
	})
}

func TestUpdateDbResource(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		updateDbResourceFn: func(_ context.Context, id string, update *store.DbResourceUpdate) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: id}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"endpoint":"new-host:5432"}`
	req := httptest.NewRequest("PATCH", "/db-resources/mydb", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestDeleteDbResource(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		deleteDbResourceFn: func(_ context.Context, id string) error { return nil },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/db-resources/mydb", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNoContent)
}

func TestCreateDbBinding(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		createDbBindingFn: func(_ context.Context, rec *store.DbBindingRecord) (*store.DbBindingRecord, error) {
			rec.ID = "bind-1"
			return rec, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"function_id":"fn-1","permissions":["read"]}`
	req := httptest.NewRequest("POST", "/db-resources/mydb/bindings", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusCreated)
}

func TestCreateDbBinding_MissingFunctionID(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{}`
	req := httptest.NewRequest("POST", "/db-resources/mydb/bindings", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestListDbBindings(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		listDbBindingsFn: func(_ context.Context, resourceID string, limit, offset int) ([]*store.DbBindingRecord, error) {
			return []*store.DbBindingRecord{}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/db-resources/mydb/bindings", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestDeleteDbBinding(t *testing.T) {
	ms := &mockMetadataStore{
		deleteDbBindingFn: func(_ context.Context, id string) error { return nil },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/db-bindings/bind-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNoContent)
}

func TestSetCredentialPolicy(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		ms := &mockMetadataStore{
			getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
				return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
			},
			getCredentialPolicyFn: func(_ context.Context, resourceID string) (*store.CredentialPolicyRecord, error) {
				return nil, fmt.Errorf("not found")
			},
			createCredentialPolicyFn: func(_ context.Context, rec *store.CredentialPolicyRecord) (*store.CredentialPolicyRecord, error) {
				rec.ID = "cp-1"
				return rec, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"auth_mode":"static","static_username":"admin"}`
		req := httptest.NewRequest("PUT", "/db-resources/mydb/credential-policy", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusCreated)
	})

	t.Run("update", func(t *testing.T) {
		ms := &mockMetadataStore{
			getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
				return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
			},
			getCredentialPolicyFn: func(_ context.Context, resourceID string) (*store.CredentialPolicyRecord, error) {
				return &store.CredentialPolicyRecord{ID: "cp-1"}, nil
			},
			updateCredentialPolicyFn: func(_ context.Context, resourceID string, update *store.CredentialPolicyUpdate) (*store.CredentialPolicyRecord, error) {
				return &store.CredentialPolicyRecord{ID: "cp-1"}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"auth_mode":"iam"}`
		req := httptest.NewRequest("PUT", "/db-resources/mydb/credential-policy", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("invalid_auth_mode", func(t *testing.T) {
		ms := &mockMetadataStore{
			getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
				return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"auth_mode":"invalid"}`
		req := httptest.NewRequest("PUT", "/db-resources/mydb/credential-policy", strings.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusBadRequest)
	})
}

func TestGetCredentialPolicy(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		getCredentialPolicyFn: func(_ context.Context, resourceID string) (*store.CredentialPolicyRecord, error) {
			return &store.CredentialPolicyRecord{ID: "cp-1"}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/db-resources/mydb/credential-policy", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestDeleteCredentialPolicy(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		deleteCredentialPolicyFn: func(_ context.Context, resourceID string) error { return nil },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/db-resources/mydb/credential-policy", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNoContent)
}

func TestListDbRequestLogs(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		listDbRequestLogsFn: func(_ context.Context, resourceID string, limit, offset int) ([]*domain.DbRequestLog, error) {
			return []*domain.DbRequestLog{}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/db-resources/mydb/logs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestCreateDbBinding_InvalidPermission(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"function_id":"fn-1","permissions":["invalid"]}`
	req := httptest.NewRequest("POST", "/db-resources/mydb/bindings", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestUpdateDbResource_Success(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		updateDbResourceFn: func(_ context.Context, id string, update *store.DbResourceUpdate) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: id, Name: "mydb"}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"display_name":"Updated DB"}`
	req := httptest.NewRequest("PATCH", "/db-resources/mydb", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestUpdateDbResource_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		updateDbResourceFn: func(_ context.Context, id string, update *store.DbResourceUpdate) (*store.DbResourceRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"display_name":"Updated DB"}`
	req := httptest.NewRequest("PATCH", "/db-resources/mydb", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestDeleteDbResource_Success(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		deleteDbResourceFn: func(_ context.Context, id string) error { return nil },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/db-resources/mydb", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNoContent)
}

func TestDeleteDbResource_DeleteError(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		deleteDbResourceFn: func(_ context.Context, id string) error { return fmt.Errorf("db error") },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/db-resources/mydb", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestDeleteCredentialPolicy_Success(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		deleteCredentialPolicyFn: func(_ context.Context, resourceID string) error { return nil },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/db-resources/mydb/credential-policy", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusNoContent {
		t.Fatalf("expected 200 or 204, got %d", w.Code)
	}
}

func TestDeleteCredentialPolicy_DeleteError(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		deleteCredentialPolicyFn: func(_ context.Context, resourceID string) error { return fmt.Errorf("db error") },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/db-resources/mydb/credential-policy", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestUpdateDbResource_InvalidTenantMode(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"tenant_mode":"invalid"}`
	req := httptest.NewRequest("PATCH", "/db-resources/mydb", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestUpdateDbResource_BadJSON(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("PATCH", "/db-resources/mydb", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestDeleteDbResource_NotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/db-resources/nope", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestListDbResources_Error(t *testing.T) {
	ms := &mockMetadataStore{
		listDbResourcesFn: func(_ context.Context, limit, offset int) ([]*store.DbResourceRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/db-resources", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestCreateDbBinding_BadJSON(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("POST", "/db-resources/mydb/bindings", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestDeleteDbBinding_Error(t *testing.T) {
	ms := &mockMetadataStore{
		deleteDbBindingFn: func(_ context.Context, id string) error { return fmt.Errorf("err") },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/db-bindings/nope", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestGetCredentialPolicy_NotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		getCredentialPolicyFn: func(_ context.Context, resourceID string) (*store.CredentialPolicyRecord, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/db-resources/mydb/credential-policy", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestDeleteCredentialPolicy_NotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("DELETE", "/db-resources/nope/credential-policy", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestListDbRequestLogs_Error(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
		listDbRequestLogsFn: func(_ context.Context, resourceID string, limit, offset int) ([]*domain.DbRequestLog, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/db-resources/mydb/logs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestSetCredentialPolicy_BadJSON(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "dbr-1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("PUT", "/db-resources/mydb/credential-policy", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestListDbBindings_ResourceNotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(_ context.Context, name string) (*store.DbResourceRecord, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/db-resources/nope/bindings", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}
