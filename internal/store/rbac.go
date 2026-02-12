package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oriys/nova/internal/domain"
)

// ─── Record Types ───────────────────────────────────────────────────────────

// RoleRecord represents a role stored in the database.
type RoleRecord struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	IsSystem  bool      `json:"is_system"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PermissionRecord represents a permission stored in the database.
type PermissionRecord struct {
	ID           string    `json:"id"`
	Code         string    `json:"code"`         // e.g. "function:create"
	ResourceType string    `json:"resource_type"` // e.g. "function"
	Action       string    `json:"action"`        // e.g. "create"
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"created_at"`
}

// RoleAssignmentRecord represents a scoped role assignment.
type RoleAssignmentRecord struct {
	ID            string              `json:"id"`
	TenantID      string              `json:"tenant_id"`
	PrincipalType domain.PrincipalType `json:"principal_type"`
	PrincipalID   string              `json:"principal_id"`
	RoleID        string              `json:"role_id"`
	ScopeType     domain.ScopeType    `json:"scope_type"`
	ScopeID       string              `json:"scope_id"`
	CreatedBy     string              `json:"created_by"`
	CreatedAt     time.Time           `json:"created_at"`
}

// ─── Roles ──────────────────────────────────────────────────────────────────

func (s *PostgresStore) CreateRole(ctx context.Context, role *RoleRecord) (*RoleRecord, error) {
	if role == nil {
		return nil, fmt.Errorf("role is required")
	}
	name := strings.TrimSpace(role.Name)
	if name == "" {
		return nil, fmt.Errorf("role name is required")
	}
	tenantID := strings.TrimSpace(role.TenantID)
	if tenantID == "" {
		tenantID = DefaultTenantID
	}
	id := strings.TrimSpace(role.ID)
	if id == "" {
		return nil, fmt.Errorf("role id is required")
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO rbac_roles (id, tenant_id, name, is_system, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
	`, id, tenantID, name, role.IsSystem)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			return nil, fmt.Errorf("role already exists: %s", name)
		}
		return nil, fmt.Errorf("create role: %w", err)
	}
	return s.GetRole(ctx, id)
}

func (s *PostgresStore) GetRole(ctx context.Context, id string) (*RoleRecord, error) {
	var r RoleRecord
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, is_system, created_at, updated_at
		FROM rbac_roles WHERE id = $1
	`, id).Scan(&r.ID, &r.TenantID, &r.Name, &r.IsSystem, &r.CreatedAt, &r.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("role not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get role: %w", err)
	}
	return &r, nil
}

func (s *PostgresStore) ListRoles(ctx context.Context, tenantID string, limit, offset int) ([]*RoleRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	tid := strings.TrimSpace(tenantID)
	if tid == "" {
		tid = DefaultTenantID
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, is_system, created_at, updated_at
		FROM rbac_roles
		WHERE tenant_id = $1
		ORDER BY name
		LIMIT $2 OFFSET $3
	`, tid, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	roles := make([]*RoleRecord, 0)
	for rows.Next() {
		var r RoleRecord
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.IsSystem, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		roles = append(roles, &r)
	}
	return roles, rows.Err()
}

func (s *PostgresStore) DeleteRole(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM rbac_roles WHERE id = $1 AND is_system = FALSE`, id)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("role not found or is a system role: %s", id)
	}
	return nil
}

// ─── Permissions ────────────────────────────────────────────────────────────

func (s *PostgresStore) CreatePermission(ctx context.Context, perm *PermissionRecord) (*PermissionRecord, error) {
	if perm == nil {
		return nil, fmt.Errorf("permission is required")
	}
	code := strings.TrimSpace(perm.Code)
	if code == "" {
		return nil, fmt.Errorf("permission code is required")
	}
	id := strings.TrimSpace(perm.ID)
	if id == "" {
		return nil, fmt.Errorf("permission id is required")
	}
	resourceType := strings.TrimSpace(perm.ResourceType)
	action := strings.TrimSpace(perm.Action)

	_, err := s.pool.Exec(ctx, `
		INSERT INTO rbac_permissions (id, code, resource_type, action, description, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`, id, code, resourceType, action, perm.Description)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			return nil, fmt.Errorf("permission already exists: %s", code)
		}
		return nil, fmt.Errorf("create permission: %w", err)
	}
	return s.GetPermission(ctx, id)
}

