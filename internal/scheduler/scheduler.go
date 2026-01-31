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
)

// CronEntry represents a scheduled function invocation
type CronEntry struct {
	ID         string          `json:"id"`
	FunctionID string          `json:"function_id"`
	Name       string          `json:"name"`        // Human-readable name
	Schedule   string          `json:"schedule"`    // Cron expression (simplified)
	Payload    json.RawMessage `json:"payload"`     // Payload to send
	Enabled    bool            `json:"enabled"`
	LastRun    time.Time       `json:"last_run"`
	NextRun    time.Time       `json:"next_run"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Scheduler manages cron-like scheduled function invocations
type Scheduler struct {
	store    *store.RedisStore
	executor *executor.Executor
	entries  sync.Map // id -> *CronEntry
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// New creates a new scheduler
func New(store *store.RedisStore, exec *executor.Executor) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		store:    store,
		executor: exec,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins the scheduler loop
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.runLoop()
}

// Stop halts the scheduler
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
}

// AddEntry adds a new scheduled entry
func (s *Scheduler) AddEntry(entry *CronEntry) error {
	if entry.ID == "" {
		return fmt.Errorf("entry ID is required")
	}

	nextRun, err := parseNextRun(entry.Schedule, time.Now())
	if err != nil {
		return fmt.Errorf("invalid schedule: %w", err)
	}
	entry.NextRun = nextRun
	entry.CreatedAt = time.Now()

	s.entries.Store(entry.ID, entry)
	logging.Op().Info("schedule added",
		"id", entry.ID,
		"name", entry.Name,
		"schedule", entry.Schedule,
		"next_run", entry.NextRun.Format(time.RFC3339))
	return nil
}

// RemoveEntry removes a scheduled entry
func (s *Scheduler) RemoveEntry(id string) {
	s.entries.Delete(id)
}

// GetEntry returns an entry by ID
func (s *Scheduler) GetEntry(id string) (*CronEntry, bool) {
	v, ok := s.entries.Load(id)
	if !ok {
		return nil, false
	}
	return v.(*CronEntry), true
}

// ListEntries returns all scheduled entries
func (s *Scheduler) ListEntries() []*CronEntry {
	var entries []*CronEntry
	s.entries.Range(func(key, value interface{}) bool {
		entries = append(entries, value.(*CronEntry))
		return true
	})
	return entries
}

func (s *Scheduler) runLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			s.checkAndRun(now)
		}
	}
}

func (s *Scheduler) checkAndRun(now time.Time) {
	s.entries.Range(func(key, value interface{}) bool {
		entry := value.(*CronEntry)
		if !entry.Enabled {
			return true
		}

		if now.After(entry.NextRun) || now.Equal(entry.NextRun) {
			go s.runEntry(entry)

			// Calculate next run
			nextRun, err := parseNextRun(entry.Schedule, now)
			if err == nil {
				entry.NextRun = nextRun
			}
			entry.LastRun = now
		}
		return true
	})
}

func (s *Scheduler) runEntry(entry *CronEntry) {
	fn, err := s.store.GetFunction(context.Background(), entry.FunctionID)
	if err != nil {
		logging.Op().Error("scheduler: get function failed",
			"function_id", entry.FunctionID,
			"error", err)
		return
	}

	payload := entry.Payload
	if payload == nil {
		payload = json.RawMessage("{}")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(fn.TimeoutS)*time.Second)
	defer cancel()

	logging.Op().Info("scheduler: running",
		"name", entry.Name,
		"function", fn.Name)
	resp, err := s.executor.Invoke(ctx, fn.Name, payload)
	if err != nil {
		logging.Op().Error("scheduler: invocation failed",
			"function", fn.Name,
			"error", err)
		return
	}

	if resp.Error != "" {
		logging.Op().Error("scheduler: function error",
			"function", fn.Name,
			"error", resp.Error)
	} else {
		logging.Op().Info("scheduler: completed",
			"function", fn.Name,
			"duration_ms", resp.DurationMs,
			"cold_start", resp.ColdStart)
	}
}

// parseNextRun parses a simplified cron expression and returns the next run time.
// Supported formats:
//   - @every <duration>  (e.g., "@every 5m", "@every 1h")
//   - @hourly            (every hour at minute 0)
//   - @daily             (every day at midnight)
//   - * * * * *          (standard cron: min hour dom month dow)
//   - Simple intervals:  "5m", "1h", "30s"
func parseNextRun(schedule string, after time.Time) (time.Time, error) {
	schedule = trimSpace(schedule)

	// Handle @every syntax
	if len(schedule) > 7 && schedule[:7] == "@every " {
		durationStr := schedule[7:]
		d, err := time.ParseDuration(durationStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid duration: %s", durationStr)
		}
		return after.Add(d), nil
	}

	// Handle predefined schedules
	switch schedule {
	case "@hourly":
		next := after.Truncate(time.Hour).Add(time.Hour)
		return next, nil
	case "@daily":
		next := time.Date(after.Year(), after.Month(), after.Day()+1, 0, 0, 0, 0, after.Location())
		return next, nil
	case "@midnight":
		next := time.Date(after.Year(), after.Month(), after.Day()+1, 0, 0, 0, 0, after.Location())
		return next, nil
	}

	// Try parsing as simple duration (e.g., "5m", "1h")
	if d, err := time.ParseDuration(schedule); err == nil {
		return after.Add(d), nil
	}

	// TODO: Implement full cron expression parsing
	// For now, return error for unsupported formats
	return time.Time{}, fmt.Errorf("unsupported schedule format: %s (use @every, @hourly, @daily, or duration like 5m)", schedule)
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// ScheduleInfo returns statistics about the scheduler
type ScheduleInfo struct {
	TotalEntries   int       `json:"total_entries"`
	EnabledEntries int       `json:"enabled_entries"`
	NextRun        time.Time `json:"next_run,omitempty"`
	Entries        []*CronEntry `json:"entries"`
}

func (s *Scheduler) Info() ScheduleInfo {
	entries := s.ListEntries()
	info := ScheduleInfo{
		TotalEntries: len(entries),
		Entries:      entries,
	}

	var nextRun time.Time
	for _, e := range entries {
		if e.Enabled {
			info.EnabledEntries++
			if nextRun.IsZero() || e.NextRun.Before(nextRun) {
				nextRun = e.NextRun
			}
		}
	}
	info.NextRun = nextRun

	return info
}
