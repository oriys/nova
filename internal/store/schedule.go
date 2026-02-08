package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Schedule represents a scheduled function invocation.
type Schedule struct {
	ID           string          `json:"id"`
	TenantID     string          `json:"tenant_id,omitempty"`
	Namespace    string          `json:"namespace,omitempty"`
	FunctionName string          `json:"function_name"`
	CronExpr     string          `json:"cron_expression"`
	Input        json.RawMessage `json:"input,omitempty"`
	Enabled      bool            `json:"enabled"`
	LastRunAt    *time.Time      `json:"last_run_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// ScheduleStore defines schedule persistence operations.
type ScheduleStore interface {
	SaveSchedule(ctx context.Context, s *Schedule) error
	ListSchedulesByFunction(ctx context.Context, functionName string) ([]*Schedule, error)
	ListAllSchedules(ctx context.Context) ([]*Schedule, error)
	GetSchedule(ctx context.Context, id string) (*Schedule, error)
	DeleteSchedule(ctx context.Context, id string) error
	UpdateScheduleLastRun(ctx context.Context, id string, t time.Time) error
	UpdateScheduleEnabled(ctx context.Context, id string, enabled bool) error
	UpdateScheduleCron(ctx context.Context, id string, cronExpr string) error
}

// NewSchedule creates a new Schedule with defaults.
func NewSchedule(functionName, cronExpr string, input json.RawMessage) *Schedule {
	now := time.Now()
	return &Schedule{
		ID:           uuid.New().String(),
		FunctionName: functionName,
		CronExpr:     cronExpr,
		Input:        input,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// ─── PostgresStore schedule methods ──────────────────────────────────────────

func (s *PostgresStore) SaveSchedule(ctx context.Context, sched *Schedule) error {
	scope := tenantScopeFromContext(ctx)
	if sched.TenantID == "" {
		sched.TenantID = scope.TenantID
	}
	if sched.Namespace == "" {
		sched.Namespace = scope.Namespace
	}

	ct, err := s.pool.Exec(ctx, `
		UPDATE schedules
		SET function_name = $4,
			cron_expression = $5,
			input = $6,
			enabled = $7,
			last_run_at = $8,
			updated_at = NOW()
		WHERE tenant_id = $1 AND namespace = $2 AND id = $3
	`, scope.TenantID, scope.Namespace, sched.ID, sched.FunctionName, sched.CronExpr, sched.Input, sched.Enabled, sched.LastRunAt)
	if err != nil {
		return fmt.Errorf("save schedule: %w", err)
	}
	if ct.RowsAffected() > 0 {
		return nil
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO schedules (id, tenant_id, namespace, function_name, cron_expression, input, enabled, last_run_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, sched.ID, scope.TenantID, scope.Namespace, sched.FunctionName, sched.CronExpr, sched.Input, sched.Enabled, sched.LastRunAt, sched.CreatedAt, sched.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save schedule: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListSchedulesByFunction(ctx context.Context, functionName string) ([]*Schedule, error) {
	scope := tenantScopeFromContext(ctx)
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, namespace, function_name, cron_expression, input, enabled, last_run_at, created_at, updated_at
		FROM schedules
		WHERE tenant_id = $1 AND namespace = $2 AND function_name = $3
		ORDER BY created_at DESC
	`, scope.TenantID, scope.Namespace, functionName)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	defer rows.Close()
	return scanSchedules(rows)
}

func (s *PostgresStore) ListAllSchedules(ctx context.Context) ([]*Schedule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, namespace, function_name, cron_expression, input, enabled, last_run_at, created_at, updated_at
		FROM schedules ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list all schedules: %w", err)
	}
	defer rows.Close()
	return scanSchedules(rows)
}

func (s *PostgresStore) GetSchedule(ctx context.Context, id string) (*Schedule, error) {
	scope := tenantScopeFromContext(ctx)
	var sched Schedule
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, namespace, function_name, cron_expression, input, enabled, last_run_at, created_at, updated_at
		FROM schedules
		WHERE id = $1 AND tenant_id = $2 AND namespace = $3
	`, id, scope.TenantID, scope.Namespace).Scan(
		&sched.ID, &sched.TenantID, &sched.Namespace, &sched.FunctionName, &sched.CronExpr, &sched.Input, &sched.Enabled, &sched.LastRunAt, &sched.CreatedAt, &sched.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("schedule not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get schedule: %w", err)
	}
	return &sched, nil
}

func (s *PostgresStore) DeleteSchedule(ctx context.Context, id string) error {
	scope := tenantScopeFromContext(ctx)
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM schedules
		WHERE id = $1 AND tenant_id = $2 AND namespace = $3
	`, id, scope.TenantID, scope.Namespace)
	if err != nil {
		return fmt.Errorf("delete schedule: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("schedule not found: %s", id)
	}
	return nil
}

func (s *PostgresStore) UpdateScheduleLastRun(ctx context.Context, id string, t time.Time) error {
	scope := tenantScopeFromContext(ctx)
	_, err := s.pool.Exec(ctx, `
		UPDATE schedules
		SET last_run_at = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND namespace = $4
	`, t, id, scope.TenantID, scope.Namespace)
	if err != nil {
		return fmt.Errorf("update schedule last_run: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateScheduleEnabled(ctx context.Context, id string, enabled bool) error {
	scope := tenantScopeFromContext(ctx)
	_, err := s.pool.Exec(ctx, `
		UPDATE schedules
		SET enabled = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND namespace = $4
	`, enabled, id, scope.TenantID, scope.Namespace)
	if err != nil {
		return fmt.Errorf("update schedule enabled: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateScheduleCron(ctx context.Context, id string, cronExpr string) error {
	scope := tenantScopeFromContext(ctx)
	_, err := s.pool.Exec(ctx, `
		UPDATE schedules
		SET cron_expression = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND namespace = $4
	`, cronExpr, id, scope.TenantID, scope.Namespace)
	if err != nil {
		return fmt.Errorf("update schedule cron: %w", err)
	}
	return nil
}

func scanSchedules(rows pgx.Rows) ([]*Schedule, error) {
	var schedules []*Schedule
	for rows.Next() {
		var sched Schedule
		if err := rows.Scan(&sched.ID, &sched.TenantID, &sched.Namespace, &sched.FunctionName, &sched.CronExpr, &sched.Input, &sched.Enabled, &sched.LastRunAt, &sched.CreatedAt, &sched.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan schedule: %w", err)
		}
		schedules = append(schedules, &sched)
	}
	return schedules, nil
}
