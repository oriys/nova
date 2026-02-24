// Package safepath provides utilities to prevent path traversal attacks.
package safepath

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Join safely joins a base directory with a relative path, returning an error
// if the resulting path escapes the base directory (e.g. via "../" sequences).
func Join(baseDir, relPath string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("empty relative path")
	}

	// Clean the relative path and reject absolute paths
	cleaned := filepath.Clean(relPath)
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("absolute path not allowed: %s", relPath)
	}

	// Reject paths that start with ".."
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal not allowed: %s", relPath)
	}

	full := filepath.Join(baseDir, cleaned)

	// Double-check: the resolved path must be under baseDir
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve base dir: %w", err)
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", fmt.Errorf("resolve full path: %w", err)
	}

	if !strings.HasPrefix(absFull, absBase+string(filepath.Separator)) && absFull != absBase {
		return "", fmt.Errorf("path traversal not allowed: %s resolves outside base dir", relPath)
	}

	return full, nil
}
