package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Async invocation status values.
type AsyncInvocationStatus string

const (
	AsyncInvocationStatusQueued    AsyncInvocationStatus = "queued"
	AsyncInvocationStatusRunning   AsyncInvocationStatus = "running"
	AsyncInvocationStatusSucceeded AsyncInvocationStatus = "succeeded"
	AsyncInvocationStatusDLQ       AsyncInvocationStatus = "dlq"
	AsyncInvocationStatusPaused    AsyncInvocationStatus = "paused"
)

const (
	DefaultAsyncMaxAttempts    = 3
	DefaultAsyncBackoffBase    = 1000  // 1s
	DefaultAsyncBackoffMax     = 60000 // 60s
	DefaultAsyncListLimit      = 50
	MaxAsyncListLimit          = 500
	DefaultAsyncLeaseTimeout   = 30 * time.Second
	DefaultAsyncIdempotencyTTL = 24 * time.Hour
	MaxAsyncIdempotencyTTL     = 7 * 24 * time.Hour
)

var ErrAsyncInvocationNotFound = errors.New("async invocation not found")
var ErrAsyncInvocationNotDLQ = errors.New("async invocation is not in dlq")
var ErrAsyncInvocationNotQueued = errors.New("async invocation is not in queued status")
var ErrAsyncInvocationNotPaused = errors.New("async invocation is not in paused status")
var ErrAsyncInvocationNotDeletable = errors.New("async invocation cannot be deleted in current status")
var ErrInvalidIdempotencyKey = errors.New("invalid idempotency key")

