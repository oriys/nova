package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// NotificationStatus values.
type NotificationStatus string

const (
	NotificationStatusUnread NotificationStatus = "unread"
	NotificationStatusRead   NotificationStatus = "read"
	NotificationStatusAll    NotificationStatus = "all"
)

// NotificationRecord represents a UI notification shown in Lumen's bell menu.
type NotificationRecord struct {
	ID           string             `json:"id"`
	TenantID     string             `json:"tenant_id,omitempty"`
	Namespace    string             `json:"namespace,omitempty"`
	Type         string             `json:"type"`
	Severity     string             `json:"severity"`
	Source       string             `json:"source,omitempty"`
	FunctionID   string             `json:"function_id,omitempty"`
	FunctionName string             `json:"function_name,omitempty"`
	Title        string             `json:"title"`
	Message      string             `json:"message"`
	Data         json.RawMessage    `json:"data,omitempty"`
	Status       NotificationStatus `json:"status"`
	CreatedAt    time.Time          `json:"created_at"`
	ReadAt       *time.Time         `json:"read_at,omitempty"`
}

func (s *PostgresStore) CreateNotification(ctx context.Context, n *NotificationRecord) error {
	if n == nil {
		return fmt.Errorf("notification is required")
	}
	if strings.TrimSpace(n.ID) == "" {
		return fmt.Errorf("notification id is required")
	}
	if strings.TrimSpace(n.Title) == "" {
		return fmt.Errorf("notification title is required")
	}
	if strings.TrimSpace(n.Message) == "" {
		return fmt.Errorf("notification message is required")
	}

	scope := tenantScopeFromContext(ctx)
	if n.TenantID == "" {
		n.TenantID = scope.TenantID
	}
	if n.Namespace == "" {
		n.Namespace = scope.Namespace
	}
	if n.Status == "" || n.Status == NotificationStatusAll {
		n.Status = NotificationStatusUnread
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}
	if len(n.Data) == 0 {
		n.Data = json.RawMessage(`{}`)
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO notifications (
			id, tenant_id, namespace, type, severity, source,
			function_id, function_name, title, message, data, status, created_at, read_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12, $13, $14
		)
		ON CONFLICT (id) DO NOTHING
	`, n.ID, n.TenantID, n.Namespace, n.Type, n.Severity, n.Source, n.FunctionID, n.FunctionName, n.Title, n.Message, n.Data, string(n.Status), n.CreatedAt, n.ReadAt)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListNotifications(ctx context.Context, limit, offset int, status NotificationStatus) ([]*NotificationRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	status = normalizeNotificationStatus(status)
	scope := tenantScopeFromContext(ctx)

	rows, err := s.pool.Query(ctx, `
		SELECT
			id, tenant_id, namespace, type, severity, source,
			function_id, function_name, title, message, data, status, created_at, read_at
		FROM notifications
		WHERE tenant_id = $1
		  AND namespace = $2
		  AND ($3 = 'all' OR status = $3)
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5
	`, scope.TenantID, scope.Namespace, string(status), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	records := make([]*NotificationRecord, 0)
	for rows.Next() {
		var rec NotificationRecord
		var source, functionID, functionName *string
		var data []byte
		var readAt *time.Time
		if err := rows.Scan(
			&rec.ID,
			&rec.TenantID,
			&rec.Namespace,
			&rec.Type,
			&rec.Severity,
			&source,
			&functionID,
			&functionName,
			&rec.Title,
			&rec.Message,
			&data,
			&rec.Status,
			&rec.CreatedAt,
			&readAt,
		); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		if source != nil {
			rec.Source = *source
		}
		if functionID != nil {
			rec.FunctionID = *functionID
		}
		if functionName != nil {
			rec.FunctionName = *functionName
		}
		if len(data) > 0 {
			rec.Data = data
		}
		rec.ReadAt = readAt
		records = append(records, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notifications rows: %w", err)
	}
	return records, nil
}

func (s *PostgresStore) GetUnreadNotificationCount(ctx context.Context) (int64, error) {
	scope := tenantScopeFromContext(ctx)

	var count int64
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM notifications
		WHERE tenant_id = $1 AND namespace = $2 AND status = 'unread'
	`, scope.TenantID, scope.Namespace).Scan(&count); err != nil {
		return 0, fmt.Errorf("get unread notification count: %w", err)
	}
	return count, nil
}

func (s *PostgresStore) MarkNotificationRead(ctx context.Context, id string) (*NotificationRecord, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("notification id is required")
	}
	scope := tenantScopeFromContext(ctx)

	var rec NotificationRecord
	var source, functionID, functionName *string
	var data []byte
	var readAt *time.Time
	err := s.pool.QueryRow(ctx, `
		UPDATE notifications
		SET status = 'read',
		    read_at = COALESCE(read_at, NOW())
		WHERE id = $1 AND tenant_id = $2 AND namespace = $3
		RETURNING
			id, tenant_id, namespace, type, severity, source,
			function_id, function_name, title, message, data, status, created_at, read_at
	`, id, scope.TenantID, scope.Namespace).Scan(
		&rec.ID,
		&rec.TenantID,
		&rec.Namespace,
		&rec.Type,
		&rec.Severity,
		&source,
		&functionID,
		&functionName,
		&rec.Title,
		&rec.Message,
		&data,
		&rec.Status,
		&rec.CreatedAt,
		&readAt,
	)
	if err != nil {
		return nil, fmt.Errorf("mark notification read: %w", err)
	}
	if source != nil {
		rec.Source = *source
	}
	if functionID != nil {
		rec.FunctionID = *functionID
	}
	if functionName != nil {
		rec.FunctionName = *functionName
	}
	if len(data) > 0 {
		rec.Data = data
	}
	rec.ReadAt = readAt
	return &rec, nil
}

func (s *PostgresStore) MarkAllNotificationsRead(ctx context.Context) (int64, error) {
	scope := tenantScopeFromContext(ctx)

	ct, err := s.pool.Exec(ctx, `
		UPDATE notifications
		SET status = 'read',
		    read_at = COALESCE(read_at, NOW())
		WHERE tenant_id = $1
		  AND namespace = $2
		  AND status = 'unread'
	`, scope.TenantID, scope.Namespace)
	if err != nil {
		return 0, fmt.Errorf("mark all notifications read: %w", err)
	}
	return ct.RowsAffected(), nil
}

func normalizeNotificationStatus(status NotificationStatus) NotificationStatus {
	switch NotificationStatus(strings.ToLower(strings.TrimSpace(string(status)))) {
	case NotificationStatusUnread:
		return NotificationStatusUnread
	case NotificationStatusRead:
		return NotificationStatusRead
	case NotificationStatusAll:
		return NotificationStatusAll
	default:
		return NotificationStatusAll
	}
}
