package slo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

const (
	defaultInterval   = 30 * time.Second
	defaultWindowS    = 900
	defaultMinSamples = 20
)

// Config controls SLO evaluation behavior.
type Config struct {
	Enabled           bool
	Interval          time.Duration
	DefaultWindowS    int
	DefaultMinSamples int
}

type alertState struct {
	active bool
}

// Service periodically evaluates function SLOs and emits in-app notifications.
type Service struct {
	store *store.Store
	cfg   Config

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}

	state sync.Map // key => alertState
}

// New creates an SLO service.
func New(s *store.Store, cfg Config) *Service {
	if cfg.Interval <= 0 {
		cfg.Interval = defaultInterval
	}
	if cfg.DefaultWindowS <= 0 {
		cfg.DefaultWindowS = defaultWindowS
	}
	if cfg.DefaultMinSamples <= 0 {
		cfg.DefaultMinSamples = defaultMinSamples
	}
	return &Service{
		store:  s,
		cfg:    cfg,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start begins periodic SLO evaluation.
func (s *Service) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running || !s.cfg.Enabled || s.store == nil {
		return
	}
	s.running = true
	go s.loop()
}

// Stop terminates the evaluation loop.
func (s *Service) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	close(s.stopCh)
	done := s.doneCh
	s.running = false
	s.mu.Unlock()
	<-done
}

func (s *Service) loop() {
	defer close(s.doneCh)
	s.evaluateAll(context.Background())

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.evaluateAll(context.Background())
		}
	}
}

func (s *Service) evaluateAll(ctx context.Context) {
	tenants, err := s.store.ListTenants(ctx, 0, 0)
	if err != nil {
		logging.Op().Warn("slo: list tenants failed", "error", err)
		return
	}

	for _, tenant := range tenants {
		namespaces, err := s.store.ListNamespaces(ctx, tenant.ID, 0, 0)
		if err != nil {
			logging.Op().Warn("slo: list namespaces failed", "tenant_id", tenant.ID, "error", err)
			continue
		}
		for _, namespace := range namespaces {
			scopeCtx := store.WithTenantScope(ctx, tenant.ID, namespace.Name)
			funcs, err := s.store.ListFunctions(scopeCtx, 0, 0)
			if err != nil {
				logging.Op().Warn("slo: list functions failed", "tenant_id", tenant.ID, "namespace", namespace.Name, "error", err)
				continue
			}
			for _, fn := range funcs {
				s.evaluateFunction(scopeCtx, fn)
			}
		}
	}
}

func (s *Service) evaluateFunction(ctx context.Context, fn *domain.Function) {
	if fn == nil {
		return
	}

	key := stateKey(ctx, fn.ID)
	if fn.SLOPolicy == nil || !fn.SLOPolicy.Enabled {
		s.state.Delete(key)
		return
	}

	policy := *fn.SLOPolicy
	windowS := policy.WindowS
	if windowS <= 0 {
		windowS = s.cfg.DefaultWindowS
	}
	minSamples := policy.MinSamples
	if minSamples <= 0 {
		minSamples = s.cfg.DefaultMinSamples
	}

	snapshot, err := s.store.GetFunctionSLOSnapshot(ctx, fn.ID, windowS)
	if err != nil {
		logging.Op().Warn("slo: snapshot failed", "function", fn.Name, "error", err)
		return
	}
	if snapshot == nil || snapshot.TotalInvocations < int64(minSamples) {
		return
	}

	breaches := make([]string, 0, 3)
	if policy.Objectives.SuccessRatePct > 0 && snapshot.SuccessRatePct < policy.Objectives.SuccessRatePct {
		breaches = append(breaches, "success_rate")
	}
	if policy.Objectives.P95DurationMs > 0 && snapshot.P95DurationMs > policy.Objectives.P95DurationMs {
		breaches = append(breaches, "p95_latency")
	}
	if policy.Objectives.ColdStartRatePct > 0 && snapshot.ColdStartRatePct > policy.Objectives.ColdStartRatePct {
		breaches = append(breaches, "cold_start_rate")
	}

	_, wasActive := s.state.Load(key)
	isBreach := len(breaches) > 0
	if isBreach && !wasActive {
		if err := s.createNotification(ctx, fn, &policy, snapshot, breaches, true); err != nil {
			logging.Op().Warn("slo: create breach notification failed", "function", fn.Name, "error", err)
		}
		s.state.Store(key, alertState{active: true})
		return
	}
	if !isBreach && wasActive {
		if err := s.createNotification(ctx, fn, &policy, snapshot, breaches, false); err != nil {
			logging.Op().Warn("slo: create recovery notification failed", "function", fn.Name, "error", err)
		}
		s.state.Delete(key)
	}
}

func (s *Service) createNotification(
	ctx context.Context,
	fn *domain.Function,
	policy *domain.SLOPolicy,
	snapshot *store.FunctionSLOSnapshot,
	breaches []string,
	isBreach bool,
) error {
	if !shouldSendInApp(policy.Notifications) {
		return nil
	}

	status := "recovered"
	severity := "info"
	title := fmt.Sprintf("SLO recovered: %s", fn.Name)
	message := fmt.Sprintf(
		"Window %ds, success %.2f%%, P95 %dms, cold start %.2f%%.",
		snapshot.WindowSeconds,
		snapshot.SuccessRatePct,
		snapshot.P95DurationMs,
		snapshot.ColdStartRatePct,
	)
	if isBreach {
		status = "breach"
		severity = "warning"
		title = fmt.Sprintf("SLO breached: %s", fn.Name)
		message = fmt.Sprintf(
			"Breach on %s. Window %ds, success %.2f%%, P95 %dms, cold start %.2f%%.",
			strings.Join(breaches, ", "),
			snapshot.WindowSeconds,
			snapshot.SuccessRatePct,
			snapshot.P95DurationMs,
			snapshot.ColdStartRatePct,
		)
	}

	payload, _ := json.Marshal(map[string]any{
		"status":      status,
		"function":    fn.Name,
		"function_id": fn.ID,
		"breaches":    breaches,
		"policy":      policy,
		"snapshot":    snapshot,
	})

	return s.store.CreateNotification(ctx, &store.NotificationRecord{
		ID:           uuid.New().String(),
		Type:         "slo_alert",
		Severity:     severity,
		Source:       "slo",
		FunctionID:   fn.ID,
		FunctionName: fn.Name,
		Title:        title,
		Message:      message,
		Data:         payload,
	})
}

func stateKey(ctx context.Context, functionID string) string {
	scope := store.TenantScopeFromContext(ctx)
	return scope.TenantID + "/" + scope.Namespace + "/" + functionID
}

func shouldSendInApp(targets []domain.SLONotificationTarget) bool {
	if len(targets) == 0 {
		return true
	}
	for _, t := range targets {
		kind := strings.ToLower(strings.TrimSpace(t.Type))
		if kind == "" || kind == "in_app" || kind == "ui" || kind == "bell" {
			return true
		}
	}
	return false
}
