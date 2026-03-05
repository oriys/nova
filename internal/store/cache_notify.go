package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	cacheInvalidationChannel = "nova_cache_invalidation"
)

// NotifyFunctionInvalidation sends a NOTIFY on the shared cache invalidation
// channel so that other instances can evict their local caches for the given
// function ID. The notification payload is the function ID.
func NotifyFunctionInvalidation(ctx context.Context, pool *pgxpool.Pool, funcID string) {
	if pool == nil || funcID == "" {
		return
	}
	_, _ = pool.Exec(ctx,
		fmt.Sprintf(`NOTIFY %s, '%s'`, cacheInvalidationChannel, funcID))
}

// StartCacheInvalidationListener starts a background goroutine that listens
// for PostgreSQL NOTIFY events on the cache invalidation channel and calls
// the provided callback for each received function ID.
// Returns a stop function to shut down the listener.
func StartCacheInvalidationListener(pool *pgxpool.Pool, onInvalidate func(funcID string)) (stop func()) {
	if pool == nil || onInvalidate == nil {
		return func() {}
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			if err := listenLoop(ctx, pool, onInvalidate); err != nil {
				// Reconnect after a brief pause on transient errors.
				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
				}
			}
		}
	}()
	return cancel
}

func listenLoop(ctx context.Context, pool *pgxpool.Pool, onInvalidate func(string)) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn for LISTEN: %w", err)
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, fmt.Sprintf("LISTEN %s", cacheInvalidationChannel))
	if err != nil {
		return fmt.Errorf("LISTEN: %w", err)
	}

	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			return fmt.Errorf("wait notification: %w", err)
		}
		funcID := strings.TrimSpace(notification.Payload)
		if funcID != "" {
			onInvalidate(funcID)
		}
	}
}
