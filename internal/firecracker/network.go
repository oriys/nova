package firecracker

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// resourcePool is a thread-safe free-list of pre-allocated resources.
// It avoids linear scanning under high concurrency by maintaining a
// ready-to-use pool of CIDs or IPs that can be acquired in O(1).
type resourcePool[T comparable] struct {
	mu       sync.Mutex
	free     []T    // free-list (stack, LIFO for cache locality)
	inUse    map[T]struct{}
}

func newResourcePool[T comparable]() *resourcePool[T] {
	return &resourcePool[T]{
		inUse: make(map[T]struct{}),
	}
}

// fill adds items to the free list, skipping any already in use.
func (p *resourcePool[T]) fill(items []T) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range items {
		if _, used := p.inUse[item]; !used {
			p.free = append(p.free, item)
		}
	}
}

// acquire pops one item from the free list in O(1).
func (p *resourcePool[T]) acquire() (T, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.free) > 0 {
		last := len(p.free) - 1
		item := p.free[last]
		p.free = p.free[:last]
		if _, used := p.inUse[item]; used {
			continue // skip stale entries
		}
		p.inUse[item] = struct{}{}
		return item, true
	}
	var zero T
	return zero, false
}

// release returns an item to the free list.
func (p *resourcePool[T]) release(item T) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.inUse[item]; ok {
		delete(p.inUse, item)
		p.free = append(p.free, item)
	}
}

// tryReserve attempts to mark item as in-use without removing it from the free
// list (it may not be there). Returns false if the item is already in use by
// another caller. This is used during snapshot restore where the CID/IP from
// the snapshot must be reserved if not already taken.
func (p *resourcePool[T]) tryReserve(item T) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, used := p.inUse[item]; used {
		return false
	}
	p.inUse[item] = struct{}{}
	return true
}

// forceReserve marks item as in-use unconditionally. Use only when the caller
// already knows the item is not in use elsewhere (e.g. snapshot restore where
// the new CID equals the original CID that was already reserved).
func (p *resourcePool[T]) forceReserve(item T) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inUse[item] = struct{}{}
}

// swapReserved atomically reserves newItem and releases oldItem.
// Returns false if newItem is already in use by a different caller.
// When oldItem == newItem, this is a no-op that returns true.
func (p *resourcePool[T]) swapReserved(oldItem, newItem T) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if oldItem == newItem {
		return true
	}
	if _, used := p.inUse[newItem]; used {
		return false
	}
	p.inUse[newItem] = struct{}{}
	if _, ok := p.inUse[oldItem]; ok {
		delete(p.inUse, oldItem)
		p.free = append(p.free, oldItem)
	}
	return true
}

// size returns the number of items currently available.
func (p *resourcePool[T]) size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.free)
}

// inUseCount returns the number of items currently in use.
func (p *resourcePool[T]) inUseCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.inUse)
}

// initCIDPool pre-fills the CID resource pool with available CIDs.
// CIDs 0-2 are reserved by the vsock spec; we start from 100.
func (m *Manager) initCIDPool() {
	const cidStart uint32 = 100
	const cidCount = 4096 // pre-allocate up to 4096 CIDs
	cids := make([]uint32, 0, cidCount)
	for i := uint32(0); i < cidCount; i++ {
		cids = append(cids, cidStart+i)
	}
	m.cidPool.fill(cids)
}

// initIPPool pre-fills the IP resource pool from the configured subnet.
func (m *Manager) initIPPool() error {
	baseIP, ipNet, err := net.ParseCIDR(m.config.Subnet)
	if err != nil {
		return fmt.Errorf("parse subnet: %w", err)
	}
	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return fmt.Errorf("unsupported subnet mask: %d", bits)
	}
	hostCount := uint32(1) << uint32(32-ones)
	if hostCount <= 3 {
		return fmt.Errorf("subnet too small for VM allocation")
	}
	startOffset := uint32(2) // .1 is the bridge gateway
	maxOffset := hostCount - 2
	base := ipToUint32(baseIP)

	ips := make([]string, 0, maxOffset-startOffset+1)
	for offset := startOffset; offset <= maxOffset; offset++ {
		ips = append(ips, uint32ToIP(base+offset))
	}
	m.ipPool.fill(ips)
	return nil
}

func (m *Manager) allocateCID() (uint32, error) {
	cid, ok := m.cidPool.acquire()
	if !ok {
		return 0, fmt.Errorf("no available vsock CIDs")
	}
	return cid, nil
}