// AsyncInvocation is a durable async function execution request.
type AsyncInvocation struct {
	ID            string                `json:"id"`
	TenantID      string                `json:"tenant_id,omitempty"`
	Namespace     string                `json:"namespace,omitempty"`
	FunctionID    string                `json:"function_id"`
	FunctionName  string                `json:"function_name"`
	WorkflowID    string                `json:"workflow_id,omitempty"`
	WorkflowName  string                `json:"workflow_name,omitempty"`
	Payload       json.RawMessage       `json:"payload"`
	Status        AsyncInvocationStatus `json:"status"`
	Attempt       int                   `json:"attempt"`
	MaxAttempts   int                   `json:"max_attempts"`
	BackoffBaseMS int                   `json:"backoff_base_ms"`
	BackoffMaxMS  int                   `json:"backoff_max_ms"`
	NextRunAt     time.Time             `json:"next_run_at"`
	LockedBy      string                `json:"locked_by,omitempty"`
	LockedUntil   *time.Time            `json:"locked_until,omitempty"`
	RequestID     string                `json:"request_id,omitempty"`
	Output        json.RawMessage       `json:"output,omitempty"`
	DurationMS    int64                 `json:"duration_ms,omitempty"`
	ColdStart     bool                  `json:"cold_start"`
	LastError     string                `json:"last_error,omitempty"`
	StartedAt     *time.Time            `json:"started_at,omitempty"`
	CompletedAt   *time.Time            `json:"completed_at,omitempty"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

// AsyncInvocationSummary describes queue backlog and consume throughput in the current scope.
type AsyncInvocationSummary struct {
	Total                   int64     `json:"total"`
	Queued                  int64     `json:"queued"`
	Running                 int64     `json:"running"`
	Paused                  int64     `json:"paused"`
	Succeeded               int64     `json:"succeeded"`
	DLQ                     int64     `json:"dlq"`
	Backlog                 int64     `json:"backlog"`
	Pending                 int64     `json:"pending"`
	ConsumedLast1m          int64     `json:"consumed_last_1m"`
	ConsumedLast5m          int64     `json:"consumed_last_5m"`
	ConsumeRatePerSec1m     float64   `json:"consume_rate_per_sec_1m"`
	ConsumeRatePerSec5m     float64   `json:"consume_rate_per_sec_5m"`
	ConsumeRatePerMinute1m  float64   `json:"consume_rate_per_minute_1m"`
	ConsumeRatePerMinute5m  float64   `json:"consume_rate_per_minute_5m"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// NewAsyncInvocation builds a queued async invocation request with defaults.
func NewAsyncInvocation(functionID, functionName string, payload json.RawMessage) *AsyncInvocation {
	now := time.Now().UTC()
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	return &AsyncInvocation{
		ID:            uuid.New().String(),
		FunctionID:    functionID,
		FunctionName:  functionName,
		Payload:       payload,
		Status:        AsyncInvocationStatusQueued,
		MaxAttempts:   DefaultAsyncMaxAttempts,
		BackoffBaseMS: DefaultAsyncBackoffBase,
		BackoffMaxMS:  DefaultAsyncBackoffMax,
		NextRunAt:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func (s *PostgresStore) GetAsyncInvocationSummary(ctx context.Context) (*AsyncInvocationSummary, error) {
	scope := tenantScopeFromContext(ctx)
	var summary AsyncInvocationSummary
	if err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*)::bigint AS total,
			COUNT(*) FILTER (WHERE status = 'queued')::bigint AS queued,
			COUNT(*) FILTER (WHERE status = 'running')::bigint AS running,
			COUNT(*) FILTER (WHERE status = 'paused')::bigint AS paused,
			COUNT(*) FILTER (WHERE status = 'succeeded')::bigint AS succeeded,
			COUNT(*) FILTER (WHERE status = 'dlq')::bigint AS dlq,
			COUNT(*) FILTER (WHERE status = 'succeeded' AND completed_at >= NOW() - INTERVAL '1 minute')::bigint AS consumed_last_1m,
			COUNT(*) FILTER (WHERE status = 'succeeded' AND completed_at >= NOW() - INTERVAL '5 minutes')::bigint AS consumed_last_5m
		FROM async_invocations
		WHERE tenant_id = $1 AND namespace = $2
	`, scope.TenantID, scope.Namespace).Scan(
		&summary.Total,
		&summary.Queued,
		&summary.Running,
		&summary.Paused,
		&summary.Succeeded,
		&summary.DLQ,
		&summary.ConsumedLast1m,
		&summary.ConsumedLast5m,
	); err != nil {
		return nil, fmt.Errorf("get async invocation summary: %w", err)
	}

	summary.Backlog = summary.Queued + summary.Running
	summary.Pending = summary.Backlog + summary.Paused
	summary.ConsumeRatePerSec1m = float64(summary.ConsumedLast1m) / 60.0
	summary.ConsumeRatePerSec5m = float64(summary.ConsumedLast5m) / 300.0
	summary.ConsumeRatePerMinute1m = float64(summary.ConsumedLast1m)
	summary.ConsumeRatePerMinute5m = float64(summary.ConsumedLast5m) / 5.0
	summary.UpdatedAt = time.Now().UTC()
	return &summary, nil
}

func (s *PostgresStore) EnqueueAsyncInvocation(ctx context.Context, inv *AsyncInvocation) error {
	if inv == nil {
		return fmt.Errorf("async invocation is required")
	}
	if inv.FunctionID == "" || inv.FunctionName == "" {
		return fmt.Errorf("function id and name are required")
	}
	scope := tenantScopeFromContext(ctx)
	inv.TenantID = scope.TenantID
	inv.Namespace = scope.Namespace
	normalizeAsyncInvocation(inv)

	if err := insertAsyncInvocation(ctx, s.pool, inv); err != nil {
		return fmt.Errorf("enqueue async invocation: %w", err)
	}
	return nil
}

// EnqueueAsyncInvocationWithIdempotency enqueues an async invocation once for a
// given (function_id, idempotency_key) within the configured idempotency window.
// If the key is duplicated and still valid, it returns the existing invocation with deduplicated=true.
func (s *PostgresStore) EnqueueAsyncInvocationWithIdempotency(ctx context.Context, inv *AsyncInvocation, idempotencyKey string, ttl time.Duration) (*AsyncInvocation, bool, error) {
	if inv == nil {
		return nil, false, fmt.Errorf("async invocation is required")
	}
	if inv.FunctionID == "" || inv.FunctionName == "" {
		return nil, false, fmt.Errorf("function id and name are required")
	}
	scopeCfg := tenantScopeFromContext(ctx)
	inv.TenantID = scopeCfg.TenantID
	inv.Namespace = scopeCfg.Namespace
	normalizeAsyncInvocation(inv)

	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return nil, false, fmt.Errorf("%w: key is required", ErrInvalidIdempotencyKey)
	}
	if len(key) > 256 {
		return nil, false, fmt.Errorf("%w: max length is 256", ErrInvalidIdempotencyKey)
	}

	ttl = normalizeIdempotencyTTL(ttl)
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	const scope = "invoke_async"
	resourceID, claimed, err := claimIdempotencyKey(ctx, tx, scope, inv.FunctionID, key, inv.ID, now, expiresAt)
	if err != nil {
		return nil, false, fmt.Errorf("claim idempotency key: %w", err)
	}

	if !claimed {
		existing, err := getAsyncInvocationByIdempotency(ctx, tx, scope, inv.FunctionID, key, now)
		if err != nil {
			// Best effort self-heal for stale/missing links.
			if _, delErr := tx.Exec(ctx, `
				DELETE FROM idempotency_keys
				WHERE scope = $1 AND scope_id = $2 AND idempotency_key = $3
			`, scope, inv.FunctionID, key); delErr != nil {
				return nil, false, fmt.Errorf("cleanup stale idempotency key: %w", delErr)
			}

			resourceID, claimed, err = claimIdempotencyKey(ctx, tx, scope, inv.FunctionID, key, inv.ID, now, expiresAt)
			if err != nil {
				return nil, false, fmt.Errorf("reclaim idempotency key: %w", err)
			}
			if !claimed {
				return nil, false, fmt.Errorf("idempotency key conflict for function %s", inv.FunctionName)
			}
			if resourceID != inv.ID {
				return nil, false, fmt.Errorf("unexpected idempotency resource mapping")
			}
		} else {
			if err := tx.Commit(ctx); err != nil {
				return nil, false, fmt.Errorf("commit replay tx: %w", err)
			}
			return existing, true, nil
		}
	}

	if resourceID != inv.ID {
		return nil, false, fmt.Errorf("unexpected idempotency resource id: %s", resourceID)
	}

	if err := insertAsyncInvocation(ctx, tx, inv); err != nil {
		return nil, false, fmt.Errorf("enqueue async invocation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit idempotent enqueue tx: %w", err)
	}
	return inv, false, nil
}

func insertAsyncInvocation(ctx context.Context, exec interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, inv *AsyncInvocation) error {
	_, err := exec.Exec(ctx, `
		INSERT INTO async_invocations (
			id, tenant_id, namespace, function_id, function_name, workflow_id, workflow_name, payload, status, attempt, max_attempts,
			backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
			request_id, output, duration_ms, cold_start, last_error, started_at,
			completed_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16,
			$17, $18, $19, $20, $21, $22,
			$23, $24, $25
		)
	`, inv.ID, inv.TenantID, inv.Namespace, inv.FunctionID, inv.FunctionName, nullIfEmpty(inv.WorkflowID), nullIfEmpty(inv.WorkflowName), inv.Payload, string(inv.Status), inv.Attempt, inv.MaxAttempts,
		inv.BackoffBaseMS, inv.BackoffMaxMS, inv.NextRunAt, nullIfEmpty(inv.LockedBy), inv.LockedUntil,
		nullIfEmpty(inv.RequestID), inv.Output, inv.DurationMS, inv.ColdStart, nullIfEmpty(inv.LastError), inv.StartedAt,
		inv.CompletedAt, inv.CreatedAt, inv.UpdatedAt)
	return err
}

func (s *PostgresStore) GetAsyncInvocation(ctx context.Context, id string) (*AsyncInvocation, error) {
	scope := tenantScopeFromContext(ctx)
	inv, err := scanAsyncInvocation(s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, namespace, function_id, function_name, workflow_id, workflow_name, payload, status, attempt, max_attempts,
		       backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
		       request_id, output, duration_ms, cold_start, last_error, started_at,
		       completed_at, created_at, updated_at
		FROM async_invocations
		WHERE id = $1 AND tenant_id = $2 AND namespace = $3
	`, id, scope.TenantID, scope.Namespace))
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrAsyncInvocationNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get async invocation: %w", err)
	}
	return inv, nil
}

