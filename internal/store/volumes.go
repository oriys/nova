package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
)

// CreateVolume creates a new persistent volume
func (s *PostgresStore) CreateVolume(ctx context.Context, vol *domain.Volume) error {
	if vol.ID == "" {
		vol.ID = uuid.New().String()
	}
	now := time.Now()
	vol.CreatedAt = now
	vol.UpdatedAt = now

	scope := TenantScopeFromContext(ctx)
	if vol.TenantID == "" {
		vol.TenantID = scope.TenantID
	}
	if vol.Namespace == "" {
		vol.Namespace = scope.Namespace
	}

	query := `
		INSERT INTO volumes (id, tenant_id, namespace, name, size_mb, image_path, shared, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := s.pool.Exec(ctx, query,
		vol.ID, vol.TenantID, vol.Namespace, vol.Name, vol.SizeMB, vol.ImagePath,
		vol.Shared, vol.Description, vol.CreatedAt, vol.UpdatedAt,
	)
	return err
}

// GetVolume retrieves a volume by ID
func (s *PostgresStore) GetVolume(ctx context.Context, id string) (*domain.Volume, error) {
	scope := TenantScopeFromContext(ctx)
	query := `
		SELECT id, tenant_id, namespace, name, size_mb, image_path, shared, description, created_at, updated_at
		FROM volumes
		WHERE id = $1 AND tenant_id = $2 AND namespace = $3
	`
	var vol domain.Volume
	err := s.pool.QueryRow(ctx, query, id, scope.TenantID, scope.Namespace).Scan(
		&vol.ID, &vol.TenantID, &vol.Namespace, &vol.Name, &vol.SizeMB, &vol.ImagePath,
		&vol.Shared, &vol.Description, &vol.CreatedAt, &vol.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &vol, nil
}

// GetVolumeByName retrieves a volume by name
func (s *PostgresStore) GetVolumeByName(ctx context.Context, name string) (*domain.Volume, error) {
	scope := TenantScopeFromContext(ctx)
	query := `
		SELECT id, tenant_id, namespace, name, size_mb, image_path, shared, description, created_at, updated_at
		FROM volumes
		WHERE name = $1 AND tenant_id = $2 AND namespace = $3
	`
	var vol domain.Volume
	err := s.pool.QueryRow(ctx, query, name, scope.TenantID, scope.Namespace).Scan(
		&vol.ID, &vol.TenantID, &vol.Namespace, &vol.Name, &vol.SizeMB, &vol.ImagePath,
		&vol.Shared, &vol.Description, &vol.CreatedAt, &vol.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &vol, nil
}

// ListVolumes lists all volumes for a tenant/namespace
func (s *PostgresStore) ListVolumes(ctx context.Context) ([]*domain.Volume, error) {
	scope := TenantScopeFromContext(ctx)
	query := `
		SELECT id, tenant_id, namespace, name, size_mb, image_path, shared, description, created_at, updated_at
		FROM volumes
		WHERE tenant_id = $1 AND namespace = $2
		ORDER BY name
	`
	rows, err := s.pool.Query(ctx, query, scope.TenantID, scope.Namespace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var volumes []*domain.Volume
	for rows.Next() {
		var vol domain.Volume
		if err := rows.Scan(
			&vol.ID, &vol.TenantID, &vol.Namespace, &vol.Name, &vol.SizeMB, &vol.ImagePath,
			&vol.Shared, &vol.Description, &vol.CreatedAt, &vol.UpdatedAt,
		); err != nil {
			return nil, err
		}
		volumes = append(volumes, &vol)
	}
	return volumes, rows.Err()
}

// UpdateVolume updates a volume's metadata
func (s *PostgresStore) UpdateVolume(ctx context.Context, id string, updates map[string]interface{}) error {
	scope := TenantScopeFromContext(ctx)
	updates["updated_at"] = time.Now()

	query := `
		UPDATE volumes
		SET description = COALESCE($1, description),
		    shared = COALESCE($2, shared),
		    updated_at = $3
		WHERE id = $4 AND tenant_id = $5 AND namespace = $6
	`
	_, err := s.pool.Exec(ctx, query,
		updates["description"], updates["shared"], updates["updated_at"],
		id, scope.TenantID, scope.Namespace,
	)
	return err
}

// DeleteVolume deletes a volume
func (s *PostgresStore) DeleteVolume(ctx context.Context, id string) error {
	scope := TenantScopeFromContext(ctx)
	query := `DELETE FROM volumes WHERE id = $1 AND tenant_id = $2 AND namespace = $3`
	result, err := s.pool.Exec(ctx, query, id, scope.TenantID, scope.Namespace)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("volume not found: %s", id)
	}
	return nil
}

// GetFunctionVolumes retrieves all volumes mounted by a function
func (s *PostgresStore) GetFunctionVolumes(ctx context.Context, functionID string) ([]*domain.Volume, error) {
	scope := TenantScopeFromContext(ctx)
	
	// Get volume IDs from function mounts
	query := `
		SELECT data->>'mounts'
		FROM functions
		WHERE id = $1 AND tenant_id = $2 AND namespace = $3
	`
	var mountsJSON string
	err := s.pool.QueryRow(ctx, query, functionID, scope.TenantID, scope.Namespace).Scan(&mountsJSON)
	if err != nil {
		return nil, err
	}

	// Parse mounts to get volume IDs
	var mounts []domain.VolumeMount
	if mountsJSON != "" && mountsJSON != "null" {
		if err := json.Unmarshal([]byte(mountsJSON), &mounts); err != nil {
			return nil, err
		}
	}

	if len(mounts) == 0 {
		return nil, nil
	}

	// Build list of volume IDs
	volumeIDs := make([]string, len(mounts))
	for i, m := range mounts {
		volumeIDs[i] = m.VolumeID
	}

	// Fetch volumes
	query = `
		SELECT id, tenant_id, namespace, name, size_mb, image_path, shared, description, created_at, updated_at
		FROM volumes
		WHERE id = ANY($1) AND tenant_id = $2 AND namespace = $3
	`
	rows, err := s.pool.Query(ctx, query, volumeIDs, scope.TenantID, scope.Namespace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var volumes []*domain.Volume
	for rows.Next() {
		var vol domain.Volume
		if err := rows.Scan(
			&vol.ID, &vol.TenantID, &vol.Namespace, &vol.Name, &vol.SizeMB, &vol.ImagePath,
			&vol.Shared, &vol.Description, &vol.CreatedAt, &vol.UpdatedAt,
		); err != nil {
			return nil, err
		}
		volumes = append(volumes, &vol)
	}
	return volumes, rows.Err()
}
