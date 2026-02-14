package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// MenuPermissionRecord represents a menu permission assigned to a tenant.
type MenuPermissionRecord struct {
	TenantID  string    `json:"tenant_id"`
	MenuKey   string    `json:"menu_key"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// ButtonPermissionRecord represents a button permission assigned to a tenant.
type ButtonPermissionRecord struct {
	TenantID      string    `json:"tenant_id"`
	PermissionKey string    `json:"permission_key"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
}

// AllMenuKeys defines every menu key the UI can render.
var AllMenuKeys = []string{
	"apiDocs",
	"apiKeys",
	"asyncJobs",
	"configurations",
	"dashboard",
	"events",
	"functions",
	"gateway",
	"history",
	"layers",
	"notifications",
	"rbac",
	"runtimes",
	"secrets",
	"snapshots",
	"tenancy",
	"volumes",
	"workflows",
}

// DefaultTenantOnlyMenuKeys are menu keys that only the default (platform)
// tenant receives when a brand-new tenant is created.
var DefaultTenantOnlyMenuKeys = map[string]bool{
	"tenancy":        true,
	"configurations": true,
	"rbac":           true,
}

// AllButtonPermissionKeys defines every button-level permission the UI can check.
var AllButtonPermissionKeys = []string{
	"function:create",
	"function:update",
	"function:delete",
	"function:invoke",
	"runtime:write",
	"config:write",
	"secret:manage",
	"apikey:manage",
	"workflow:manage",
	"schedule:manage",
	"gateway:manage",
	"rbac:manage",
}

// ─── Menu Permissions ───────────────────────────────────────────────────────

func (s *PostgresStore) ListTenantMenuPermissions(ctx context.Context, tenantID string) ([]*MenuPermissionRecord, error) {
	tid := strings.TrimSpace(tenantID)
	if tid == "" {
		tid = DefaultTenantID
	}
	rows, err := s.pool.Query(ctx, `
		SELECT tenant_id, menu_key, enabled, created_at
		FROM tenant_menu_permissions
		WHERE tenant_id = $1
		ORDER BY menu_key
	`, tid)
	if err != nil {
		return nil, fmt.Errorf("list tenant menu permissions: %w", err)
	}
	defer rows.Close()

	perms := make([]*MenuPermissionRecord, 0)
	for rows.Next() {
		var p MenuPermissionRecord
		if err := rows.Scan(&p.TenantID, &p.MenuKey, &p.Enabled, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan menu permission: %w", err)
		}
		perms = append(perms, &p)
	}
	return perms, rows.Err()
}

