package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
)

// ── DbResource CRUD ─────────────────────────────────────────────────────────

func (s *PostgresStore) CreateDbResource(ctx context.Context, res *DbResourceRecord) (*DbResourceRecord, error) {
	if res.ID == "" {
		res.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	res.CreatedAt = now
	res.UpdatedAt = now

	caps, err := json.Marshal(res.Capabilities)
	if err != nil {
		return nil, fmt.Errorf("marshal capabilities: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO db_resources (id, tenant_id, name, type, endpoint, port, database_name, region, tenant_mode, network_policy, capabilities, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		res.ID, res.TenantID, res.Name, string(res.Type), res.Endpoint, res.Port,
		res.DatabaseName, res.Region, string(res.TenantMode), res.NetworkPolicy,
		caps, res.CreatedAt, res.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert db_resource: %w", err)
	}
	return res, nil
}

func (s *PostgresStore) GetDbResource(ctx context.Context, id string) (*DbResourceRecord, error) {
	r := &DbResourceRecord{}
	var capsJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, type, endpoint, port, database_name, region, tenant_mode, network_policy, capabilities, created_at, updated_at
		FROM db_resources WHERE id = $1`, id).Scan(
		&r.ID, &r.TenantID, &r.Name, &r.Type, &r.Endpoint, &r.Port,
		&r.DatabaseName, &r.Region, &r.TenantMode, &r.NetworkPolicy,
		&capsJSON, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get db_resource %s: %w", id, err)
	}
	if len(capsJSON) > 0 {
		r.Capabilities = &domain.DbCapabilities{}
		if err := json.Unmarshal(capsJSON, r.Capabilities); err != nil {
			return nil, fmt.Errorf("unmarshal capabilities: %w", err)
		}
	}
	return r, nil
}

func (s *PostgresStore) GetDbResourceByName(ctx context.Context, name string) (*DbResourceRecord, error) {
	r := &DbResourceRecord{}
	var capsJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, type, endpoint, port, database_name, region, tenant_mode, network_policy, capabilities, created_at, updated_at
		FROM db_resources WHERE name = $1`, name).Scan(
		&r.ID, &r.TenantID, &r.Name, &r.Type, &r.Endpoint, &r.Port,
		&r.DatabaseName, &r.Region, &r.TenantMode, &r.NetworkPolicy,
		&capsJSON, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get db_resource by name %s: %w", name, err)
	}
	if len(capsJSON) > 0 {
		r.Capabilities = &domain.DbCapabilities{}
		if err := json.Unmarshal(capsJSON, r.Capabilities); err != nil {
			return nil, fmt.Errorf("unmarshal capabilities: %w", err)
		}
	}
	return r, nil
}

func (s *PostgresStore) ListDbResources(ctx context.Context, limit, offset int) ([]*DbResourceRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, type, endpoint, port, database_name, region, tenant_mode, network_policy, capabilities, created_at, updated_at
		FROM db_resources ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list db_resources: %w", err)
	}
	defer rows.Close()

	var results []*DbResourceRecord
	for rows.Next() {
		r := &DbResourceRecord{}
		var capsJSON []byte
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Type, &r.Endpoint, &r.Port,
			&r.DatabaseName, &r.Region, &r.TenantMode, &r.NetworkPolicy,
			&capsJSON, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan db_resource: %w", err)
		}
		if len(capsJSON) > 0 {
			r.Capabilities = &domain.DbCapabilities{}
			if err := json.Unmarshal(capsJSON, r.Capabilities); err != nil {
				return nil, fmt.Errorf("unmarshal capabilities: %w", err)
			}
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *PostgresStore) UpdateDbResource(ctx context.Context, id string, update *DbResourceUpdate) (*DbResourceRecord, error) {
	existing, err := s.GetDbResource(ctx, id)
	if err != nil {
		return nil, err
	}
	if update.Name != nil {
		existing.Name = *update.Name
	}
	if update.Endpoint != nil {
		existing.Endpoint = *update.Endpoint
	}
	if update.Port != nil {
		existing.Port = *update.Port
	}
	if update.DatabaseName != nil {
		existing.DatabaseName = *update.DatabaseName
	}
	if update.Region != nil {
		existing.Region = *update.Region
	}
	if update.TenantMode != nil {
		existing.TenantMode = *update.TenantMode
	}
	if update.NetworkPolicy != nil {
		existing.NetworkPolicy = *update.NetworkPolicy
	}
	if update.Capabilities != nil {
		existing.Capabilities = update.Capabilities
	}
	existing.UpdatedAt = time.Now().UTC()

	caps, err := json.Marshal(existing.Capabilities)
	if err != nil {
		return nil, fmt.Errorf("marshal capabilities: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE db_resources SET name=$1, endpoint=$2, port=$3, database_name=$4, region=$5,
		tenant_mode=$6, network_policy=$7, capabilities=$8, updated_at=$9 WHERE id=$10`,
		existing.Name, existing.Endpoint, existing.Port, existing.DatabaseName, existing.Region,
		string(existing.TenantMode), existing.NetworkPolicy, caps, existing.UpdatedAt, id)
	if err != nil {
		return nil, fmt.Errorf("update db_resource: %w", err)
	}
	return existing, nil
}

func (s *PostgresStore) DeleteDbResource(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM db_resources WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete db_resource: %w", err)
	}
	return nil
}

// ── DbBinding CRUD ──────────────────────────────────────────────────────────

func (s *PostgresStore) CreateDbBinding(ctx context.Context, b *DbBindingRecord) (*DbBindingRecord, error) {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	b.CreatedAt = now
	b.UpdatedAt = now

	perms, err := json.Marshal(b.Permissions)
	if err != nil {
		return nil, fmt.Errorf("marshal permissions: %w", err)
	}
	quota, err := json.Marshal(b.Quota)
	if err != nil {
		return nil, fmt.Errorf("marshal quota: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO db_bindings (id, tenant_id, function_id, version_selector, db_resource_id, permissions, quota, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		b.ID, b.TenantID, b.FunctionID, b.VersionSelector, b.DbResourceID,
		perms, quota, b.CreatedAt, b.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert db_binding: %w", err)
	}
	return b, nil
}

func (s *PostgresStore) GetDbBinding(ctx context.Context, id string) (*DbBindingRecord, error) {
	b := &DbBindingRecord{}
	var permsJSON, quotaJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, function_id, version_selector, db_resource_id, permissions, quota, created_at, updated_at
		FROM db_bindings WHERE id = $1`, id).Scan(
		&b.ID, &b.TenantID, &b.FunctionID, &b.VersionSelector, &b.DbResourceID,
		&permsJSON, &quotaJSON, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get db_binding %s: %w", id, err)
	}
	if err := json.Unmarshal(permsJSON, &b.Permissions); err != nil {
		return nil, fmt.Errorf("unmarshal permissions: %w", err)
	}
	if len(quotaJSON) > 0 && string(quotaJSON) != "null" {
		b.Quota = &domain.DbBindingQuota{}
		if err := json.Unmarshal(quotaJSON, b.Quota); err != nil {
			return nil, fmt.Errorf("unmarshal quota: %w", err)
		}
	}
	return b, nil
}

func (s *PostgresStore) ListDbBindings(ctx context.Context, dbResourceID string, limit, offset int) ([]*DbBindingRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, function_id, version_selector, db_resource_id, permissions, quota, created_at, updated_at
		FROM db_bindings WHERE db_resource_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		dbResourceID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list db_bindings: %w", err)
	}
	defer rows.Close()
	return scanDbBindings(rows)
}

func (s *PostgresStore) ListDbBindingsByFunction(ctx context.Context, functionID string, limit, offset int) ([]*DbBindingRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, function_id, version_selector, db_resource_id, permissions, quota, created_at, updated_at
		FROM db_bindings WHERE function_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		functionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list db_bindings by function: %w", err)
	}
	defer rows.Close()
	return scanDbBindings(rows)
}

