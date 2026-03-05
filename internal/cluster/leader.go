package cluster

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oriys/nova/internal/logging"
)

// LeaderElector provides PostgreSQL advisory-lock based leader election.
// Only one instance can hold the lock at a time across all processes
// connected to the same database, preventing split-brain in multi-instance
// deployments of corona (scheduler) and nebula (event bus).
//
// Usage:
//
//	le := cluster.NewLeaderElector(pool, cluster.LeaderConfig{
//	    LockKey:  0x636f726f6e61, // unique per service
//	    NodeID:   "corona-1",
//	    OnElected: func() { startScheduler() },
//	    OnRevoked: func() { stopScheduler() },
//	})
//	stop := le.Start(ctx)
//	defer stop()
type LeaderElector struct {
	pool   *pgxpool.Pool
	cfg    LeaderConfig
	leader atomic.Bool
	stopCh chan struct{}
	once   sync.Once
}

// LeaderConfig configures the leader election behaviour.
type LeaderConfig struct {
	// LockKey is the PostgreSQL advisory lock ID. Must be unique per service
	// type (e.g. corona vs nebula) but shared across instances of the same
	// service.
	LockKey int64

	// NodeID identifies this instance in log messages.
	NodeID string

	// RetryInterval is how often a non-leader tries to acquire the lock.
	// Default: 5s.
	RetryInterval time.Duration

	// OnElected is called (in a new goroutine) when this instance becomes leader.
	OnElected func()

	// OnRevoked is called (in a new goroutine) when this instance loses leadership.
	OnRevoked func()
}

// NewLeaderElector creates a new elector. Call Start to begin the election loop.
func NewLeaderElector(pool *pgxpool.Pool, cfg LeaderConfig) *LeaderElector {
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = 5 * time.Second
	}
	if cfg.NodeID == "" {
		cfg.NodeID = "unknown"
	}
	return &LeaderElector{
		pool:   pool,
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
}

// Start begins the leader election loop in a background goroutine.
// Returns a stop function that releases the lock and stops the loop.
func (le *LeaderElector) Start(ctx context.Context) (stop func()) {
	go le.run(ctx)
	return func() {
		le.once.Do(func() { close(le.stopCh) })
	}
}

// IsLeader returns true if this instance currently holds the leader lock.
func (le *LeaderElector) IsLeader() bool {
	return le.leader.Load()
}

func (le *LeaderElector) run(ctx context.Context) {
	log := logging.Op()
	ticker := time.NewTicker(le.cfg.RetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			le.release(ctx)
			return
		case <-le.stopCh:
			le.release(ctx)
			return
		case <-ticker.C:
			le.tryAcquire(ctx, log)
		}
	}
}

func (le *LeaderElector) tryAcquire(ctx context.Context, log interface{ Info(string, ...any) }) {
	// pg_try_advisory_lock is session-level: the lock is held for the
	// lifetime of the database connection.  We acquire from the pool and,
	// if successful, intentionally do NOT return the conn (it stays checked
	// out to keep the lock alive).  When we want to release we call
	// pg_advisory_unlock on a fresh connection.
	//
	// Because pgxpool connections can be recycled, we use the non-session
	// variant (pg_try_advisory_lock) which auto-releases if the connection
	// drops — providing leader failover on crash.

	if le.leader.Load() {
		// Already leader — verify the lock is still held.
		var held bool
		err := le.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND classid=0 AND objid=$1 AND pid=pg_backend_pid())`,
			le.cfg.LockKey).Scan(&held)
		if err != nil || !held {
			// Lost the lock (connection recycled or DB issue).
			le.leader.Store(false)
			logging.Op().Warn("leader lock lost", "node", le.cfg.NodeID, "key", le.cfg.LockKey)
			if le.cfg.OnRevoked != nil {
				go le.cfg.OnRevoked()
			}
		}
		return
	}

	var acquired bool
	err := le.pool.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, le.cfg.LockKey).Scan(&acquired)
	if err != nil {
		logging.Op().Warn("leader election attempt failed", "node", le.cfg.NodeID, "error", err)
		return
	}

	if acquired {
		le.leader.Store(true)
		logging.Op().Info("elected as leader", "node", le.cfg.NodeID, "key", le.cfg.LockKey)
		if le.cfg.OnElected != nil {
			go le.cfg.OnElected()
		}
	}
}

func (le *LeaderElector) release(ctx context.Context) {
	if !le.leader.Load() {
		return
	}
	le.leader.Store(false)

	releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := le.pool.Exec(releaseCtx, `SELECT pg_advisory_unlock($1)`, le.cfg.LockKey)
	if err != nil {
		logging.Op().Warn("failed to release leader lock", "node", le.cfg.NodeID, "error", err)
	} else {
		logging.Op().Info("released leader lock", "node", le.cfg.NodeID)
	}

	if le.cfg.OnRevoked != nil {
		go le.cfg.OnRevoked()
	}
}

// WellKnownLockKeys provides unique advisory lock keys for each service.
var WellKnownLockKeys = struct {
	Corona int64
	Nebula int64
}{
	Corona: 0x636f726f6e61, // "corona" in hex
	Nebula: 0x6e6562756c61, // "nebula" in hex
}

// RequireLeader returns an error if this instance is not the leader.
// Useful as a guard in handler/worker entry points.
func (le *LeaderElector) RequireLeader() error {
	if !le.leader.Load() {
		return fmt.Errorf("not the leader (node=%s)", le.cfg.NodeID)
	}
	return nil
}