func (s *PostgresStore) GetPermission(ctx context.Context, id string) (*PermissionRecord, error) {
	var p PermissionRecord
	err := s.pool.QueryRow(ctx, `
		SELECT id, code, resource_type, action, description, created_at
		FROM rbac_permissions WHERE id = $1
	`, id).Scan(&p.ID, &p.Code, &p.ResourceType, &p.Action, &p.Description, &p.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("permission not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get permission: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) ListPermissions(ctx context.Context, limit, offset int) ([]*PermissionRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, code, resource_type, action, description, created_at
		FROM rbac_permissions
		ORDER BY code
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()

	perms := make([]*PermissionRecord, 0)
	for rows.Next() {
		var p PermissionRecord
		if err := rows.Scan(&p.ID, &p.Code, &p.ResourceType, &p.Action, &p.Description, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		perms = append(perms, &p)
	}
	return perms, rows.Err()
}

func (s *PostgresStore) DeletePermission(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM rbac_permissions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete permission: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("permission not found: %s", id)
	}
	return nil
}

// ─── Role ↔ Permission Mapping ──────────────────────────────────────────────

func (s *PostgresStore) AssignPermissionToRole(ctx context.Context, roleID, permissionID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO rbac_role_permissions (role_id, permission_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, roleID, permissionID)
	if err != nil {
		return fmt.Errorf("assign permission to role: %w", err)
	}
	return nil
}

func (s *PostgresStore) RevokePermissionFromRole(ctx context.Context, roleID, permissionID string) error {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM rbac_role_permissions WHERE role_id = $1 AND permission_id = $2
	`, roleID, permissionID)
	if err != nil {
		return fmt.Errorf("revoke permission from role: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("role-permission mapping not found")
	}
	return nil
}

func (s *PostgresStore) ListRolePermissions(ctx context.Context, roleID string) ([]*PermissionRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.code, p.resource_type, p.action, p.description, p.created_at
		FROM rbac_permissions p
		JOIN rbac_role_permissions rp ON rp.permission_id = p.id
		WHERE rp.role_id = $1
		ORDER BY p.code
	`, roleID)
	if err != nil {
		return nil, fmt.Errorf("list role permissions: %w", err)
	}
	defer rows.Close()

	perms := make([]*PermissionRecord, 0)
	for rows.Next() {
		var p PermissionRecord
		if err := rows.Scan(&p.ID, &p.Code, &p.ResourceType, &p.Action, &p.Description, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan role permission: %w", err)
		}
		perms = append(perms, &p)
	}
	return perms, rows.Err()
}

// ─── Role Assignments ───────────────────────────────────────────────────────

func (s *PostgresStore) CreateRoleAssignment(ctx context.Context, ra *RoleAssignmentRecord) (*RoleAssignmentRecord, error) {
	if ra == nil {
		return nil, fmt.Errorf("role assignment is required")
	}
	id := strings.TrimSpace(ra.ID)
	if id == "" {
		return nil, fmt.Errorf("role assignment id is required")
	}
	if !domain.ValidPrincipalType(ra.PrincipalType) {
		return nil, fmt.Errorf("invalid principal_type: %s", ra.PrincipalType)
	}
	if !domain.ValidScopeType(ra.ScopeType) {
		return nil, fmt.Errorf("invalid scope_type: %s", ra.ScopeType)
	}
	if strings.TrimSpace(ra.PrincipalID) == "" {
		return nil, fmt.Errorf("principal_id is required")
	}
	if strings.TrimSpace(ra.RoleID) == "" {
		return nil, fmt.Errorf("role_id is required")
	}
	tenantID := strings.TrimSpace(ra.TenantID)
	if tenantID == "" {
		tenantID = DefaultTenantID
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO rbac_role_assignments (id, tenant_id, principal_type, principal_id, role_id, scope_type, scope_id, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`, id, tenantID, string(ra.PrincipalType), ra.PrincipalID, ra.RoleID, string(ra.ScopeType), ra.ScopeID, ra.CreatedBy)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			return nil, fmt.Errorf("role assignment already exists")
		}
		return nil, fmt.Errorf("create role assignment: %w", err)
	}
	return s.GetRoleAssignment(ctx, id)
}

func (s *PostgresStore) GetRoleAssignment(ctx context.Context, id string) (*RoleAssignmentRecord, error) {
	var ra RoleAssignmentRecord
	var principalType, scopeType string
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, principal_type, principal_id, role_id, scope_type, scope_id, created_by, created_at
		FROM rbac_role_assignments WHERE id = $1
	`, id).Scan(&ra.ID, &ra.TenantID, &principalType, &ra.PrincipalID, &ra.RoleID, &scopeType, &ra.ScopeID, &ra.CreatedBy, &ra.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("role assignment not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get role assignment: %w", err)
	}
	ra.PrincipalType = domain.PrincipalType(principalType)
	ra.ScopeType = domain.ScopeType(scopeType)
	return &ra, nil
}

func (s *PostgresStore) ListRoleAssignments(ctx context.Context, tenantID string, limit, offset int) ([]*RoleAssignmentRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	tid := strings.TrimSpace(tenantID)
	if tid == "" {
		tid = DefaultTenantID
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, principal_type, principal_id, role_id, scope_type, scope_id, created_by, created_at
		FROM rbac_role_assignments
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tid, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list role assignments: %w", err)
	}
	defer rows.Close()

	assignments := make([]*RoleAssignmentRecord, 0)
	for rows.Next() {
		var ra RoleAssignmentRecord
		var principalType, scopeType string
		if err := rows.Scan(&ra.ID, &ra.TenantID, &principalType, &ra.PrincipalID, &ra.RoleID, &scopeType, &ra.ScopeID, &ra.CreatedBy, &ra.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan role assignment: %w", err)
		}
		ra.PrincipalType = domain.PrincipalType(principalType)
		ra.ScopeType = domain.ScopeType(scopeType)
		assignments = append(assignments, &ra)
	}
	return assignments, rows.Err()
}

func (s *PostgresStore) ListRoleAssignmentsByPrincipal(ctx context.Context, tenantID string, principalType domain.PrincipalType, principalID string) ([]*RoleAssignmentRecord, error) {
	tid := strings.TrimSpace(tenantID)
	if tid == "" {
		tid = DefaultTenantID
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, principal_type, principal_id, role_id, scope_type, scope_id, created_by, created_at
		FROM rbac_role_assignments
		WHERE tenant_id = $1 AND principal_type = $2 AND principal_id = $3
		ORDER BY created_at DESC
	`, tid, string(principalType), principalID)
	if err != nil {
		return nil, fmt.Errorf("list role assignments by principal: %w", err)
	}
	defer rows.Close()

	assignments := make([]*RoleAssignmentRecord, 0)
	for rows.Next() {
		var ra RoleAssignmentRecord
		var pt, st string
		if err := rows.Scan(&ra.ID, &ra.TenantID, &pt, &ra.PrincipalID, &ra.RoleID, &st, &ra.ScopeID, &ra.CreatedBy, &ra.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan role assignment: %w", err)
		}
		ra.PrincipalType = domain.PrincipalType(pt)
		ra.ScopeType = domain.ScopeType(st)
		assignments = append(assignments, &ra)
	}
	return assignments, rows.Err()
}

func (s *PostgresStore) DeleteRoleAssignment(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM rbac_role_assignments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete role assignment: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("role assignment not found: %s", id)
	}
	return nil
}
