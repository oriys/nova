package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	TenantDimensionInvocations    = "invocations"
	TenantDimensionEventPublishes = "event_publishes"
	TenantDimensionAsyncQueueDepth = "async_queue_depth"
	TenantDimensionFunctionsCount = "functions_count"
	TenantDimensionMemoryMB       = "memory_mb"
	TenantDimensionVCPUMilli      = "vcpu_milli"
	TenantDimensionDiskIOPS       = "disk_iops"
)

var tenantDimensionSet = map[string]struct{}{
	TenantDimensionInvocations:    {},
	TenantDimensionEventPublishes: {},
	TenantDimensionAsyncQueueDepth: {},
	TenantDimensionFunctionsCount: {},
	TenantDimensionMemoryMB:       {},
	TenantDimensionVCPUMilli:      {},
	TenantDimensionDiskIOPS:       {},
}

// TenantQuotaRecord stores quota settings for a tenant and dimension.
type TenantQuotaRecord struct {
	TenantID  string    `json:"tenant_id"`
	Dimension string    `json:"dimension"`
	HardLimit int64     `json:"hard_limit"`
	SoftLimit int64     `json:"soft_limit"`
	Burst     int64     `json:"burst"`
	WindowS   int       `json:"window_s"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TenantUsageRecord stores current usage for a tenant and dimension.
type TenantUsageRecord struct {
	TenantID  string    `json:"tenant_id"`
	Dimension string    `json:"dimension"`
	Used      int64     `json:"used"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TenantQuotaDecision is the result of checking a quota.
type TenantQuotaDecision struct {
	TenantID    string `json:"tenant_id"`
	Dimension   string `json:"dimension"`
	Allowed     bool   `json:"allowed"`
	Used        int64  `json:"used"`
	Limit       int64  `json:"limit"`
	WindowS     int    `json:"window_s,omitempty"`
	RetryAfterS int    `json:"retry_after_s,omitempty"`
}

func normalizeTenantDimension(dimension string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(dimension))
	d = strings.ReplaceAll(d, "-", "_")
	if d == "" {
		return "", fmt.Errorf("dimension is required")
	}
	if _, ok := tenantDimensionSet[d]; !ok {
		return "", fmt.Errorf("unsupported dimension: %s", dimension)
	}
	return d, nil
}

func defaultTenantWindow(dimension string) int {
	switch dimension {
	case TenantDimensionInvocations, TenantDimensionEventPublishes:
		return 60
	default:
		return 60
	}
}

func isWindowedTenantDimension(dimension string) bool {
	return dimension == TenantDimensionInvocations || dimension == TenantDimensionEventPublishes
}

func safeQuotaLimit(hard, burst int64) int64 {
	if hard <= 0 {
		return 0
	}
	if burst < 0 {
		burst = 0
	}
	return hard + burst
}

func (s *PostgresStore) getTenantQuota(ctx context.Context, tenantID, dimension string) (*TenantQuotaRecord, error) {
	var quota TenantQuotaRecord
	err := s.pool.QueryRow(ctx, `
		SELECT tenant_id, dimension, hard_limit, soft_limit, burst, window_s, updated_at
		FROM tenant_quotas
		WHERE tenant_id = $1 AND dimension = $2
	`, tenantID, dimension).Scan(
		&quota.TenantID,
		&quota.Dimension,
		&quota.HardLimit,
		&quota.SoftLimit,
		&quota.Burst,
		&quota.WindowS,
		&quota.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant quota: %w", err)
	}
	if quota.WindowS <= 0 {
		quota.WindowS = defaultTenantWindow(quota.Dimension)
	}
	return &quota, nil
}

