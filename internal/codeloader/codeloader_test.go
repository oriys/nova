package codeloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLayerCache_PutAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	lc := NewLayerCache(tmpDir)

	// Create a fake layer file
	srcPath := filepath.Join(tmpDir, "source.ext4")
	if err := os.WriteFile(srcPath, []byte("fake-ext4-data"), 0644); err != nil {
		t.Fatal(err)
	}

	hash := "abc123def456"
	cachedPath, err := lc.Put(hash, srcPath)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get should return the cached path
	gotPath, ok := lc.Get(hash)
	if !ok {
		t.Fatal("Get should find cached layer")
	}
	if gotPath != cachedPath {
		t.Fatalf("expected %s, got %s", cachedPath, gotPath)
	}

	// Size should be 1
	if lc.Size() != 1 {
		t.Fatalf("expected size 1, got %d", lc.Size())
	}
}

func TestLayerCache_Deduplication(t *testing.T) {
	tmpDir := t.TempDir()
	lc := NewLayerCache(tmpDir)

	srcPath := filepath.Join(tmpDir, "source.ext4")
	if err := os.WriteFile(srcPath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	hash := "dedup-hash-123"
	path1, err := lc.Put(hash, srcPath)
	if err != nil {
		t.Fatal(err)
	}

	// Second Put with same hash should return same path
	path2, err := lc.Put(hash, srcPath)
	if err != nil {
		t.Fatal(err)
	}

	if path1 != path2 {
		t.Fatalf("expected same path for duplicate, got %s and %s", path1, path2)
	}

	if lc.Size() != 1 {
		t.Fatalf("expected size 1 after dedup, got %d", lc.Size())
	}
}

func TestLayerCache_Evict(t *testing.T) {
	tmpDir := t.TempDir()
	lc := NewLayerCache(tmpDir)

	srcPath := filepath.Join(tmpDir, "source.ext4")
	if err := os.WriteFile(srcPath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	hash := "evict-hash-123"
	lc.Put(hash, srcPath)

	lc.Evict(hash)

	_, ok := lc.Get(hash)
	if ok {
		t.Fatal("Get should not find evicted layer")
	}

	if lc.Size() != 0 {
		t.Fatalf("expected size 0 after evict, got %d", lc.Size())
	}
}

func TestLayerCache_LoadExisting(t *testing.T) {
	tmpDir := t.TempDir()

	// Pre-populate cache directory
	hash := "existing-hash-abc"
	if err := os.WriteFile(filepath.Join(tmpDir, hash+".ext4"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create new cache - should load existing entries
	lc := NewLayerCache(tmpDir)

	if lc.Size() != 1 {
		t.Fatalf("expected 1 pre-loaded entry, got %d", lc.Size())
	}

	_, ok := lc.Get(hash)
	if !ok {
		t.Fatal("should find pre-loaded layer")
	}
}

func TestLayerCache_GetMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	lc := NewLayerCache(tmpDir)

	// Manually add an entry that doesn't exist on disk
	lc.mu.Lock()
	lc.entries["missing"] = "/nonexistent/path.ext4"
	lc.mu.Unlock()

	_, ok := lc.Get("missing")
	if ok {
		t.Fatal("should not find layer with missing file")
	}
}

func TestContentHash(t *testing.T) {
	hash1 := ContentHash([]byte("hello"))
	hash2 := ContentHash([]byte("hello"))
	hash3 := ContentHash([]byte("world"))

	if hash1 != hash2 {
		t.Fatal("same content should produce same hash")
	}
	if hash1 == hash3 {
		t.Fatal("different content should produce different hash")
	}
	if len(hash1) != 64 {
		t.Fatalf("expected 64 char hex hash, got %d", len(hash1))
	}
}
