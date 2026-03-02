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
	"accessControl",
	"alerting",
	"alerts",
	"apiDocs",
	"apiKeys",
	"asyncJobs",
	"auditLogs",
	"cluster",
	"configurations",
	"dashboard",
	"docs",
	"events",
	"functions",
	"gateway",
	"history",
	"invocations",
	"layers",
	"notifications",
	"rbac",
	"replay",
	"runtimes",
	"secrets",
	"settings",
	"snapshots",
	"tenancy",
	"tickets",
	"triggers",
	"tuning",
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

// DefaultTenantOnlyButtonPermKeys are button-level permissions reserved for
// the default (platform) tenant.  Non-default tenants get these disabled
// because the underlying resources are platform-global.
var DefaultTenantOnlyButtonPermKeys = map[string]bool{
	"runtime:write":  true, // runtimes are shared across tenants
	"config:write":   true, // platform configuration is global
	"gateway:manage": true, // gateway routing is platform-level
	"rbac:manage":    true, // RBAC management is platform-admin only
}

// ─── System Roles ───────────────────────────────────────────────────────────

const (
	SystemRoleAdminID = "system:admin"
	SystemRoleBasicID = "system:basic"
)

// AllPermissionCodes lists every permission code that should be seeded into
// the rbac_permissions table.
var AllPermissionCodes = []struct {
	Code         string
	ResourceType string
	Action       string
	Description  string
}{
	{"function:create", "function", "create", "Create functions"},
	{"function:read", "function", "read", "View functions"},
	{"function:update", "function", "update", "Update functions"},
	{"function:delete", "function", "delete", "Delete functions"},
	{"function:invoke", "function", "invoke", "Invoke functions"},
	{"runtime:read", "runtime", "read", "View runtimes"},
	{"runtime:write", "runtime", "write", "Manage runtimes"},
	{"config:read", "config", "read", "View platform configuration"},
	{"config:write", "config", "write", "Modify platform configuration"},
	{"snapshot:read", "snapshot", "read", "View snapshots"},
	{"snapshot:write", "snapshot", "write", "Manage snapshots"},
	{"apikey:manage", "apikey", "manage", "Manage API keys"},
	{"secret:manage", "secret", "manage", "Manage secrets"},
	{"workflow:manage", "workflow", "manage", "Manage workflows"},
	{"workflow:invoke", "workflow", "invoke", "Invoke workflows"},
	{"schedule:manage", "schedule", "manage", "Manage schedules"},
	{"gateway:manage", "gateway", "manage", "Manage gateway routes"},
	{"log:read", "log", "read", "View logs"},
	{"metrics:read", "metrics", "read", "View metrics"},
	{"app:publish", "app", "publish", "Publish applications"},
	{"app:read", "app", "read", "View applications"},
	{"app:install", "app", "install", "Install applications"},
	{"app:manage", "app", "manage", "Manage applications"},
	{"rbac:manage", "rbac", "manage", "Manage RBAC roles and assignments"},
	{"ticket:create", "ticket", "create", "Create tickets"},
	{"ticket:read", "ticket", "read", "View tickets"},
	{"ticket:update", "ticket", "update", "Update tickets"},
	{"ticket:delete", "ticket", "delete", "Delete tickets"},
}

