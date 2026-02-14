package cache

import (
	"context"
	"testing"
	"time"
)

func TestTieredCache_L1Hit(t *testing.T) {
	l1 := NewInMemoryCache()
	l2 := NewInMemoryCache()
	defer l1.Close()
	defer l2.Close()

	tc := NewTieredCache(l1, l2, 10*time.Second)
	defer tc.Close()

	ctx := context.Background()

	// Set value in tiered cache
	if err := tc.Set(ctx, "key1", []byte("value1"), time.Minute); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Should hit L1
	val, err := tc.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(val) != "value1" {
		t.Fatalf("expected 'value1', got '%s'", string(val))
	}
}

func TestTieredCache_L2Fallthrough(t *testing.T) {
	l1 := NewInMemoryCache()
	l2 := NewInMemoryCache()
	defer l1.Close()
	defer l2.Close()

	tc := NewTieredCache(l1, l2, 10*time.Second)
	defer tc.Close()

	ctx := context.Background()

	// Set value directly in L2 (simulating L1 miss)
	if err := l2.Set(ctx, "key2", []byte("value2"), time.Minute); err != nil {
		t.Fatalf("L2 Set failed: %v", err)
	}

	// Should miss L1, hit L2, and populate L1
	val, err := tc.Get(ctx, "key2")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(val) != "value2" {
		t.Fatalf("expected 'value2', got '%s'", string(val))
	}

	// Now L1 should have the value
	val, err = l1.Get(ctx, "key2")
	if err != nil {
		t.Fatalf("L1 Get after fallthrough failed: %v", err)
	}
	if string(val) != "value2" {
		t.Fatalf("expected 'value2' in L1, got '%s'", string(val))
	}
}

func TestTieredCache_BothMiss(t *testing.T) {
	l1 := NewInMemoryCache()
	l2 := NewInMemoryCache()
	defer l1.Close()
	defer l2.Close()

	tc := NewTieredCache(l1, l2, 10*time.Second)
	defer tc.Close()

	ctx := context.Background()

	_, err := tc.Get(ctx, "missing")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestTieredCache_Delete(t *testing.T) {
	l1 := NewInMemoryCache()
	l2 := NewInMemoryCache()
	defer l1.Close()
	defer l2.Close()

	tc := NewTieredCache(l1, l2, 10*time.Second)
	defer tc.Close()

	ctx := context.Background()

	tc.Set(ctx, "del-key", []byte("value"), time.Minute)

	// Delete should remove from both layers
	if err := tc.Delete(ctx, "del-key"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Both L1 and L2 should miss
	_, err := l1.Get(ctx, "del-key")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound in L1 after delete, got: %v", err)
	}
	_, err = l2.Get(ctx, "del-key")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound in L2 after delete, got: %v", err)
	}
}

func TestTieredCache_Exists(t *testing.T) {
	l1 := NewInMemoryCache()
	l2 := NewInMemoryCache()
	defer l1.Close()
	defer l2.Close()

	tc := NewTieredCache(l1, l2, 10*time.Second)
	defer tc.Close()

	ctx := context.Background()

	exists, err := tc.Exists(ctx, "missing")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Fatal("expected missing key to not exist")
	}

	tc.Set(ctx, "present", []byte("value"), time.Minute)
	exists, err = tc.Exists(ctx, "present")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Fatal("expected present key to exist")
	}
}

func TestTieredCache_Ping(t *testing.T) {
	l1 := NewInMemoryCache()
	l2 := NewInMemoryCache()
	defer l1.Close()
	defer l2.Close()

	tc := NewTieredCache(l1, l2, 10*time.Second)
	defer tc.Close()

	if err := tc.Ping(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestTieredCache_DefaultL1TTL(t *testing.T) {
	l1 := NewInMemoryCache()
	l2 := NewInMemoryCache()
	defer l1.Close()
	defer l2.Close()

	// Zero TTL should default to 10s
	tc := NewTieredCache(l1, l2, 0)
	defer tc.Close()

	ctx := context.Background()
	tc.Set(ctx, "key", []byte("val"), time.Minute)

	// Should be retrievable
	val, err := tc.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(val) != "val" {
		t.Fatalf("expected 'val', got '%s'", string(val))
	}
}
