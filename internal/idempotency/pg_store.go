package idempotency

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgStore is a PostgreSQL-backed idempotency store suitable for multi-node
// deployments. It uses the pre-existing idempotency_store table.
type PgStore struct {
	pool *pgxpool.Pool
	cfg  Config
	ctx  context.Context
	stop context.CancelFunc
}

// NewPgStore creates a Postgres-backed idempotency store.
func NewPgStore(pool *pgxpool.Pool, cfg Config) *PgStore {
	ctx, cancel := context.WithCancel(context.Background())
	s := &PgStore{
		pool: pool,
		cfg:  cfg,
		ctx:  ctx,
		stop: cancel,
	}
	go s.cleanupLoop()
	return s
}

// Check looks up an idempotency key. If the key is new, it claims it atomically.
func (s *PgStore) Check(ctx context.Context, key string, workerID string) CheckResult {
	now := time.Now()

	// Try to insert (claim) the key. ON CONFLICT returns the existing row.
	var status Status
	var result json.RawMessage
	var errMsg *string

	err := s.pool.QueryRow(ctx, `
		INSERT INTO idempotency_store (key, status, claimed_by, claimed_at, expires_at)
		VALUES ($1, 'claimed', $2, $3, $4)
		ON CONFLICT (key) DO UPDATE
			SET claimed_by = CASE
				WHEN idempotency_store.status = 'claimed'
				 AND idempotency_store.claimed_at < $5
				THEN EXCLUDED.claimed_by
				ELSE idempotency_store.claimed_by
			END,
			claimed_at = CASE
				WHEN idempotency_store.status = 'claimed'
				 AND idempotency_store.claimed_at < $5
				THEN EXCLUDED.claimed_at
				ELSE idempotency_store.claimed_at
			END
		RETURNING status, result, error_msg`,
		key, workerID, now, now.Add(s.cfg.DefaultTTL),
		now.Add(-s.cfg.ClaimTimeout),
	).Scan(&status, &result, &errMsg)

	if err != nil {
		// On error, treat as miss to allow execution.
		return CheckResult{Hit: false}
	}

	switch status {
	case StatusCompleted:
		return CheckResult{Hit: true, Status: StatusCompleted, Result: result}
	case StatusFailed:
		e := ""
		if errMsg != nil {
			e = *errMsg
		}
		return CheckResult{Hit: true, Status: StatusFailed, Error: e}
	case StatusClaimed:
		// If we just claimed it (our workerID), it's a miss (proceed with execution).
		var claimedBy string
		_ = s.pool.QueryRow(ctx, `SELECT claimed_by FROM idempotency_store WHERE key = $1`, key).Scan(&claimedBy)
		if claimedBy == workerID {
			return CheckResult{Hit: false}
		}
		return CheckResult{Hit: true, Status: StatusClaimed}
	}

	return CheckResult{Hit: false}
}

// Complete marks an idempotency key as completed with the given result.
func (s *PgStore) Complete(key string, result json.RawMessage) error {
	now := time.Now()
	tag, err := s.pool.Exec(context.Background(), `
		UPDATE idempotency_store
		SET status = 'completed', result = $2, completed_at = $3, expires_at = $4
		WHERE key = $1`,
		key, result, now, now.Add(s.cfg.DefaultTTL))
	if err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("idempotency key not found: %s", key)
	}
	return nil
}

// Fail marks an idempotency key as failed (retryable).
func (s *PgStore) Fail(key string, errMsg string) error {
	now := time.Now()
	tag, err := s.pool.Exec(context.Background(), `
		UPDATE idempotency_store
		SET status = 'failed', error_msg = $2, completed_at = $3, expires_at = $4
		WHERE key = $1`,
		key, errMsg, now, now.Add(s.cfg.DefaultTTL))
	if err != nil {
		return fmt.Errorf("fail idempotency key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("idempotency key not found: %s", key)
	}
	return nil
}

// Release removes a claim, allowing retry with the same key.
func (s *PgStore) Release(key string) {
	_, _ = s.pool.Exec(context.Background(),
		`DELETE FROM idempotency_store WHERE key = $1`, key)
}

// Stop shuts down the cleanup goroutine.
func (s *PgStore) Stop() {
	s.stop()
}

func (s *PgStore) cleanupLoop() {
	ticker := time.NewTicker(s.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.pool.Exec(s.ctx,
				`DELETE FROM idempotency_store WHERE expires_at < $1`,
				time.Now())
		}
	}
}

// Ensure PgStore and Store satisfy the same implicit interface at compile time.
var (
	_ interface {
		Check(context.Context, string, string) CheckResult
		Complete(string, json.RawMessage) error
		Fail(string, string) error
		Release(string)
		Stop()
	} = (*PgStore)(nil)
	_ interface {
		Check(context.Context, string, string) CheckResult
		Complete(string, json.RawMessage) error
		Fail(string, string) error
		Release(string)
		Stop()
	} = (*Store)(nil)
)