func (s *PostgresStore) ListAsyncInvocations(ctx context.Context, limit, offset int, statuses []AsyncInvocationStatus) ([]*AsyncInvocation, error) {
	limit = normalizeAsyncListLimit(limit)
	if offset < 0 {
		offset = 0
	}
	scope := tenantScopeFromContext(ctx)
	query := `
		SELECT id, tenant_id, namespace, function_id, function_name, workflow_id, workflow_name, NULL::jsonb, status, attempt, max_attempts,
		       backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
		       request_id, NULL::jsonb, duration_ms, cold_start, last_error, started_at,
		       completed_at, created_at, updated_at
		FROM async_invocations
		WHERE tenant_id = $1 AND namespace = $2
	`
	args := []any{scope.TenantID, scope.Namespace}

	if len(statuses) > 0 {
		args = append(args, statusesToStrings(statuses))
		query += " AND status = ANY($" + strconv.Itoa(len(args)) + ")"
	}

	args = append(args, limit)
	query += " ORDER BY created_at DESC LIMIT $" + strconv.Itoa(len(args))
	args = append(args, offset)
	query += " OFFSET $" + strconv.Itoa(len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list async invocations: %w", err)
	}
	defer rows.Close()

	out := make([]*AsyncInvocation, 0, limit)
	for rows.Next() {
		inv, err := scanAsyncInvocation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan async invocation: %w", err)
		}
		out = append(out, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list async invocations rows: %w", err)
	}
	return out, nil
}

func (s *PostgresStore) CountAsyncInvocations(ctx context.Context, statuses []AsyncInvocationStatus) (int64, error) {
	scope := tenantScopeFromContext(ctx)
	query := `
		SELECT COUNT(*)
		FROM async_invocations
		WHERE tenant_id = $1 AND namespace = $2
	`
	args := []any{scope.TenantID, scope.Namespace}

	if len(statuses) > 0 {
		args = append(args, statusesToStrings(statuses))
		query += " AND status = ANY($" + strconv.Itoa(len(args)) + ")"
	}

	var total int64
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count async invocations: %w", err)
	}
	return total, nil
}

