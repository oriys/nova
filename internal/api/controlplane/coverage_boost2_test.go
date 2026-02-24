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
	"github.com/oriys/nova/internal/scheduler"
	"github.com/oriys/nova/internal/store"
	"github.com/oriys/nova/internal/workflow"
)

// ─── Event subscription workflow-type branches ──────────────────────────────

func setupHandlerWithWorkflowService(t *testing.T, ms *mockMetadataStore, ws *mockWorkflowStore) *http.ServeMux {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	if ws == nil {
		ws = &mockWorkflowStore{}
	}
	s := newCompositeStore(ms, nil, ws)
	wfSvc := workflow.NewService(s)
	h := &Handler{Store: s, WorkflowService: wfSvc}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestCreateEventSubscription_WorkflowType_Success(t *testing.T) {
	ms := &mockMetadataStore{
		getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
			return &store.EventTopic{ID: "t1", Name: name}, nil
		},
		createEventSubscriptionFn: func(ctx context.Context, sub *store.EventSubscription) error {
			return nil
		},
	}
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(ctx context.Context, name string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: "wf-1", Name: name}, nil
		},
	}
	mux := setupHandlerWithWorkflowService(t, ms, ws)
	body := `{"name":"sub1","type":"workflow","workflow_name":"my-wf","webhook_url":"https://example.com","webhook_method":"POST","webhook_headers":{"X-Key":"val"},"webhook_signing_secret":"secret","webhook_timeout_ms":5000}`
	req := httptest.NewRequest("POST", "/topics/my-topic/subscriptions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusCreated)
}

