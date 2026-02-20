// Package circuitbreaker implements the per-function circuit breaker that
// protects the invocation pipeline from cascading failures.
//
// # State machine
//
// The breaker follows the standard three-state model:
//
//	Closed ──(error rate ≥ threshold)──► Open ──(OpenDuration elapsed)──► HalfOpen
//	  ▲                                                                        │
//	  └──────────────(all probes succeed)───────────────────────────────────────┘
//	                  (any probe fails) ──────────────────────────────────► Open
//
// # Why sliding window, not counters
//
// A fixed counter resets on schedule regardless of traffic volume, which
// means a burst of errors just before a reset window is silently lost.
// A sliding window always reflects the last WindowDuration of traffic, so
// the error rate is meaningful even under irregular load patterns.
//
// # Concurrency
//
// All public methods (Allow, RecordSuccess, RecordFailure, State) are safe
// for concurrent use; they acquire the internal mutex for every call.
// The Registry uses a separate read-write mutex so that the common
// read path (Get for an existing breaker) does not contend with the rare
// write path (new function registered or deleted).
//
// # Invariants
//
//   - The successes and failures slices contain only timestamps within the
//     current sliding window; trimWindow is called after every write.
//   - maxWindowEntries caps both slices to prevent unbounded memory growth
//     under pathological load (e.g. thousands of errors per second).
//   - halfOpenProbes counts the number of probe requests dispatched in the
//     HalfOpen state; it is reset to 0 on every Open→HalfOpen transition.
package circuitbreaker

import (
	"sync"
	"time"
)

// State represents the circuit breaker state.
type State int

const (
	StateClosed   State = iota // Normal operation, requests pass through
	StateOpen                  // Requests are rejected
	StateHalfOpen              // Limited probe requests are allowed
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// Config holds the circuit breaker configuration.
// These fields map to CapacityPolicy's reserved breaker fields.
type Config struct {
	ErrorPct       float64       // Error percentage threshold to trip the breaker (0-100)
	WindowDuration time.Duration // Sliding window for error rate calculation
	OpenDuration   time.Duration // How long the breaker stays open before transitioning to half-open
	HalfOpenProbes int           // Number of probe requests allowed in half-open state
}

// Breaker is a per-function circuit breaker.
type Breaker struct {
	mu             sync.Mutex
	cfg            Config
	state          State
	successes      []time.Time // timestamps of recent successes within window
	failures       []time.Time // timestamps of recent failures within window
	openedAt       time.Time   // when the breaker transitioned to open
	halfOpenProbes int         // number of probes allowed so far in half-open
	halfOpenOK     int         // number of successful probes in half-open
}

// New creates a new circuit breaker with the given configuration.
func New(cfg Config) *Breaker {
	if cfg.HalfOpenProbes <= 0 {
		cfg.HalfOpenProbes = 1
	}
	return &Breaker{
		cfg: cfg,
	}
}

// Allow checks whether a request should be allowed through the breaker.
// Returns true if the request is permitted.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(b.openedAt) >= b.cfg.OpenDuration {
			b.state = StateHalfOpen
			b.halfOpenProbes = 0
			b.halfOpenOK = 0
			b.halfOpenProbes++
			return true
		}
		return false
	case StateHalfOpen:
		if b.halfOpenProbes < b.cfg.HalfOpenProbes {
			b.halfOpenProbes++
			return true
		}
		return false
	}
	return true
}

// RecordSuccess records a successful invocation.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	switch b.state {
	case StateClosed:
		b.successes = append(b.successes, now)
		b.trimWindow(now)
	case StateHalfOpen:
		b.halfOpenOK++
		if b.halfOpenOK >= b.cfg.HalfOpenProbes {
			// All probes succeeded, close the breaker
			b.state = StateClosed
			b.successes = b.successes[:0]
			b.failures = b.failures[:0]
		}
	}
}

// RecordFailure records a failed invocation.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	switch b.state {
	case StateClosed:
		b.failures = append(b.failures, now)
		b.trimWindow(now)
		b.checkThreshold(now)
	case StateHalfOpen:
		// Probe failed, reopen immediately
		b.state = StateOpen
		b.openedAt = now
	}
}

// State returns the current breaker state.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check for automatic transition from open to half-open
	if b.state == StateOpen && time.Since(b.openedAt) >= b.cfg.OpenDuration {
		b.state = StateHalfOpen
		b.halfOpenProbes = 0
		b.halfOpenOK = 0
	}
	return b.state
}

// maxWindowEntries is a hard cap on sliding window entries to prevent memory exhaustion.
const maxWindowEntries = 10000

// trimWindow removes entries outside the sliding window. Must be called under lock.
func (b *Breaker) trimWindow(now time.Time) {
	cutoff := now.Add(-b.cfg.WindowDuration)
	b.successes = trimBefore(b.successes, cutoff)
	b.failures = trimBefore(b.failures, cutoff)

	// Hard cap to prevent memory exhaustion under extreme load
	if len(b.successes) > maxWindowEntries {
		b.successes = b.successes[len(b.successes)-maxWindowEntries:]
	}
	if len(b.failures) > maxWindowEntries {
		b.failures = b.failures[len(b.failures)-maxWindowEntries:]
	}
}

// checkThreshold trips the breaker if error rate exceeds the configured threshold. Must be called under lock.
func (b *Breaker) checkThreshold(now time.Time) {
	total := len(b.successes) + len(b.failures)
	if total == 0 {
		return
	}
	errorPct := float64(len(b.failures)) / float64(total) * 100
	if errorPct >= b.cfg.ErrorPct {
		b.state = StateOpen
		b.openedAt = now
	}
}

// trimBefore removes timestamps before the cutoff time.
func trimBefore(times []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(times) && times[i].Before(cutoff) {
		i++
	}
	if i == 0 {
		return times
	}
	copy(times, times[i:])
	return times[:len(times)-i]
}

// Registry holds per-function circuit breakers.
type Registry struct {
	mu       sync.RWMutex
	breakers map[string]*Breaker
}

// NewRegistry creates a new breaker registry.
func NewRegistry() *Registry {
	return &Registry{
		breakers: make(map[string]*Breaker),
	}
}

// Get returns the breaker for a function, creating one if the config is valid.
// Returns nil if circuit breaking is not configured for this function.
func (r *Registry) Get(funcID string, cfg Config) *Breaker {
	if cfg.ErrorPct <= 0 || cfg.WindowDuration <= 0 || cfg.OpenDuration <= 0 {
		return nil
	}

	r.mu.RLock()
	b, ok := r.breakers[funcID]
	r.mu.RUnlock()
	if ok {
		return b
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// Double check
	if b, ok := r.breakers[funcID]; ok {
		return b
	}
	b = New(cfg)
	r.breakers[funcID] = b
	return b
}

// Remove deletes the breaker for a function (e.g., when function is deleted).
func (r *Registry) Remove(funcID string) {
	r.mu.Lock()
	delete(r.breakers, funcID)
	r.mu.Unlock()
}

// Snapshot returns a map of function ID to breaker state for observability.
func (r *Registry) Snapshot() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.breakers))
	for id, b := range r.breakers {
		out[id] = b.State().String()
	}
	return out
}
