package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// TenantRecord represents a tenant configuration record.
type TenantRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Tier      string    `json:"tier"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TenantUpdate contains mutable tenant fields.
type TenantUpdate struct {
	Name   *string `json:"name,omitempty"`
	Status *string `json:"status,omitempty"`
	Tier   *string `json:"tier,omitempty"`
}

// NamespaceRecord represents a namespace under a tenant.
type NamespaceRecord struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NamespaceUpdate contains mutable namespace fields.
type NamespaceUpdate struct {
	Name *string `json:"name,omitempty"`
}

func (s *PostgresStore) ListTenants(ctx context.Context, limit, offset int) ([]*TenantRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, status, tier, created_at, updated_at
		FROM tenants
		ORDER BY id
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	tenants := make([]*TenantRecord, 0)
	for rows.Next() {
		var tenant TenantRecord
		if err := rows.Scan(
			&tenant.ID,
			&tenant.Name,
			&tenant.Status,
			&tenant.Tier,
			&tenant.CreatedAt,
			&tenant.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, &tenant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tenants rows: %w", err)
	}
	return tenants, nil
}

func (s *PostgresStore) GetTenant(ctx context.Context, id string) (*TenantRecord, error) {
	tenantID, err := validateScopeIdentifier("tenant id", id)
	if err != nil {
		return nil, err
	}

	var tenant TenantRecord
	err = s.pool.QueryRow(ctx, `
		SELECT id, name, status, tier, created_at, updated_at
		FROM tenants
		WHERE id = $1
	`, tenantID).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Status,
		&tenant.Tier,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("tenant not found: %s", tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	return &tenant, nil
}

func (s *PostgresStore) CreateTenant(ctx context.Context, tenant *TenantRecord) (*TenantRecord, error) {
	if tenant == nil {
		return nil, fmt.Errorf("tenant is required")
	}

	tenantID, err := validateScopeIdentifier("tenant id", tenant.ID)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(tenant.Name)
	if name == "" {
		name = tenantID
	}
	status := strings.TrimSpace(tenant.Status)
	if status == "" {
		status = "active"
	}
	tier := strings.TrimSpace(tenant.Tier)
	if tier == "" {
		tier = "default"
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tenant create tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO tenants (id, name, status, tier, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
	`, tenantID, name, status, tier)
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO namespaces (id, tenant_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, buildNamespaceID(tenantID, DefaultNamespace), tenantID, DefaultNamespace)
	if err != nil {
		return nil, fmt.Errorf("create default namespace for tenant %s: %w", tenantID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tenant create tx: %w", err)
	}

	return s.GetTenant(ctx, tenantID)
}

