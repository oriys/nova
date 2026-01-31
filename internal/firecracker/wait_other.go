//go:build !linux

package firecracker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// waitForFileInotify is a fallback polling implementation for non-Linux platforms.
// On Linux, the inotify-based implementation in wait_linux.go is used instead.
func waitForFileInotify(ctx context.Context, dir, filename string, deadline time.Time) error {
	fullPath := filepath.Join(dir, filename)

	for time.Now().Before(deadline) {
		if _, err := os.Stat(fullPath); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond): // Fast polling as fallback
		}
	}

	return fmt.Errorf("timeout waiting for %s", filename)
}
