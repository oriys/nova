package jobtracker

import (
	"sync"
	"time"
)

// Progress represents the current progress of a long-running job.
type Progress struct {
	JobID      string    `json:"job_id"`
	Percent    int       `json:"percent"`     // 0-100
	Message    string    `json:"message"`     // Human-readable status message
	Phase      string    `json:"phase"`       // Current phase (e.g., "initializing", "processing", "finalizing")
	UpdatedAt  time.Time `json:"updated_at"`
	HeartbeatAt time.Time `json:"heartbeat_at"`
}

// Tracker maintains in-memory progress for long-running async jobs.
// It is designed to be lightweight and used alongside the persistent
// async invocation store.
type Tracker struct {
	mu       sync.RWMutex
	progress map[string]*Progress // job ID -> progress
	ttl      time.Duration        // how long to keep completed/stale entries
	maxSize  int                  // hard cap on tracked entries (0 = unlimited)
}

// New creates a new job progress tracker.
func New(ttl time.Duration) *Tracker {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	t := &Tracker{
		progress: make(map[string]*Progress),
		ttl:      ttl,
		maxSize:  10000,
	}
	go t.cleanupLoop()
	return t
}

// Update sets the progress for a job.
func (t *Tracker) Update(jobID string, percent int, message, phase string) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()

	p, ok := t.progress[jobID]
	if !ok {
		// Enforce max size limit
		if t.maxSize > 0 && len(t.progress) >= t.maxSize {
			return
		}
		p = &Progress{JobID: jobID}
		t.progress[jobID] = p
	}
	p.Percent = percent
	p.Message = message
	p.Phase = phase
	p.UpdatedAt = now
	p.HeartbeatAt = now
}

// Heartbeat updates the heartbeat timestamp without changing progress.
func (t *Tracker) Heartbeat(jobID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if p, ok := t.progress[jobID]; ok {
		p.HeartbeatAt = time.Now()
	}
}

// Get returns the progress for a job, or nil if not tracked.
func (t *Tracker) Get(jobID string) *Progress {
	t.mu.RLock()
	defer t.mu.RUnlock()

	p, ok := t.progress[jobID]
	if !ok {
		return nil
	}
	// Return a copy
	cp := *p
	return &cp
}

// Remove deletes the progress entry for a job.
func (t *Tracker) Remove(jobID string) {
	t.mu.Lock()
	delete(t.progress, jobID)
	t.mu.Unlock()
}

// IsStale returns true if the job's heartbeat is older than the given timeout.
func (t *Tracker) IsStale(jobID string, timeout time.Duration) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	p, ok := t.progress[jobID]
	if !ok {
		return true
	}
	return time.Since(p.HeartbeatAt) > timeout
}

// ListActive returns all currently tracked progress entries.
func (t *Tracker) ListActive() []*Progress {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make([]*Progress, 0, len(t.progress))
	for _, p := range t.progress {
		cp := *p
		out = append(out, &cp)
	}
	return out
}

// cleanupLoop periodically removes stale progress entries.
func (t *Tracker) cleanupLoop() {
	ticker := time.NewTicker(t.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		t.mu.Lock()
		now := time.Now()
		for id, p := range t.progress {
			if now.Sub(p.HeartbeatAt) > t.ttl {
				delete(t.progress, id)
			}
		}
		t.mu.Unlock()
	}
}
