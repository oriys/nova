package idempotency

import (
	"sync"
	"time"
)

// Lease represents ownership of a task execution.
type Lease struct {
	Key        string    `json:"key"`
	Owner      string    `json:"owner"` // Worker ID
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Heartbeat  time.Time `json:"heartbeat"`
}

// LeaseManager manages task execution leases for exactly-once semantics.
// If a worker dies mid-execution, its lease expires and another worker can reclaim.
type LeaseManager struct {
	mu     sync.Mutex
	leases map[string]*Lease
	ttl    time.Duration
}

// NewLeaseManager creates a new lease manager with the given lease TTL.
func NewLeaseManager(ttl time.Duration) *LeaseManager {
	lm := &LeaseManager{
		leases: make(map[string]*Lease),
		ttl:    ttl,
	}
	go lm.expiryLoop()
	return lm
}

// Acquire attempts to acquire a lease for the given key.
// Returns the lease if acquired, nil if already held by another owner.
func (lm *LeaseManager) Acquire(key, owner string) *Lease {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	now := time.Now()
	if existing, ok := lm.leases[key]; ok {
		// Check if lease has expired
		if now.Before(existing.ExpiresAt) {
			if existing.Owner == owner {
				// Same owner, refresh
				existing.Heartbeat = now
				existing.ExpiresAt = now.Add(lm.ttl)
				return existing
			}
			return nil // Still held by another worker
		}
		// Lease expired, reclaim
	}

	lease := &Lease{
		Key:        key,
		Owner:      owner,
		AcquiredAt: now,
		ExpiresAt:  now.Add(lm.ttl),
		Heartbeat:  now,
	}
	lm.leases[key] = lease
	return lease
}

// Heartbeat refreshes a lease to prevent expiry.
func (lm *LeaseManager) Heartbeat(key, owner string) bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lease, ok := lm.leases[key]
	if !ok || lease.Owner != owner {
		return false
	}

	now := time.Now()
	lease.Heartbeat = now
	lease.ExpiresAt = now.Add(lm.ttl)
	return true
}

// Release explicitly releases a lease.
func (lm *LeaseManager) Release(key, owner string) bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lease, ok := lm.leases[key]
	if !ok || lease.Owner != owner {
		return false
	}

	delete(lm.leases, key)
	return true
}

// IsHeld returns true if the key has an active (non-expired) lease.
func (lm *LeaseManager) IsHeld(key string) bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lease, ok := lm.leases[key]
	return ok && time.Now().Before(lease.ExpiresAt)
}

func (lm *LeaseManager) expiryLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		lm.mu.Lock()
		now := time.Now()
		for key, lease := range lm.leases {
			if now.After(lease.ExpiresAt) {
				delete(lm.leases, key)
			}
		}
		lm.mu.Unlock()
	}
}
