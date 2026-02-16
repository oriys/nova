package executor

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oriys/nova/internal/circuitbreaker"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

func (e *Executor) selectRolloutTarget(ctx context.Context, primary *domain.Function) *domain.Function {
	if primary == nil || primary.RolloutPolicy == nil || !primary.RolloutPolicy.Enabled {
		return primary
	}

	canaryName := strings.TrimSpace(primary.RolloutPolicy.CanaryFunction)
	if canaryName == "" || strings.EqualFold(canaryName, primary.Name) {
		return primary
	}

	percent := primary.RolloutPolicy.CanaryPercent
	if percent <= 0 {
		return primary
	}
	if percent > 100 {
		percent = 100
	}

	if rand.IntN(100) >= percent {
		return primary
	}

	canary, err := e.store.GetFunctionByName(ctx, canaryName)
	if err != nil {
		logging.Op().Warn(
			"rollout canary not found, fallback to primary",
			"primary_function", primary.Name,
			"canary_function", canaryName,
			"error", err.Error(),
		)
		return primary
	}
	return canary
}

// getBreakerForFunction returns the circuit breaker for a function based on its CapacityPolicy.
// Returns nil if the function has no circuit breaker configured.
func (e *Executor) getBreakerForFunction(fn *domain.Function) *circuitbreaker.Breaker {
	if fn.CapacityPolicy == nil || !fn.CapacityPolicy.Enabled {
		return nil
	}
	if fn.CapacityPolicy.BreakerErrorPct <= 0 || fn.CapacityPolicy.BreakerWindowS <= 0 || fn.CapacityPolicy.BreakerOpenS <= 0 {
		return nil
	}
	return e.breakers.Get(fn.ID, circuitbreaker.Config{
		ErrorPct:       fn.CapacityPolicy.BreakerErrorPct,
		WindowDuration: time.Duration(fn.CapacityPolicy.BreakerWindowS) * time.Second,
		OpenDuration:   time.Duration(fn.CapacityPolicy.BreakerOpenS) * time.Second,
		HalfOpenProbes: fn.CapacityPolicy.HalfOpenProbes,
	})
}

// BreakerSnapshot returns the current circuit breaker states for observability.
func (e *Executor) BreakerSnapshot() map[string]string {
	return e.breakers.Snapshot()
}

// persistInvocationLog asynchronously saves an invocation log to Postgres
func (e *Executor) persistInvocationLog(reqID string, fn *domain.Function, durationMs int64, coldStart, success bool, errMsg string, inputSize, outputSize int, input, output json.RawMessage, stdout, stderr string) {
	if !e.persistPayloads {
		input = nil
		output = nil
		stdout = ""
		stderr = ""
	}
	e.logBatcher.Enqueue(&store.InvocationLog{
		ID:           reqID,
		TenantID:     fn.TenantID,
		Namespace:    fn.Namespace,
		FunctionID:   fn.ID,
		FunctionName: fn.Name,
		Runtime:      string(fn.Runtime),
		DurationMs:   durationMs,
		ColdStart:    coldStart,
		Success:      success,
		ErrorMessage: errMsg,
		InputSize:    inputSize,
		OutputSize:   outputSize,
		Input:        input,
		Output:       output,
		Stdout:       stdout,
		Stderr:       stderr,
		CreatedAt:    time.Now(),
	})
}

// InvalidateSnapshot removes the snapshot for a function (e.g., after code update)
func InvalidateSnapshot(snapshotDir, funcID string) error {
	if snapshotDir == "" {
		return nil
	}
	metaPath := filepath.Join(snapshotDir, funcID+".meta")
	if metaData, err := os.ReadFile(metaPath); err == nil {
		var meta struct {
			CodeDrive       string `json:"code_drive"`
			CodeDriveBackup string `json:"code_drive_backup"`
		}
		if json.Unmarshal(metaData, &meta) == nil {
			if meta.CodeDrive != "" {
				if err := os.Remove(meta.CodeDrive); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			if meta.CodeDriveBackup != "" {
				if err := os.Remove(meta.CodeDriveBackup); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
		}
	}

	paths := []string{
		filepath.Join(snapshotDir, funcID+".snap"),
		filepath.Join(snapshotDir, funcID+".mem"),
		metaPath,
	}

	var lastErr error
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			lastErr = err
		}
	}
	return lastErr
}

// HasSnapshot checks if a function has a valid snapshot
func HasSnapshot(snapshotDir, funcID string) bool {
	if snapshotDir == "" {
		return false
	}
	snapPath := filepath.Join(snapshotDir, funcID+".snap")
	memPath := filepath.Join(snapshotDir, funcID+".mem")

	if _, err := os.Stat(snapPath); err != nil {
		return false
	}
	if _, err := os.Stat(memPath); err != nil {
		return false
	}
	return true
}

// Shutdown gracefully shuts down the executor, waiting for in-flight requests
func (e *Executor) Shutdown(timeout time.Duration) {
	e.closing.Store(true)

	// Wait for in-flight requests with timeout
	done := make(chan struct{})
	go func() {
		e.inflight.Wait()
		close(done)
	}()

	select {
	case <-done:
		logging.Op().Info("all in-flight requests completed")
	case <-time.After(timeout):
		logging.Op().Warn("shutdown timeout waiting for in-flight requests", "timeout", timeout)
	}

	e.logBatcher.Shutdown(timeout)
	e.pool.Shutdown()
}
