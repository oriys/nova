package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// RuntimeRecord represents a runtime configuration in the database
type RuntimeRecord struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Version        string            `json:"version"`
	Status         string            `json:"status"`
	ImageName      string            `json:"image_name,omitempty"`
	Entrypoint     []string          `json:"entrypoint,omitempty"`
	FileExtension  string            `json:"file_extension,omitempty"`
	EnvVars        map[string]string `json:"env_vars,omitempty"`
	FunctionsCount int               `json:"functions_count"`
	CreatedAt      time.Time         `json:"created_at"`
}

func (s *PostgresStore) SaveRuntime(ctx context.Context, rt *RuntimeRecord) error {
	if rt.ID == "" {
		return fmt.Errorf("runtime id is required")
	}
	if rt.Status == "" {
		rt.Status = "available"
	}
	if rt.CreatedAt.IsZero() {
		rt.CreatedAt = time.Now()
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO runtimes (id, name, version, status, image_name, entrypoint, file_extension, env_vars, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			version = EXCLUDED.version,
			status = EXCLUDED.status,
			image_name = EXCLUDED.image_name,
			entrypoint = EXCLUDED.entrypoint,
			file_extension = EXCLUDED.file_extension,
			env_vars = EXCLUDED.env_vars
	`, rt.ID, rt.Name, rt.Version, rt.Status, rt.ImageName, rt.Entrypoint, rt.FileExtension, toJSON(rt.EnvVars), rt.CreatedAt)
	if err != nil {
		return fmt.Errorf("save runtime: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetRuntime(ctx context.Context, id string) (*RuntimeRecord, error) {
	var rt RuntimeRecord
	var rtEnvVars []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, version, status, COALESCE(image_name, ''), entrypoint, COALESCE(file_extension, ''), COALESCE(env_vars, '{}'::jsonb), created_at
		FROM runtimes
		WHERE id = $1
	`, id).Scan(&rt.ID, &rt.Name, &rt.Version, &rt.Status, &rt.ImageName, &rt.Entrypoint, &rt.FileExtension, &rtEnvVars, &rt.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("runtime not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get runtime: %w", err)
	}
	if err := json.Unmarshal(rtEnvVars, &rt.EnvVars); err != nil {
		return nil, fmt.Errorf("decode runtime env vars: %w", err)
	}
	return &rt, nil
}

func (s *PostgresStore) ListRuntimes(ctx context.Context) ([]*RuntimeRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.name, r.version, r.status, COALESCE(r.image_name, ''), r.entrypoint, COALESCE(r.file_extension, ''), COALESCE(r.env_vars, '{}'::jsonb), r.created_at, COUNT(f.id) as functions_count
		FROM runtimes r
		LEFT JOIN functions f ON f.data->>'runtime' = r.id
		GROUP BY r.id, r.name, r.version, r.status, r.image_name, r.entrypoint, r.file_extension, r.env_vars, r.created_at
		ORDER BY r.name, r.version DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list runtimes: %w", err)
	}
	defer rows.Close()

	var runtimes []*RuntimeRecord
	for rows.Next() {
		var rt RuntimeRecord
		var envVarsRaw []byte
		if err := rows.Scan(&rt.ID, &rt.Name, &rt.Version, &rt.Status, &rt.ImageName, &rt.Entrypoint, &rt.FileExtension, &envVarsRaw, &rt.CreatedAt, &rt.FunctionsCount); err != nil {
			return nil, fmt.Errorf("scan runtime: %w", err)
		}
		if err := json.Unmarshal(envVarsRaw, &rt.EnvVars); err != nil {
			return nil, fmt.Errorf("decode runtime env vars: %w", err)
		}
		runtimes = append(runtimes, &rt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runtimes rows: %w", err)
	}
	return runtimes, nil
}

func toJSON(v map[string]string) []byte {
	if v == nil {
		v = map[string]string{}
	}
	b, _ := json.Marshal(v)
	return b
}

func (s *PostgresStore) DeleteRuntime(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM runtimes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete runtime: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("runtime not found: %s", id)
	}
	return nil
}

func (s *PostgresStore) GetConfig(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, value FROM config ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}
	defer rows.Close()

	config := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan config: %w", err)
		}
		config[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get config rows: %w", err)
	}
	return config, nil
}

func (s *PostgresStore) SetConfig(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO config (key, value, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			updated_at = NOW()
	`, key, value)
	if err != nil {
		return fmt.Errorf("set config: %w", err)
	}
	return nil
}
