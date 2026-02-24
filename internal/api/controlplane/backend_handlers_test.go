package controlplane

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListBackends(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	req := httptest.NewRequest("GET", "/backends", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}
