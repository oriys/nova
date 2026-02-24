package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

func TestCreateEventTopic(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"orders","description":"Order events"}`
		req := httptest.NewRequest("POST", "/topics", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["name"] != "orders" {
			t.Fatalf("unexpected name: %v", resp["name"])
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/topics", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		ms := &mockMetadataStore{
			createEventTopicFn: func(ctx context.Context, topic *store.EventTopic) error {
				return fmt.Errorf("topic already exists")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"orders","description":"dup"}`
		req := httptest.NewRequest("POST", "/topics", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})

	t.Run("with_retention", func(t *testing.T) {
		var saved *store.EventTopic
		ms := &mockMetadataStore{
			createEventTopicFn: func(ctx context.Context, topic *store.EventTopic) error {
				saved = topic
				return nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"events","description":"test","retention_hours":48}`
		req := httptest.NewRequest("POST", "/topics", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		if saved == nil || saved.RetentionHours != 48 {
			t.Fatalf("expected retention 48, got %v", saved)
		}
	})
}

func TestListEventTopics(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("with_data", func(t *testing.T) {
		ms := &mockMetadataStore{
			listEventTopicsFn: func(ctx context.Context, limit, offset int) ([]*store.EventTopic, error) {
				return []*store.EventTopic{{Name: "orders"}, {Name: "payments"}}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("store_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			listEventTopicsFn: func(ctx context.Context, limit, offset int) ([]*store.EventTopic, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestGetEventTopic(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics/orders", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return nil, store.ErrEventTopicNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics/nonexistent", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestDeleteEventTopic(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/topics/orders", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
		var resp map[string]string
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["status"] != "deleted" {
			t.Fatalf("unexpected status: %v", resp)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteEventTopicByNameFn: func(ctx context.Context, name string) error {
				return store.ErrEventTopicNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/topics/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("generic_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteEventTopicByNameFn: func(ctx context.Context, name string) error {
				return fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/topics/orders", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestPublishEvent(t *testing.T) {
	topicStub := func() *mockMetadataStore {
		return &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
			publishEventFn: func(ctx context.Context, topicID, orderingKey string, payload, headers json.RawMessage) (*store.EventMessage, int, error) {
				return &store.EventMessage{ID: "msg-1", TopicID: topicID}, 1, nil
			},
		}
	}

	t.Run("success", func(t *testing.T) {
		ms := topicStub()
		_, mux := setupTestHandler(t, ms)
		body := `{"payload":{"key":"value"}}`
		req := httptest.NewRequest("POST", "/topics/orders/publish", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("topic_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return nil, store.ErrEventTopicNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/topics/nope/publish", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("store_error_invalid_ordering", func(t *testing.T) {
		ms := &mockMetadataStore{
			publishEventFn: func(ctx context.Context, topicID, orderingKey string, payload, headers json.RawMessage) (*store.EventMessage, int, error) {
				return nil, 0, store.ErrInvalidOrderingKey
			},
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"payload":{"key":"value"},"ordering_key":"bad"}`
		req := httptest.NewRequest("POST", "/topics/orders/publish", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/topics/orders/publish", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("generic_publish_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
			publishEventFn: func(ctx context.Context, topicID, orderingKey string, payload, headers json.RawMessage) (*store.EventMessage, int, error) {
				return nil, 0, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"payload":{"key":"value"}}`
		req := httptest.NewRequest("POST", "/topics/orders/publish", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})

	t.Run("topic_generic_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"payload":{"key":"value"}}`
		req := httptest.NewRequest("POST", "/topics/orders/publish", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestCreateEventOutbox(t *testing.T) {
	topicStub := func() *mockMetadataStore {
		return &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
		}
	}

	t.Run("success", func(t *testing.T) {
		ms := topicStub()
		_, mux := setupTestHandler(t, ms)
		body := `{"payload":{"key":"value"}}`
		req := httptest.NewRequest("POST", "/topics/orders/outbox", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusAccepted {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("topic_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return nil, store.ErrEventTopicNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/topics/nope/outbox", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		ms := topicStub()
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/topics/orders/outbox", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("with_ordering_key_and_headers", func(t *testing.T) {
		var captured *store.EventOutbox
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
			createEventOutboxFn: func(ctx context.Context, outbox *store.EventOutbox) error {
				captured = outbox
				return nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"payload":{"k":"v"},"ordering_key":"order-123","headers":{"x":"y"}}`
		req := httptest.NewRequest("POST", "/topics/orders/outbox", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusAccepted {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		if captured == nil || captured.OrderingKey != "order-123" {
			t.Fatalf("expected ordering_key 'order-123', got %v", captured)
		}
	})

	t.Run("with_retry_options", func(t *testing.T) {
		ms := topicStub()
		_, mux := setupTestHandler(t, ms)
		body := `{"payload":{"k":"v"},"max_attempts":5,"backoff_base_ms":200,"backoff_max_ms":10000}`
		req := httptest.NewRequest("POST", "/topics/orders/outbox", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusAccepted {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("store_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
			createEventOutboxFn: func(ctx context.Context, outbox *store.EventOutbox) error {
				return fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"payload":{"k":"v"}}`
		req := httptest.NewRequest("POST", "/topics/orders/outbox", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("topic_generic_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"payload":{"k":"v"}}`
		req := httptest.NewRequest("POST", "/topics/orders/outbox", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestListEventOutbox(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics/orders/outbox", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("topic_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return nil, store.ErrEventTopicNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics/nope/outbox", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("invalid_status_filter", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics/orders/outbox?status=invalid_status", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestListTopicMessages(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics/orders/messages", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("topic_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return nil, store.ErrEventTopicNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics/nope/messages", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("topic_generic_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics/orders/messages", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("list_messages_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
			listEventMessagesFn: func(ctx context.Context, topicID string, limit, offset int) ([]*store.EventMessage, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics/orders/messages", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestCreateEventSubscription(t *testing.T) {
	topicAndFnStub := func() *mockMetadataStore {
		return &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-1", Name: name}, nil
			},
		}
	}

	t.Run("success_function_type", func(t *testing.T) {
		ms := topicAndFnStub()
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"order-sub","function_name":"processor","consumer_group":"cg1"}`
		req := httptest.NewRequest("POST", "/topics/orders/subscriptions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		ms := topicAndFnStub()
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/topics/orders/subscriptions", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_name", func(t *testing.T) {
		ms := topicAndFnStub()
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"","function_name":"processor"}`
		req := httptest.NewRequest("POST", "/topics/orders/subscriptions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing_function_name", func(t *testing.T) {
		ms := topicAndFnStub()
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"sub1","function_name":""}`
		req := httptest.NewRequest("POST", "/topics/orders/subscriptions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("topic_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return nil, store.ErrEventTopicNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"sub1","function_name":"fn1"}`
		req := httptest.NewRequest("POST", "/topics/nope/subscriptions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("function_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"sub1","function_name":"nonexistent"}`
		req := httptest.NewRequest("POST", "/topics/orders/subscriptions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("invalid_type", func(t *testing.T) {
		ms := topicAndFnStub()
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"sub1","type":"invalid","function_name":"fn1"}`
		req := httptest.NewRequest("POST", "/topics/orders/subscriptions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		ms := topicAndFnStub()
		ms.createEventSubscriptionFn = func(ctx context.Context, sub *store.EventSubscription) error {
			return fmt.Errorf("subscription already exists")
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"sub1","function_name":"fn1"}`
		req := httptest.NewRequest("POST", "/topics/orders/subscriptions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})

	t.Run("workflow_type_no_service", func(t *testing.T) {
		ms := topicAndFnStub()
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"sub1","type":"workflow","workflow_name":"mywf"}`
		req := httptest.NewRequest("POST", "/topics/orders/subscriptions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})

	t.Run("workflow_type_missing_name", func(t *testing.T) {
		ms := topicAndFnStub()
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"sub1","type":"workflow"}`
		req := httptest.NewRequest("POST", "/topics/orders/subscriptions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("with_optional_fields", func(t *testing.T) {
		ms := topicAndFnStub()
		_, mux := setupTestHandler(t, ms)
		enabled := true
		_ = enabled
		body := `{"name":"sub1","function_name":"fn1","enabled":true,"max_attempts":3,"backoff_base_ms":100,"backoff_max_ms":5000,"max_inflight":10,"rate_limit_per_s":100}`
		req := httptest.NewRequest("POST", "/topics/orders/subscriptions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("store_error", func(t *testing.T) {
		ms := topicAndFnStub()
		ms.createEventSubscriptionFn = func(ctx context.Context, sub *store.EventSubscription) error {
			return fmt.Errorf("db error")
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"sub1","function_name":"fn1"}`
		req := httptest.NewRequest("POST", "/topics/orders/subscriptions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("topic_generic_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"sub1","function_name":"fn1"}`
		req := httptest.NewRequest("POST", "/topics/orders/subscriptions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestListEventSubscriptions(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics/orders/subscriptions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("topic_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return nil, store.ErrEventTopicNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics/nope/subscriptions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("topic_generic_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics/orders/subscriptions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("list_subs_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
				return &store.EventTopic{ID: "topic-1", Name: name}, nil
			},
			listEventSubscriptionsFn: func(ctx context.Context, topicID string, limit, offset int) ([]*store.EventSubscription, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/topics/orders/subscriptions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

func TestGetEventSubscription(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventSubscriptionFn: func(ctx context.Context, id string) (*store.EventSubscription, error) {
				return &store.EventSubscription{ID: id, Name: "sub1"}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/subscriptions/sub-1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventSubscriptionFn: func(ctx context.Context, id string) (*store.EventSubscription, error) {
				return nil, store.ErrEventSubscriptionNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/subscriptions/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestUpdateEventSubscription(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			updateEventSubscriptionFn: func(ctx context.Context, id string, update *store.EventSubscriptionUpdate) (*store.EventSubscription, error) {
				return &store.EventSubscription{ID: id, Name: "updated"}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"updated"}`
		req := httptest.NewRequest("PATCH", "/subscriptions/sub-1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("PATCH", "/subscriptions/sub-1", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			updateEventSubscriptionFn: func(ctx context.Context, id string, update *store.EventSubscriptionUpdate) (*store.EventSubscription, error) {
				return nil, store.ErrEventSubscriptionNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"updated"}`
		req := httptest.NewRequest("PATCH", "/subscriptions/nope", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("with_function_name_update", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return &domain.Function{ID: "fn-2", Name: name}, nil
			},
			updateEventSubscriptionFn: func(ctx context.Context, id string, update *store.EventSubscriptionUpdate) (*store.EventSubscription, error) {
				return &store.EventSubscription{ID: id, FunctionName: *update.FunctionName}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"function_name":"new-fn"}`
		req := httptest.NewRequest("PATCH", "/subscriptions/sub-1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("function_name_empty", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"function_name":""}`
		req := httptest.NewRequest("PATCH", "/subscriptions/sub-1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("function_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"function_name":"nope"}`
		req := httptest.NewRequest("PATCH", "/subscriptions/sub-1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("workflow_name_empty", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"workflow_name":""}`
		req := httptest.NewRequest("PATCH", "/subscriptions/sub-1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("workflow_service_nil", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"workflow_name":"mywf"}`
		req := httptest.NewRequest("PATCH", "/subscriptions/sub-1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})

	t.Run("conflict_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			updateEventSubscriptionFn: func(ctx context.Context, id string, update *store.EventSubscriptionUpdate) (*store.EventSubscription, error) {
				return nil, fmt.Errorf("subscription already exists")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"name":"updated"}`
		req := httptest.NewRequest("PATCH", "/subscriptions/sub-1", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})
}

func TestDeleteEventSubscription(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/subscriptions/sub-1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteEventSubscriptionFn: func(ctx context.Context, id string) error {
				return store.ErrEventSubscriptionNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/subscriptions/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("generic_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			deleteEventSubscriptionFn: func(ctx context.Context, id string) error {
				return fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("DELETE", "/subscriptions/sub-1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestListEventDeliveries(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/subscriptions/sub-1/deliveries", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("with_status_filter", func(t *testing.T) {
		ms := &mockMetadataStore{
			listEventDeliveriesFn: func(ctx context.Context, subscriptionID string, limit, offset int, statuses []store.EventDeliveryStatus) ([]*store.EventDelivery, error) {
				return []*store.EventDelivery{}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/subscriptions/sub-1/deliveries?status=queued,succeeded", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("invalid_status", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("GET", "/subscriptions/sub-1/deliveries?status=invalid", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestReplayEventSubscription(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			replayEventSubscriptionFn: func(ctx context.Context, subscriptionID string, fromSequence int64, limit int) (int, error) {
				return 5, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"from_sequence":10,"limit":100}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/replay", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["status"] != "replayed" {
			t.Fatalf("unexpected status: %v", resp)
		}
	})

	t.Run("empty_body", func(t *testing.T) {
		ms := &mockMetadataStore{
			replayEventSubscriptionFn: func(ctx context.Context, subscriptionID string, fromSequence int64, limit int) (int, error) {
				return 0, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/replay", nil)
		req.ContentLength = 0
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			replayEventSubscriptionFn: func(ctx context.Context, subscriptionID string, fromSequence int64, limit int) (int, error) {
				return 0, store.ErrEventSubscriptionNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/subscriptions/nope/replay", nil)
		req.ContentLength = 0
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("with_from_time", func(t *testing.T) {
		ms := &mockMetadataStore{
			resolveEventReplaySequenceByTimeFn: func(ctx context.Context, subscriptionID string, from time.Time) (int64, error) {
				return 42, nil
			},
			replayEventSubscriptionFn: func(ctx context.Context, subscriptionID string, fromSequence int64, limit int) (int, error) {
				if fromSequence != 42 {
					t.Fatalf("expected fromSequence=42, got %d", fromSequence)
				}
				return 3, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"from_time":"2024-01-01T00:00:00Z"}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/replay", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid_from_time", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"from_time":"not-a-date"}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/replay", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("with_reset_cursor", func(t *testing.T) {
		ms := &mockMetadataStore{
			setEventSubscriptionCursorFn: func(ctx context.Context, subscriptionID string, lastAckedSequence int64) (*store.EventSubscription, error) {
				return &store.EventSubscription{ID: subscriptionID}, nil
			},
			replayEventSubscriptionFn: func(ctx context.Context, subscriptionID string, fromSequence int64, limit int) (int, error) {
				return 2, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"from_sequence":5,"reset_cursor":true}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/replay", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		ms := &mockMetadataStore{}
		_, mux := setupTestHandler(t, ms)
		body := `{bad`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/replay", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("from_time_resolve_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			resolveEventReplaySequenceByTimeFn: func(ctx context.Context, subscriptionID string, from time.Time) (int64, error) {
				return 0, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"from_time":"2024-01-01T00:00:00Z"}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/replay", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("from_time_sub_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			resolveEventReplaySequenceByTimeFn: func(ctx context.Context, subscriptionID string, from time.Time) (int64, error) {
				return 0, store.ErrEventSubscriptionNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"from_time":"2024-01-01T00:00:00Z"}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/replay", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("reset_cursor_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			setEventSubscriptionCursorFn: func(ctx context.Context, subscriptionID string, lastAckedSequence int64) (*store.EventSubscription, error) {
				return nil, store.ErrEventSubscriptionNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"from_sequence":5,"reset_cursor":true}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/replay", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("replay_generic_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			replayEventSubscriptionFn: func(ctx context.Context, subscriptionID string, fromSequence int64, limit int) (int, error) {
				return 0, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"from_sequence":5}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/replay", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestSeekEventSubscription(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			setEventSubscriptionCursorFn: func(ctx context.Context, subscriptionID string, lastAckedSequence int64) (*store.EventSubscription, error) {
				return &store.EventSubscription{ID: subscriptionID}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"from_sequence":10}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/seek", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["status"] != "seeked" {
			t.Fatalf("unexpected status: %v", resp)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			setEventSubscriptionCursorFn: func(ctx context.Context, subscriptionID string, lastAckedSequence int64) (*store.EventSubscription, error) {
				return nil, store.ErrEventSubscriptionNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"from_sequence":10}`
		req := httptest.NewRequest("POST", "/subscriptions/nope/seek", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("with_from_time", func(t *testing.T) {
		ms := &mockMetadataStore{
			resolveEventReplaySequenceByTimeFn: func(ctx context.Context, subscriptionID string, from time.Time) (int64, error) {
				return 50, nil
			},
			setEventSubscriptionCursorFn: func(ctx context.Context, subscriptionID string, lastAckedSequence int64) (*store.EventSubscription, error) {
				if lastAckedSequence != 49 { // fromSequence-1
					t.Fatalf("expected cursor=49, got %d", lastAckedSequence)
				}
				return &store.EventSubscription{ID: subscriptionID}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"from_time":"2024-06-01T12:00:00Z"}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/seek", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid_from_time", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{"from_time":"bad-date"}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/seek", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("empty_body_defaults_sequence", func(t *testing.T) {
		ms := &mockMetadataStore{
			setEventSubscriptionCursorFn: func(ctx context.Context, subscriptionID string, lastAckedSequence int64) (*store.EventSubscription, error) {
				// fromSequence defaults to 1 when <=0, so cursor = 1-1 = 0
				return &store.EventSubscription{ID: subscriptionID}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/seek", nil)
		req.ContentLength = 0
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("from_time_resolve_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			resolveEventReplaySequenceByTimeFn: func(ctx context.Context, subscriptionID string, from time.Time) (int64, error) {
				return 0, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"from_time":"2024-06-01T12:00:00Z"}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/seek", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("from_time_sub_not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			resolveEventReplaySequenceByTimeFn: func(ctx context.Context, subscriptionID string, from time.Time) (int64, error) {
				return 0, store.ErrEventSubscriptionNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"from_time":"2024-06-01T12:00:00Z"}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/seek", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("cursor_generic_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			setEventSubscriptionCursorFn: func(ctx context.Context, subscriptionID string, lastAckedSequence int64) (*store.EventSubscription, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"from_sequence":10}`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/seek", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		body := `{bad`
		req := httptest.NewRequest("POST", "/subscriptions/sub-1/seek", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestGetEventDelivery(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventDeliveryFn: func(ctx context.Context, id string) (*store.EventDelivery, error) {
				return &store.EventDelivery{ID: id}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/deliveries/d-1", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			getEventDeliveryFn: func(ctx context.Context, id string) (*store.EventDelivery, error) {
				return nil, store.ErrEventDeliveryNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("GET", "/deliveries/nope", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestRetryEventDelivery(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			requeueEventDeliveryFn: func(ctx context.Context, id string, maxAttempts int) (*store.EventDelivery, error) {
				return &store.EventDelivery{ID: id}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/deliveries/d-1/retry", nil)
		req.ContentLength = 0
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			requeueEventDeliveryFn: func(ctx context.Context, id string, maxAttempts int) (*store.EventDelivery, error) {
				return nil, store.ErrEventDeliveryNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/deliveries/nope/retry", nil)
		req.ContentLength = 0
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("not_dlq", func(t *testing.T) {
		ms := &mockMetadataStore{
			requeueEventDeliveryFn: func(ctx context.Context, id string, maxAttempts int) (*store.EventDelivery, error) {
				return nil, store.ErrEventDeliveryNotDLQ
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/deliveries/d-1/retry", nil)
		req.ContentLength = 0
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})

	t.Run("with_max_attempts", func(t *testing.T) {
		var captured int
		ms := &mockMetadataStore{
			requeueEventDeliveryFn: func(ctx context.Context, id string, maxAttempts int) (*store.EventDelivery, error) {
				captured = maxAttempts
				return &store.EventDelivery{ID: id}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"max_attempts":5}`
		req := httptest.NewRequest("POST", "/deliveries/d-1/retry", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d", w.Code)
		}
		if captured != 5 {
			t.Fatalf("expected max_attempts=5, got %d", captured)
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/deliveries/d-1/retry", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("generic_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			requeueEventDeliveryFn: func(ctx context.Context, id string, maxAttempts int) (*store.EventDelivery, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/deliveries/d-1/retry", nil)
		req.ContentLength = 0
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestRetryEventOutbox(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ms := &mockMetadataStore{
			requeueEventOutboxFn: func(ctx context.Context, id string, maxAttempts int) (*store.EventOutbox, error) {
				return &store.EventOutbox{ID: id}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/outbox/ob-1/retry", nil)
		req.ContentLength = 0
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("not_found", func(t *testing.T) {
		ms := &mockMetadataStore{
			requeueEventOutboxFn: func(ctx context.Context, id string, maxAttempts int) (*store.EventOutbox, error) {
				return nil, store.ErrEventOutboxNotFound
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/outbox/nope/retry", nil)
		req.ContentLength = 0
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("not_failed", func(t *testing.T) {
		ms := &mockMetadataStore{
			requeueEventOutboxFn: func(ctx context.Context, id string, maxAttempts int) (*store.EventOutbox, error) {
				return nil, store.ErrEventOutboxNotFailed
			},
		}
		_, mux := setupTestHandler(t, ms)
		req := httptest.NewRequest("POST", "/outbox/ob-1/retry", nil)
		req.ContentLength = 0
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		_, mux := setupTestHandler(t, nil)
		req := httptest.NewRequest("POST", "/outbox/ob-1/retry", strings.NewReader("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("generic_error", func(t *testing.T) {
		ms := &mockMetadataStore{
			requeueEventOutboxFn: func(ctx context.Context, id string, maxAttempts int) (*store.EventOutbox, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"max_attempts":3}`
		req := httptest.NewRequest("POST", "/outbox/ob-1/retry", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("with_max_attempts", func(t *testing.T) {
		ms := &mockMetadataStore{
			requeueEventOutboxFn: func(ctx context.Context, id string, maxAttempts int) (*store.EventOutbox, error) {
				return &store.EventOutbox{ID: id, MaxAttempts: maxAttempts}, nil
			},
		}
		_, mux := setupTestHandler(t, ms)
		body := `{"max_attempts":5}`
		req := httptest.NewRequest("POST", "/outbox/ob-1/retry", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d, body: %s", w.Code, w.Body.String())
		}
	})
}

func TestParseEventStatuses(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		statuses, err := parseEventStatuses("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(statuses) != 0 {
			t.Fatalf("expected empty, got %v", statuses)
		}
	})

	t.Run("valid", func(t *testing.T) {
		statuses, err := parseEventStatuses("queued,succeeded")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(statuses) != 2 {
			t.Fatalf("expected 2, got %d", len(statuses))
		}
	})

	t.Run("invalid", func(t *testing.T) {
		_, err := parseEventStatuses("invalid")
		if err == nil {
			t.Fatal("expected error for invalid status")
		}
	})
}

func TestParseOutboxStatuses(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		statuses, err := parseOutboxStatuses("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(statuses) != 0 {
			t.Fatalf("expected empty, got %v", statuses)
		}
	})

	t.Run("valid", func(t *testing.T) {
		statuses, err := parseOutboxStatuses("pending,published")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(statuses) != 2 {
			t.Fatalf("expected 2, got %d", len(statuses))
		}
	})

	t.Run("invalid", func(t *testing.T) {
		_, err := parseOutboxStatuses("invalid")
		if err == nil {
			t.Fatal("expected error for invalid outbox status")
		}
	})
}

func TestParseEventLimitQuery(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		limit := parseEventLimitQuery("", store.DefaultEventListLimit, store.MaxEventListLimit)
		if limit != store.DefaultEventListLimit {
			t.Fatalf("expected default limit %d, got %d", store.DefaultEventListLimit, limit)
		}
	})

	t.Run("custom_value", func(t *testing.T) {
		limit := parseEventLimitQuery("25", store.DefaultEventListLimit, store.MaxEventListLimit)
		if limit != 25 {
			t.Fatalf("expected limit 25, got %d", limit)
		}
	})

	t.Run("max_cap", func(t *testing.T) {
		limit := parseEventLimitQuery("999", store.DefaultEventListLimit, store.MaxEventListLimit)
		if limit != store.MaxEventListLimit {
			t.Fatalf("expected capped limit %d, got %d", store.MaxEventListLimit, limit)
		}
	})

	t.Run("invalid_string", func(t *testing.T) {
		limit := parseEventLimitQuery("abc", store.DefaultEventListLimit, store.MaxEventListLimit)
		if limit != store.DefaultEventListLimit {
			t.Fatalf("expected fallback limit, got %d", limit)
		}
	})

	t.Run("negative_value", func(t *testing.T) {
		limit := parseEventLimitQuery("-5", store.DefaultEventListLimit, store.MaxEventListLimit)
		if limit != store.DefaultEventListLimit {
			t.Fatalf("expected fallback limit on negative, got %d", limit)
		}
	})

	t.Run("zero_value", func(t *testing.T) {
		limit := parseEventLimitQuery("0", store.DefaultEventListLimit, store.MaxEventListLimit)
		if limit != store.DefaultEventListLimit {
			t.Fatalf("expected fallback limit on zero, got %d", limit)
		}
	})
}
