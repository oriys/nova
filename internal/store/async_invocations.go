package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Async invocation status values.
type AsyncInvocationStatus string

const (
	AsyncInvocationStatusQueued    AsyncInvocationStatus = "queued"
	AsyncInvocationStatusRunning   AsyncInvocationStatus = "running"
	AsyncInvocationStatusSucceeded AsyncInvocationStatus = "succeeded"
	AsyncInvocationStatusDLQ       AsyncInvocationStatus = "dlq"
)

const (
	DefaultAsyncMaxAttempts  = 3
	DefaultAsyncBackoffBase  = 1000  // 1s
	DefaultAsyncBackoffMax   = 60000 // 60s
	DefaultAsyncListLimit    = 50
	MaxAsyncListLimit        = 500
	DefaultAsyncLeaseTimeout = 30 * time.Second
)

var ErrAsyncInvocationNotFound = errors.New("async invocation not found")
var ErrAsyncInvocationNotDLQ = errors.New("async invocation is not in dlq")

// AsyncInvocation is a durable async function execution request.
type AsyncInvocation struct {
	ID            string                `json:"id"`
	FunctionID    string                `json:"function_id"`
	FunctionName  string                `json:"function_name"`
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

func (s *PostgresStore) EnqueueAsyncInvocation(ctx context.Context, inv *AsyncInvocation) error {
	if inv == nil {
		return fmt.Errorf("async invocation is required")
	}
	if inv.FunctionID == "" || inv.FunctionName == "" {
		return fmt.Errorf("function id and name are required")
	}
	normalizeAsyncInvocation(inv)

	_, err := s.pool.Exec(ctx, `
		INSERT INTO async_invocations (
			id, function_id, function_name, payload, status, attempt, max_attempts,
			backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
			request_id, output, duration_ms, cold_start, last_error, started_at,
			completed_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18,
			$19, $20, $21
		)
	`, inv.ID, inv.FunctionID, inv.FunctionName, inv.Payload, string(inv.Status), inv.Attempt, inv.MaxAttempts,
		inv.BackoffBaseMS, inv.BackoffMaxMS, inv.NextRunAt, nullIfEmpty(inv.LockedBy), inv.LockedUntil,
		nullIfEmpty(inv.RequestID), inv.Output, inv.DurationMS, inv.ColdStart, nullIfEmpty(inv.LastError), inv.StartedAt,
		inv.CompletedAt, inv.CreatedAt, inv.UpdatedAt)
	if err != nil {
		return fmt.Errorf("enqueue async invocation: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetAsyncInvocation(ctx context.Context, id string) (*AsyncInvocation, error) {
	inv, err := scanAsyncInvocation(s.pool.QueryRow(ctx, `
		SELECT id, function_id, function_name, payload, status, attempt, max_attempts,
		       backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
		       request_id, output, duration_ms, cold_start, last_error, started_at,
		       completed_at, created_at, updated_at
		FROM async_invocations
		WHERE id = $1
	`, id))
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrAsyncInvocationNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get async invocation: %w", err)
	}
	return inv, nil
}

func (s *PostgresStore) ListAsyncInvocations(ctx context.Context, limit int, statuses []AsyncInvocationStatus) ([]*AsyncInvocation, error) {
	limit = normalizeAsyncListLimit(limit)
	query := `
		SELECT id, function_id, function_name, payload, status, attempt, max_attempts,
		       backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
		       request_id, output, duration_ms, cold_start, last_error, started_at,
		       completed_at, created_at, updated_at
		FROM async_invocations
	`
	args := []any{}

	if len(statuses) > 0 {
		args = append(args, statusesToStrings(statuses))
		query += " WHERE status = ANY($" + strconv.Itoa(len(args)) + ")"
	}

	args = append(args, limit)
	query += " ORDER BY created_at DESC LIMIT $" + strconv.Itoa(len(args))

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

func (s *PostgresStore) ListFunctionAsyncInvocations(ctx context.Context, functionID string, limit int, statuses []AsyncInvocationStatus) ([]*AsyncInvocation, error) {
	limit = normalizeAsyncListLimit(limit)
	query := `
		SELECT id, function_id, function_name, payload, status, attempt, max_attempts,
		       backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
		       request_id, output, duration_ms, cold_start, last_error, started_at,
		       completed_at, created_at, updated_at
		FROM async_invocations
		WHERE function_id = $1
	`
	args := []any{functionID}

	if len(statuses) > 0 {
		args = append(args, statusesToStrings(statuses))
		query += " AND status = ANY($" + strconv.Itoa(len(args)) + ")"
	}

	args = append(args, limit)
	query += " ORDER BY created_at DESC LIMIT $" + strconv.Itoa(len(args))

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
		RETURNING id, function_id, function_name, payload, status, attempt, max_attempts,
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
		WHERE id = $1 AND status = 'dlq'
		RETURNING id, function_id, function_name, payload, status, attempt, max_attempts,
		          backoff_base_ms, backoff_max_ms, next_run_at, locked_by, locked_until,
		          request_id, output, duration_ms, cold_start, last_error, started_at,
		          completed_at, created_at, updated_at
	`, id, maxAttempts, now))
	if err == pgx.ErrNoRows {
		var status string
		statusErr := s.pool.QueryRow(ctx, `SELECT status FROM async_invocations WHERE id = $1`, id).Scan(&status)
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

func normalizeAsyncInvocation(inv *AsyncInvocation) {
	now := time.Now().UTC()
	if inv.ID == "" {
		inv.ID = uuid.New().String()
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
	var lockedBy *string
	var requestID *string
	var output []byte
	var lastError *string

	err := scanner.Scan(
		&inv.ID,
		&inv.FunctionID,
		&inv.FunctionName,
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