func TestCreateEventSubscription_WorkflowType_NotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
			return &store.EventTopic{ID: "t1", Name: name}, nil
		},
	}
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(ctx context.Context, name string) (*domain.Workflow, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	mux := setupHandlerWithWorkflowService(t, ms, ws)
	body := `{"name":"sub1","type":"workflow","workflow_name":"nope"}`
	req := httptest.NewRequest("POST", "/topics/my-topic/subscriptions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestCreateEventSubscription_WorkflowType_MissingName(t *testing.T) {
	ms := &mockMetadataStore{
		getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
			return &store.EventTopic{ID: "t1", Name: name}, nil
		},
	}
	mux := setupHandlerWithWorkflowService(t, ms, nil)
	body := `{"name":"sub1","type":"workflow","workflow_name":""}`
	req := httptest.NewRequest("POST", "/topics/my-topic/subscriptions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestCreateEventSubscription_WorkflowType_NoService(t *testing.T) {
	ms := &mockMetadataStore{
		getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
			return &store.EventTopic{ID: "t1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms) // No WorkflowService
	body := `{"name":"sub1","type":"workflow","workflow_name":"my-wf"}`
	req := httptest.NewRequest("POST", "/topics/my-topic/subscriptions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── UpdateEventSubscription workflow-type branches ─────────────────────────

func TestUpdateEventSubscription_WorkflowType(t *testing.T) {
	ms := &mockMetadataStore{
		getEventSubscriptionFn: func(ctx context.Context, id string) (*store.EventSubscription, error) {
			return &store.EventSubscription{ID: id, Type: store.EventSubscriptionTypeWorkflow}, nil
		},
		updateEventSubscriptionFn: func(ctx context.Context, id string, update *store.EventSubscriptionUpdate) (*store.EventSubscription, error) {
			return &store.EventSubscription{ID: id, Type: store.EventSubscriptionTypeWorkflow}, nil
		},
	}
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(ctx context.Context, name string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: "wf-1", Name: name}, nil
		},
	}
	mux := setupHandlerWithWorkflowService(t, ms, ws)
	body := `{"workflow_name":"my-wf","webhook_url":"https://example.com","webhook_timeout_ms":5000}`
	req := httptest.NewRequest("PATCH", "/subscriptions/sub1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestUpdateEventSubscription_WorkflowNotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getEventSubscriptionFn: func(ctx context.Context, id string) (*store.EventSubscription, error) {
			return &store.EventSubscription{ID: id, Type: store.EventSubscriptionTypeWorkflow}, nil
		},
	}
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(ctx context.Context, name string) (*domain.Workflow, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	mux := setupHandlerWithWorkflowService(t, ms, ws)
	body := `{"workflow_name":"nope"}`
	req := httptest.NewRequest("PATCH", "/subscriptions/sub1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

// ─── PublishEvent store error branches ──────────────────────────────────────

func TestPublishEvent_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
			return &store.EventTopic{ID: "t1", Name: name}, nil
		},
		publishEventFn: func(ctx context.Context, topicID, orderingKey string, payload, headers json.RawMessage) (*store.EventMessage, int, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"payload":{"key":"value"}}`
	req := httptest.NewRequest("POST", "/topics/my-topic/publish", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── CreateEventOutbox store error ──────────────────────────────────────────

func TestCreateEventOutbox_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
			return &store.EventTopic{ID: "t1", Name: name}, nil
		},
		createEventOutboxFn: func(ctx context.Context, outbox *store.EventOutbox) error {
			return fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"ordering_key":"key1","payload":{"k":"v"}}`
	req := httptest.NewRequest("POST", "/topics/my-topic/outbox", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

// ─── ListEventOutbox store error ────────────────────────────────────────────

func TestListEventOutbox_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
			return &store.EventTopic{ID: "t1", Name: name}, nil
		},
		listEventOutboxFn: func(ctx context.Context, topicID string, limit, offset int, statuses []store.EventOutboxStatus) ([]*store.EventOutbox, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/topics/my-topic/outbox", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── ListEventDeliveries store error ────────────────────────────────────────

func TestListEventDeliveries_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		listEventDeliveriesFn: func(ctx context.Context, subscriptionID string, limit, offset int, statuses []store.EventDeliveryStatus) ([]*store.EventDelivery, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/subscriptions/sub1/deliveries", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── Schedule handlers: with Scheduler (exercising Scheduler branches) ──────

func setupScheduleWithSchedulerMock(t *testing.T, ms *mockMetadataStore, ss *mockScheduleStore) *http.ServeMux {
	t.Helper()
	if ms == nil {
		ms = &mockMetadataStore{}
	}
	if ss == nil {
		ss = &mockScheduleStore{}
	}
	s := newCompositeStore(ms, ss, nil)
	sched := scheduler.New(s, nil)
	h := &ScheduleHandler{Store: s, Scheduler: sched}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestCreateSchedule_WithSchedulerRegistered(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
	}
	ss := &mockScheduleStore{
		saveScheduleFn: func(_ context.Context, s *store.Schedule) error { return nil },
	}
	mux := setupScheduleWithSchedulerMock(t, ms, ss)
	body := `{"cron_expression":"@every 5m"}`
	req := httptest.NewRequest("POST", "/functions/hello/schedules", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusCreated)
}

func TestDeleteSchedule_WithSchedulerRegistered(t *testing.T) {
	ss := &mockScheduleStore{
		getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
			return &store.Schedule{ID: id, FunctionName: "hello"}, nil
		},
		deleteScheduleFn: func(_ context.Context, id string) error { return nil },
	}
	mux := setupScheduleWithSchedulerMock(t, nil, ss)
	req := httptest.NewRequest("DELETE", "/functions/hello/schedules/s1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestToggleSchedule_WithSchedulerEnable2(t *testing.T) {
	ss := &mockScheduleStore{
		getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
			return &store.Schedule{ID: id, FunctionName: "hello", Enabled: false, CronExpr: "@every 5m"}, nil
		},
		updateScheduleEnabledFn: func(_ context.Context, id string, enabled bool) error { return nil },
	}
	mux := setupScheduleWithSchedulerMock(t, nil, ss)
	body := `{"enabled":true}`
	req := httptest.NewRequest("PATCH", "/functions/hello/schedules/s1", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestToggleSchedule_WithSchedulerDisable2(t *testing.T) {
	ss := &mockScheduleStore{
		getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
			return &store.Schedule{ID: id, FunctionName: "hello", Enabled: true, CronExpr: "@every 5m"}, nil
		},
		updateScheduleEnabledFn: func(_ context.Context, id string, enabled bool) error { return nil },
	}
	mux := setupScheduleWithSchedulerMock(t, nil, ss)
	body := `{"enabled":false}`
	req := httptest.NewRequest("PATCH", "/functions/hello/schedules/s1", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestToggleSchedule_UpdateCronWithScheduler(t *testing.T) {
	ss := &mockScheduleStore{
		getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
			return &store.Schedule{ID: id, FunctionName: "hello", Enabled: true, CronExpr: "@every 5m"}, nil
		},
		updateScheduleCronFn: func(_ context.Context, id, cronExpr string) error { return nil },
	}
	mux := setupScheduleWithSchedulerMock(t, nil, ss)
	body := `{"cron_expression":"@every 10m"}`
	req := httptest.NewRequest("PATCH", "/functions/hello/schedules/s1", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

func TestDeleteSchedule_FnMismatch(t *testing.T) {
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
}

// ─── ListSchedules store error ──────────────────────────────────────────────

func TestListSchedules_StoreError(t *testing.T) {
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

// ─── DeleteSchedule store delete error ──────────────────────────────────────

func TestDeleteSchedule_StoreDeleteError(t *testing.T) {
	ss := &mockScheduleStore{
		getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
			return &store.Schedule{ID: id, FunctionName: "hello"}, nil
		},
		deleteScheduleFn: func(_ context.Context, id string) error {
			return fmt.Errorf("db error")
		},
	}
	mux := setupScheduleHandlerFull(t, nil, ss)
	req := httptest.NewRequest("DELETE", "/functions/hello/schedules/s1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── ToggleSchedule: function name mismatch ─────────────────────────────────

func TestToggleSchedule_FnMismatch(t *testing.T) {
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
}

func TestToggleSchedule_UpdateEnableError(t *testing.T) {
	ss := &mockScheduleStore{
		getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
			return &store.Schedule{ID: id, FunctionName: "hello", Enabled: false}, nil
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
}

func TestToggleSchedule_UpdateCronError(t *testing.T) {
	ss := &mockScheduleStore{
		getScheduleFn: func(_ context.Context, id string) (*store.Schedule, error) {
			return &store.Schedule{ID: id, FunctionName: "hello", Enabled: true}, nil
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
}

// ─── CreateEventTopic store error ───────────────────────────────────────────

func TestCreateEventTopic_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		createEventTopicFn: func(ctx context.Context, topic *store.EventTopic) error {
			return fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"name":"my-topic"}`
	req := httptest.NewRequest("POST", "/topics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

// ─── CreateEventSubscription store error ────────────────────────────────────

func TestCreateEventSubscription_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
			return &store.EventTopic{ID: "t1", Name: name}, nil
		},
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name}, nil
		},
		createEventSubscriptionFn: func(ctx context.Context, sub *store.EventSubscription) error {
			return fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"name":"sub1","function_name":"hello"}`
	req := httptest.NewRequest("POST", "/topics/my-topic/subscriptions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

// ─── UpdateEventSubscription store error ────────────────────────────────────

func TestUpdateEventSubscription_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		getEventSubscriptionFn: func(ctx context.Context, id string) (*store.EventSubscription, error) {
			return &store.EventSubscription{ID: id, Type: store.EventSubscriptionTypeFunction}, nil
		},
		updateEventSubscriptionFn: func(ctx context.Context, id string, update *store.EventSubscriptionUpdate) (*store.EventSubscription, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"enabled":false}`
	req := httptest.NewRequest("PATCH", "/subscriptions/sub1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

// ─── Layer: CreateLayer bad JSON and missing fields ─────────────────────────

func TestCreateLayer_BadBase64(t *testing.T) {
	_, mux := setupLayerHandler(t, nil)
	body := `{"name":"mylib","runtime":"python","files":{"lib.py":"not-base64!!!"}}`
	req := httptest.NewRequest("POST", "/layers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestCreateLayer_MissingFiles(t *testing.T) {
	_, mux := setupLayerHandler(t, nil)
	body := `{"name":"mylib","runtime":"python"}`
	req := httptest.NewRequest("POST", "/layers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestCreateLayer_StoreError(t *testing.T) {
	ms := &mockMetadataStore{
		saveLayerFn: func(ctx context.Context, layer *domain.Layer) error {
			return fmt.Errorf("db error")
		},
	}
	_, mux := setupLayerHandler(t, ms)
	body := `{"name":"mylib","runtime":"python","files":{"lib.py":"cHJpbnQoJ2hlbGxvJyk="}}`
	req := httptest.NewRequest("POST", "/layers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── Layer: SetFunctionLayers more branches ─────────────────────────────────

func TestSetFunctionLayers_LayerNotFound(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		getLayerFn: func(ctx context.Context, id string) (*domain.Layer, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupLayerHandler(t, ms)
	body := `{"layer_ids":["l1"]}`
	req := httptest.NewRequest("PUT", "/functions/hello/layers", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestSetFunctionLayers_RuntimeMismatch(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
			return &domain.Function{ID: "fn-1", Name: name, Runtime: "python"}, nil
		},
		getLayerFn: func(ctx context.Context, id string) (*domain.Layer, error) {
			return &domain.Layer{ID: id, Name: "golib", Runtime: "go", SizeMB: 10}, nil
		},
	}
	_, mux := setupLayerHandler(t, ms)
	body := `{"layer_ids":["l1"]}`
	req := httptest.NewRequest("PUT", "/functions/hello/layers", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

// ─── ListWorkflowVersions pagination params ─────────────────────────────────

func TestListWorkflowVersions_WithPagination2(t *testing.T) {
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(ctx context.Context, name string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: "wf-1", Name: name}, nil
		},
		listWorkflowVersionsFn: func(ctx context.Context, wfID string, limit, offset int) ([]*domain.WorkflowVersion, error) {
			return []*domain.WorkflowVersion{}, nil
		},
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	req := httptest.NewRequest("GET", "/workflows/hello/versions?limit=5&offset=0", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusOK)
}

// ─── InvokeWorkflowAsync: workflow not found ────────────────────────────────

func TestInvokeWorkflowAsync_NotFound2(t *testing.T) {
	ws := &mockWorkflowStore{
		getWorkflowByNameFn: func(ctx context.Context, name string) (*domain.Workflow, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupWorkflowHandler(t, nil, ws)
	body := `{"input":{"key":"value"}}`
	req := httptest.NewRequest("POST", "/workflows/nope/invoke", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

// ─── GetScalingPolicy: not found error ──────────────────────────────────────

func TestGetScalingPolicy_Error(t *testing.T) {
	ms := &mockMetadataStore{
		getFunctionByNameFn: func(_ context.Context, name string) (*domain.Function, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/functions/nope/scaling", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

// ─── DbAccess: invalid tenant_mode ──────────────────────────────────────────

func TestCreateDbResource_InvalidTenantMode(t *testing.T) {
	_, mux := setupTestHandler(t, nil)
	body := `{"name":"mydb","type":"postgres","endpoint":"localhost","tenant_mode":"invalid"}`
	req := httptest.NewRequest("POST", "/db-resources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestCreateDbResource_StoreError2(t *testing.T) {
	ms := &mockMetadataStore{
		createDbResourceFn: func(ctx context.Context, rec *store.DbResourceRecord) (*store.DbResourceRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"name":"mydb","type":"postgres","endpoint":"localhost"}`
	req := httptest.NewRequest("POST", "/db-resources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestGetDbResource_NotFound2(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(ctx context.Context, name string) (*store.DbResourceRecord, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/db-resources/res1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusNotFound)
}

func TestUpdateDbResource_BadJSON2(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(ctx context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "id1", Name: name}, nil
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("PATCH", "/db-resources/res1", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestUpdateDbResource_StoreError2(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(ctx context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "id1", Name: name}, nil
		},
		updateDbResourceFn: func(ctx context.Context, id string, update *store.DbResourceUpdate) (*store.DbResourceRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"endpoint":"new-host"}`
	req := httptest.NewRequest("PATCH", "/db-resources/res1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestCreateDbBinding_StoreError2(t *testing.T) {
	ms := &mockMetadataStore{
		getDbResourceByNameFn: func(ctx context.Context, name string) (*store.DbResourceRecord, error) {
			return &store.DbResourceRecord{ID: "res1", Name: name}, nil
		},
		createDbBindingFn: func(ctx context.Context, rec *store.DbBindingRecord) (*store.DbBindingRecord, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	body := `{"function_id":"fn1","permissions":["read"]}`
	req := httptest.NewRequest("POST", "/db-resources/hello/bindings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestListEventSubscriptions_StoreError2(t *testing.T) {
	ms := &mockMetadataStore{
		getEventTopicByNameFn: func(ctx context.Context, name string) (*store.EventTopic, error) {
			return &store.EventTopic{ID: "t1", Name: name}, nil
		},
		listEventSubscriptionsFn: func(ctx context.Context, topicID string, limit, offset int) ([]*store.EventSubscription, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	_, mux := setupTestHandler(t, ms)
	req := httptest.NewRequest("GET", "/topics/my-topic/subscriptions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	expectStatus(t, w, http.StatusInternalServerError)
}

// ─── Volume: SetFunctionMounts deeper branches ──────────────────────────────

func TestSetFunctionMounts_VolumeNotFound(t *testing.T) {
ms := &mockMetadataStore{
getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
return &domain.Function{ID: "fn-1", Name: name}, nil
},
getVolumeFn: func(ctx context.Context, id string) (*domain.Volume, error) {
return nil, fmt.Errorf("not found")
},
}
_, mux := setupVolumeHandler(t, ms)
body := `{"mounts":[{"volume_id":"vol-1","mount_path":"/data"}]}`
req := httptest.NewRequest("PUT", "/functions/hello/mounts", strings.NewReader(body))
req.Header.Set("Content-Type", "application/json")
w := httptest.NewRecorder()
mux.ServeHTTP(w, req)
expectStatus(t, w, http.StatusBadRequest)
}

func TestSetFunctionMounts_VolumeNoImage(t *testing.T) {
ms := &mockMetadataStore{
getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
return &domain.Function{ID: "fn-1", Name: name}, nil
},
getVolumeFn: func(ctx context.Context, id string) (*domain.Volume, error) {
return &domain.Volume{ID: id, Name: "vol1", ImagePath: ""}, nil
},
}
_, mux := setupVolumeHandler(t, ms)
body := `{"mounts":[{"volume_id":"vol-1","mount_path":"/data"}]}`
req := httptest.NewRequest("PUT", "/functions/hello/mounts", strings.NewReader(body))
req.Header.Set("Content-Type", "application/json")
w := httptest.NewRecorder()
mux.ServeHTTP(w, req)
expectStatus(t, w, http.StatusBadRequest)
}

func TestSetFunctionMounts_SaveError(t *testing.T) {
ms := &mockMetadataStore{
getFunctionByNameFn: func(ctx context.Context, name string) (*domain.Function, error) {
return &domain.Function{ID: "fn-1", Name: name}, nil
},
getVolumeFn: func(ctx context.Context, id string) (*domain.Volume, error) {
return &domain.Volume{ID: id, Name: "vol1", ImagePath: "/path/to/image"}, nil
},
saveFunctionFn: func(ctx context.Context, fn *domain.Function) error {
return fmt.Errorf("db error")
},
}
_, mux := setupVolumeHandler(t, ms)
body := `{"mounts":[{"volume_id":"vol-1","mount_path":"/data"}]}`
req := httptest.NewRequest("PUT", "/functions/hello/mounts", strings.NewReader(body))
req.Header.Set("Content-Type", "application/json")
w := httptest.NewRecorder()
mux.ServeHTTP(w, req)
expectStatus(t, w, http.StatusInternalServerError)
}

// ─── Workflow: CreateWorkflow store error ────────────────────────────────────

func TestCreateWorkflow_StoreErr2(t *testing.T) {
ws := &mockWorkflowStore{
createWorkflowFn: func(ctx context.Context, w *domain.Workflow) error {
return fmt.Errorf("db error")
},
}
_, mux := setupWorkflowHandler(t, nil, ws)
body := `{"name":"my-wf","description":"test"}`
req := httptest.NewRequest("POST", "/workflows", strings.NewReader(body))
req.Header.Set("Content-Type", "application/json")
w := httptest.NewRecorder()
mux.ServeHTTP(w, req)
expectStatus(t, w, http.StatusInternalServerError)
}
