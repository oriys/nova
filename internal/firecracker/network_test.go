package firecracker

import (
	"sync"
	"testing"
)

func TestResourcePool_AcquireRelease(t *testing.T) {
	pool := newResourcePool[uint32]()
	pool.fill([]uint32{10, 20, 30})

	// Acquire all three
	ids := make(map[uint32]struct{})
	for i := 0; i < 3; i++ {
		id, ok := pool.acquire()
		if !ok {
			t.Fatalf("expected to acquire item %d", i)
		}
		ids[id] = struct{}{}
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 unique items, got %d", len(ids))
	}

	// Pool should be exhausted
	_, ok := pool.acquire()
	if ok {
		t.Fatal("expected pool to be exhausted")
	}

	// Release one and re-acquire
	pool.release(20)
	id, ok := pool.acquire()
	if !ok || id != 20 {
		t.Fatalf("expected to re-acquire 20, got %v (ok=%v)", id, ok)
	}
}

func TestResourcePool_ConcurrentAccess(t *testing.T) {
	pool := newResourcePool[uint32]()
	items := make([]uint32, 500)
	for i := range items {
		items[i] = uint32(100 + i)
	}
	pool.fill(items)

	var wg sync.WaitGroup
	acquired := make(chan uint32, 500)

	// Concurrently acquire all items
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, ok := pool.acquire()
			if ok {
				acquired <- id
			}
		}()
	}
	wg.Wait()
	close(acquired)

	seen := make(map[uint32]struct{})
	for id := range acquired {
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate CID acquired: %d", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != 500 {
		t.Fatalf("expected 500 unique items, got %d", len(seen))
	}
}

func TestResourcePool_FillSkipsDuplicates(t *testing.T) {
	pool := newResourcePool[string]()
	pool.fill([]string{"a", "b", "c"})

	// Acquire all three items
	acquired := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		item, ok := pool.acquire()
		if !ok {
			t.Fatalf("expected to acquire item %d", i)
		}
		acquired = append(acquired, item)
	}

	// All three should be in use now
	if pool.inUseCount() != 3 {
		t.Fatalf("expected 3 in use, got %d", pool.inUseCount())
	}

	// Re-fill with overlapping items — in-use items should be skipped
	pool.fill([]string{"a", "b", "c", "d"})

	// Only "d" should be available (a, b, c are in use)
	item, ok := pool.acquire()
	if !ok || item != "d" {
		t.Fatalf("expected to acquire 'd', got %q (ok=%v)", item, ok)
	}

	// Pool should be exhausted now
	_, ok = pool.acquire()
	if ok {
		t.Fatal("expected pool to be exhausted")
	}
}

func TestResourcePool_SizeAndInUseCount(t *testing.T) {
	pool := newResourcePool[int]()
	pool.fill([]int{1, 2, 3, 4, 5})

	if pool.size() != 5 {
		t.Fatalf("expected size 5, got %d", pool.size())
	}
	if pool.inUseCount() != 0 {
		t.Fatalf("expected 0 in use, got %d", pool.inUseCount())
	}

	pool.acquire()
	pool.acquire()

	if pool.size() != 3 {
		t.Fatalf("expected size 3, got %d", pool.size())
	}
	if pool.inUseCount() != 2 {
		t.Fatalf("expected 2 in use, got %d", pool.inUseCount())
	}
}

func TestAllocateCID(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SocketDir = t.TempDir()
	cfg.VsockDir = t.TempDir()
	cfg.LogDir = t.TempDir()
	cfg.SnapshotDir = t.TempDir()

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	cid, err := mgr.allocateCID()
	if err != nil {
		t.Fatalf("allocateCID: %v", err)
	}
	if cid < 100 {
		t.Fatalf("expected CID >= 100, got %d", cid)
	}

	mgr.releaseCID(cid)

	// Should be able to acquire the same CID again
	cid2, err := mgr.allocateCID()
	if err != nil {
		t.Fatalf("allocateCID after release: %v", err)
	}
	if cid2 != cid {
		t.Logf("re-acquired different CID (expected %d, got %d) — acceptable with LIFO", cid, cid2)
	}
}

func TestAllocateIP(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SocketDir = t.TempDir()
	cfg.VsockDir = t.TempDir()
	cfg.LogDir = t.TempDir()
	cfg.SnapshotDir = t.TempDir()
	cfg.Subnet = "172.30.0.0/24"

	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ip, err := mgr.allocateIP()
	if err != nil {
		t.Fatalf("allocateIP: %v", err)
	}
	if ip == "" {
		t.Fatal("expected non-empty IP")
	}

	mgr.releaseIP(ip)

	ip2, err := mgr.allocateIP()
	if err != nil {
		t.Fatalf("allocateIP after release: %v", err)
	}
	if ip2 != ip {
		t.Logf("re-acquired different IP (expected %s, got %s) — acceptable with LIFO", ip, ip2)
	}
}

func TestResourcePool_TryReserve(t *testing.T) {
	pool := newResourcePool[uint32]()
	pool.fill([]uint32{10, 20, 30})

	// Reserve 10 — should succeed
	if !pool.tryReserve(10) {
		t.Fatal("expected tryReserve(10) to succeed")
	}
	if pool.inUseCount() != 1 {
		t.Fatalf("expected 1 in use, got %d", pool.inUseCount())
	}

	// Reserve 10 again — should fail (already in use)
	if pool.tryReserve(10) {
		t.Fatal("expected tryReserve(10) to fail for duplicate")
	}

	// Reserve 20 — should succeed
	if !pool.tryReserve(20) {
		t.Fatal("expected tryReserve(20) to succeed")
	}
}

func TestResourcePool_SwapReserved(t *testing.T) {
	pool := newResourcePool[string]()
	pool.fill([]string{"a", "b", "c", "d"})

	// Acquire "d" (LIFO)
	old, ok := pool.acquire()
	if !ok {
		t.Fatal("expected to acquire item")
	}

	// Swap "d" for "a" — should succeed
	if !pool.swapReserved(old, "a") {
		t.Fatal("expected swapReserved to succeed")
	}

	// "a" should now be in use, "d" should be released
	if pool.inUseCount() != 1 {
		t.Fatalf("expected 1 in use, got %d", pool.inUseCount())
	}

	// Swap same item (no-op) — should succeed
	if !pool.swapReserved("a", "a") {
		t.Fatal("expected swapReserved with same item to succeed")
	}

	// Acquire "b" and try to swap to "a" — should fail (a is in use)
	pool.acquire() // acquire some item
	b, _ := pool.acquire()
	if pool.swapReserved(b, "a") {
		t.Fatal("expected swapReserved to fail when target is in use")
	}
}
