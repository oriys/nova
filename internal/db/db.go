// Package db defines an abstract database interface for connection management
// and transactional operations. This allows the MetadataStore to be backed by
// different databases (PostgreSQL, MySQL, SQLite, CockroachDB, etc.) without
// changing the business logic layer.
package db

import (
	"context"
)

// Row represents a single row returned by a query.
type Row interface {
	Scan(dest ...any) error
}

// Rows represents a set of rows returned by a query.
type Rows interface {
	// Next advances to the next row, returning false when exhausted.
	Next() bool
	// Scan reads column values from the current row.
	Scan(dest ...any) error
	// Err returns any error encountered during iteration.
	Err() error
	// Close releases the rows.
	Close()
}

// Result describes the outcome of an executed statement.
type Result interface {
	// RowsAffected returns the number of rows affected by the statement.
	RowsAffected() int64
}

// Executor can execute queries and statements. Both DB and Tx satisfy this
// interface, enabling code that works inside or outside a transaction.
type Executor interface {
	// Exec executes a statement that does not return rows.
	Exec(ctx context.Context, sql string, args ...any) (Result, error)
	// QueryRow executes a query expected to return at most one row.
	QueryRow(ctx context.Context, sql string, args ...any) Row
	// Query executes a query that returns multiple rows.
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
}

// Tx represents a database transaction. Implementations must ensure that
// Commit or Rollback is called exactly once.
type Tx interface {
	Executor
	// Commit commits the transaction.
	Commit(ctx context.Context) error
	// Rollback rolls back the transaction.
	Rollback(ctx context.Context) error
}

// TxOptions configures transaction behavior.
type TxOptions struct {
	// ReadOnly hints that the transaction will only perform reads.
	ReadOnly bool
	// IsolationLevel sets the transaction isolation level.
	// Supported values are implementation-specific (e.g. "serializable",
	// "read committed").
	IsolationLevel string
}

// Database abstracts a SQL-compatible database connection pool.
// Implementations should handle connection pooling, health checks, and
// reconnection internally.
type Database interface {
	Executor

	// BeginTx starts a new transaction with the given options.
	BeginTx(ctx context.Context, opts *TxOptions) (Tx, error)

	// Ping verifies database connectivity.
	Ping(ctx context.Context) error

	// Close releases all connections in the pool.
	Close() error

	// DriverName returns the name of the underlying database driver
	// (e.g. "postgres", "mysql", "sqlite").
	DriverName() string
}