func (s *PostgresStore) UpdateTenant(ctx context.Context, id string, update *TenantUpdate) (*TenantRecord, error) {
	tenantID, err := validateScopeIdentifier("tenant id", id)
	if err != nil {
		return nil, err
	}

	tenant, err := s.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if update == nil {
		return tenant, nil
	}

	if update.Name != nil {
		name := strings.TrimSpace(*update.Name)
		if name == "" {
			return nil, fmt.Errorf("name is required")
		}
		tenant.Name = name
	}
	if update.Status != nil {
		status := strings.TrimSpace(*update.Status)
		if status == "" {
			return nil, fmt.Errorf("status is required")
		}
		tenant.Status = status
	}
	if update.Tier != nil {
		tier := strings.TrimSpace(*update.Tier)
		if tier == "" {
			return nil, fmt.Errorf("tier is required")
		}
		tenant.Tier = tier
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE tenants
		SET name = $2, status = $3, tier = $4, updated_at = NOW()
		WHERE id = $1
	`, tenantID, tenant.Name, tenant.Status, tenant.Tier)
	if err != nil {
		return nil, fmt.Errorf("update tenant: %w", err)
	}

	return s.GetTenant(ctx, tenantID)
}

func (s *PostgresStore) DeleteTenant(ctx context.Context, id string) error {
	tenantID, err := validateScopeIdentifier("tenant id", id)
	if err != nil {
		return err
	}
	if tenantID == DefaultTenantID {
		return fmt.Errorf("default tenant cannot be deleted")
	}

	inUse, err := s.tenantHasManagedResources(ctx, tenantID)
	if err != nil {
		return err
	}
	if inUse {
		return fmt.Errorf("tenant %s still has managed resources", tenantID)
	}

	ct, err := s.pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	if err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("tenant not found: %s", tenantID)
	}
	return nil
}

func (s *PostgresStore) ListNamespaces(ctx context.Context, tenantID string, limit, offset int) ([]*NamespaceRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	scopedTenantID, err := validateScopeIdentifier("tenant id", tenantID)
	if err != nil {
		return nil, err
	}

	if _, err := s.GetTenant(ctx, scopedTenantID); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, created_at, updated_at
		FROM namespaces
		WHERE tenant_id = $1
		ORDER BY name
		LIMIT $2 OFFSET $3
	`, scopedTenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	defer rows.Close()

	namespaces := make([]*NamespaceRecord, 0)
	for rows.Next() {
		var namespace NamespaceRecord
		if err := rows.Scan(
			&namespace.ID,
			&namespace.TenantID,
			&namespace.Name,
			&namespace.CreatedAt,
			&namespace.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan namespace: %w", err)
		}
		namespaces = append(namespaces, &namespace)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list namespaces rows: %w", err)
	}
	return namespaces, nil
}

func (s *PostgresStore) GetNamespace(ctx context.Context, tenantID, name string) (*NamespaceRecord, error) {
	scopedTenantID, err := validateScopeIdentifier("tenant id", tenantID)
	if err != nil {
		return nil, err
	}
	namespaceName, err := validateScopeIdentifier("namespace", name)
	if err != nil {
		return nil, err
	}

	namespaceID := buildNamespaceID(scopedTenantID, namespaceName)
	var namespace NamespaceRecord
	err = s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, created_at, updated_at
		FROM namespaces
		WHERE id = $1
	`, namespaceID).Scan(
		&namespace.ID,
		&namespace.TenantID,
		&namespace.Name,
		&namespace.CreatedAt,
		&namespace.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("namespace not found: %s/%s", scopedTenantID, namespaceName)
	}
	if err != nil {
		return nil, fmt.Errorf("get namespace: %w", err)
	}
	return &namespace, nil
}

func (s *PostgresStore) CreateNamespace(ctx context.Context, namespace *NamespaceRecord) (*NamespaceRecord, error) {
	if namespace == nil {
		return nil, fmt.Errorf("namespace is required")
	}
	scopedTenantID, err := validateScopeIdentifier("tenant id", namespace.TenantID)
	if err != nil {
		return nil, err
	}
	namespaceName, err := validateScopeIdentifier("namespace", namespace.Name)
	if err != nil {
		return nil, err
	}
	namespaceID := buildNamespaceID(scopedTenantID, namespaceName)

	_, err = s.pool.Exec(ctx, `
		INSERT INTO namespaces (id, tenant_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
	`, namespaceID, scopedTenantID, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("create namespace: %w", err)
	}

	return s.GetNamespace(ctx, scopedTenantID, namespaceName)
}

func (s *PostgresStore) UpdateNamespace(ctx context.Context, tenantID, name string, update *NamespaceUpdate) (*NamespaceRecord, error) {
	scopedTenantID, err := validateScopeIdentifier("tenant id", tenantID)
	if err != nil {
		return nil, err
	}
	namespaceName, err := validateScopeIdentifier("namespace", name)
	if err != nil {
		return nil, err
	}

	existing, err := s.GetNamespace(ctx, scopedTenantID, namespaceName)
	if err != nil {
		return nil, err
	}
	if update == nil || update.Name == nil {
		return existing, nil
	}

	newName, err := validateScopeIdentifier("namespace", *update.Name)
	if err != nil {
		return nil, err
	}
	if newName == namespaceName {
		return existing, nil
	}
	if scopedTenantID == DefaultTenantID && namespaceName == DefaultNamespace {
		return nil, fmt.Errorf("default namespace cannot be renamed")
	}

	inUse, err := s.namespaceHasManagedResources(ctx, scopedTenantID, namespaceName)
	if err != nil {
		return nil, err
	}
	if inUse {
		return nil, fmt.Errorf("namespace %s/%s still has managed resources", scopedTenantID, namespaceName)
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE namespaces
		SET id = $3, name = $4, updated_at = NOW()
		WHERE tenant_id = $1 AND name = $2
	`, scopedTenantID, namespaceName, buildNamespaceID(scopedTenantID, newName), newName)
	if err != nil {
		return nil, fmt.Errorf("update namespace: %w", err)
	}

	return s.GetNamespace(ctx, scopedTenantID, newName)
}

func (s *PostgresStore) DeleteNamespace(ctx context.Context, tenantID, name string) error {
	scopedTenantID, err := validateScopeIdentifier("tenant id", tenantID)
	if err != nil {
		return err
	}
	namespaceName, err := validateScopeIdentifier("namespace", name)
	if err != nil {
		return err
	}
	if scopedTenantID == DefaultTenantID && namespaceName == DefaultNamespace {
		return fmt.Errorf("default namespace cannot be deleted")
	}

	inUse, err := s.namespaceHasManagedResources(ctx, scopedTenantID, namespaceName)
	if err != nil {
		return err
	}
	if inUse {
		return fmt.Errorf("namespace %s/%s still has managed resources", scopedTenantID, namespaceName)
	}

	var count int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM namespaces WHERE tenant_id = $1`, scopedTenantID).Scan(&count); err != nil {
		return fmt.Errorf("count namespaces for tenant %s: %w", scopedTenantID, err)
	}
	if count <= 1 {
		return fmt.Errorf("cannot delete the last namespace of tenant %s", scopedTenantID)
	}

	ct, err := s.pool.Exec(ctx, `DELETE FROM namespaces WHERE tenant_id = $1 AND name = $2`, scopedTenantID, namespaceName)
	if err != nil {
		return fmt.Errorf("delete namespace: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("namespace not found: %s/%s", scopedTenantID, namespaceName)
	}
	return nil
}

func (s *PostgresStore) tenantHasManagedResources(ctx context.Context, tenantID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT
			EXISTS(SELECT 1 FROM functions WHERE tenant_id = $1)
			OR EXISTS(SELECT 1 FROM async_invocations WHERE tenant_id = $1)
			OR EXISTS(SELECT 1 FROM event_topics WHERE tenant_id = $1)
			OR EXISTS(SELECT 1 FROM dag_workflows WHERE tenant_id = $1)
			OR EXISTS(SELECT 1 FROM api_keys WHERE tenant_id = $1)
			OR EXISTS(SELECT 1 FROM secrets WHERE tenant_id = $1)
			OR EXISTS(SELECT 1 FROM schedules WHERE tenant_id = $1)
	`, tenantID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check tenant resources: %w", err)
	}
	return exists, nil
}

