package main

import (
	"context"
	"github.com/oriys/nova/internal/config"
	"github.com/oriys/nova/internal/store"
)

func getStore() (*store.Store, error) {
	cfg := config.DefaultConfig()
	config.LoadFromEnv(cfg)

	if pgDSN != "" {
		cfg.Postgres.DSN = pgDSN
	}

	pgStore, err := store.NewPostgresStore(context.Background(), cfg.Postgres.DSN)
	if err != nil {
		return nil, err
	}

	return store.NewStore(pgStore), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
