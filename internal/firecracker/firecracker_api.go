package firecracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/domain"
)

// buildRateLimiter creates a Firecracker rate limiter config
func buildRateLimiter(bandwidth, ops int64) map[string]interface{} {
	limiter := make(map[string]interface{})
	if bandwidth > 0 {
		limiter["bandwidth"] = map[string]interface{}{
			"size":           bandwidth,
			"refill_time":    1000, // 1 second in ms
			"one_time_burst": 0,
		}
	}
	if ops > 0 {
		limiter["ops"] = map[string]interface{}{
			"size":           ops,
			"refill_time":    1000,
			"one_time_burst": 0,
		}
	}
	return limiter
}

// apiBoot configures and boots the VM via API
func (m *Manager) apiBoot(ctx context.Context, vm *VM, rootfs, codeDrive string, fn *domain.Function) error {
	mem := fn.MemoryMB
	if mem <= 0 {
		mem = 128
	}
	vcpus := 1
	if fn.Limits != nil && fn.Limits.VCPUs > 0 {
		vcpus = fn.Limits.VCPUs
	}

	// Parse gateway IP for boot args
	parts := strings.Split(m.config.Subnet, "/")
	baseIP := strings.TrimSuffix(parts[0], ".0")
	gatewayIP := baseIP + ".1"

	// 0. Logger (configure early for debugging)
	logPath := filepath.Join(m.config.LogDir, vm.ID+"-fc.log")
	_ = m.apiCall(ctx, vm, "PUT", "/logger", map[string]interface{}{
		"log_path": logPath,
		"level":    m.config.LogLevel,
	})

	// 1. Boot Source - add IP config via kernel cmdline
	netmask, err := netmaskFromCIDR(m.config.Subnet)
	if err != nil {
		return fmt.Errorf("parse subnet: %w", err)
	}
	bootArgs := fmt.Sprintf(
		"console=ttyS0 reboot=k panic=1 pci=off init=/init quiet 8250.nr_uarts=0 ip=%s::%s:%s::eth0:off",
		vm.GuestIP, gatewayIP, netmask,
	)
	bs := map[string]interface{}{
		"kernel_image_path": m.config.KernelPath,
		"boot_args":         bootArgs,
	}
	if err := m.apiCall(ctx, vm, "PUT", "/boot-source", bs); err != nil {
		return fmt.Errorf("boot-source: %w", err)
	}

	// 2. Drives with optional rate limiting and async IO
	root := map[string]interface{}{
		"drive_id":       "rootfs",
		"path_on_host":   rootfs,
		"is_root_device": true,
		"is_read_only":   true,
		"io_engine":      "Async",
	}
	if err := m.apiCall(ctx, vm, "PUT", "/drives/rootfs", root); err != nil {
		return fmt.Errorf("drive rootfs: %w", err)
	}

	code := map[string]interface{}{
		"drive_id":       "code",
		"path_on_host":   codeDrive,
		"is_root_device": false,
		"is_read_only":   true,
		"io_engine":      "Async",
	}
	if fn.Limits != nil && (fn.Limits.DiskIOPS > 0 || fn.Limits.DiskBandwidth > 0) {
		code["rate_limiter"] = buildRateLimiter(fn.Limits.DiskBandwidth, fn.Limits.DiskIOPS)
	}
	if err := m.apiCall(ctx, vm, "PUT", "/drives/code", code); err != nil {
		return fmt.Errorf("drive code: %w", err)
	}

	// Layer drives (read-only shared dependency images)
	for i, layerPath := range fn.LayerPaths {
		driveID := fmt.Sprintf("layer%d", i)
		layerDrive := map[string]interface{}{
			"drive_id":       driveID,
			"path_on_host":   layerPath,
			"is_root_device": false,
			"is_read_only":   true,
			"io_engine":      "Async",
		}
		if err := m.apiCall(ctx, vm, "PUT", "/drives/"+driveID, layerDrive); err != nil {
			return fmt.Errorf("drive %s: %w", driveID, err)
		}
	}

	// Volume drives (persistent storage attached after layers)
	for i, rm := range fn.ResolvedMounts {
		driveID := fmt.Sprintf("vol%d", i)
		volDrive := map[string]interface{}{
			"drive_id":       driveID,
			"path_on_host":   rm.ImagePath,
			"is_root_device": false,
			"is_read_only":   rm.ReadOnly,
			"io_engine":      "Async",
		}
		if err := m.apiCall(ctx, vm, "PUT", "/drives/"+driveID, volDrive); err != nil {
			return fmt.Errorf("drive %s: %w", driveID, err)
		}
	}

	// 3. Network interface
	netIface := map[string]interface{}{
		"iface_id":      "eth0",
		"guest_mac":     vm.GuestMAC,
		"host_dev_name": vm.TapDevice,
	}
	// Apply network rate limiter if configured
	if fn.Limits != nil && (fn.Limits.NetRxBandwidth > 0 || fn.Limits.NetTxBandwidth > 0) {
		if fn.Limits.NetRxBandwidth > 0 {
			netIface["rx_rate_limiter"] = buildRateLimiter(fn.Limits.NetRxBandwidth, 0)
		}
		if fn.Limits.NetTxBandwidth > 0 {
			netIface["tx_rate_limiter"] = buildRateLimiter(fn.Limits.NetTxBandwidth, 0)
		}
	}
	if err := m.apiCall(ctx, vm, "PUT", "/network-interfaces/eth0", netIface); err != nil {
		return fmt.Errorf("network interface: %w", err)
	}

	// 4. Vsock
	vs := map[string]interface{}{
		"guest_cid": vm.CID,
		"uds_path":  vm.VsockPath,
	}
	if err := m.apiCall(ctx, vm, "PUT", "/vsock", vs); err != nil {
		return fmt.Errorf("vsock: %w", err)
	}

	// 5. Machine Config
	mc := map[string]interface{}{
		"vcpu_count":   vcpus,
		"mem_size_mib": mem,
	}
	if err := m.apiCall(ctx, vm, "PUT", "/machine-config", mc); err != nil {
		return fmt.Errorf("machine-config: %w", err)
	}

	// 6. Action: InstanceStart
	if err := m.apiCall(ctx, vm, "PUT", "/actions", map[string]interface{}{"action_type": "InstanceStart"}); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	return nil
}

