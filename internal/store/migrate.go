package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration represents a single schema migration with an up (apply) and
// optional down (rollback) function.  Migrations are executed inside a
// transaction that also holds the advisory lock used by ensureSchema.
type Migration struct {
	Version     int
	Description string
	Up          func(ctx context.Context, tx pgx.Tx) error
	Down        func(ctx context.Context, tx pgx.Tx) error // nil = irreversible
}

// MigrationRecord is a row in the schema_migrations table.
type MigrationRecord struct {
	Version     int
	Description string
	AppliedAt   time.Time
}

// migrationSchemaSQL bootstraps the schema_migrations table itself.
const migrationSchemaSQL = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version     INTEGER PRIMARY KEY,
	description TEXT NOT NULL DEFAULT '',
	applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`

// RunMigrations applies all pending migrations in order.  It acquires the same
// advisory lock as ensureSchema to serialise concurrent callers.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, migrations []Migration) error {
	// Sort by version.
	sorted := make([]Migration, len(migrations))
	copy(sorted, migrations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Version < sorted[j].Version })

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Use the same advisory lock key as ensureSchema (0x6e6f7661 = "nova").
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(0x6e6f7661)); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}

	// Ensure the tracking table exists.
	if _, err := tx.Exec(ctx, migrationSchemaSQL); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	// Load already-applied versions.
	applied, err := loadAppliedVersions(ctx, tx)
	if err != nil {
		return err
	}

	for _, m := range sorted {
		if applied[m.Version] {
			continue
		}
		if err := m.Up(ctx, tx); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.Version, m.Description, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version, description) VALUES ($1, $2)`,
			m.Version, m.Description,
		); err != nil {
			return fmt.Errorf("record migration %d (%s): %w", m.Version, m.Description, err)
		}
		fmt.Printf("[migrate] applied %d: %s\n", m.Version, m.Description)
	}

	return tx.Commit(ctx)
}

// RollbackMigration rolls back a single migration by version number.
func RollbackMigration(ctx context.Context, pool *pgxpool.Pool, migrations []Migration, version int) error {
	var target *Migration
	for i := range migrations {
		if migrations[i].Version == version {
			target = &migrations[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("migration %d not found", version)
	}
	if target.Down == nil {
		return fmt.Errorf("migration %d (%s) is irreversible", version, target.Description)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rollback tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(0x6e6f7661)); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}

	applied, err := loadAppliedVersions(ctx, tx)
	if err != nil {
		return err
	}
	if !applied[version] {
		return fmt.Errorf("migration %d is not applied", version)
	}

	if err := target.Down(ctx, tx); err != nil {
		return fmt.Errorf("rollback migration %d (%s): %w", version, target.Description, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version); err != nil {
		return fmt.Errorf("remove migration record %d: %w", version, err)
	}
	fmt.Printf("[migrate] rolled back %d: %s\n", version, target.Description)

	return tx.Commit(ctx)
}

// MigrationStatus returns the list of applied migrations.
func MigrationStatus(ctx context.Context, pool *pgxpool.Pool) ([]MigrationRecord, error) {
	// Ensure table exists for first-run queries.
	if _, err := pool.Exec(ctx, migrationSchemaSQL); err != nil {
		return nil, fmt.Errorf("ensure schema_migrations: %w", err)
	}
	rows, err := pool.Query(ctx, `SELECT version, description, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("query migrations: %w", err)
	}
	defer rows.Close()

	var recs []MigrationRecord
	for rows.Next() {
		var r MigrationRecord
		if err := rows.Scan(&r.Version, &r.Description, &r.AppliedAt); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return recs, rows.Err()
}

// CurrentVersion returns the highest applied migration version, or 0 if none.
func CurrentVersion(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	// Ensure table exists.
	if _, err := pool.Exec(ctx, migrationSchemaSQL); err != nil {
		return 0, fmt.Errorf("ensure schema_migrations: %w", err)
	}
	var v *int
	err := pool.QueryRow(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v)
	if err != nil {
		return 0, err
	}
	if v == nil {
		return 0, nil
	}
	return *v, nil
}

func loadAppliedVersions(ctx context.Context, tx pgx.Tx) (map[int]bool, error) {
	rows, err := tx.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}
