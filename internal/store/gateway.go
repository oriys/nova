package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oriys/nova/internal/domain"
)

// SaveGatewayRoute creates or updates a gateway route
func (s *PostgresStore) SaveGatewayRoute(ctx context.Context, route *domain.GatewayRoute) error {
	data, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("marshal gateway route: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO gateway_routes (id, domain, path, function_name, data, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			domain = EXCLUDED.domain,
			path = EXCLUDED.path,
			function_name = EXCLUDED.function_name,
			data = EXCLUDED.data,
			enabled = EXCLUDED.enabled,
			updated_at = NOW()
	`, route.ID, route.Domain, route.Path, route.FunctionName, data, route.Enabled, route.CreatedAt, route.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save gateway route: %w", err)
	}
	return nil
}

// GetGatewayRoute retrieves a gateway route by ID
func (s *PostgresStore) GetGatewayRoute(ctx context.Context, id string) (*domain.GatewayRoute, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, `SELECT data FROM gateway_routes WHERE id = $1`, id).Scan(&data)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("gateway route not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get gateway route: %w", err)
	}
	var route domain.GatewayRoute
	if err := json.Unmarshal(data, &route); err != nil {
		return nil, fmt.Errorf("unmarshal gateway route: %w", err)
	}
	return &route, nil
}

// GetRouteByDomainPath finds a route matching the given domain and path
func (s *PostgresStore) GetRouteByDomainPath(ctx context.Context, routeDomain, path string) (*domain.GatewayRoute, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, `
		SELECT data FROM gateway_routes
		WHERE domain = $1 AND path = $2 AND enabled = TRUE
	`, routeDomain, path).Scan(&data)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get route by domain/path: %w", err)
	}
	var route domain.GatewayRoute
	if err := json.Unmarshal(data, &route); err != nil {
		return nil, fmt.Errorf("unmarshal gateway route: %w", err)
	}
	return &route, nil
}

// ListGatewayRoutes returns all gateway routes
func (s *PostgresStore) ListGatewayRoutes(ctx context.Context, limit, offset int) ([]*domain.GatewayRoute, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `SELECT data FROM gateway_routes ORDER BY domain, path LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list gateway routes: %w", err)
	}
	defer rows.Close()

	var routes []*domain.GatewayRoute
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan gateway route: %w", err)
		}
		var route domain.GatewayRoute
		if err := json.Unmarshal(data, &route); err != nil {
			return nil, fmt.Errorf("unmarshal gateway route: %w", err)
		}
		routes = append(routes, &route)
	}
	return routes, nil
}

// ListRoutesByDomain returns routes for a specific domain
func (s *PostgresStore) ListRoutesByDomain(ctx context.Context, routeDomain string, limit, offset int) ([]*domain.GatewayRoute, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `
		SELECT data FROM gateway_routes WHERE domain = $1 ORDER BY path LIMIT $2 OFFSET $3
	`, routeDomain, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list routes by domain: %w", err)
	}
	defer rows.Close()

	var routes []*domain.GatewayRoute
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan gateway route: %w", err)
		}
		var route domain.GatewayRoute
		if err := json.Unmarshal(data, &route); err != nil {
			return nil, fmt.Errorf("unmarshal gateway route: %w", err)
		}
		routes = append(routes, &route)
	}
	return routes, nil
}

// DeleteGatewayRoute removes a gateway route
func (s *PostgresStore) DeleteGatewayRoute(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM gateway_routes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete gateway route: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("gateway route not found: %s", id)
	}
	return nil
}

// UpdateGatewayRoute partially updates a gateway route
func (s *PostgresStore) UpdateGatewayRoute(ctx context.Context, id string, route *domain.GatewayRoute) error {
	route.UpdatedAt = time.Now()
	data, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("marshal gateway route: %w", err)
	}
	ct, err := s.pool.Exec(ctx, `
		UPDATE gateway_routes SET
			domain = $2, path = $3, function_name = $4,
			data = $5, enabled = $6, updated_at = NOW()
		WHERE id = $1
	`, id, route.Domain, route.Path, route.FunctionName, data, route.Enabled)
	if err != nil {
		return fmt.Errorf("update gateway route: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("gateway route not found: %s", id)
	}
	return nil
}
