package firecracker

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

func (m *Manager) allocateCID() (uint32, error) {
	m.cidMu.Lock()
	defer m.cidMu.Unlock()
	for i := 0; i < 1<<16; i++ {
		cid := m.nextCID
		m.nextCID++
		if m.nextCID == 0 {
			m.nextCID = 100
		}
		if _, ok := m.usedCIDs[cid]; ok {
			continue
		}
		m.usedCIDs[cid] = struct{}{}
		return cid, nil
	}
	return 0, fmt.Errorf("no available vsock CIDs")
}

// allocateIP returns next available IP in subnet (e.g., "172.30.0.2")
func (m *Manager) allocateIP() (string, error) {
	m.ipMu.Lock()
	defer m.ipMu.Unlock()
	baseIP, ipNet, err := net.ParseCIDR(m.config.Subnet)
	if err != nil {
		return "", fmt.Errorf("parse subnet: %w", err)
	}
	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return "", fmt.Errorf("unsupported subnet mask: %d", bits)
	}
	hostCount := uint32(1) << uint32(32-ones)
	if hostCount <= 3 {
		return "", fmt.Errorf("subnet too small for VM allocation")
	}
	startOffset := uint32(2)
	maxOffset := hostCount - 2

	base := ipToUint32(baseIP)
	for i := uint32(0); i < maxOffset-startOffset+1; i++ {
		offset := m.nextIP
		if offset < startOffset || offset > maxOffset {
			offset = startOffset
		}
		candidate := uint32ToIP(base + offset)
		m.nextIP = offset + 1
		if m.nextIP > maxOffset {
			m.nextIP = startOffset
		}
		if _, ok := m.usedIPs[candidate]; ok {
			continue
		}
		m.usedIPs[candidate] = struct{}{}
		return candidate, nil
	}
	return "", fmt.Errorf("no available IPs in subnet")
}

func (m *Manager) releaseCID(cid uint32) {
	if cid == 0 {
		return
	}
	m.cidMu.Lock()
	delete(m.usedCIDs, cid)
	m.cidMu.Unlock()
}

func (m *Manager) releaseIP(ip string) {
	if ip == "" {
		return
	}
	m.ipMu.Lock()
	delete(m.usedIPs, ip)
	m.ipMu.Unlock()
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
