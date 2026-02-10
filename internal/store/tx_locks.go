package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const deleteOperationLockKey int64 = 0x6e6f76615f64656c // "nova_del"

func (s *PostgresStore) acquireDeleteOperationLock(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, deleteOperationLockKey); err != nil {
		return fmt.Errorf("acquire delete operation lock: %w", err)
	}
	return nil
}
