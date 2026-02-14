package codeloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/oriys/nova/internal/logging"
)

// OverlayStrategy implements Strategy using OverlayFS to share read-only
// runtime layers across multiple VMs. Each VM gets a thin upper layer for
// function-specific code, while expensive runtime dependencies (e.g., Python
// packages, Node modules) are mounted as shared lower layers.
//
// This dramatically reduces per-VM disk I/O and storage by deduplicating
// shared content across VMs using the same runtime.
type OverlayStrategy struct {
	mu          sync.Mutex
	baseDir     string     // base directory for overlay mounts
	layerCache  *LayerCache
	activeMounts map[string]string // codePath -> overlay merged dir
}

// NewOverlayStrategy creates a new overlay-based code loading strategy.
func NewOverlayStrategy(baseDir string, cache *LayerCache) *OverlayStrategy {
	if baseDir == "" {
		baseDir = "/opt/nova/overlays"
	}
	os.MkdirAll(baseDir, 0755)
	return &OverlayStrategy{
		baseDir:      baseDir,
		layerCache:   cache,
		activeMounts: make(map[string]string),
	}
}

// Prepare creates an overlay mount for a function. The runtime rootfs serves
// as the lower (shared, read-only) layer and the function code is placed in
// the upper (writable) layer. Multiple VMs for the same runtime share the
// same lower layer, while each VM gets its own upper layer.
func (s *OverlayStrategy) Prepare(ctx context.Context, req PrepareRequest) (string, error) {
	// Compute content hash for caching
	codeHash := req.CodeHash
	if codeHash == "" {
		h := sha256.Sum256(req.Code)
		codeHash = hex.EncodeToString(h[:])
	}

	// Create per-VM overlay directory structure
	vmDir := filepath.Join(s.baseDir, req.FunctionID, codeHash[:12])
	upperDir := filepath.Join(vmDir, "upper")
	workDir := filepath.Join(vmDir, "work")
	mergedDir := filepath.Join(vmDir, "merged")

	for _, dir := range []string{upperDir, workDir, mergedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("create overlay dir %s: %w", dir, err)
		}
	}

	// Write function code to the upper layer
	if len(req.Files) > 0 {
		for name, content := range req.Files {
			path := filepath.Join(upperDir, name)
			if dir := filepath.Dir(path); dir != upperDir {
				os.MkdirAll(dir, 0755)
			}
			if err := os.WriteFile(path, content, 0755); err != nil {
				return "", fmt.Errorf("write file %s: %w", name, err)
			}
		}
	} else if len(req.Code) > 0 {
		codePath := filepath.Join(upperDir, "handler")
		if err := os.WriteFile(codePath, req.Code, 0755); err != nil {
			return "", fmt.Errorf("write handler: %w", err)
		}
	}

	// Check if a cached runtime layer exists (shared across all functions
	// with the same runtime).
	runtimeHash := "runtime:" + req.Runtime
	lowerDir := ""
	if cachedPath, ok := s.layerCache.Get(runtimeHash); ok {
		lowerDir = cachedPath
	}

	// If we have a shared lower layer, mount overlayfs; otherwise just
	// use the upper dir directly (no overlay mount needed).
	if lowerDir != "" {
		opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
		cmd := exec.CommandContext(ctx, "mount", "-t", "overlay", "overlay", "-o", opts, mergedDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			logging.Op().Warn("overlayfs mount failed, falling back to upper dir",
				"error", err,
				"output", string(out))
			return upperDir, nil
		}

		s.mu.Lock()
		s.activeMounts[mergedDir] = mergedDir
		s.mu.Unlock()

		logging.Op().Debug("overlay mount created",
			"function", req.FunctionName,
			"merged", mergedDir)
		return mergedDir, nil
	}

	return upperDir, nil
}

// Cleanup unmounts and removes the overlay directory for a code image.
func (s *OverlayStrategy) Cleanup(codePath string) error {
	s.mu.Lock()
	_, isOverlay := s.activeMounts[codePath]
	if isOverlay {
		delete(s.activeMounts, codePath)
	}
	s.mu.Unlock()

	if isOverlay {
		// Unmount the overlayfs
		cmd := exec.Command("umount", codePath)
		if err := cmd.Run(); err != nil {
			logging.Op().Warn("overlay unmount failed", "path", codePath, "error", err)
		}
	}

	// Remove the parent VM directory (upper + work + merged)
	vmDir := filepath.Dir(codePath)
	return os.RemoveAll(vmDir)
}

// CacheRuntimeLayer stores a runtime's base filesystem as a shared lower layer
// that can be reused by all functions using this runtime.
func (s *OverlayStrategy) CacheRuntimeLayer(runtime string, layerPath string) error {
	runtimeHash := "runtime:" + runtime
	_, err := s.layerCache.Put(runtimeHash, layerPath)
	return err
}

// Stats returns overlay strategy statistics.
func (s *OverlayStrategy) Stats() map[string]interface{} {
	s.mu.Lock()
	activeCount := len(s.activeMounts)
	s.mu.Unlock()

	return map[string]interface{}{
		"active_mounts":  activeCount,
		"cached_layers":  s.layerCache.Size(),
		"base_dir":       s.baseDir,
	}
}