func (m *Manager) apiLoadSnapshot(ctx context.Context, vm *VM, snapPath, memPath, funcID string, originalCID uint32) error {
	// Per Firecracker docs (v1.12+), only Logger and Metrics may be configured
	// before snapshot/load. All other resources (vsock, drives, network) are
	// restored from the snapshot state.
	//
	// The vsock UDS path is restored to the path used when the snapshot was
	// created, so we read the saved metadata and update vm.VsockPath accordingly.
	// Network TAP devices can be overridden via the network_overrides field
	// added in Firecracker v1.12.

	// Load snapshot metadata to get the original vsock path
	metaPath := filepath.Join(m.config.SnapshotDir, funcID+".meta")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("read snapshot metadata: %w", err)
	}
	var meta snapshotMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return fmt.Errorf("parse snapshot metadata: %w", err)
	}

	// Validate snapshot has required metadata (CodeDrive was added in a later version)
	if meta.CodeDrive == "" {
		return fmt.Errorf("snapshot metadata missing code_drive field (created by older version)")
	}

	// Reserve the snapshot's CID to prevent conflicts.
	// If the CID is already in use by another VM, the snapshot cannot be loaded.
	// Note: We don't release the original CID here - the caller handles CID cleanup.
	m.cidMu.Lock()
	if _, inUse := m.usedCIDs[meta.VsockCID]; inUse && meta.VsockCID != originalCID {
		m.cidMu.Unlock()
		return fmt.Errorf("snapshot CID %d is already in use", meta.VsockCID)
	}
	if meta.VsockCID != originalCID {
		m.usedCIDs[meta.VsockCID] = struct{}{}
	}
	m.cidMu.Unlock()

	// Clean up stale vsock socket at the original path and update VM
	_ = os.Remove(meta.VsockPath)
	vm.VsockPath = meta.VsockPath
	vm.CID = meta.VsockCID

	// Restore guest IP/MAC from snapshot metadata to avoid conflicts.
	// The snapshot was created with a specific IP baked into the kernel boot args,
	// so the restored VM will use that IP regardless of what we allocated.
	if meta.GuestIP != "" {
		newIP := vm.GuestIP
		m.ipMu.Lock()
		// Reserve the snapshot's IP
		m.usedIPs[meta.GuestIP] = struct{}{}
		// Release the newly allocated IP (unless it's the same)
		if newIP != meta.GuestIP {
			delete(m.usedIPs, newIP)
		}
		m.ipMu.Unlock()
		vm.GuestIP = meta.GuestIP
	}
	if meta.GuestMAC != "" {
		vm.GuestMAC = meta.GuestMAC
	}

	// Handle code drive: Firecracker expects the drive at meta.CodeDrive (the
	// path recorded when the snapshot was created, typically under /tmp).
	// If it's been cleaned up, restore from the persistent backup in SnapshotDir,
	// or fall back to the newly-built code drive.
	originalCodeDrive := vm.CodeDrive // newly created code drive
	if _, err := os.Stat(meta.CodeDrive); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(meta.CodeDrive), 0755); err != nil {
			// Clean up reserved resources on failure
			if meta.VsockCID != originalCID {
				m.releaseCID(meta.VsockCID)
			}
			vm.CID = originalCID
			return fmt.Errorf("create snapshot code drive dir: %w", err)
		}
		// Prefer restoring from persistent backup (identical to the original)
		restoreSource := originalCodeDrive
		if meta.CodeDriveBackup != "" {
			if _, err := os.Stat(meta.CodeDriveBackup); err == nil {
				restoreSource = meta.CodeDriveBackup
			}
		}
		if err := copyFile(restoreSource, meta.CodeDrive); err != nil {
			// Clean up reserved resources on failure
			if meta.VsockCID != originalCID {
				m.releaseCID(meta.VsockCID)
			}
			vm.CID = originalCID
			return fmt.Errorf("restore snapshot code drive backing file: %w", err)
		}
	}
	// Don't update vm.CodeDrive or delete originalCodeDrive yet —
	// wait until the Firecracker API call succeeds. If it fails, the
	// caller can fall back to fresh boot using the original code drive.

	// Configure logger before snapshot load (allowed per API)
	logPath := filepath.Join(m.config.LogDir, vm.ID+"-fc.log")
	_ = m.apiCall(ctx, vm, "PUT", "/logger", map[string]interface{}{
		"log_path": logPath,
		"level":    m.config.LogLevel,
	})

	req := map[string]interface{}{
		"snapshot_path": snapPath,
		"mem_backend": map[string]interface{}{
			"backend_type": "File",
			"backend_path": memPath,
		},
		"resume_vm": true,
	}

	// Use network_overrides (v1.12+) to rebind the restored network interface
	// to the new TAP device created for this VM.
	if vm.TapDevice != "" {
		req["network_overrides"] = []map[string]interface{}{
			{
				"iface_id":      "eth0",
				"host_dev_name": vm.TapDevice,
			},
		}
	}

	if err := m.apiCall(ctx, vm, "PUT", "/snapshot/load", req); err != nil {
		// Clean up reserved resources on failure
		if meta.VsockCID != originalCID {
			m.releaseCID(meta.VsockCID)
		}
		vm.CID = originalCID // restore original CID for caller cleanup
		// Restore vm.CodeDrive so caller can fall back to fresh boot
		vm.CodeDrive = originalCodeDrive
		vm.PreserveCodeDrive = false
		return fmt.Errorf("load snapshot: %w", err)
	}

	// Snapshot loaded successfully
	vm.CodeDrive = meta.CodeDrive
	vm.PreserveCodeDrive = true

	// Clean up the newly created code drive since we're using the snapshot's
	if originalCodeDrive != meta.CodeDrive {
		os.Remove(originalCodeDrive)
	}

	// Release the original CID since we're now using the snapshot's CID
	if originalCID != meta.VsockCID {
		m.releaseCID(originalCID)
	}

	return nil
}