func (s *PostgresStore) ListTenantQuotas(ctx context.Context, tenantID string) ([]*TenantQuotaRecord, error) {
	scopedTenantID, err := validateScopeIdentifier("tenant id", tenantID)
	if err != nil {
		return nil, err
	}
	if _, err := s.GetTenant(ctx, scopedTenantID); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT tenant_id, dimension, hard_limit, soft_limit, burst, window_s, updated_at
		FROM tenant_quotas
		WHERE tenant_id = $1
		ORDER BY dimension
	`, scopedTenantID)
	if err != nil {
		return nil, fmt.Errorf("list tenant quotas: %w", err)
	}
	defer rows.Close()

	quotas := make([]*TenantQuotaRecord, 0)
	for rows.Next() {
		var quota TenantQuotaRecord
		if err := rows.Scan(
			&quota.TenantID,
			&quota.Dimension,
			&quota.HardLimit,
			&quota.SoftLimit,
			&quota.Burst,
			&quota.WindowS,
			&quota.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan tenant quota: %w", err)
		}
		if quota.WindowS <= 0 {
			quota.WindowS = defaultTenantWindow(quota.Dimension)
		}
		quotas = append(quotas, &quota)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tenant quotas rows: %w", err)
	}
	return quotas, nil
}

func (s *PostgresStore) UpsertTenantQuota(ctx context.Context, quota *TenantQuotaRecord) (*TenantQuotaRecord, error) {
	if quota == nil {
		return nil, fmt.Errorf("quota is required")
	}
	scopedTenantID, err := validateScopeIdentifier("tenant id", quota.TenantID)
	if err != nil {
		return nil, err
	}
	dimension, err := normalizeTenantDimension(quota.Dimension)
	if err != nil {
		return nil, err
	}
	if _, err := s.GetTenant(ctx, scopedTenantID); err != nil {
		return nil, err
	}

	if quota.HardLimit < 0 {
		return nil, fmt.Errorf("hard_limit must be >= 0")
	}
	if quota.SoftLimit < 0 {
		return nil, fmt.Errorf("soft_limit must be >= 0")
	}
	if quota.Burst < 0 {
		return nil, fmt.Errorf("burst must be >= 0")
	}
	windowS := quota.WindowS
	if windowS <= 0 {
		windowS = defaultTenantWindow(dimension)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO tenant_quotas (tenant_id, dimension, hard_limit, soft_limit, burst, window_s, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (tenant_id, dimension) DO UPDATE SET
			hard_limit = EXCLUDED.hard_limit,
			soft_limit = EXCLUDED.soft_limit,
			burst = EXCLUDED.burst,
			window_s = EXCLUDED.window_s,
			updated_at = NOW()
	`, scopedTenantID, dimension, quota.HardLimit, quota.SoftLimit, quota.Burst, windowS)
	if err != nil {
		return nil, fmt.Errorf("upsert tenant quota: %w", err)
	}

	return s.getTenantQuota(ctx, scopedTenantID, dimension)
}

func (s *PostgresStore) DeleteTenantQuota(ctx context.Context, tenantID, dimension string) error {
	scopedTenantID, err := validateScopeIdentifier("tenant id", tenantID)
	if err != nil {
		return err
	}
	dimension, err = normalizeTenantDimension(dimension)
	if err != nil {
		return err
	}

	ct, err := s.pool.Exec(ctx, `
		DELETE FROM tenant_quotas
		WHERE tenant_id = $1 AND dimension = $2
	`, scopedTenantID, dimension)
	if err != nil {
		return fmt.Errorf("delete tenant quota: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("tenant quota not found: %s/%s", scopedTenantID, dimension)
	}
	return nil
}

func (s *PostgresStore) upsertTenantUsageCurrent(ctx context.Context, tenantID, dimension string, used int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tenant_usage_current (tenant_id, dimension, used, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (tenant_id, dimension) DO UPDATE SET
			used = EXCLUDED.used,
			updated_at = NOW()
	`, tenantID, dimension, used)
	if err != nil {
		return fmt.Errorf("upsert tenant usage current: %w", err)
	}
	return nil
}

func (s *PostgresStore) getWindowedTenantUsage(ctx context.Context, tenantID, dimension string, windowS int) (int64, error) {
	if windowS <= 0 {
		windowS = defaultTenantWindow(dimension)
	}
	now := time.Now().UTC()
	from := now.Add(-time.Duration(windowS) * time.Second)
	var used int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(used), 0)
		FROM tenant_usage_timeseries
		WHERE tenant_id = $1 AND dimension = $2 AND ts >= $3
	`, tenantID, dimension, from).Scan(&used)
	if err != nil {
		return 0, fmt.Errorf("get windowed tenant usage: %w", err)
	}
	return used, nil
}

func (s *PostgresStore) CheckAndConsumeTenantQuota(ctx context.Context, tenantID, dimension string, amount int64) (*TenantQuotaDecision, error) {
	scopedTenantID, err := validateScopeIdentifier("tenant id", tenantID)
	if err != nil {
		return nil, err
	}
	dimension, err = normalizeTenantDimension(dimension)
	if err != nil {
		return nil, err
	}
	if amount <= 0 {
		amount = 1
	}
	if !isWindowedTenantDimension(dimension) {
		return nil, fmt.Errorf("dimension %s is not windowed", dimension)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tenant quota tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var hardLimit, softLimit, burst int64
	var windowS int
	err = tx.QueryRow(ctx, `
		SELECT hard_limit, soft_limit, burst, window_s
		FROM tenant_quotas
		WHERE tenant_id = $1 AND dimension = $2
		FOR UPDATE
	`, scopedTenantID, dimension).Scan(&hardLimit, &softLimit, &burst, &windowS)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("load tenant quota: %w", err)
	}
	if err == pgx.ErrNoRows {
		return &TenantQuotaDecision{
			TenantID:  scopedTenantID,
			Dimension: dimension,
			Allowed:   true,
			Used:      0,
			Limit:     0,
			WindowS:   defaultTenantWindow(dimension),
		}, nil
	}

	_ = softLimit
	if windowS <= 0 {
		windowS = defaultTenantWindow(dimension)
	}
	limit := safeQuotaLimit(hardLimit, burst)
	if limit <= 0 {
		return &TenantQuotaDecision{
			TenantID:  scopedTenantID,
			Dimension: dimension,
			Allowed:   true,
			Used:      0,
			Limit:     0,
			WindowS:   windowS,
		}, nil
	}

	now := time.Now().UTC()
	bucket := now.Truncate(time.Second)
	from := now.Add(-time.Duration(windowS) * time.Second)

	_, err = tx.Exec(ctx, `
		INSERT INTO tenant_usage_timeseries (tenant_id, dimension, ts, used)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id, dimension, ts) DO UPDATE SET
			used = tenant_usage_timeseries.used + EXCLUDED.used
	`, scopedTenantID, dimension, bucket, amount)
	if err != nil {
		return nil, fmt.Errorf("increment tenant usage timeseries: %w", err)
	}

	var used int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(used), 0)
		FROM tenant_usage_timeseries
		WHERE tenant_id = $1 AND dimension = $2 AND ts >= $3
	`, scopedTenantID, dimension, from).Scan(&used); err != nil {
		return nil, fmt.Errorf("sum tenant usage window: %w", err)
	}

	decision := &TenantQuotaDecision{
		TenantID:  scopedTenantID,
		Dimension: dimension,
		Allowed:   used <= limit,
		Used:      used,
		Limit:     limit,
		WindowS:   windowS,
	}
	if !decision.Allowed {
		decision.RetryAfterS = windowS
		return decision, nil
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO tenant_usage_current (tenant_id, dimension, used, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (tenant_id, dimension) DO UPDATE SET
			used = EXCLUDED.used,
			updated_at = NOW()
	`, scopedTenantID, dimension, used); err != nil {
		return nil, fmt.Errorf("update tenant usage current for windowed dimension: %w", err)
	}

	_, _ = tx.Exec(ctx, `
		DELETE FROM tenant_usage_timeseries
		WHERE tenant_id = $1 AND dimension = $2 AND ts < $3
	`, scopedTenantID, dimension, now.Add(-time.Duration(windowS*10)*time.Second))

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tenant quota tx: %w", err)
	}

	return decision, nil
}

