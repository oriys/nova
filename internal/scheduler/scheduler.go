package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/oriys/nova/internal/executor"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
	"github.com/robfig/cron/v3"
)

// Scheduler manages cron-scheduled function invocations.
type Scheduler struct {
	cron    *cron.Cron
	store   *store.Store
	exec    executor.Invoker
	entries map[string]cron.EntryID // schedule ID -> cron entry ID
	mu      sync.Mutex
}

// New creates a new Scheduler.
func New(s *store.Store, exec executor.Invoker) *Scheduler {
	return &Scheduler{
		cron:    cron.New(cron.WithParser(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor))),
		store:   s,
		exec:    exec,
		entries: make(map[string]cron.EntryID),
	}
}

// Start loads all enabled schedules from the store and starts the cron scheduler.
func (s *Scheduler) Start() error {
	if s == nil || s.store == nil || s.store.ScheduleStore == nil {
		return fmt.Errorf("schedule store not configured")
	}

	ctx := context.Background()
	schedules, err := s.store.ListAllSchedules(ctx, 0, 0)
	if err != nil {
		return err
	}

	for _, sched := range schedules {
		if sched.Enabled {
			if err := s.Add(sched); err != nil {
				logging.Op().Warn("failed to register schedule", "id", sched.ID, "function", sched.FunctionName, "error", err)
			}
		}
	}

	s.cron.Start()
	logging.Op().Info("scheduler started", "schedules", len(schedules))
	return nil
}

// Stop stops the cron scheduler.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// Add registers a new cron entry for a schedule.
func (s *Scheduler) Add(sched *store.Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing entry if present
	if entryID, ok := s.entries[sched.ID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, sched.ID)
	}

	schedID := sched.ID
	fnName := sched.FunctionName
	input := sched.Input
	tenantID := sched.TenantID
	namespace := sched.Namespace

	entryID, err := s.cron.AddFunc(sched.CronExpr, func() {
		s.invoke(schedID, tenantID, namespace, fnName, input)
	})
	if err != nil {
		return err
	}

	s.entries[sched.ID] = entryID
	return nil
}

// Remove unregisters a cron entry.
func (s *Scheduler) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, ok := s.entries[id]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, id)
	}
}

func (s *Scheduler) invoke(schedID, tenantID, namespace, fnName string, input json.RawMessage) {
	scopedCtx := store.WithTenantScope(context.Background(), tenantID, namespace)
	ctx, cancel := context.WithTimeout(scopedCtx, 30*time.Second)
	defer cancel()

	payload := input
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	_, err := s.exec.Invoke(ctx, fnName, payload)
	if err != nil {
		logging.Op().Error("scheduled invocation failed", "schedule", schedID, "function", fnName, "error", err)
	} else {
		logging.Op().Debug("scheduled invocation succeeded", "schedule", schedID, "function", fnName)
	}

	// Update last_run_at
	if err := s.store.UpdateScheduleLastRun(scopedCtx, schedID, time.Now()); err != nil {
		logging.Op().Warn("failed to update schedule last_run", "schedule", schedID, "error", err)
	}
}
