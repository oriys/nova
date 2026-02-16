package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ClusterNodeRecord represents a row in the cluster_nodes table.
type ClusterNodeRecord struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Address       string            `json:"address"`
	State         string            `json:"state"`
	CPUCores      int               `json:"cpu_cores"`
	MemoryMB      int               `json:"memory_mb"`
	MaxVMs        int               `json:"max_vms"`
	ActiveVMs     int               `json:"active_vms"`
	QueueDepth    int               `json:"queue_depth"`
	Version       string            `json:"version"`
	Labels        map[string]string `json:"labels"`
	LastHeartbeat time.Time         `json:"last_heartbeat"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// UpsertClusterNode inserts or updates a cluster node record.
func (s *PostgresStore) UpsertClusterNode(ctx context.Context, node *ClusterNodeRecord) error {
	now := time.Now()
	if node.Labels == nil {
		node.Labels = map[string]string{}
	}
	labelsJSON, err := json.Marshal(node.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}

	query := `
		INSERT INTO cluster_nodes (id, name, address, state, cpu_cores, memory_mb, max_vms, active_vms, queue_depth, version, labels, last_heartbeat, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO UPDATE SET
			name           = EXCLUDED.name,
			address        = EXCLUDED.address,
			state          = EXCLUDED.state,
			cpu_cores      = EXCLUDED.cpu_cores,
			memory_mb      = EXCLUDED.memory_mb,
			max_vms        = EXCLUDED.max_vms,
			active_vms     = EXCLUDED.active_vms,
			queue_depth    = EXCLUDED.queue_depth,
			version        = EXCLUDED.version,
			labels         = EXCLUDED.labels,
			last_heartbeat = EXCLUDED.last_heartbeat,
			updated_at     = EXCLUDED.updated_at
	`
	_, err = s.pool.Exec(ctx, query,
		node.ID, node.Name, node.Address, node.State,
		node.CPUCores, node.MemoryMB, node.MaxVMs, node.ActiveVMs,
		node.QueueDepth, node.Version, labelsJSON,
		now, now, now,
	)
	return err
}

// GetClusterNode retrieves a cluster node by ID.
func (s *PostgresStore) GetClusterNode(ctx context.Context, id string) (*ClusterNodeRecord, error) {
	query := `
		SELECT id, name, address, state, cpu_cores, memory_mb, max_vms, active_vms, queue_depth, version, labels, last_heartbeat, created_at, updated_at
		FROM cluster_nodes
		WHERE id = $1
	`
	return s.scanClusterNode(s.pool.QueryRow(ctx, query, id))
}

// ListClusterNodes lists cluster nodes with pagination.
func (s *PostgresStore) ListClusterNodes(ctx context.Context, limit, offset int) ([]*ClusterNodeRecord, error) {
	query := `
		SELECT id, name, address, state, cpu_cores, memory_mb, max_vms, active_vms, queue_depth, version, labels, last_heartbeat, created_at, updated_at
		FROM cluster_nodes
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := s.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*ClusterNodeRecord
	for rows.Next() {
		node, err := s.scanClusterNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

// UpdateClusterNodeHeartbeat updates the heartbeat fields for a cluster node.
func (s *PostgresStore) UpdateClusterNodeHeartbeat(ctx context.Context, id string, activeVMs, queueDepth int) error {
	now := time.Now()
	query := `
		UPDATE cluster_nodes
		SET active_vms = $1, queue_depth = $2, last_heartbeat = $3, updated_at = $4
		WHERE id = $5
	`
	result, err := s.pool.Exec(ctx, query, activeVMs, queueDepth, now, now, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("cluster node not found: %s", id)
	}
	return nil
}

// DeleteClusterNode deletes a cluster node by ID.
func (s *PostgresStore) DeleteClusterNode(ctx context.Context, id string) error {
	query := `DELETE FROM cluster_nodes WHERE id = $1`
	result, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("cluster node not found: %s", id)
	}
	return nil
}

// ListActiveClusterNodes lists all cluster nodes with state 'active'.
func (s *PostgresStore) ListActiveClusterNodes(ctx context.Context) ([]*ClusterNodeRecord, error) {
	query := `
		SELECT id, name, address, state, cpu_cores, memory_mb, max_vms, active_vms, queue_depth, version, labels, last_heartbeat, created_at, updated_at
		FROM cluster_nodes
		WHERE state = 'active'
		ORDER BY last_heartbeat DESC
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*ClusterNodeRecord
	for rows.Next() {
		node, err := s.scanClusterNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

// clusterNodeScanner is a helper interface for scanning cluster node rows.
type clusterNodeScanner interface {
	Scan(dest ...interface{}) error
}

func (s *PostgresStore) scanClusterNode(row clusterNodeScanner) (*ClusterNodeRecord, error) {
	var node ClusterNodeRecord
	var labelsJSON []byte
	err := row.Scan(
		&node.ID, &node.Name, &node.Address, &node.State,
		&node.CPUCores, &node.MemoryMB, &node.MaxVMs, &node.ActiveVMs,
		&node.QueueDepth, &node.Version, &labelsJSON,
		&node.LastHeartbeat, &node.CreatedAt, &node.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(labelsJSON) > 0 {
		if err := json.Unmarshal(labelsJSON, &node.Labels); err != nil {
			return nil, fmt.Errorf("unmarshal labels: %w", err)
		}
	}
	return &node, nil
}