func (s *PostgresStore) ListFunctionAsyncInvocations(ctx context.Context, functionID string, limit, offset int, statuses []AsyncInvocationStatus) ([]*AsyncInvocation, error) {
	limit = normalizeAsyncListLimit(limit)
	if offset < 0 {
		offset = 0
	}
	scope := tenantScopeFromContext(ctx)
	query := `
		SELECT id, tenant_id, namespace, function_id, function_name, workflow_id, workflow_name, NULL::jsonb, status, attempt, max_attempts,
		       backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
		       request_id, NULL::jsonb, duration_ms, cold_start, last_error, started_at,
		       completed_at, created_at, updated_at
		FROM async_invocations
		WHERE tenant_id = $1 AND namespace = $2 AND function_id = $3
	`
	args := []any{scope.TenantID, scope.Namespace, functionID}

	if len(statuses) > 0 {
		args = append(args, statusesToStrings(statuses))
		query += " AND status = ANY($" + strconv.Itoa(len(args)) + ")"
	}

	args = append(args, limit)
	query += " ORDER BY created_at DESC LIMIT $" + strconv.Itoa(len(args))
	args = append(args, offset)
	query += " OFFSET $" + strconv.Itoa(len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list function async invocations: %w", err)
	}
	defer rows.Close()

	out := make([]*AsyncInvocation, 0, limit)
	for rows.Next() {
		inv, err := scanAsyncInvocation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan async invocation: %w", err)
		}
		out = append(out, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list function async invocations rows: %w", err)
	}
	return out, nil
}

func (s *PostgresStore) CountFunctionAsyncInvocations(ctx context.Context, functionID string, statuses []AsyncInvocationStatus) (int64, error) {
	scope := tenantScopeFromContext(ctx)
	query := `
		SELECT COUNT(*)
		FROM async_invocations
		WHERE tenant_id = $1 AND namespace = $2 AND function_id = $3
	`
	args := []any{scope.TenantID, scope.Namespace, functionID}

	if len(statuses) > 0 {
		args = append(args, statusesToStrings(statuses))
		query += " AND status = ANY($" + strconv.Itoa(len(args)) + ")"
	}

	var total int64
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count function async invocations: %w", err)
	}
	return total, nil
}

// AcquireDueAsyncInvocation atomically leases one queued async invocation that is due.
func (s *PostgresStore) AcquireDueAsyncInvocation(ctx context.Context, workerID string, leaseDuration time.Duration) (*AsyncInvocation, error) {
	if workerID == "" {
		workerID = "async-worker"
	}
	if leaseDuration <= 0 {
		leaseDuration = DefaultAsyncLeaseTimeout
	}

	now := time.Now().UTC()
	leaseUntil := now.Add(leaseDuration)
	inv, err := scanAsyncInvocation(s.pool.QueryRow(ctx, `
		UPDATE async_invocations SET
			status = 'running',
			attempt = attempt + 1,
			locked_by = $1,
			locked_until = $2,
			started_at = COALESCE(started_at, $3),
			updated_at = $3
		WHERE id = (
			SELECT id FROM async_invocations
			WHERE ((status = 'queued' AND next_run_at <= $3) OR (status = 'running' AND locked_until < $3))
			ORDER BY next_run_at ASC, created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, tenant_id, namespace, function_id, function_name, workflow_id, workflow_name, payload, status, attempt, max_attempts,
		          backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
		          request_id, output, duration_ms, cold_start, last_error, started_at,
		          completed_at, created_at, updated_at
	`, workerID, leaseUntil, now))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("acquire async invocation: %w", err)
	}
	return inv, nil
}

