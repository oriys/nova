package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SaveAuditLog persists a single audit log entry.
func (s *PostgresStore) SaveAuditLog(ctx context.Context, log *AuditLog) error {
	if log == nil {
		return fmt.Errorf("audit log is required")
	}
	scope := tenantScopeFromContext(ctx)
	if log.TenantID == "" {
		log.TenantID = scope.TenantID
	}
	if log.Namespace == "" {
		log.Namespace = scope.Namespace
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (id, tenant_id, namespace, actor, actor_type, action,
			resource_type, resource_name, http_method, http_path, status_code,
			request_body, response_summary, ip_address, user_agent, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		log.ID, log.TenantID, log.Namespace, log.Actor, log.ActorType, log.Action,
		log.ResourceType, log.ResourceName, log.HTTPMethod, log.HTTPPath, log.StatusCode,
		log.RequestBody, log.ResponseSummary, log.IPAddress, log.UserAgent, log.CreatedAt,
	)
	return err
}

// ListAuditLogs returns audit log entries matching the given filter, ordered by time descending.
func (s *PostgresStore) ListAuditLogs(ctx context.Context, filter *AuditLogFilter, limit, offset int) ([]*AuditLog, error) {
	scope := tenantScopeFromContext(ctx)
	where := []string{"tenant_id = $1", "namespace = $2"}
	args := []interface{}{scope.TenantID, scope.Namespace}
	idx := 3

	if filter != nil {
		if filter.Actor != "" {
			where = append(where, fmt.Sprintf("actor = $%d", idx))
			args = append(args, filter.Actor)
			idx++
		}
		if filter.ResourceType != "" {
			where = append(where, fmt.Sprintf("resource_type = $%d", idx))
			args = append(args, filter.ResourceType)
			idx++
		}
		if filter.ResourceName != "" {
			where = append(where, fmt.Sprintf("resource_name = $%d", idx))
			args = append(args, filter.ResourceName)
			idx++
		}
		if filter.Action != "" {
			where = append(where, fmt.Sprintf("action = $%d", idx))
			args = append(args, filter.Action)
			idx++
		}
		if filter.Since != nil {
			where = append(where, fmt.Sprintf("created_at >= $%d", idx))
			args = append(args, *filter.Since)
			idx++
		}
		if filter.Until != nil {
			where = append(where, fmt.Sprintf("created_at <= $%d", idx))
			args = append(args, *filter.Until)
			idx++
		}
	}

	query := fmt.Sprintf(`SELECT id, tenant_id, namespace, actor, actor_type, action,
		resource_type, resource_name, http_method, http_path, status_code,
		request_body, response_summary, ip_address, user_agent, created_at
		FROM audit_logs WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		strings.Join(where, " AND "), idx, idx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()

	var logs []*AuditLog
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.TenantID, &l.Namespace, &l.Actor, &l.ActorType,
			&l.Action, &l.ResourceType, &l.ResourceName, &l.HTTPMethod, &l.HTTPPath,
			&l.StatusCode, &l.RequestBody, &l.ResponseSummary, &l.IPAddress, &l.UserAgent,
			&l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		logs = append(logs, &l)
	}
	return logs, nil
}

// GetAuditLog returns a single audit log entry by ID.
func (s *PostgresStore) GetAuditLog(ctx context.Context, id string) (*AuditLog, error) {
	scope := tenantScopeFromContext(ctx)
	var l AuditLog
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, namespace, actor, actor_type, action,
			resource_type, resource_name, http_method, http_path, status_code,
			request_body, response_summary, ip_address, user_agent, created_at
		FROM audit_logs WHERE id = $1 AND tenant_id = $2 AND namespace = $3`,
		id, scope.TenantID, scope.Namespace,
	).Scan(&l.ID, &l.TenantID, &l.Namespace, &l.Actor, &l.ActorType,
		&l.Action, &l.ResourceType, &l.ResourceName, &l.HTTPMethod, &l.HTTPPath,
		&l.StatusCode, &l.RequestBody, &l.ResponseSummary, &l.IPAddress, &l.UserAgent,
		&l.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get audit log: %w", err)
	}
	return &l, nil
}

// CountAuditLogs returns the total number of audit log entries matching the filter.
func (s *PostgresStore) CountAuditLogs(ctx context.Context, filter *AuditLogFilter) (int64, error) {
	scope := tenantScopeFromContext(ctx)
	where := []string{"tenant_id = $1", "namespace = $2"}
	args := []interface{}{scope.TenantID, scope.Namespace}
	idx := 3

	if filter != nil {
		if filter.Actor != "" {
			where = append(where, fmt.Sprintf("actor = $%d", idx))
			args = append(args, filter.Actor)
			idx++
		}
		if filter.ResourceType != "" {
			where = append(where, fmt.Sprintf("resource_type = $%d", idx))
			args = append(args, filter.ResourceType)
			idx++
		}
		if filter.ResourceName != "" {
			where = append(where, fmt.Sprintf("resource_name = $%d", idx))
			args = append(args, filter.ResourceName)
			idx++
		}
		if filter.Action != "" {
			where = append(where, fmt.Sprintf("action = $%d", idx))
			args = append(args, filter.Action)
			idx++
		}
		if filter.Since != nil {
			where = append(where, fmt.Sprintf("created_at >= $%d", idx))
			args = append(args, *filter.Since)
			idx++
		}
		if filter.Until != nil {
			where = append(where, fmt.Sprintf("created_at <= $%d", idx))
			args = append(args, *filter.Until)
			idx++
		}
	}

	query := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs WHERE %s", strings.Join(where, " AND "))
	var count int64
	err := s.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count audit logs: %w", err)
	}
	return count, nil
}

// timePtr creates a pointer to a time.Time value.
func timePtr(t time.Time) *time.Time {
	return &t
}
