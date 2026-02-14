// Package dbaccess implements the DbAccess Gateway, the unified data-plane
// entry point for function-to-database communication.
//
// It provides:
//   - Connection pooling & reuse (avoids connection storms)
//   - Per-binding quota enforcement (QPS, sessions, tx concurrency)
//   - Audit logging of every database request
//   - gRPC/HTTP-style Query / Execute / Transaction API
package dbaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

// Gateway is the DbAccess Gateway service. It enforces quotas, pools
// connections, and logs every access for audit.
type Gateway struct {
	store  *store.Store
	logger *slog.Logger

	mu    sync.RWMutex
	pools map[string]*ConnPool // keyed by db_resource_id
}

// GatewayConfig holds configuration for the DbAccess Gateway.
type GatewayConfig struct {
	DefaultMaxConns int // default max connections per pool (default: 10)
	DefaultMaxQPS   int // default max QPS per binding (default: 100)
}

// NewGateway creates a new DbAccess Gateway backed by the given store.
func NewGateway(s *store.Store, logger *slog.Logger) *Gateway {
	return &Gateway{
		store:  s,
		logger: logger,
		pools:  make(map[string]*ConnPool),
	}
}

// ConnPool tracks connection usage for a single database resource.
type ConnPool struct {
	mu             sync.Mutex
	resourceID     string
	activeSessions int
	activeTx       int
	maxSessions    int
	maxTxConc      int
}

// QueryRequest represents a read query request from a function.
type QueryRequest struct {
	RequestID    string            `json:"request_id"`
	FunctionID   string            `json:"function_id"`
	FunctionName string            `json:"function_name,omitempty"`
	Version      int               `json:"version,omitempty"`
	TenantID     string            `json:"tenant_id,omitempty"`
	DbResourceID string            `json:"db_resource_id"`
	SQL          string            `json:"sql"`
	Params       []interface{}     `json:"params,omitempty"`
	TimeoutMs    int64             `json:"timeout_ms,omitempty"`
}

// ExecuteRequest represents a write/execute request from a function.
type ExecuteRequest struct {
	RequestID    string            `json:"request_id"`
	FunctionID   string            `json:"function_id"`
	FunctionName string            `json:"function_name,omitempty"`
	Version      int               `json:"version,omitempty"`
	TenantID     string            `json:"tenant_id,omitempty"`
	DbResourceID string            `json:"db_resource_id"`
	SQL          string            `json:"sql"`
	Params       []interface{}     `json:"params,omitempty"`
	TimeoutMs    int64             `json:"timeout_ms,omitempty"`
}

// DbResponse represents the response from a database operation.
type DbResponse struct {
	RequestID    string `json:"request_id"`
	RowsAffected int64  `json:"rows_affected,omitempty"`
	RowsReturned int64  `json:"rows_returned,omitempty"`
	LatencyMs    int64  `json:"latency_ms"`
	Error        string `json:"error,omitempty"`
}

// QuotaError is returned when a quota limit is exceeded.
type QuotaError struct {
	Dimension string
	Limit     int
	Current   int
}

func (e *QuotaError) Error() string {
	return fmt.Sprintf("DB_BUSY: quota exceeded for %s (limit=%d, current=%d)", e.Dimension, e.Limit, e.Current)
}

// CheckBinding validates that the function is allowed to access the given
// database resource and that the operation is permitted.
func (g *Gateway) CheckBinding(ctx context.Context, functionID, dbResourceID string, requiredPerm domain.DbPermission) (*store.DbBindingRecord, error) {
	bindings, err := g.store.ListDbBindingsByFunction(ctx, functionID, 100, 0)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	for _, b := range bindings {
		if b.DbResourceID != dbResourceID {
			continue
		}
		for _, p := range b.Permissions {
			if p == requiredPerm || p == domain.DbPermAdmin {
				return b, nil
			}
		}
		return nil, fmt.Errorf("function %s does not have %s permission on db %s", functionID, requiredPerm, dbResourceID)
	}
	return nil, fmt.Errorf("no binding found for function %s on db %s", functionID, dbResourceID)
}

// AcquireSession checks quota and acquires a session slot for the given binding.
func (g *Gateway) AcquireSession(binding *store.DbBindingRecord) error {
	pool := g.getOrCreatePool(binding.DbResourceID, binding.Quota)

	pool.mu.Lock()
	defer pool.mu.Unlock()

	if pool.maxSessions > 0 && pool.activeSessions >= pool.maxSessions {
		return &QuotaError{Dimension: "max_sessions", Limit: pool.maxSessions, Current: pool.activeSessions}
	}
	pool.activeSessions++
	return nil
}

