// Package codeloader provides pluggable strategies for loading function code
// into Firecracker microVMs. It supports:
//
//   - "ext4" (default): Traditional approach, building a full ext4 code drive
//     via mkfs.ext4+debugfs and attaching it to the VM.
//   - "nbd": Network Block Device approach for lazy loading. Instead of
//     writing the entire code image up front, an NBD server exposes code
//     blocks on demand, reducing cold start I/O.
//
// The package also supports host-side layer caching via overlayfs, where
// shared runtime dependencies are mounted as read-only layers and overlaid
// to avoid redundant disk writes across VMs.
package codeloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/oriys/nova/internal/logging"
)

// Strategy defines the interface for code loading strategies.
type Strategy interface {
	// Prepare builds/prepares the code image for a function invocation.
	// Returns the path to the code drive or device to be attached to the VM.
	Prepare(ctx context.Context, req PrepareRequest) (string, error)

	// Cleanup releases resources associated with a code image.
	Cleanup(codePath string) error
}

// PrepareRequest contains all information needed to prepare a code image.
type PrepareRequest struct {
	FunctionID   string
	FunctionName string
	Code         []byte
	Files        map[string][]byte // for multi-file functions
	Runtime      string
	CodeHash     string
}

// LayerCache provides host-side caching of shared code/dependency layers.
// When multiple functions share the same dependency set (e.g., Python
// libraries), only one copy of the layer image is stored on the host.
type LayerCache struct {
	mu       sync.RWMutex
	cacheDir string
	entries  map[string]string // contentHash -> imagePath
}

// NewLayerCache creates a new host-side layer cache.
func NewLayerCache(cacheDir string) *LayerCache {
	if cacheDir == "" {
		cacheDir = "/opt/nova/layer-cache"
	}
	os.MkdirAll(cacheDir, 0755)
	lc := &LayerCache{
		cacheDir: cacheDir,
		entries:  make(map[string]string),
	}
	// Load existing cached layers
	lc.loadExisting()
	return lc
}

// Get returns the path to a cached layer image if it exists.
func (lc *LayerCache) Get(contentHash string) (string, bool) {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	path, ok := lc.entries[contentHash]
	if ok {
		if _, err := os.Stat(path); err != nil {
			return "", false
		}
	}
	return path, ok
}

// Put stores a layer image in the cache, deduplicating by content hash.
func (lc *LayerCache) Put(contentHash, sourcePath string) (string, error) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	// Check if already cached
	if existing, ok := lc.entries[contentHash]; ok {
		if _, err := os.Stat(existing); err == nil {
			return existing, nil
		}
	}

	cachedPath := filepath.Join(lc.cacheDir, contentHash+".ext4")

	// Create hard link instead of copy to save space and time
	if err := os.Link(sourcePath, cachedPath); err != nil {
		// Fall back to copy if hard link fails (cross-device)
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return "", fmt.Errorf("read source layer: %w", err)
		}
		if err := os.WriteFile(cachedPath, data, 0644); err != nil {
			return "", fmt.Errorf("write cached layer: %w", err)
		}
	}

	lc.entries[contentHash] = cachedPath
	logging.Op().Info("layer cached", "hash", contentHash[:12], "path", cachedPath)
	return cachedPath, nil
}

// Evict removes a cached layer by content hash.
func (lc *LayerCache) Evict(contentHash string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if path, ok := lc.entries[contentHash]; ok {
		os.Remove(path)
		delete(lc.entries, contentHash)
	}
}

// Size returns the number of cached layers.
func (lc *LayerCache) Size() int {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return len(lc.entries)
}

// loadExisting scans the cache directory for existing layer images.
func (lc *LayerCache) loadExisting() {
	entries, err := os.ReadDir(lc.cacheDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext == ".ext4" {
			hash := name[:len(name)-len(ext)]
			lc.entries[hash] = filepath.Join(lc.cacheDir, name)
		}
	}
	if len(lc.entries) > 0 {
		logging.Op().Info("loaded cached layers", "count", len(lc.entries))
	}
}

// ContentHash computes a SHA256 hash of code content for deduplication.
func ContentHash(code []byte) string {
	h := sha256.Sum256(code)
	return hex.EncodeToString(h[:])
}