func (s *PostgresStore) MarkAsyncInvocationSucceeded(ctx context.Context, id, requestID string, output json.RawMessage, durationMS int64, coldStart bool) error {
	now := time.Now().UTC()
	ct, err := s.pool.Exec(ctx, `
		UPDATE async_invocations SET
			status = 'succeeded',
			request_id = $2,
			output = $3,
			duration_ms = $4,
			cold_start = $5,
			last_error = NULL,
			locked_by = NULL,
			locked_until = NULL,
			completed_at = $6,
			updated_at = $6
		WHERE id = $1
	`, id, nullIfEmpty(requestID), output, durationMS, coldStart, now)
	if err != nil {
		return fmt.Errorf("mark async invocation succeeded: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrAsyncInvocationNotFound, id)
	}
	return nil
}

func (s *PostgresStore) MarkAsyncInvocationForRetry(ctx context.Context, id, lastError string, nextRunAt time.Time) error {
	if nextRunAt.IsZero() {
		nextRunAt = time.Now().UTC()
	}
	ct, err := s.pool.Exec(ctx, `
		UPDATE async_invocations SET
			status = 'queued',
			last_error = $2,
			next_run_at = $3,
			locked_by = NULL,
			locked_until = NULL,
			updated_at = NOW()
		WHERE id = $1
	`, id, nullIfEmpty(lastError), nextRunAt)
	if err != nil {
		return fmt.Errorf("mark async invocation retry: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrAsyncInvocationNotFound, id)
	}
	return nil
}

func (s *PostgresStore) MarkAsyncInvocationDLQ(ctx context.Context, id, lastError string) error {
	now := time.Now().UTC()
	ct, err := s.pool.Exec(ctx, `
		UPDATE async_invocations SET
			status = 'dlq',
			last_error = $2,
			locked_by = NULL,
			locked_until = NULL,
			completed_at = $3,
			updated_at = $3
		WHERE id = $1
	`, id, nullIfEmpty(lastError), now)
	if err != nil {
		return fmt.Errorf("mark async invocation dlq: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrAsyncInvocationNotFound, id)
	}
	return nil
}

func (s *PostgresStore) RequeueAsyncInvocation(ctx context.Context, id string, maxAttempts int) (*AsyncInvocation, error) {
	now := time.Now().UTC()
	if maxAttempts <= 0 {
		maxAttempts = DefaultAsyncMaxAttempts
	}
	scope := tenantScopeFromContext(ctx)

	inv, err := scanAsyncInvocation(s.pool.QueryRow(ctx, `
		UPDATE async_invocations SET
			status = 'queued',
			attempt = 0,
			max_attempts = $2,
			next_run_at = $3,
			locked_by = NULL,
			locked_until = NULL,
			request_id = NULL,
			output = NULL,
			duration_ms = 0,
			cold_start = FALSE,
			last_error = NULL,
			started_at = NULL,
			completed_at = NULL,
			updated_at = $3
		WHERE id = $1 AND tenant_id = $4 AND namespace = $5 AND status = 'dlq'
		RETURNING id, tenant_id, namespace, function_id, function_name, workflow_id, workflow_name, payload, status, attempt, max_attempts,
		          backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
		          request_id, output, duration_ms, cold_start, last_error, started_at,
		          completed_at, created_at, updated_at
	`, id, maxAttempts, now, scope.TenantID, scope.Namespace))
	if err == pgx.ErrNoRows {
		var status string
		statusErr := s.pool.QueryRow(ctx, `SELECT status FROM async_invocations WHERE id = $1 AND tenant_id = $2 AND namespace = $3`, id, scope.TenantID, scope.Namespace).Scan(&status)
		if statusErr == pgx.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", ErrAsyncInvocationNotFound, id)
		}
		if statusErr != nil {
			return nil, fmt.Errorf("requeue async invocation lookup: %w", statusErr)
		}
		return nil, fmt.Errorf("%w: %s (%s)", ErrAsyncInvocationNotDLQ, id, status)
	}
	if err != nil {
		return nil, fmt.Errorf("requeue async invocation: %w", err)
	}
	return inv, nil
}

// PauseAsyncInvocation transitions a queued async invocation to paused status.
func (s *PostgresStore) PauseAsyncInvocation(ctx context.Context, id string) (*AsyncInvocation, error) {
	now := time.Now().UTC()
	scope := tenantScopeFromContext(ctx)

	inv, err := scanAsyncInvocation(s.pool.QueryRow(ctx, `
		UPDATE async_invocations SET
			status = 'paused',
			locked_by = NULL,
			locked_until = NULL,
			updated_at = $2
		WHERE id = $1 AND tenant_id = $3 AND namespace = $4 AND status = 'queued'
		RETURNING id, tenant_id, namespace, function_id, function_name, workflow_id, workflow_name, payload, status, attempt, max_attempts,
		          backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
		          request_id, output, duration_ms, cold_start, last_error, started_at,
		          completed_at, created_at, updated_at
	`, id, now, scope.TenantID, scope.Namespace))
	if err == pgx.ErrNoRows {
		var status string
		statusErr := s.pool.QueryRow(ctx, `SELECT status FROM async_invocations WHERE id = $1 AND tenant_id = $2 AND namespace = $3`, id, scope.TenantID, scope.Namespace).Scan(&status)
		if statusErr == pgx.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", ErrAsyncInvocationNotFound, id)
		}
		if statusErr != nil {
			return nil, fmt.Errorf("pause async invocation lookup: %w", statusErr)
		}
		return nil, fmt.Errorf("%w: %s (%s)", ErrAsyncInvocationNotQueued, id, status)
	}
	if err != nil {
		return nil, fmt.Errorf("pause async invocation: %w", err)
	}
	return inv, nil
}

// ResumeAsyncInvocation transitions a paused async invocation back to queued status.
func (s *PostgresStore) ResumeAsyncInvocation(ctx context.Context, id string) (*AsyncInvocation, error) {
	now := time.Now().UTC()
	scope := tenantScopeFromContext(ctx)

	inv, err := scanAsyncInvocation(s.pool.QueryRow(ctx, `
		UPDATE async_invocations SET
			status = 'queued',
			next_run_at = $2,
			updated_at = $2
		WHERE id = $1 AND tenant_id = $3 AND namespace = $4 AND status = 'paused'
		RETURNING id, tenant_id, namespace, function_id, function_name, workflow_id, workflow_name, payload, status, attempt, max_attempts,
		          backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
		          request_id, output, duration_ms, cold_start, last_error, started_at,
		          completed_at, created_at, updated_at
	`, id, now, scope.TenantID, scope.Namespace))
	if err == pgx.ErrNoRows {
		var status string
		statusErr := s.pool.QueryRow(ctx, `SELECT status FROM async_invocations WHERE id = $1 AND tenant_id = $2 AND namespace = $3`, id, scope.TenantID, scope.Namespace).Scan(&status)
		if statusErr == pgx.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", ErrAsyncInvocationNotFound, id)
		}
		if statusErr != nil {
			return nil, fmt.Errorf("resume async invocation lookup: %w", statusErr)
		}
		return nil, fmt.Errorf("%w: %s (%s)", ErrAsyncInvocationNotPaused, id, status)
	}
	if err != nil {
		return nil, fmt.Errorf("resume async invocation: %w", err)
	}
	return inv, nil
}

