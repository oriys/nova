package scheduler

import (
	"context"
	"encoding/json"
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
	exec    *executor.Executor
	entries map[string]cron.EntryID // schedule ID -> cron entry ID
	mu      sync.Mutex
}

// New creates a new Scheduler.
func New(s *store.Store, exec *executor.Executor) *Scheduler {
	return &Scheduler{
		cron:    cron.New(cron.WithParser(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor))),
		store:   s,
		exec:    exec,
		entries: make(map[string]cron.EntryID),
	}
}

// Start loads all enabled schedules from the store and starts the cron scheduler.
func (s *Scheduler) Start() error {
	ctx := context.Background()
	schedules, err := s.store.ListAllSchedules(ctx)
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

	entryID, err := s.cron.AddFunc(sched.CronExpr, func() {
		s.invoke(schedID, fnName, input)
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

func (s *Scheduler) invoke(schedID, fnName string, input json.RawMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	if err := s.store.UpdateScheduleLastRun(context.Background(), schedID, time.Now()); err != nil {
		logging.Op().Warn("failed to update schedule last_run", "schedule", schedID, "error", err)
	}
}
