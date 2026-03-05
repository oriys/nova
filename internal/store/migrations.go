package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Migrations is the ordered list of all schema migrations.
// Existing schema created by ensureSchema is treated as version 0 (baseline).
// New structural changes should be added here as numbered migrations.
//
// Example of adding a new migration:
//
//	{
//	    Version:     2,
//	    Description: "add foo column to functions",
//	    Up: func(ctx context.Context, tx pgx.Tx) error {
//	        _, err := tx.Exec(ctx, `ALTER TABLE functions ADD COLUMN IF NOT EXISTS foo TEXT DEFAULT ''`)
//	        return err
//	    },
//	    Down: func(ctx context.Context, tx pgx.Tx) error {
//	        _, err := tx.Exec(ctx, `ALTER TABLE functions DROP COLUMN IF EXISTS foo`)
//	        return err
//	    },
//	},
var Migrations = []Migration{
	{
		Version:     1,
		Description: "baseline: mark existing ensureSchema as applied",
		Up: func(ctx context.Context, tx pgx.Tx) error {
			// The existing ensureSchema() already created all tables.
			// This migration only records the baseline version.
			return nil
		},
		Down: nil, // irreversible — cannot undo the entire schema
	},
}