// DeleteAsyncInvocation deletes an async invocation that has not been consumed (queued or paused).
func (s *PostgresStore) DeleteAsyncInvocation(ctx context.Context, id string) error {
	scope := tenantScopeFromContext(ctx)

	ct, err := s.pool.Exec(ctx, `
		DELETE FROM async_invocations
		WHERE id = $1 AND tenant_id = $2 AND namespace = $3 AND status IN ('queued', 'paused')
	`, id, scope.TenantID, scope.Namespace)
	if err != nil {
		return fmt.Errorf("delete async invocation: %w", err)
	}
	if ct.RowsAffected() == 0 {
		var status string
		statusErr := s.pool.QueryRow(ctx, `SELECT status FROM async_invocations WHERE id = $1 AND tenant_id = $2 AND namespace = $3`, id, scope.TenantID, scope.Namespace).Scan(&status)
		if statusErr == pgx.ErrNoRows {
			return fmt.Errorf("%w: %s", ErrAsyncInvocationNotFound, id)
		}
		if statusErr != nil {
			return fmt.Errorf("delete async invocation lookup: %w", statusErr)
		}
		return fmt.Errorf("%w: %s (%s)", ErrAsyncInvocationNotDeletable, id, status)
	}
	return nil
}

func claimIdempotencyKey(ctx context.Context, tx pgx.Tx, scope, scopeID, key, resourceID string, now, expiresAt time.Time) (string, bool, error) {
	var claimedResourceID string
	err := tx.QueryRow(ctx, `
		INSERT INTO idempotency_keys (
			scope, scope_id, idempotency_key, resource_type, resource_id, expires_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, 'async_invocation', $4, $5, $6, $6
		)
		ON CONFLICT (scope, scope_id, idempotency_key) DO UPDATE
			SET resource_id = EXCLUDED.resource_id,
			    resource_type = EXCLUDED.resource_type,
			    expires_at = EXCLUDED.expires_at,
			    updated_at = EXCLUDED.updated_at
		WHERE idempotency_keys.expires_at <= $6
		RETURNING resource_id
	`, scope, scopeID, key, resourceID, expiresAt, now).Scan(&claimedResourceID)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return claimedResourceID, true, nil
}

func getAsyncInvocationByIdempotency(ctx context.Context, tx pgx.Tx, scope, scopeID, key string, now time.Time) (*AsyncInvocation, error) {
	inv, err := scanAsyncInvocation(tx.QueryRow(ctx, `
		SELECT ai.id, ai.tenant_id, ai.namespace, ai.function_id, ai.function_name, ai.workflow_id, ai.workflow_name, ai.payload, ai.status, ai.attempt, ai.max_attempts,
		       ai.backoff_base_ms, ai.backoff_max_ms, ai.next_run_at, ai.locked_by, ai.locked_until,
		       ai.request_id, ai.output, ai.duration_ms, ai.cold_start, ai.last_error, ai.started_at,
		       ai.completed_at, ai.created_at, ai.updated_at
		FROM idempotency_keys ik
		JOIN async_invocations ai ON ai.id = ik.resource_id
		WHERE ik.scope = $1
		  AND ik.scope_id = $2
		  AND ik.idempotency_key = $3
		  AND ik.expires_at > $4
	`, scope, scopeID, key, now))
	if err == pgx.ErrNoRows {
		return nil, ErrAsyncInvocationNotFound
	}
	return inv, err
}

func normalizeIdempotencyTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return DefaultAsyncIdempotencyTTL
	}
	if ttl > MaxAsyncIdempotencyTTL {
		return MaxAsyncIdempotencyTTL
	}
	return ttl
}

