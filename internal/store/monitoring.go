package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// InvocationLog represents a single function invocation record
type InvocationLog struct {
	ID           string          `json:"id"`
	FunctionID   string          `json:"function_id"`
	FunctionName string          `json:"function_name"`
	Runtime      string          `json:"runtime"`
	DurationMs   int64           `json:"duration_ms"`
	ColdStart    bool            `json:"cold_start"`
	Success      bool            `json:"success"`
	ErrorMessage string          `json:"error_message,omitempty"`
	InputSize    int             `json:"input_size"`
	OutputSize   int             `json:"output_size"`
	Input        json.RawMessage `json:"input,omitempty"`
	Output       json.RawMessage `json:"output,omitempty"`
	Stdout       string          `json:"stdout,omitempty"`
	Stderr       string          `json:"stderr,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// TimeSeriesBucket represents aggregated metrics for a time period
type TimeSeriesBucket struct {
	Timestamp   time.Time `json:"timestamp"`
	Invocations int64     `json:"invocations"`
	Errors      int64     `json:"errors"`
	AvgDuration float64   `json:"avg_duration"`
}

func (s *PostgresStore) SaveInvocationLog(ctx context.Context, log *InvocationLog) error {
	if log.ID == "" {
		return fmt.Errorf("invocation log id is required")
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO invocation_logs (id, function_id, function_name, runtime, duration_ms, cold_start, success, error_message, input_size, output_size, input, output, stdout, stderr, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (id) DO NOTHING
	`, log.ID, log.FunctionID, log.FunctionName, log.Runtime, log.DurationMs, log.ColdStart, log.Success, log.ErrorMessage, log.InputSize, log.OutputSize, log.Input, log.Output, log.Stdout, log.Stderr, log.CreatedAt)
	if err != nil {
		return fmt.Errorf("save invocation log: %w", err)
	}
	return nil
}

func (s *PostgresStore) SaveInvocationLogs(ctx context.Context, logs []*InvocationLog) error {
	if len(logs) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, log := range logs {
		if log.ID == "" {
			return fmt.Errorf("invocation log id is required")
		}
		if log.CreatedAt.IsZero() {
			log.CreatedAt = time.Now()
		}
		batch.Queue(`
			INSERT INTO invocation_logs (id, function_id, function_name, runtime, duration_ms, cold_start, success, error_message, input_size, output_size, input, output, stdout, stderr, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
			ON CONFLICT (id) DO NOTHING
		`, log.ID, log.FunctionID, log.FunctionName, log.Runtime, log.DurationMs, log.ColdStart, log.Success, log.ErrorMessage, log.InputSize, log.OutputSize, log.Input, log.Output, log.Stdout, log.Stderr, log.CreatedAt)
	}

	results := s.pool.SendBatch(ctx, batch)
	defer results.Close()

	for range logs {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("save invocation logs: %w", err)
		}
	}

	return nil
}

func (s *PostgresStore) ListInvocationLogs(ctx context.Context, functionID string, limit int) ([]*InvocationLog, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, function_id, function_name, runtime, duration_ms, cold_start, success, error_message, input_size, output_size, input, output, stdout, stderr, created_at
		FROM invocation_logs
		WHERE function_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, functionID, limit)
	if err != nil {
		return nil, fmt.Errorf("list invocation logs: %w", err)
	}
	defer rows.Close()

	var logs []*InvocationLog
	for rows.Next() {
		var log InvocationLog
		var errorMessage, stdout, stderr *string
		var input, output []byte
		if err := rows.Scan(&log.ID, &log.FunctionID, &log.FunctionName, &log.Runtime, &log.DurationMs, &log.ColdStart, &log.Success, &errorMessage, &log.InputSize, &log.OutputSize, &input, &output, &stdout, &stderr, &log.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan invocation log: %w", err)
		}
		if errorMessage != nil {
			log.ErrorMessage = *errorMessage
		}
		if input != nil {
			log.Input = input
		}
		if output != nil {
			log.Output = output
		}
		if stdout != nil {
			log.Stdout = *stdout
		}
		if stderr != nil {
			log.Stderr = *stderr
		}
		logs = append(logs, &log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list invocation logs rows: %w", err)
	}
	return logs, nil
}

