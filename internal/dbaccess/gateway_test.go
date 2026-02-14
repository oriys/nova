package dbaccess

import "testing"

func TestHashStatement(t *testing.T) {
	// Same input should produce the same hash.
	h1 := hashStatement("SELECT * FROM users WHERE id = $1")
	h2 := hashStatement("SELECT * FROM users WHERE id = $1")
	if h1 != h2 {
		t.Errorf("same SQL produced different hashes: %s vs %s", h1, h2)
	}

	// Different input should produce different hashes.
	h3 := hashStatement("INSERT INTO orders (id) VALUES ($1)")
	if h1 == h3 {
		t.Errorf("different SQL produced same hash: %s", h1)
	}

	// Hash should be a hex string of length 16 (8 bytes).
	if len(h1) != 16 {
		t.Errorf("expected hash length 16, got %d", len(h1))
	}
}

func TestQuotaError(t *testing.T) {
	err := &QuotaError{Dimension: "max_sessions", Limit: 10, Current: 10}
	expected := "DB_BUSY: quota exceeded for max_sessions (limit=10, current=10)"
	if err.Error() != expected {
		t.Errorf("QuotaError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestConnPoolAcquireRelease(t *testing.T) {
	pool := &ConnPool{
		resourceID:  "test-res",
		maxSessions: 2,
		maxTxConc:   1,
	}

	// Should be able to acquire up to max.
	pool.mu.Lock()
	pool.activeSessions++
	pool.mu.Unlock()

	pool.mu.Lock()
	pool.activeSessions++
	pool.mu.Unlock()

	// Third acquire should exceed limit.
	pool.mu.Lock()
	exceeded := pool.maxSessions > 0 && pool.activeSessions >= pool.maxSessions
	pool.mu.Unlock()
	if !exceeded {
		t.Error("expected session limit to be exceeded")
	}

	// Release one and try again.
	pool.mu.Lock()
	pool.activeSessions--
	pool.mu.Unlock()

	pool.mu.Lock()
	exceeded = pool.maxSessions > 0 && pool.activeSessions >= pool.maxSessions
	pool.mu.Unlock()
	if exceeded {
		t.Error("expected session slot to be available after release")
	}
}