func (s *PostgresStore) CheckTenantAbsoluteQuota(ctx context.Context, tenantID, dimension string, value int64) (*TenantQuotaDecision, error) {
	scopedTenantID, err := validateScopeIdentifier("tenant id", tenantID)
	if err != nil {
		return nil, err
	}
	dimension, err = normalizeTenantDimension(dimension)
	if err != nil {
		return nil, err
	}
	if value < 0 {
		value = 0
	}

	quota, err := s.getTenantQuota(ctx, scopedTenantID, dimension)
	if err != nil {
		return nil, err
	}
	if err := s.upsertTenantUsageCurrent(ctx, scopedTenantID, dimension, value); err != nil {
		return nil, err
	}

	if quota == nil {
		return &TenantQuotaDecision{
			TenantID:  scopedTenantID,
			Dimension: dimension,
			Allowed:   true,
			Used:      value,
			Limit:     0,
			WindowS:   defaultTenantWindow(dimension),
		}, nil
	}

	limit := safeQuotaLimit(quota.HardLimit, quota.Burst)
	if limit <= 0 {
		return &TenantQuotaDecision{
			TenantID:  scopedTenantID,
			Dimension: dimension,
			Allowed:   true,
			Used:      value,
			Limit:     0,
			WindowS:   quota.WindowS,
		}, nil
	}

	return &TenantQuotaDecision{
		TenantID:  scopedTenantID,
		Dimension: dimension,
		Allowed:   value <= limit,
		Used:      value,
		Limit:     limit,
		WindowS:   quota.WindowS,
	}, nil
}

func (s *PostgresStore) GetTenantFunctionCount(ctx context.Context, tenantID string) (int64, error) {
	scopedTenantID, err := validateScopeIdentifier("tenant id", tenantID)
	if err != nil {
		return 0, err
	}
	var count int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::bigint
		FROM functions
		WHERE tenant_id = $1
	`, scopedTenantID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count tenant functions: %w", err)
	}
	return count, nil
}

func (s *PostgresStore) GetTenantAsyncQueueDepth(ctx context.Context, tenantID string) (int64, error) {
	scopedTenantID, err := validateScopeIdentifier("tenant id", tenantID)
	if err != nil {
		return 0, err
	}
	var depth int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::bigint
		FROM async_invocations
		WHERE tenant_id = $1
		  AND status IN ('queued', 'running')
	`, scopedTenantID).Scan(&depth); err != nil {
		return 0, fmt.Errorf("count tenant async queue depth: %w", err)
	}
	return depth, nil
}