func (s *PostgresStore) ListAllInvocationLogs(ctx context.Context, limit int) ([]*InvocationLog, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, function_id, function_name, runtime, duration_ms, cold_start, success, error_message, input_size, output_size, input, output, stdout, stderr, created_at
		FROM invocation_logs
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list all invocation logs: %w", err)
	}
	defer rows.Close()

	var logs []*InvocationLog
	for rows.Next() {
		var log InvocationLog
		var errorMessage, stdout, stderr *string
		var input, output []byte
		if err := rows.Scan(&log.ID, &log.FunctionID, &log.FunctionName, &log.Runtime, &log.DurationMs, &log.ColdStart, &log.Success, &errorMessage, &log.InputSize, &log.OutputSize, &input, &output, &stdout, &stderr, &log.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan invocation log: %w", err)
		}
		if errorMessage != nil {
			log.ErrorMessage = *errorMessage
		}
		if input != nil {
			log.Input = input
		}
		if output != nil {
			log.Output = output
		}
		if stdout != nil {
			log.Stdout = *stdout
		}
		if stderr != nil {
			log.Stderr = *stderr
		}
		logs = append(logs, &log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list all invocation logs rows: %w", err)
	}
	return logs, nil
}

func (s *PostgresStore) GetInvocationLog(ctx context.Context, requestID string) (*InvocationLog, error) {
	var log InvocationLog
	var errorMessage, stdout, stderr *string
	var input, output []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, function_id, function_name, runtime, duration_ms, cold_start, success, error_message, input_size, output_size, input, output, stdout, stderr, created_at
		FROM invocation_logs
		WHERE id = $1
	`, requestID).Scan(&log.ID, &log.FunctionID, &log.FunctionName, &log.Runtime, &log.DurationMs, &log.ColdStart, &log.Success, &errorMessage, &log.InputSize, &log.OutputSize, &input, &output, &stdout, &stderr, &log.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("invocation log not found: %s", requestID)
	}
	if err != nil {
		return nil, fmt.Errorf("get invocation log: %w", err)
	}
	if errorMessage != nil {
		log.ErrorMessage = *errorMessage
	}
	if input != nil {
		log.Input = input
	}
	if output != nil {
		log.Output = output
	}
	if stdout != nil {
		log.Stdout = *stdout
	}
	if stderr != nil {
		log.Stderr = *stderr
	}
	return &log, nil
}

func (s *PostgresStore) GetFunctionTimeSeries(ctx context.Context, functionID string, rangeSeconds, bucketSeconds int) ([]TimeSeriesBucket, error) {
	if rangeSeconds <= 0 {
		rangeSeconds = 3600
	}
	if bucketSeconds <= 0 {
		bucketSeconds = 60
	}

	rows, err := s.pool.Query(ctx, `
		WITH buckets AS (
			SELECT generate_series(
				to_timestamp(floor(extract(epoch from NOW() - make_interval(secs => $2::double precision)) / $3) * $3),
				to_timestamp(floor(extract(epoch from NOW()) / $3) * $3),
				make_interval(secs => $3::double precision)
			) AS bucket
		),
		data AS (
			SELECT
				to_timestamp(floor(extract(epoch from created_at) / $3) * $3) AS bucket,
				COUNT(*) AS invocations,
				COUNT(*) FILTER (WHERE NOT success) AS errors,
				AVG(duration_ms) AS avg_duration
			FROM invocation_logs
			WHERE function_id = $1
			  AND created_at >= NOW() - make_interval(secs => $2::double precision)
			GROUP BY bucket
		)
		SELECT
			b.bucket,
			COALESCE(d.invocations, 0) AS invocations,
			COALESCE(d.errors, 0) AS errors,
			COALESCE(d.avg_duration, 0) AS avg_duration
		FROM buckets b
		LEFT JOIN data d ON b.bucket = d.bucket
		ORDER BY b.bucket ASC
	`, functionID, rangeSeconds, bucketSeconds)
	if err != nil {
		return nil, fmt.Errorf("get function time series: %w", err)
	}
	defer rows.Close()

	buckets := make([]TimeSeriesBucket, 0)
	for rows.Next() {
		var bucket TimeSeriesBucket
		if err := rows.Scan(&bucket.Timestamp, &bucket.Invocations, &bucket.Errors, &bucket.AvgDuration); err != nil {
			return nil, fmt.Errorf("scan time series: %w", err)
		}
		buckets = append(buckets, bucket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get function time series rows: %w", err)
	}
	return buckets, nil
}

// DailyCount represents a single day's invocation count for heatmaps.
type DailyCount struct {
	Date        string `json:"date"`
	Invocations int64  `json:"invocations"`
}

func (s *PostgresStore) GetFunctionDailyHeatmap(ctx context.Context, functionID string, weeks int) ([]DailyCount, error) {
	if weeks <= 0 {
		weeks = 52
	}
	days := weeks * 7

	rows, err := s.pool.Query(ctx, `
		WITH days AS (
			SELECT generate_series(
				(CURRENT_DATE - make_interval(days => $2))::date,
				CURRENT_DATE,
				'1 day'::interval
			)::date AS day
		)
		SELECT
			d.day::text,
			COALESCE(COUNT(l.id), 0) AS invocations
		FROM days d
		LEFT JOIN invocation_logs l
			ON l.function_id = $1
			AND l.created_at::date = d.day
		GROUP BY d.day
		ORDER BY d.day ASC
	`, functionID, days)
	if err != nil {
		return nil, fmt.Errorf("get function daily heatmap: %w", err)
	}
	defer rows.Close()

	result := make([]DailyCount, 0, days+1)
	for rows.Next() {
		var dc DailyCount
		if err := rows.Scan(&dc.Date, &dc.Invocations); err != nil {
			return nil, fmt.Errorf("scan daily heatmap: %w", err)
		}
		result = append(result, dc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get function daily heatmap rows: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) GetGlobalDailyHeatmap(ctx context.Context, weeks int) ([]DailyCount, error) {
	if weeks <= 0 {
		weeks = 52
	}
	days := weeks * 7

	rows, err := s.pool.Query(ctx, `
		WITH days AS (
			SELECT generate_series(
				(CURRENT_DATE - make_interval(days => $1))::date,
				CURRENT_DATE,
				'1 day'::interval
			)::date AS day
		)
		SELECT
			d.day::text,
			COALESCE(COUNT(l.id), 0) AS invocations
		FROM days d
		LEFT JOIN invocation_logs l
			ON l.created_at::date = d.day
		GROUP BY d.day
		ORDER BY d.day ASC
	`, days)
	if err != nil {
		return nil, fmt.Errorf("get global daily heatmap: %w", err)
	}
	defer rows.Close()

	result := make([]DailyCount, 0, days+1)
	for rows.Next() {
		var dc DailyCount
		if err := rows.Scan(&dc.Date, &dc.Invocations); err != nil {
			return nil, fmt.Errorf("scan global daily heatmap: %w", err)
		}
		result = append(result, dc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get global daily heatmap rows: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) GetGlobalTimeSeries(ctx context.Context, rangeSeconds, bucketSeconds int) ([]TimeSeriesBucket, error) {
	if rangeSeconds <= 0 {
		rangeSeconds = 3600
	}
	if bucketSeconds <= 0 {
		bucketSeconds = 60
	}

	rows, err := s.pool.Query(ctx, `
		WITH buckets AS (
			SELECT generate_series(
				to_timestamp(floor(extract(epoch from NOW() - make_interval(secs => $1::double precision)) / $2) * $2),
				to_timestamp(floor(extract(epoch from NOW()) / $2) * $2),
				make_interval(secs => $2::double precision)
			) AS bucket
		),
		data AS (
			SELECT
				to_timestamp(floor(extract(epoch from created_at) / $2) * $2) AS bucket,
				COUNT(*) AS invocations,
				COUNT(*) FILTER (WHERE NOT success) AS errors,
				AVG(duration_ms) AS avg_duration
			FROM invocation_logs
			WHERE created_at >= NOW() - make_interval(secs => $1::double precision)
			GROUP BY bucket
		)
		SELECT
			b.bucket,
			COALESCE(d.invocations, 0) AS invocations,
			COALESCE(d.errors, 0) AS errors,
			COALESCE(d.avg_duration, 0) AS avg_duration
		FROM buckets b
		LEFT JOIN data d ON b.bucket = d.bucket
		ORDER BY b.bucket ASC
	`, rangeSeconds, bucketSeconds)
	if err != nil {
		return nil, fmt.Errorf("get global time series: %w", err)
	}
	defer rows.Close()

	buckets := make([]TimeSeriesBucket, 0)
	for rows.Next() {
		var bucket TimeSeriesBucket
		if err := rows.Scan(&bucket.Timestamp, &bucket.Invocations, &bucket.Errors, &bucket.AvgDuration); err != nil {
			return nil, fmt.Errorf("scan time series: %w", err)
		}
		buckets = append(buckets, bucket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get global time series rows: %w", err)
	}
	return buckets, nil
}