func scanDbBindings(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]*DbBindingRecord, error) {
	var results []*DbBindingRecord
	for rows.Next() {
		b := &DbBindingRecord{}
		var permsJSON, quotaJSON []byte
		if err := rows.Scan(&b.ID, &b.TenantID, &b.FunctionID, &b.VersionSelector, &b.DbResourceID,
			&permsJSON, &quotaJSON, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan db_binding: %w", err)
		}
		if err := json.Unmarshal(permsJSON, &b.Permissions); err != nil {
			return nil, fmt.Errorf("unmarshal permissions: %w", err)
		}
		if len(quotaJSON) > 0 && string(quotaJSON) != "null" {
			b.Quota = &domain.DbBindingQuota{}
			if err := json.Unmarshal(quotaJSON, b.Quota); err != nil {
				return nil, fmt.Errorf("unmarshal quota: %w", err)
			}
		}
		results = append(results, b)
	}
	return results, rows.Err()
}

func (s *PostgresStore) UpdateDbBinding(ctx context.Context, id string, update *DbBindingUpdate) (*DbBindingRecord, error) {
	existing, err := s.GetDbBinding(ctx, id)
	if err != nil {
		return nil, err
	}
	if update.VersionSelector != nil {
		existing.VersionSelector = *update.VersionSelector
	}
	if update.Permissions != nil {
		existing.Permissions = update.Permissions
	}
	if update.Quota != nil {
		existing.Quota = update.Quota
	}
	existing.UpdatedAt = time.Now().UTC()

	perms, err := json.Marshal(existing.Permissions)
	if err != nil {
		return nil, fmt.Errorf("marshal permissions: %w", err)
	}
	quota, err := json.Marshal(existing.Quota)
	if err != nil {
		return nil, fmt.Errorf("marshal quota: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE db_bindings SET version_selector=$1, permissions=$2, quota=$3, updated_at=$4 WHERE id=$5`,
		existing.VersionSelector, perms, quota, existing.UpdatedAt, id)
	if err != nil {
		return nil, fmt.Errorf("update db_binding: %w", err)
	}
	return existing, nil
}

func (s *PostgresStore) DeleteDbBinding(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM db_bindings WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete db_binding: %w", err)
	}
	return nil
}