func (s *PostgresStore) ListTenantUsage(ctx context.Context, tenantID string) ([]*TenantUsageRecord, error) {
	scopedTenantID, err := validateScopeIdentifier("tenant id", tenantID)
	if err != nil {
		return nil, err
	}
	if _, err := s.GetTenant(ctx, scopedTenantID); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT tenant_id, dimension, used, updated_at
		FROM tenant_usage_current
		WHERE tenant_id = $1
		ORDER BY dimension
	`, scopedTenantID)
	if err != nil {
		return nil, fmt.Errorf("list tenant usage: %w", err)
	}
	defer rows.Close()

	usage := make([]*TenantUsageRecord, 0)
	for rows.Next() {
		var item TenantUsageRecord
		if err := rows.Scan(&item.TenantID, &item.Dimension, &item.Used, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan tenant usage: %w", err)
		}
		usage = append(usage, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tenant usage rows: %w", err)
	}
	return usage, nil
}

func (s *PostgresStore) RefreshTenantUsage(ctx context.Context, tenantID string) ([]*TenantUsageRecord, error) {
	scopedTenantID, err := validateScopeIdentifier("tenant id", tenantID)
	if err != nil {
		return nil, err
	}
	if _, err := s.GetTenant(ctx, scopedTenantID); err != nil {
		return nil, err
	}

	functionCount, err := s.GetTenantFunctionCount(ctx, scopedTenantID)
	if err != nil {
		return nil, err
	}
	asyncDepth, err := s.GetTenantAsyncQueueDepth(ctx, scopedTenantID)
	if err != nil {
		return nil, err
	}

	var memoryMB int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(COALESCE(NULLIF(data->>'memory_mb', '')::bigint, 0)), 0)::bigint
		FROM functions
		WHERE tenant_id = $1
	`, scopedTenantID).Scan(&memoryMB); err != nil {
		return nil, fmt.Errorf("sum tenant memory_mb: %w", err)
	}

	var vcpuMilli int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(
			COALESCE(
				ROUND(NULLIF(data->'limits'->>'vcpus', '')::numeric * 1000),
				1000
			)::bigint
		), 0)::bigint
		FROM functions
		WHERE tenant_id = $1
	`, scopedTenantID).Scan(&vcpuMilli); err != nil {
		return nil, fmt.Errorf("sum tenant vcpu_milli: %w", err)
	}

	var diskIOPS int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(COALESCE(NULLIF(data->'limits'->>'disk_iops', '')::bigint, 0)), 0)::bigint
		FROM functions
		WHERE tenant_id = $1
	`, scopedTenantID).Scan(&diskIOPS); err != nil {
		return nil, fmt.Errorf("sum tenant disk_iops: %w", err)
	}

	invWindow := defaultTenantWindow(TenantDimensionInvocations)
	if quota, err := s.getTenantQuota(ctx, scopedTenantID, TenantDimensionInvocations); err == nil && quota != nil && quota.WindowS > 0 {
		invWindow = quota.WindowS
	}
	eventWindow := defaultTenantWindow(TenantDimensionEventPublishes)
	if quota, err := s.getTenantQuota(ctx, scopedTenantID, TenantDimensionEventPublishes); err == nil && quota != nil && quota.WindowS > 0 {
		eventWindow = quota.WindowS
	}

	invUsage, err := s.getWindowedTenantUsage(ctx, scopedTenantID, TenantDimensionInvocations, invWindow)
	if err != nil {
		return nil, err
	}
	eventUsage, err := s.getWindowedTenantUsage(ctx, scopedTenantID, TenantDimensionEventPublishes, eventWindow)
	if err != nil {
		return nil, err
	}

	current := map[string]int64{
		TenantDimensionFunctionsCount: functionCount,
		TenantDimensionAsyncQueueDepth: asyncDepth,
		TenantDimensionMemoryMB:       memoryMB,
		TenantDimensionVCPUMilli:      vcpuMilli,
		TenantDimensionDiskIOPS:       diskIOPS,
		TenantDimensionInvocations:    invUsage,
		TenantDimensionEventPublishes: eventUsage,
	}
	for dimension, used := range current {
		if err := s.upsertTenantUsageCurrent(ctx, scopedTenantID, dimension, used); err != nil {
			return nil, err
		}
	}

	return s.ListTenantUsage(ctx, scopedTenantID)
}