// httpClientForSocket returns a cached HTTP client that dials the given Unix socket.
// Each unique socket path gets its own client with connection pooling.
var (
	socketClients   = make(map[string]*http.Client)
	socketClientsMu sync.Mutex
)

func httpClientForSocket(socketPath string) *http.Client {
	socketClientsMu.Lock()
	defer socketClientsMu.Unlock()

	if c, ok := socketClients[socketPath]; ok {
		return c
	}
	c := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
			MaxIdleConns:        2,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     30 * time.Second,
		},
	}
	socketClients[socketPath] = c
	return c
}

func removeSocketClient(socketPath string) {
	socketClientsMu.Lock()
	defer socketClientsMu.Unlock()
	if c, ok := socketClients[socketPath]; ok {
		c.CloseIdleConnections()
		delete(socketClients, socketPath)
	}
}

func (m *Manager) apiCall(ctx context.Context, vm *VM, method, path string, body interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://localhost"+path, bodyReader)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := httpClientForSocket(vm.SocketPath)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api error %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (m *Manager) waitForSocket(ctx context.Context, path string, proc *os.Process, timeout time.Duration) error {
	deadline, _ := ctx.Deadline()
	if deadline.IsZero() {
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		deadline = time.Now().Add(timeout)
	}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if proc != nil {
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				return fmt.Errorf("firecracker exited before socket ready: %w", err)
			}
		}
		if _, err := os.Stat(path); err == nil {
			conn, err := net.Dial("unix", path)
			if err == nil {
				conn.Close()
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("socket timeout")
}

func netmaskFromCIDR(subnet string) (string, error) {
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return "", err
	}
	mask := ipNet.Mask
	if len(mask) != 4 {
		return "", fmt.Errorf("unexpected netmask length: %d", len(mask))
	}
	return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3]), nil
}
