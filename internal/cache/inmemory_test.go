package cache

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryCache_SetAndGet(t *testing.T) {
	c := NewInMemoryCache()
	defer c.Close()

	ctx := context.Background()

	// Set a value
	if err := c.Set(ctx, "key1", []byte("value1"), time.Minute); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get the value
	val, err := c.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(val) != "value1" {
		t.Fatalf("expected 'value1', got '%s'", string(val))
	}
}

func TestInMemoryCache_GetMissing(t *testing.T) {
	c := NewInMemoryCache()
	defer c.Close()

	ctx := context.Background()

	_, err := c.Get(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestInMemoryCache_Expiry(t *testing.T) {
	c := NewInMemoryCache()
	defer c.Close()

	ctx := context.Background()

	// Set with very short TTL
	if err := c.Set(ctx, "expiring", []byte("value"), 10*time.Millisecond); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Should exist immediately
	val, err := c.Get(ctx, "expiring")
	if err != nil {
		t.Fatalf("Get failed immediately after set: %v", err)
	}
	if string(val) != "value" {
		t.Fatalf("expected 'value', got '%s'", string(val))
	}

	// Wait for expiry
	time.Sleep(20 * time.Millisecond)

	_, err = c.Get(ctx, "expiring")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after expiry, got: %v", err)
	}
}

func TestInMemoryCache_Delete(t *testing.T) {
	c := NewInMemoryCache()
	defer c.Close()

	ctx := context.Background()

	c.Set(ctx, "del-key", []byte("value"), time.Minute)

	if err := c.Delete(ctx, "del-key"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := c.Get(ctx, "del-key")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got: %v", err)
	}

	// Delete non-existent key should not error
	if err := c.Delete(ctx, "nonexistent"); err != nil {
		t.Fatalf("Delete non-existent should not fail: %v", err)
	}
}

func TestInMemoryCache_Exists(t *testing.T) {
	c := NewInMemoryCache()
	defer c.Close()

	ctx := context.Background()

	exists, err := c.Exists(ctx, "missing")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Fatal("expected missing key to not exist")
	}

	c.Set(ctx, "present", []byte("value"), time.Minute)

	exists, err = c.Exists(ctx, "present")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Fatal("expected present key to exist")
	}
}

func TestInMemoryCache_Ping(t *testing.T) {
	c := NewInMemoryCache()
	defer c.Close()

	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestInMemoryCache_ValueIsolation(t *testing.T) {
	c := NewInMemoryCache()
	defer c.Close()

	ctx := context.Background()

	original := []byte("original")
	c.Set(ctx, "iso", original, time.Minute)

	// Mutate original - should not affect cached value
	original[0] = 'X'

	val, _ := c.Get(ctx, "iso")
	if string(val) != "original" {
		t.Fatal("cache should store a copy, not reference to original slice")
	}

	// Mutate returned value - should not affect cached value
	val[0] = 'Z'
	val2, _ := c.Get(ctx, "iso")
	if string(val2) != "original" {
		t.Fatal("cache should return a copy, not reference to internal slice")
	}
}

func TestInMemoryCache_ZeroTTL(t *testing.T) {
	c := NewInMemoryCache()
	defer c.Close()

	ctx := context.Background()

	// Zero TTL = no expiration
	if err := c.Set(ctx, "forever", []byte("value"), 0); err != nil {
		t.Fatalf("Set with zero TTL failed: %v", err)
	}

	val, err := c.Get(ctx, "forever")
	if err != nil {
		t.Fatalf("Get with zero TTL failed: %v", err)
	}
	if string(val) != "value" {
		t.Fatalf("expected 'value', got '%s'", string(val))
	}
}