// ── CredentialPolicy CRUD ───────────────────────────────────────────────────

func (s *PostgresStore) CreateCredentialPolicy(ctx context.Context, p *CredentialPolicyRecord) (*CredentialPolicyRecord, error) {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := s.pool.Exec(ctx, `
		INSERT INTO credential_policies (id, db_resource_id, auth_mode, rotation_days, static_username, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		p.ID, p.DbResourceID, string(p.AuthMode), p.RotationDays, p.StaticUsername,
		p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert credential_policy: %w", err)
	}
	return p, nil
}

func (s *PostgresStore) GetCredentialPolicy(ctx context.Context, dbResourceID string) (*CredentialPolicyRecord, error) {
	p := &CredentialPolicyRecord{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, db_resource_id, auth_mode, rotation_days, static_username, created_at, updated_at
		FROM credential_policies WHERE db_resource_id = $1`, dbResourceID).Scan(
		&p.ID, &p.DbResourceID, &p.AuthMode, &p.RotationDays, &p.StaticUsername,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get credential_policy for %s: %w", dbResourceID, err)
	}
	return p, nil
}

func (s *PostgresStore) UpdateCredentialPolicy(ctx context.Context, dbResourceID string, update *CredentialPolicyUpdate) (*CredentialPolicyRecord, error) {
	existing, err := s.GetCredentialPolicy(ctx, dbResourceID)
	if err != nil {
		return nil, err
	}
	if update.AuthMode != nil {
		existing.AuthMode = *update.AuthMode
	}
	if update.RotationDays != nil {
		existing.RotationDays = *update.RotationDays
	}
	if update.StaticUsername != nil {
		existing.StaticUsername = *update.StaticUsername
	}
	existing.UpdatedAt = time.Now().UTC()

	_, err = s.pool.Exec(ctx, `
		UPDATE credential_policies SET auth_mode=$1, rotation_days=$2, static_username=$3, updated_at=$4 WHERE db_resource_id=$5`,
		string(existing.AuthMode), existing.RotationDays, existing.StaticUsername, existing.UpdatedAt, dbResourceID)
	if err != nil {
		return nil, fmt.Errorf("update credential_policy: %w", err)
	}
	return existing, nil
}

func (s *PostgresStore) DeleteCredentialPolicy(ctx context.Context, dbResourceID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM credential_policies WHERE db_resource_id = $1`, dbResourceID)
	if err != nil {
		return fmt.Errorf("delete credential_policy: %w", err)
	}
	return nil
}

// ── DbRequestLog (audit) ───────────────────────────────────────────────────

func (s *PostgresStore) SaveDbRequestLog(ctx context.Context, log *domain.DbRequestLog) error {
	if log.ID == "" {
		log.ID = uuid.New().String()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO db_request_logs (id, request_id, function_id, function_name, version, tenant_id,
			db_resource_id, statement_hash, tables, rows_returned, rows_affected, latency_ms, error_code, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		log.ID, log.RequestID, log.FunctionID, log.FunctionName, log.Version, log.TenantID,
		log.DbResourceID, log.StatementHash, log.Tables, log.RowsReturned, log.RowsAffected,
		log.LatencyMs, log.ErrorCode, log.CreatedAt)
	if err != nil {
		return fmt.Errorf("save db_request_log: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListDbRequestLogs(ctx context.Context, dbResourceID string, limit, offset int) ([]*domain.DbRequestLog, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, request_id, function_id, function_name, version, tenant_id, db_resource_id,
			statement_hash, tables, rows_returned, rows_affected, latency_ms, error_code, created_at
		FROM db_request_logs WHERE db_resource_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		dbResourceID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list db_request_logs: %w", err)
	}
	defer rows.Close()

	var results []*domain.DbRequestLog
	for rows.Next() {
		l := &domain.DbRequestLog{}
		if err := rows.Scan(&l.ID, &l.RequestID, &l.FunctionID, &l.FunctionName, &l.Version, &l.TenantID,
			&l.DbResourceID, &l.StatementHash, &l.Tables, &l.RowsReturned, &l.RowsAffected,
			&l.LatencyMs, &l.ErrorCode, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan db_request_log: %w", err)
		}
		results = append(results, l)
	}
	return results, rows.Err()
}