func normalizeAsyncInvocation(inv *AsyncInvocation) {
	now := time.Now().UTC()
	if inv.ID == "" {
		inv.ID = uuid.New().String()
	}
	if inv.TenantID == "" {
		inv.TenantID = DefaultTenantID
	}
	if inv.Namespace == "" {
		inv.Namespace = DefaultNamespace
	}
	if len(inv.Payload) == 0 {
		inv.Payload = json.RawMessage(`{}`)
	}
	if inv.Status == "" {
		inv.Status = AsyncInvocationStatusQueued
	}
	if inv.MaxAttempts <= 0 {
		inv.MaxAttempts = DefaultAsyncMaxAttempts
	}
	if inv.BackoffBaseMS <= 0 {
		inv.BackoffBaseMS = DefaultAsyncBackoffBase
	}
	if inv.BackoffMaxMS <= 0 {
		inv.BackoffMaxMS = DefaultAsyncBackoffMax
	}
	if inv.BackoffMaxMS < inv.BackoffBaseMS {
		inv.BackoffMaxMS = inv.BackoffBaseMS
	}
	if inv.NextRunAt.IsZero() {
		inv.NextRunAt = now
	}
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = now
	}
	inv.UpdatedAt = now
}

func normalizeAsyncListLimit(limit int) int {
	if limit <= 0 {
		return DefaultAsyncListLimit
	}
	if limit > MaxAsyncListLimit {
		return MaxAsyncListLimit
	}
	return limit
}