func (s *PostgresStore) namespaceHasManagedResources(ctx context.Context, tenantID, namespace string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT
			EXISTS(SELECT 1 FROM functions WHERE tenant_id = $1 AND namespace = $2)
			OR EXISTS(SELECT 1 FROM async_invocations WHERE tenant_id = $1 AND namespace = $2)
			OR EXISTS(SELECT 1 FROM event_topics WHERE tenant_id = $1 AND namespace = $2)
			OR EXISTS(SELECT 1 FROM dag_workflows WHERE tenant_id = $1 AND namespace = $2)
			OR EXISTS(SELECT 1 FROM api_keys WHERE tenant_id = $1 AND namespace = $2)
			OR EXISTS(SELECT 1 FROM secrets WHERE tenant_id = $1 AND namespace = $2)
			OR EXISTS(SELECT 1 FROM schedules WHERE tenant_id = $1 AND namespace = $2)
	`, tenantID, namespace).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check namespace resources: %w", err)
	}
	return exists, nil
}

func validateScopeIdentifier(field, value string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if !IsValidTenantScopePart(v) {
		return "", fmt.Errorf("%s must match ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$", field)
	}
	return v, nil
}

func buildNamespaceID(tenantID, namespace string) string {
	return fmt.Sprintf("%s/%s", tenantID, namespace)
}
