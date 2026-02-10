package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oriys/nova/internal/domain"
)

// SaveLayer creates or updates a layer record
func (s *PostgresStore) SaveLayer(ctx context.Context, layer *domain.Layer) error {
	if layer.ID == "" || layer.Name == "" {
		return fmt.Errorf("layer id and name are required")
	}

	now := time.Now()
	if layer.CreatedAt.IsZero() {
		layer.CreatedAt = now
	}
	if layer.UpdatedAt.IsZero() {
		layer.UpdatedAt = now
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO layers (id, name, runtime, version, size_mb, files, image_path, content_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			runtime = EXCLUDED.runtime,
			version = EXCLUDED.version,
			size_mb = EXCLUDED.size_mb,
			files = EXCLUDED.files,
			image_path = EXCLUDED.image_path,
			content_hash = EXCLUDED.content_hash,
			updated_at = EXCLUDED.updated_at
	`, layer.ID, layer.Name, string(layer.Runtime), layer.Version, layer.SizeMB, layer.Files, layer.ImagePath, layer.ContentHash, layer.CreatedAt, layer.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save layer: %w", err)
	}
	return nil
}

// GetLayer retrieves a layer by ID
func (s *PostgresStore) GetLayer(ctx context.Context, id string) (*domain.Layer, error) {
	var layer domain.Layer
	var runtime string
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, runtime, version, size_mb, files, image_path, content_hash, created_at, updated_at
		FROM layers WHERE id = $1
	`, id).Scan(&layer.ID, &layer.Name, &runtime, &layer.Version, &layer.SizeMB, &layer.Files, &layer.ImagePath, &layer.ContentHash, &layer.CreatedAt, &layer.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("layer not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get layer: %w", err)
	}
	layer.Runtime = domain.Runtime(runtime)
	return &layer, nil
}

// GetLayerByName retrieves a layer by name
func (s *PostgresStore) GetLayerByName(ctx context.Context, name string) (*domain.Layer, error) {
	var layer domain.Layer
	var runtime string
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, runtime, version, size_mb, files, image_path, content_hash, created_at, updated_at
		FROM layers WHERE name = $1
	`, name).Scan(&layer.ID, &layer.Name, &runtime, &layer.Version, &layer.SizeMB, &layer.Files, &layer.ImagePath, &layer.ContentHash, &layer.CreatedAt, &layer.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("layer not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("get layer by name: %w", err)
	}
	layer.Runtime = domain.Runtime(runtime)
	return &layer, nil
}

// GetLayerByContentHash returns a layer with matching content hash, or nil if not found
func (s *PostgresStore) GetLayerByContentHash(ctx context.Context, hash string) (*domain.Layer, error) {
	var layer domain.Layer
	var runtime string
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, runtime, version, size_mb, files, image_path, content_hash, created_at, updated_at
		FROM layers WHERE content_hash = $1
	`, hash).Scan(&layer.ID, &layer.Name, &runtime, &layer.Version, &layer.SizeMB, &layer.Files, &layer.ImagePath, &layer.ContentHash, &layer.CreatedAt, &layer.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get layer by content hash: %w", err)
	}
	layer.Runtime = domain.Runtime(runtime)
	return &layer, nil
}

// ListLayers returns all layers ordered by name
func (s *PostgresStore) ListLayers(ctx context.Context, limit, offset int) ([]*domain.Layer, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, runtime, version, size_mb, files, image_path, content_hash, created_at, updated_at
		FROM layers ORDER BY name
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list layers: %w", err)
	}
	defer rows.Close()

	var layers []*domain.Layer
	for rows.Next() {
		var layer domain.Layer
		var runtime string
		if err := rows.Scan(&layer.ID, &layer.Name, &runtime, &layer.Version, &layer.SizeMB, &layer.Files, &layer.ImagePath, &layer.ContentHash, &layer.CreatedAt, &layer.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list layers scan: %w", err)
		}
		layer.Runtime = domain.Runtime(runtime)
		layers = append(layers, &layer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list layers rows: %w", err)
	}
	return layers, nil
}

// DeleteLayer removes a layer by ID
func (s *PostgresStore) DeleteLayer(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM layers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete layer: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("layer not found: %s", id)
	}
	return nil
}

// SetFunctionLayers replaces the layer associations for a function
func (s *PostgresStore) SetFunctionLayers(ctx context.Context, funcID string, layerIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Delete existing associations
	if _, err := tx.Exec(ctx, `DELETE FROM function_layers WHERE function_id = $1`, funcID); err != nil {
		return fmt.Errorf("delete existing function layers: %w", err)
	}

	// Insert new associations with position
	for i, layerID := range layerIDs {
		_, err := tx.Exec(ctx, `
			INSERT INTO function_layers (function_id, layer_id, position)
			VALUES ($1, $2, $3)
		`, funcID, layerID, i)
		if err != nil {
			return fmt.Errorf("insert function layer %s: %w", layerID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// GetFunctionLayers returns layers associated with a function, ordered by position
func (s *PostgresStore) GetFunctionLayers(ctx context.Context, funcID string) ([]*domain.Layer, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT l.id, l.name, l.runtime, l.version, l.size_mb, l.files, l.image_path, l.content_hash, l.created_at, l.updated_at
		FROM layers l
		JOIN function_layers fl ON fl.layer_id = l.id
		WHERE fl.function_id = $1
		ORDER BY fl.position
	`, funcID)
	if err != nil {
		return nil, fmt.Errorf("get function layers: %w", err)
	}
	defer rows.Close()

	var layers []*domain.Layer
	for rows.Next() {
		var layer domain.Layer
		var runtime string
		if err := rows.Scan(&layer.ID, &layer.Name, &runtime, &layer.Version, &layer.SizeMB, &layer.Files, &layer.ImagePath, &layer.ContentHash, &layer.CreatedAt, &layer.UpdatedAt); err != nil {
			return nil, fmt.Errorf("get function layers scan: %w", err)
		}
		layer.Runtime = domain.Runtime(runtime)
		layers = append(layers, &layer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get function layers rows: %w", err)
	}
	return layers, nil
}

// ListFunctionsByLayer returns function IDs that reference a given layer
func (s *PostgresStore) ListFunctionsByLayer(ctx context.Context, layerID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT function_id FROM function_layers WHERE layer_id = $1
	`, layerID)
	if err != nil {
		return nil, fmt.Errorf("list functions by layer: %w", err)
	}
	defer rows.Close()

	var funcIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list functions by layer scan: %w", err)
		}
		funcIDs = append(funcIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list functions by layer rows: %w", err)
	}
	return funcIDs, nil
}