func statusesToStrings(statuses []AsyncInvocationStatus) []string {
	out := make([]string, 0, len(statuses))
	for _, status := range statuses {
		if status == "" {
			continue
		}
		out = append(out, string(status))
	}
	return out
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type asyncInvocationScanner interface {
	Scan(dest ...any) error
}

func scanAsyncInvocation(scanner asyncInvocationScanner) (*AsyncInvocation, error) {
	var inv AsyncInvocation
	var status string
	var payload []byte
	var workflowID *string
	var workflowName *string
	var lockedBy *string
	var requestID *string
	var output []byte
	var lastError *string

	err := scanner.Scan(
		&inv.ID,
		&inv.TenantID,
		&inv.Namespace,
		&inv.FunctionID,
		&inv.FunctionName,
		&workflowID,
		&workflowName,
		&payload,
		&status,
		&inv.Attempt,
		&inv.MaxAttempts,
		&inv.BackoffBaseMS,
		&inv.BackoffMaxMS,
		&inv.NextRunAt,
		&lockedBy,
		&inv.LockedUntil,
		&requestID,
		&output,
		&inv.DurationMS,
		&inv.ColdStart,
		&lastError,
		&inv.StartedAt,
		&inv.CompletedAt,
		&inv.CreatedAt,
		&inv.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	inv.Status = AsyncInvocationStatus(status)
	if len(payload) > 0 {
		inv.Payload = payload
	}
	if len(output) > 0 {
		inv.Output = output
	}
	if workflowID != nil {
		inv.WorkflowID = *workflowID
	}
	if workflowName != nil {
		inv.WorkflowName = *workflowName
	}
	if lockedBy != nil {
		inv.LockedBy = *lockedBy
	}
	if requestID != nil {
		inv.RequestID = *requestID
	}
	if lastError != nil {
		inv.LastError = *lastError
	}
	return &inv, nil
}

// ListWorkflowAsyncInvocations lists async invocations for a specific workflow.
func (s *PostgresStore) ListWorkflowAsyncInvocations(ctx context.Context, workflowID string, limit, offset int, statuses []AsyncInvocationStatus) ([]*AsyncInvocation, error) {
	limit = normalizeAsyncListLimit(limit)
	if offset < 0 {
		offset = 0
	}
	scope := tenantScopeFromContext(ctx)
	query := `
		SELECT id, tenant_id, namespace, function_id, function_name, workflow_id, workflow_name, NULL::jsonb, status, attempt, max_attempts,
		       backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
		       request_id, NULL::jsonb, duration_ms, cold_start, last_error, started_at,
		       completed_at, created_at, updated_at
		FROM async_invocations
		WHERE tenant_id = $1 AND namespace = $2 AND workflow_id = $3
	`
	args := []any{scope.TenantID, scope.Namespace, workflowID}

	if len(statuses) > 0 {
		args = append(args, statusesToStrings(statuses))
		query += " AND status = ANY($" + strconv.Itoa(len(args)) + ")"
	}

	args = append(args, limit)
	query += " ORDER BY created_at DESC LIMIT $" + strconv.Itoa(len(args))
	args = append(args, offset)
	query += " OFFSET $" + strconv.Itoa(len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list workflow async invocations: %w", err)
	}
	defer rows.Close()

	out := make([]*AsyncInvocation, 0, limit)
	for rows.Next() {
		inv, err := scanAsyncInvocation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan async invocation: %w", err)
		}
		out = append(out, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow async invocations rows: %w", err)
	}
	return out, nil
}

// CountWorkflowAsyncInvocations counts async invocations for a specific workflow.
func (s *PostgresStore) CountWorkflowAsyncInvocations(ctx context.Context, workflowID string, statuses []AsyncInvocationStatus) (int64, error) {
	scope := tenantScopeFromContext(ctx)
	query := `
		SELECT COUNT(*)
		FROM async_invocations
		WHERE tenant_id = $1 AND namespace = $2 AND workflow_id = $3
	`
	args := []any{scope.TenantID, scope.Namespace, workflowID}

	if len(statuses) > 0 {
		args = append(args, statusesToStrings(statuses))
		query += " AND status = ANY($" + strconv.Itoa(len(args)) + ")"
	}

	var total int64
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count workflow async invocations: %w", err)
	}
	return total, nil
}

// PauseAsyncInvocationsByFunction pauses all queued invocations for a specific function.
func (s *PostgresStore) PauseAsyncInvocationsByFunction(ctx context.Context, functionID string) (int, error) {
	scope := tenantScopeFromContext(ctx)
	now := time.Now().UTC()

	ct, err := s.pool.Exec(ctx, `
		UPDATE async_invocations SET
			status = 'paused',
			locked_by = NULL,
			locked_until = NULL,
			updated_at = $4
		WHERE tenant_id = $1 AND namespace = $2 AND function_id = $3 AND status = 'queued'
	`, scope.TenantID, scope.Namespace, functionID, now)
	if err != nil {
		return 0, fmt.Errorf("pause async invocations by function: %w", err)
	}
	return int(ct.RowsAffected()), nil
}

// ResumeAsyncInvocationsByFunction resumes all paused invocations for a specific function.
func (s *PostgresStore) ResumeAsyncInvocationsByFunction(ctx context.Context, functionID string) (int, error) {
	scope := tenantScopeFromContext(ctx)
	now := time.Now().UTC()

	ct, err := s.pool.Exec(ctx, `
		UPDATE async_invocations SET
			status = 'queued',
			next_run_at = $4,
			updated_at = $4
		WHERE tenant_id = $1 AND namespace = $2 AND function_id = $3 AND status = 'paused'
	`, scope.TenantID, scope.Namespace, functionID, now)
	if err != nil {
		return 0, fmt.Errorf("resume async invocations by function: %w", err)
	}
	return int(ct.RowsAffected()), nil
}

// PauseAsyncInvocationsByWorkflow pauses all queued invocations for a specific workflow.
func (s *PostgresStore) PauseAsyncInvocationsByWorkflow(ctx context.Context, workflowID string) (int, error) {
	scope := tenantScopeFromContext(ctx)
	now := time.Now().UTC()

	ct, err := s.pool.Exec(ctx, `
		UPDATE async_invocations SET
			status = 'paused',
			locked_by = NULL,
			locked_until = NULL,
			updated_at = $4
		WHERE tenant_id = $1 AND namespace = $2 AND workflow_id = $3 AND status = 'queued'
	`, scope.TenantID, scope.Namespace, workflowID, now)
	if err != nil {
		return 0, fmt.Errorf("pause async invocations by workflow: %w", err)
	}
	return int(ct.RowsAffected()), nil
}

// ResumeAsyncInvocationsByWorkflow resumes all paused invocations for a specific workflow.
func (s *PostgresStore) ResumeAsyncInvocationsByWorkflow(ctx context.Context, workflowID string) (int, error) {
	scope := tenantScopeFromContext(ctx)
	now := time.Now().UTC()

	ct, err := s.pool.Exec(ctx, `
		UPDATE async_invocations SET
			status = 'queued',
			next_run_at = $4,
			updated_at = $4
		WHERE tenant_id = $1 AND namespace = $2 AND workflow_id = $3 AND status = 'paused'
	`, scope.TenantID, scope.Namespace, workflowID, now)
	if err != nil {
		return 0, fmt.Errorf("resume async invocations by workflow: %w", err)
	}
	return int(ct.RowsAffected()), nil
}

// SetGlobalAsyncPause sets the global async queue pause state.
func (s *PostgresStore) SetGlobalAsyncPause(ctx context.Context, paused bool) error {
value := "false"
if paused {
value = "true"
}
return s.SetConfig(ctx, "async_queue_paused", value)
}

// GetGlobalAsyncPause retrieves the global async queue pause state.
func (s *PostgresStore) GetGlobalAsyncPause(ctx context.Context) (bool, error) {
config, err := s.GetConfig(ctx)
if err != nil {
return false, fmt.Errorf("get global async pause: %w", err)
}
value, ok := config["async_queue_paused"]
if !ok {
return false, nil
}
return value == "true", nil
}
