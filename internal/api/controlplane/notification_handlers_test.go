package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/oriys/nova/internal/store"
)

func TestListNotifications(t *testing.T) {
	ms := &mockMetadataStore{
		listNotificationsFn: func(_ context.Context, limit, offset int, status store.NotificationStatus) ([]*store.NotificationRecord, error) {
			return []*store.NotificationRecord{{ID: "n1"}}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/notifications", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestListNotifications_WithStatus(t *testing.T) {
	ms := &mockMetadataStore{
		listNotificationsFn: func(_ context.Context, limit, offset int, status store.NotificationStatus) ([]*store.NotificationRecord, error) {
			return []*store.NotificationRecord{}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/notifications?status=unread", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestListNotifications_Error(t *testing.T) {
	ms := &mockMetadataStore{
		listNotificationsFn: func(_ context.Context, limit, offset int, status store.NotificationStatus) ([]*store.NotificationRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/notifications", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestGetUnreadNotificationCount(t *testing.T) {
	ms := &mockMetadataStore{
		getUnreadNotificationCountFn: func(_ context.Context) (int64, error) { return 5, nil },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/notifications/unread-count", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestGetUnreadNotificationCount_Error(t *testing.T) {
	ms := &mockMetadataStore{
		getUnreadNotificationCountFn: func(_ context.Context) (int64, error) { return 0, fmt.Errorf("err") },
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/notifications/unread-count", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestMarkNotificationRead(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			markNotificationReadFn: func(_ context.Context, id string) (*store.NotificationRecord, error) {
				return &store.NotificationRecord{ID: id}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/notifications/n1/read", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("error", func(t *testing.T) {
		ms := &mockMetadataStore{
			markNotificationReadFn: func(_ context.Context, id string) (*store.NotificationRecord, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/notifications/nope/read", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == http.StatusOK {
			t.Fatalf("expected error status, got 200")
		}
	})
}

func TestMarkAllNotificationsRead(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			markAllNotificationsReadFn: func(_ context.Context) (int64, error) { return 3, nil },
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/notifications/read-all", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	})

	t.Run("error", func(t *testing.T) {
		ms := &mockMetadataStore{
			markAllNotificationsReadFn: func(_ context.Context) (int64, error) {
				return 0, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/notifications/read-all", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusInternalServerError)
	})
}

func TestMarkNotificationRead_PgxNoRows(t *testing.T) {
	ms := &mockMetadataStore{
		markNotificationReadFn: func(_ context.Context, id string) (*store.NotificationRecord, error) {
			return nil, pgx.ErrNoRows
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("POST", "/notifications/n1/read", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestListNotifications_WithStatusFilter(t *testing.T) {
	ms := &mockMetadataStore{
		listNotificationsFn: func(_ context.Context, limit, offset int, status store.NotificationStatus) ([]*store.NotificationRecord, error) {
			return []*store.NotificationRecord{}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)

	for _, status := range []string{"unread", "read", "all", ""} {
		req := httptest.NewRequest("GET", "/notifications?status="+status, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		expectStatus(t, w, http.StatusOK)
	}
}