// basicRolePermissions lists the permission codes granted to the basic tenant
// role. Platform-admin-only permissions are excluded.
var basicRolePermissions = map[string]bool{
	"function:create": true,
	"function:read":   true,
	"function:update": true,
	"function:delete": true,
	"function:invoke": true,
	"runtime:read":    true,
	"snapshot:read":   true,
	"snapshot:write":  true,
	"apikey:manage":   true,
	"secret:manage":   true,
	"workflow:manage": true,
	"workflow:invoke": true,
	"schedule:manage": true,
	"log:read":        true,
	"metrics:read":    true,
	"ticket:create":   true,
	"ticket:read":     true,
	"ticket:update":   true,
	"app:read":        true,
	"app:install":     true,
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
	isDefault := tenantID == DefaultTenantID
	for _, key := range AllButtonPermissionKeys {
		enabled := true
		if !isDefault && DefaultTenantOnlyButtonPermKeys[key] {
			enabled = false
		}
		_, err := exec.Exec(ctx, `
			INSERT INTO tenant_button_permissions (tenant_id, permission_key, enabled, created_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (tenant_id, permission_key) DO NOTHING
		`, tenantID, key, enabled)
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
// for a tenant. The default tenant gets all buttons enabled; other tenants
// get everything except platform-admin-only buttons.
func (s *PostgresStore) SeedDefaultButtonPermissions(ctx context.Context, tenantID string) error {
	return seedButtonPermissions(ctx, s.pool, tenantID)
}

// FixNonDefaultTenantButtonPermissions disables platform-admin-only button
// permissions for all non-default tenants. This is a one-time migration for
// tenants created before the restricted seeding was introduced.
func (s *PostgresStore) FixNonDefaultTenantButtonPermissions(ctx context.Context) (int64, error) {
	keys := make([]string, 0, len(DefaultTenantOnlyButtonPermKeys))
	for k := range DefaultTenantOnlyButtonPermKeys {
		keys = append(keys, k)
	}
	ct, err := s.pool.Exec(ctx, `
		UPDATE tenant_button_permissions
		SET enabled = FALSE
		WHERE tenant_id != $1
		  AND permission_key = ANY($2)
		  AND enabled = TRUE
	`, DefaultTenantID, keys)
	if err != nil {
		return 0, fmt.Errorf("fix non-default tenant button permissions: %w", err)
	}
	return ct.RowsAffected(), nil
}

// ─── System Role Seeding ────────────────────────────────────────────────────

// seedSystemRolesAndPermissions seeds rbac_permissions, the two system roles
// (admin & basic), their permission mappings, and the default-tenant admin
// assignment.  It is idempotent (ON CONFLICT DO NOTHING everywhere).
func seedSystemRolesAndPermissions(ctx context.Context, exec dbExecer) error {
	// 1. Seed all permission rows.
	for _, p := range AllPermissionCodes {
		if _, err := exec.Exec(ctx, `
			INSERT INTO rbac_permissions (id, code, resource_type, action, description, created_at)
			VALUES ($1, $2, $3, $4, $5, NOW())
			ON CONFLICT (id) DO NOTHING
		`, p.Code, p.Code, p.ResourceType, p.Action, p.Description); err != nil {
			return fmt.Errorf("seed permission %s: %w", p.Code, err)
		}
	}

	// 2. Seed system roles.
	for _, role := range []struct {
		id   string
		name string
	}{
		{SystemRoleAdminID, "admin"},
		{SystemRoleBasicID, "basic"},
	} {
		if _, err := exec.Exec(ctx, `
			INSERT INTO rbac_roles (id, tenant_id, name, is_system, created_at, updated_at)
			VALUES ($1, $2, $3, TRUE, NOW(), NOW())
			ON CONFLICT (id) DO NOTHING
		`, role.id, DefaultTenantID, role.name); err != nil {
			return fmt.Errorf("seed system role %s: %w", role.name, err)
		}
	}

	// 3. Map permissions to roles.
	for _, p := range AllPermissionCodes {
		// admin gets every permission
		if _, err := exec.Exec(ctx, `
			INSERT INTO rbac_role_permissions (role_id, permission_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, SystemRoleAdminID, p.Code); err != nil {
			return fmt.Errorf("map admin permission %s: %w", p.Code, err)
		}

		// basic gets only the non-platform-admin permissions
		if basicRolePermissions[p.Code] {
			if _, err := exec.Exec(ctx, `
				INSERT INTO rbac_role_permissions (role_id, permission_id)
				VALUES ($1, $2)
				ON CONFLICT DO NOTHING
			`, SystemRoleBasicID, p.Code); err != nil {
				return fmt.Errorf("map basic permission %s: %w", p.Code, err)
			}
		}
	}

	// 4. Assign admin role to default tenant.
	if _, err := exec.Exec(ctx, `
		INSERT INTO rbac_role_assignments
			(id, tenant_id, principal_type, principal_id, role_id, scope_type, scope_id, created_by, created_at)
		VALUES ($1, $2, 'group', $2, $3, 'tenant', $2, 'system', NOW())
		ON CONFLICT (id) DO NOTHING
	`, "system:default:admin", DefaultTenantID, SystemRoleAdminID); err != nil {
		return fmt.Errorf("assign admin role to default tenant: %w", err)
	}

	return nil
}

// seedTenantBasicRole assigns the basic system role to a tenant.
// exec may be a pool or transaction.
func seedTenantBasicRole(ctx context.Context, exec dbExecer, tenantID string) error {
	assignmentID := "system:" + tenantID + ":basic"
	if _, err := exec.Exec(ctx, `
		INSERT INTO rbac_role_assignments
			(id, tenant_id, principal_type, principal_id, role_id, scope_type, scope_id, created_by, created_at)
		VALUES ($1, $2, 'group', $2, $3, 'tenant', $2, 'system', NOW())
		ON CONFLICT (id) DO NOTHING
	`, assignmentID, tenantID, SystemRoleBasicID); err != nil {
		return fmt.Errorf("assign basic role to tenant %s: %w", tenantID, err)
	}
	return nil
}

// SeedBasicRoleForExistingTenants assigns the basic role to all non-default
// tenants that don't already have it.  Idempotent.
func (s *PostgresStore) SeedBasicRoleForExistingTenants(ctx context.Context) (int64, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id FROM tenants WHERE id != $1
	`, DefaultTenantID)
	if err != nil {
		return 0, fmt.Errorf("list non-default tenants: %w", err)
	}
	defer rows.Close()

	var count int64
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err != nil {
			return count, fmt.Errorf("scan tenant id: %w", err)
		}
		if err := seedTenantBasicRole(ctx, s.pool, tid); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}