// allocateIP returns next available IP in subnet (e.g., "172.30.0.2")
func (m *Manager) allocateIP() (string, error) {
	ip, ok := m.ipPool.acquire()
	if !ok {
		return "", fmt.Errorf("no available IPs in subnet")
	}
	return ip, nil
}

func (m *Manager) releaseCID(cid uint32) {
	if cid == 0 {
		return
	}
	m.cidPool.release(cid)
}

func (m *Manager) releaseIP(ip string) {
	if ip == "" {
		return
	}
	m.ipPool.release(ip)
}

func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func uint32ToIP(value uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
}

// generateMAC creates a locally-administered MAC address from VM ID
func generateMAC(vmID string) string {
	// Use VM ID hash for last 3 bytes, prefix with 02:FC:00 (locally administered)
	h := 0
	for _, c := range vmID {
		h = h*31 + int(c)
	}
	return fmt.Sprintf("02:FC:00:%02X:%02X:%02X", (h>>16)&0xFF, (h>>8)&0xFF, h&0xFF)
}

// ensureBridge creates the network bridge if it doesn't exist
func (m *Manager) ensureBridge() error {
	if m.bridgeReady.Load() {
		return nil
	}
	m.bridgeMu.Lock()
	defer m.bridgeMu.Unlock()
	if m.bridgeReady.Load() {
		return nil
	}

	bridge := m.config.BridgeName
	// Parse gateway IP from subnet (e.g., "172.30.0.0/24" -> "172.30.0.1/24")
	parts := strings.Split(m.config.Subnet, "/")
	baseIP := strings.TrimSuffix(parts[0], ".0")
	gatewayIP := baseIP + ".1"
	cidr := "24"
	if len(parts) > 1 {
		cidr = parts[1]
	}

	// Check if bridge exists
	if _, err := exec.Command("ip", "link", "show", bridge).Output(); err != nil {
		// Create bridge
		if out, err := exec.Command("ip", "link", "add", bridge, "type", "bridge").CombinedOutput(); err != nil {
			return fmt.Errorf("create bridge: %s: %w", out, err)
		}
	}

	// Set bridge IP
	exec.Command("ip", "addr", "flush", "dev", bridge).Run()
	if out, err := exec.Command("ip", "addr", "add", gatewayIP+"/"+cidr, "dev", bridge).CombinedOutput(); err != nil {
		// Ignore "already exists" error
		if !strings.Contains(string(out), "RTNETLINK answers: File exists") {
			return fmt.Errorf("set bridge ip: %s: %w", out, err)
		}
	}

	// Bring up bridge
	if out, err := exec.Command("ip", "link", "set", bridge, "up").CombinedOutput(); err != nil {
		return fmt.Errorf("bring up bridge: %s: %w", out, err)
	}

	// Enable IP forwarding
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644); err != nil {
		return fmt.Errorf("enable ip forwarding: %w", err)
	}

	// Setup NAT (masquerade) for outbound traffic
	if err := exec.Command("iptables", "-t", "nat", "-C", "POSTROUTING", "-s", m.config.Subnet, "-j", "MASQUERADE").Run(); err != nil {
		if out, err := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", m.config.Subnet, "-j", "MASQUERADE").CombinedOutput(); err != nil {
			return fmt.Errorf("setup NAT: %s: %w", out, err)
		}
	}

	m.bridgeReady.Store(true)
	return nil
}

// createTAP creates a TAP device and attaches it to the bridge
func (m *Manager) createTAP(vmID string) (string, error) {
	tap := "nova-" + vmID[:6]

	// Create TAP device
	if out, err := exec.Command("ip", "tuntap", "add", tap, "mode", "tap").CombinedOutput(); err != nil {
		return "", fmt.Errorf("create tap: %s: %w", out, err)
	}

	// Attach to bridge
	if out, err := exec.Command("ip", "link", "set", tap, "master", m.config.BridgeName).CombinedOutput(); err != nil {
		exec.Command("ip", "link", "del", tap).Run()
		return "", fmt.Errorf("attach tap to bridge: %s: %w", out, err)
	}

	// Bring up TAP
	if out, err := exec.Command("ip", "link", "set", tap, "up").CombinedOutput(); err != nil {
		exec.Command("ip", "link", "del", tap).Run()
		return "", fmt.Errorf("bring up tap: %s: %w", out, err)
	}

	return tap, nil
}

// deleteTAP removes a TAP device
func deleteTAP(tap string) {
	if tap != "" {
		exec.Command("ip", "link", "del", tap).Run()
	}
}