// ReleaseSession releases a session slot.
func (g *Gateway) ReleaseSession(dbResourceID string) {
	g.mu.RLock()
	pool, ok := g.pools[dbResourceID]
	g.mu.RUnlock()
	if !ok {
		return
	}
	pool.mu.Lock()
	if pool.activeSessions > 0 {
		pool.activeSessions--
	}
	pool.mu.Unlock()
}

// AcquireTx checks quota and acquires a transaction slot.
func (g *Gateway) AcquireTx(binding *store.DbBindingRecord) error {
	pool := g.getOrCreatePool(binding.DbResourceID, binding.Quota)

	pool.mu.Lock()
	defer pool.mu.Unlock()

	if pool.maxTxConc > 0 && pool.activeTx >= pool.maxTxConc {
		return &QuotaError{Dimension: "max_tx_concurrency", Limit: pool.maxTxConc, Current: pool.activeTx}
	}
	pool.activeTx++
	return nil
}

// ReleaseTx releases a transaction slot.
func (g *Gateway) ReleaseTx(dbResourceID string) {
	g.mu.RLock()
	pool, ok := g.pools[dbResourceID]
	g.mu.RUnlock()
	if !ok {
		return
	}
	pool.mu.Lock()
	if pool.activeTx > 0 {
		pool.activeTx--
	}
	pool.mu.Unlock()
}

// RecordAccess writes an audit log entry for a database access.
func (g *Gateway) RecordAccess(ctx context.Context, req *QueryRequest, resp *DbResponse) {
	log := &domain.DbRequestLog{
		ID:            uuid.New().String(),
		RequestID:     req.RequestID,
		FunctionID:    req.FunctionID,
		FunctionName:  req.FunctionName,
		Version:       req.Version,
		TenantID:      req.TenantID,
		DbResourceID:  req.DbResourceID,
		StatementHash: hashStatement(req.SQL),
		RowsReturned:  resp.RowsReturned,
		RowsAffected:  resp.RowsAffected,
		LatencyMs:     resp.LatencyMs,
		ErrorCode:     resp.Error,
		CreatedAt:     time.Now().UTC(),
	}
	if err := g.store.SaveDbRequestLog(ctx, log); err != nil {
		g.logger.Error("failed to save db request log", "error", err, "request_id", req.RequestID)
	}
}

// RecordExecuteAccess writes an audit log entry for an execute operation.
func (g *Gateway) RecordExecuteAccess(ctx context.Context, req *ExecuteRequest, resp *DbResponse) {
	log := &domain.DbRequestLog{
		ID:            uuid.New().String(),
		RequestID:     req.RequestID,
		FunctionID:    req.FunctionID,
		FunctionName:  req.FunctionName,
		Version:       req.Version,
		TenantID:      req.TenantID,
		DbResourceID:  req.DbResourceID,
		StatementHash: hashStatement(req.SQL),
		RowsAffected:  resp.RowsAffected,
		LatencyMs:     resp.LatencyMs,
		ErrorCode:     resp.Error,
		CreatedAt:     time.Now().UTC(),
	}
	if err := g.store.SaveDbRequestLog(ctx, log); err != nil {
		g.logger.Error("failed to save db request log", "error", err, "request_id", req.RequestID)
	}
}

// PoolStats returns current pool statistics for a database resource.
func (g *Gateway) PoolStats(dbResourceID string) (activeSessions, activeTx int) {
	g.mu.RLock()
	pool, ok := g.pools[dbResourceID]
	g.mu.RUnlock()
	if !ok {
		return 0, 0
	}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	return pool.activeSessions, pool.activeTx
}

func (g *Gateway) getOrCreatePool(resourceID string, quota *domain.DbBindingQuota) *ConnPool {
	g.mu.RLock()
	pool, ok := g.pools[resourceID]
	g.mu.RUnlock()
	if ok {
		return pool
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	// Double-check after acquiring write lock.
	if pool, ok = g.pools[resourceID]; ok {
		return pool
	}

	maxSessions := 0
	maxTxConc := 0
	if quota != nil {
		maxSessions = quota.MaxSessions
		maxTxConc = quota.MaxTxConcurrency
	}
	pool = &ConnPool{
		resourceID:  resourceID,
		maxSessions: maxSessions,
		maxTxConc:   maxTxConc,
	}
	g.pools[resourceID] = pool
	return pool
}

func hashStatement(sql string) string {
	h := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(h[:8])
}