func (s *PostgresStore) UpsertTenantMenuPermission(ctx context.Context, tenantID, menuKey string, enabled bool) (*MenuPermissionRecord, error) {
	tid := strings.TrimSpace(tenantID)
	if tid == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	key := strings.TrimSpace(menuKey)
	if key == "" {
		return nil, fmt.Errorf("menu_key is required")
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO tenant_menu_permissions (tenant_id, menu_key, enabled, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (tenant_id, menu_key)
		DO UPDATE SET enabled = EXCLUDED.enabled
	`, tid, key, enabled)
	if err != nil {
		return nil, fmt.Errorf("upsert tenant menu permission: %w", err)
	}

	var p MenuPermissionRecord
	err = s.pool.QueryRow(ctx, `
		SELECT tenant_id, menu_key, enabled, created_at
		FROM tenant_menu_permissions
		WHERE tenant_id = $1 AND menu_key = $2
	`, tid, key).Scan(&p.TenantID, &p.MenuKey, &p.Enabled, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get tenant menu permission: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) DeleteTenantMenuPermission(ctx context.Context, tenantID, menuKey string) error {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM tenant_menu_permissions WHERE tenant_id = $1 AND menu_key = $2
	`, tenantID, menuKey)
	if err != nil {
		return fmt.Errorf("delete tenant menu permission: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("menu permission not found: %s/%s", tenantID, menuKey)
	}
	return nil
}

// ─── Button Permissions ─────────────────────────────────────────────────────

func (s *PostgresStore) ListTenantButtonPermissions(ctx context.Context, tenantID string) ([]*ButtonPermissionRecord, error) {
	tid := strings.TrimSpace(tenantID)
	if tid == "" {
		tid = DefaultTenantID
	}
	rows, err := s.pool.Query(ctx, `
		SELECT tenant_id, permission_key, enabled, created_at
		FROM tenant_button_permissions
		WHERE tenant_id = $1
		ORDER BY permission_key
	`, tid)
	if err != nil {
		return nil, fmt.Errorf("list tenant button permissions: %w", err)
	}
	defer rows.Close()

	perms := make([]*ButtonPermissionRecord, 0)
	for rows.Next() {
		var p ButtonPermissionRecord
		if err := rows.Scan(&p.TenantID, &p.PermissionKey, &p.Enabled, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan button permission: %w", err)
		}
		perms = append(perms, &p)
	}
	return perms, rows.Err()
}

func (s *PostgresStore) UpsertTenantButtonPermission(ctx context.Context, tenantID, permissionKey string, enabled bool) (*ButtonPermissionRecord, error) {
	tid := strings.TrimSpace(tenantID)
	if tid == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	key := strings.TrimSpace(permissionKey)
	if key == "" {
		return nil, fmt.Errorf("permission_key is required")
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO tenant_button_permissions (tenant_id, permission_key, enabled, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (tenant_id, permission_key)
		DO UPDATE SET enabled = EXCLUDED.enabled
	`, tid, key, enabled)
	if err != nil {
		return nil, fmt.Errorf("upsert tenant button permission: %w", err)
	}

	var p ButtonPermissionRecord
	err = s.pool.QueryRow(ctx, `
		SELECT tenant_id, permission_key, enabled, created_at
		FROM tenant_button_permissions
		WHERE tenant_id = $1 AND permission_key = $2
	`, tid, key).Scan(&p.TenantID, &p.PermissionKey, &p.Enabled, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get tenant button permission: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) DeleteTenantButtonPermission(ctx context.Context, tenantID, permissionKey string) error {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM tenant_button_permissions WHERE tenant_id = $1 AND permission_key = $2
	`, tenantID, permissionKey)
	if err != nil {
		return fmt.Errorf("delete tenant button permission: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("button permission not found: %s/%s", tenantID, permissionKey)
	}
	return nil
}

// ─── Seed Helpers ───────────────────────────────────────────────────────────

// dbExecer is satisfied by both *pgxpool.Pool and pgx.Tx.
type dbExecer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// seedMenuPermissions inserts the default menu permissions using the given
// executor (a pool or transaction).
func seedMenuPermissions(ctx context.Context, exec dbExecer, tenantID string) error {
	isDefault := tenantID == DefaultTenantID
	for _, key := range AllMenuKeys {
		enabled := true
		if !isDefault && DefaultTenantOnlyMenuKeys[key] {
			enabled = false
		}
		_, err := exec.Exec(ctx, `
			INSERT INTO tenant_menu_permissions (tenant_id, menu_key, enabled, created_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (tenant_id, menu_key) DO NOTHING
		`, tenantID, key, enabled)
		if err != nil {
			return fmt.Errorf("seed menu permission %s for tenant %s: %w", key, tenantID, err)
		}
	}
	return nil
}

// seedButtonPermissions inserts the default button permissions using the given
// executor (a pool or transaction).
func seedButtonPermissions(ctx context.Context, exec dbExecer, tenantID string) error {
	for _, key := range AllButtonPermissionKeys {
		_, err := exec.Exec(ctx, `
			INSERT INTO tenant_button_permissions (tenant_id, permission_key, enabled, created_at)
			VALUES ($1, $2, TRUE, NOW())
			ON CONFLICT (tenant_id, permission_key) DO NOTHING
		`, tenantID, key)
		if err != nil {
			return fmt.Errorf("seed button permission %s for tenant %s: %w", key, tenantID, err)
		}
	}
	return nil
}

// SeedDefaultMenuPermissions inserts the default set of menu permissions for a
// tenant. The default (platform) tenant gets all menus enabled; other tenants
// get everything except the "defaultOnly" menus.
func (s *PostgresStore) SeedDefaultMenuPermissions(ctx context.Context, tenantID string) error {
	return seedMenuPermissions(ctx, s.pool, tenantID)
}

// SeedDefaultButtonPermissions inserts the default set of button permissions
// for a tenant. All button permissions are enabled by default.
func (s *PostgresStore) SeedDefaultButtonPermissions(ctx context.Context, tenantID string) error {
	return seedButtonPermissions(ctx, s.pool, tenantID)
}
