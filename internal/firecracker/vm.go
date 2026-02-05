package firecracker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
)

type VMState string

const (
	VMStateCreating VMState = "creating"
	VMStateRunning  VMState = "running"
	VMStatePaused   VMState = "paused"
	VMStateStopped  VMState = "stopped"

	// Fixed path inside VM where function code lives
	GuestCodeDir  = "/code"
	GuestCodePath = "/code/handler"

	// Default code drive size for template (16MB, suitable for most functions)
	defaultCodeDriveSizeMB = 16

	// Minimum code drive size (4MB) for small functions
	minCodeDriveSizeMB = 4

	// Ext4 overhead factor - actual usable space is ~85% of drive size
	ext4OverheadFactor = 0.85

	// Default vsock port used by the guest agent (must match cmd/agent)
	defaultVsockPort = 9999

	// Maximum vsock message size to protect against oversized responses.
	maxVsockMessageBytes = 8 * 1024 * 1024 // 8MB
)

type Config struct {
	Backend        string // "firecracker" or "docker"
	FirecrackerBin string
	KernelPath     string
	RootfsDir      string
	SnapshotDir    string
	SocketDir      string
	VsockDir       string
	LogDir         string
	BridgeName     string
	Subnet         string
	BootTimeout    time.Duration
	LogLevel       string // Firecracker log level: Error, Warning, Info, Debug
}

// NovaDir is the base installation directory for nova
const NovaDir = "/opt/nova"

func DefaultConfig() *Config {
	backend := "firecracker"
	if v := os.Getenv("NOVA_BACKEND"); v != "" {
		backend = v
	}
	return &Config{
		Backend:        backend,
		FirecrackerBin: NovaDir + "/bin/firecracker",
		KernelPath:     NovaDir + "/kernel/vmlinux",
		RootfsDir:      NovaDir + "/rootfs",
		SnapshotDir:    NovaDir + "/snapshots",
		SocketDir:      "/tmp/nova/sockets",
		VsockDir:       "/tmp/nova/vsock",
		LogDir:         "/tmp/nova/logs",
		BridgeName:     "novabr0",
		Subnet:         "172.30.0.0/24",
		BootTimeout:    10 * time.Second,
		LogLevel:       "Warning",
	}
}

type VM struct {
	ID                string
	Runtime           domain.Runtime
	State             VMState
	CID               uint32
	SocketPath        string
	VsockPath         string
	CodeDrive         string // path to per-VM code drive
	TapDevice         string // TAP device name (e.g., "nova-abc123")
	GuestIP           string // IP assigned to guest (e.g., "172.30.0.2")
	GuestMAC          string // MAC address for guest
	Cmd               *exec.Cmd
	DockerContainerID string // For Docker backend
	AssignedPort      int    // For Docker backend (host port mapped to agent)
	CreatedAt         time.Time
	LastUsed          time.Time
	mu                sync.RWMutex
}

type Manager struct {
	config        *Config
	vms           map[string]*VM
	mu            sync.RWMutex
	nextCID       uint32
	nextIP        uint32 // last octet for IP allocation
	cidMu         sync.Mutex
	ipMu          sync.Mutex
	usedCIDs      map[uint32]struct{}
	usedIPs       map[string]struct{}
	templateReady atomic.Bool
	templateMu    sync.Mutex
	bridgeReady   atomic.Bool
	bridgeMu      sync.Mutex
	httpClient    *http.Client
}

func NewManager(cfg *Config) (*Manager, error) {
	for _, dir := range []string{cfg.SocketDir, cfg.VsockDir, cfg.LogDir, cfg.SnapshotDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	return &Manager{
		config:   cfg,
		vms:      make(map[string]*VM),
		nextCID:  100,
		nextIP:   2, // Start from .2 (.1 is bridge)
		usedCIDs: make(map[uint32]struct{}),
		usedIPs:  make(map[string]struct{}),
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, addr string) (net.Conn, error) {
					return net.Dial("unix", addr)
				},
			},
		},
	}, nil
}

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

// CreateVM boots a microVM for the given function.
// Checks for existing snapshot first.
func (m *Manager) CreateVM(ctx context.Context, fn *domain.Function, codeContent []byte) (*VM, error) {
	vmID := uuid.New().String()[:8]
	cid, err := m.allocateCID()
	if err != nil {
		return nil, err
	}
	cidAllocated := true

	vm := &VM{
		ID:         vmID,
		Runtime:    fn.Runtime,
		State:      VMStateCreating,
		CID:        cid,
		SocketPath: filepath.Join(m.config.SocketDir, vmID+".sock"),
		VsockPath:  filepath.Join(m.config.VsockDir, vmID+".vsock"),
		CreatedAt:  time.Now(),
		LastUsed:   time.Now(),
	}
	defer func() {
		if vm.State == VMStateStopped {
			if cidAllocated {
				m.releaseCID(cid)
			}
			m.releaseIP(vm.GuestIP)
		}
	}()

	// Clean up any stale sockets before starting Firecracker.
	_ = os.Remove(vm.SocketPath)
	_ = os.Remove(vm.VsockPath)

	// Prepare resources
	rootfsPath := filepath.Join(m.config.RootfsDir, rootfsForRuntime(fn.Runtime))
	if _, err := os.Stat(rootfsPath); os.IsNotExist(err) {
		vm.State = VMStateStopped
		return nil, fmt.Errorf("rootfs not found: %s", rootfsPath)
	}

	codeDrive := filepath.Join(m.config.SocketDir, vmID+"-code.ext4")
	if err := m.buildCodeDrive(codeDrive, codeContent); err != nil {
		vm.State = VMStateStopped
		return nil, fmt.Errorf("build code drive: %w", err)
	}
	vm.CodeDrive = codeDrive

	// Setup network
	if err := m.ensureBridge(); err != nil {
		vm.State = VMStateStopped
		return nil, fmt.Errorf("ensure bridge: %w", err)
	}
	tap, err := m.createTAP(vmID)
	if err != nil {
		vm.State = VMStateStopped
		return nil, fmt.Errorf("create tap: %w", err)
	}
	vm.TapDevice = tap
	ip, err := m.allocateIP()
	if err != nil {
		vm.State = VMStateStopped
		deleteTAP(vm.TapDevice)
		return nil, err
	}
	vm.GuestIP = ip
	vm.GuestMAC = generateMAC(vmID)

	// Check for snapshot
	snapshotPath := filepath.Join(m.config.SnapshotDir, fn.ID+".snap")
	memPath := filepath.Join(m.config.SnapshotDir, fn.ID+".mem")
	useSnapshot := false
	if _, err := os.Stat(snapshotPath); err == nil {
		if _, err := os.Stat(memPath); err == nil {
			useSnapshot = true
		}
	}

	// Start Firecracker process
	logFile, err := os.Create(filepath.Join(m.config.LogDir, vmID+".log"))
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	// Note: We don't pass --config-file if loading from snapshot,
	// or we pass a minimal one. For simplicity, we start without config
	// and use API to configure/load.
	// Use exec.Command (not CommandContext) so the process survives beyond
	// the HTTP request that created it.
	cmd := exec.Command(m.config.FirecrackerBin,
		"--api-sock", vm.SocketPath,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		deleteTAP(vm.TapDevice)
		vm.State = VMStateStopped
		return nil, fmt.Errorf("start firecracker: %w", err)
	}
	if err := logFile.Close(); err != nil {
		m.StopVM(vm.ID)
		return nil, fmt.Errorf("close log file: %w", err)
	}
	vm.Cmd = cmd

	// Wait for API socket
	if err := m.waitForSocket(ctx, vm.SocketPath, cmd.Process, m.config.BootTimeout); err != nil {
		m.StopVM(vm.ID) // cleanup
		return nil, fmt.Errorf("wait api socket: %w", err)
	}

	if useSnapshot {
		// Load Snapshot (pass funcID for metadata lookup)
		err = m.apiLoadSnapshot(ctx, vm, snapshotPath, memPath, fn.ID)
	} else {
		// Regular Boot
		err = m.apiBoot(ctx, vm, rootfsPath, codeDrive, fn)
	}

	if err != nil {
		m.StopVM(vm.ID)
		return nil, err
	}

	vm.State = VMStateRunning
	m.mu.Lock()
	m.vms[vm.ID] = vm
	m.mu.Unlock()

	// Record metrics
	metrics.Global().RecordVMCreated()
	if useSnapshot {
		metrics.Global().RecordSnapshotHit()
	}

	// Monitor the Firecracker process - clean up if it dies unexpectedly
	go m.monitorProcess(vm)

	if err := m.waitForVsock(ctx, vm); err != nil {
		m.StopVM(vm.ID)
		return nil, fmt.Errorf("wait vsock: %w", err)
	}

	return vm, nil
}

// snapshotMeta stores metadata needed for snapshot restore.
type snapshotMeta struct {
	VsockPath string `json:"vsock_path"`
	VsockCID  uint32 `json:"vsock_cid"`
}

// CreateSnapshot pauses the VM, creates snapshot files, and stops the VM.
func (m *Manager) CreateSnapshot(ctx context.Context, vmID string, funcID string) error {
	m.mu.RLock()
	vm, ok := m.vms[vmID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("vm not found")
	}

	// Pause
	if err := m.apiCall(ctx, vm, "PATCH", "/vm", map[string]interface{}{"state": "Paused"}); err != nil {
		return fmt.Errorf("pause vm: %w", err)
	}

	// Create Snapshot
	snapPath := filepath.Join(m.config.SnapshotDir, funcID+".snap")
	memPath := filepath.Join(m.config.SnapshotDir, funcID+".mem")

	req := map[string]interface{}{
		"snapshot_type": "Full",
		"snapshot_path": snapPath,
		"mem_file_path": memPath,
	}
	if err := m.apiCall(ctx, vm, "PUT", "/snapshot/create", req); err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}

	// Save metadata for snapshot restore (vsock path, CID, etc.)
	meta := snapshotMeta{
		VsockPath: vm.VsockPath,
		VsockCID:  vm.CID,
	}
	metaData, _ := json.Marshal(meta)
	metaPath := filepath.Join(m.config.SnapshotDir, funcID+".meta")
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		return errors.New("write snapshot metadata: " + err.Error())
	}

	return nil
}

// ResumeVM resumes a paused VM (e.g., after snapshot creation)
func (m *Manager) ResumeVM(ctx context.Context, vmID string) error {
	m.mu.RLock()
	vm, ok := m.vms[vmID]
	m.mu.RUnlock()
	if !ok {
		return errors.New("vm not found")
	}

	return m.apiCall(ctx, vm, "PATCH", "/vm", map[string]interface{}{"state": "Resumed"})
}

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

func (m *Manager) apiLoadSnapshot(ctx context.Context, vm *VM, snapPath, memPath, funcID string) error {
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

	// Clean up stale vsock socket at the original path and update VM
	_ = os.Remove(meta.VsockPath)
	vm.VsockPath = meta.VsockPath
	vm.CID = meta.VsockCID

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
		return fmt.Errorf("load snapshot: %w", err)
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

// buildCodeDrive creates an ext4 image and injects the function code at /handler.
// Uses a cached template image for small functions to avoid repeated mkfs calls.
// For larger functions, creates a custom-sized drive.
func (m *Manager) buildCodeDrive(drivePath string, codeContent []byte) error {
	// Get code size
	codeSizeMB := float64(len(codeContent)) / (1024 * 1024)

	// Calculate required drive size (code + ext4 overhead + buffer)
	requiredSizeMB := int(codeSizeMB/ext4OverheadFactor) + 2 // +2MB buffer for ext4 metadata

	// Determine if we can use the standard template
	useTemplate := requiredSizeMB <= defaultCodeDriveSizeMB
	var driveSizeMB int

	if useTemplate {
		// Use cached template for small functions
		templatePath := filepath.Join(m.config.SocketDir, "template-code.ext4")

		// Retryable template creation using atomic bool + mutex
		if !m.templateReady.Load() {
			m.templateMu.Lock()
			if !m.templateReady.Load() {
				if err := createTemplateDrive(templatePath, defaultCodeDriveSizeMB); err != nil {
					m.templateMu.Unlock()
					return err
				}
				m.templateReady.Store(true)
			}
			m.templateMu.Unlock()
		}

		// Buffered copy of template to new drive
		if err := copyFileBuffered(templatePath, drivePath); err != nil {
			return err
		}
		driveSizeMB = defaultCodeDriveSizeMB
	} else {
		// Create custom-sized drive for large functions
		driveSizeMB = requiredSizeMB
		if driveSizeMB < minCodeDriveSizeMB {
			driveSizeMB = minCodeDriveSizeMB
		}
		logging.Op().Info("creating custom code drive",
			"size_mb", driveSizeMB,
			"code_size_mb", codeSizeMB)
		if err := createTemplateDrive(drivePath, driveSizeMB); err != nil {
			return err
		}
	}

	// Write code content to a temp file for debugfs
	tmpFile, err := os.CreateTemp("", "nova-code-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(codeContent); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	// Inject function code using debugfs (no mount needed)
	debugfsCmd := fmt.Sprintf("write %s handler\nsif handler mode 0100755\n", tmpPath)
	cmd := exec.Command("debugfs", "-w", drivePath)
	cmd.Stdin = strings.NewReader(debugfsCmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("debugfs inject (drive=%dMB, code=%.1fMB): %s: %w", driveSizeMB, codeSizeMB, out, err)
	}

	return nil
}

func createTemplateDrive(path string, sizeMB int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := f.Truncate(int64(sizeMB) * 1024 * 1024); err != nil {
		f.Close()
		return err
	}
	f.Close()

	if out, err := exec.Command("mkfs.ext4", "-F", "-q", path).CombinedOutput(); err != nil {
		os.Remove(path)
		return fmt.Errorf("mkfs.ext4: %s: %w", out, err)
	}
	return nil
}

func copyFileBuffered(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, 256*1024) // 256KB buffer
	_, err = io.CopyBuffer(out, bufio.NewReaderSize(in, 256*1024), buf)
	return err
}

func (m *Manager) waitForVsock(ctx context.Context, vm *VM) error {
	deadline := time.Now().Add(m.config.BootTimeout)

	// Phase 1: Wait for socket file to be created using inotify
	socketDir := filepath.Dir(vm.VsockPath)
	socketName := filepath.Base(vm.VsockPath)

	// Check if socket already exists
	if _, err := os.Stat(vm.VsockPath); err != nil {
		// Socket doesn't exist, use inotify to wait for it
		if err := waitForFileInotify(ctx, socketDir, socketName, deadline); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			// Inotify failed, fall back to polling
			for time.Now().Before(deadline) {
				if _, err := os.Stat(vm.VsockPath); err == nil {
					break
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(50 * time.Millisecond):
				}
			}
		}
	}

	// Phase 2: Socket file exists, wait for it to be connectable
	// Use shorter intervals since socket file is already present
	var lastDialErr error
	for time.Now().Before(deadline) {
		if _, err := os.Stat(vm.VsockPath); err != nil {
			// Socket disappeared, wait for it again
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(20 * time.Millisecond):
			}
			continue
		}

		conn, err := dialVsock(vm, time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		lastDialErr = err

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond): // Faster polling once socket exists
		}
	}

	if lastDialErr != nil {
		return fmt.Errorf("vsock timeout: %w", lastDialErr)
	}
	return fmt.Errorf("vsock timeout: socket not created: %s", vm.VsockPath)
}

// monitorProcess watches a Firecracker process and cleans up if it dies unexpectedly.
func (m *Manager) monitorProcess(vm *VM) {
	if vm.Cmd == nil || vm.Cmd.Process == nil {
		return
	}

	// Wait for process to exit
	err := vm.Cmd.Wait()

	// Check if VM is still in our map (if not, it was intentionally stopped)
	m.mu.RLock()
	_, stillTracked := m.vms[vm.ID]
	m.mu.RUnlock()

	if stillTracked {
		// Process died unexpectedly - clean up
		exitCode := -1
		if vm.Cmd.ProcessState != nil {
			exitCode = vm.Cmd.ProcessState.ExitCode()
		}
		logging.Op().Error("VM died unexpectedly",
			"vm_id", vm.ID,
			"exit_code", exitCode,
			"error", err)

		// Record crash metric
		metrics.Global().RecordVMCrashed()

		// Remove from manager and clean up resources
		m.mu.Lock()
		delete(m.vms, vm.ID)
		m.mu.Unlock()

		// Clean up per-VM files
		removeSocketClient(vm.SocketPath)
		deleteTAP(vm.TapDevice)
		os.Remove(vm.SocketPath)
		os.Remove(vm.VsockPath)
		os.Remove(vm.CodeDrive)
		os.Remove(filepath.Join(m.config.SocketDir, vm.ID+".json"))
		m.releaseCID(vm.CID)
		m.releaseIP(vm.GuestIP)

		vm.State = VMStateStopped
	}
}

func (m *Manager) StopVM(vmID string) error {
	m.mu.Lock()
	vm, ok := m.vms[vmID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("vm not found: %s", vmID)
	}
	delete(m.vms, vmID)
	m.mu.Unlock()

	// Record metric
	metrics.Global().RecordVMStopped()

	vm.mu.Lock()
	defer vm.mu.Unlock()

	// 1. Try graceful shutdown via vsock
	if conn, err := dialVsock(vm, time.Second); err == nil {
		msg, _ := json.Marshal(&VsockMessage{Type: MsgTypeStop})
		lenBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(lenBuf, uint32(len(msg)))
		conn.Write(lenBuf)
		conn.Write(msg)
		conn.Close()
		// Give agent a moment to exit
		time.Sleep(200 * time.Millisecond)
	}

	if vm.Cmd != nil && vm.Cmd.Process != nil {
		// 2. SIGTERM
		syscall.Kill(-vm.Cmd.Process.Pid, syscall.SIGTERM)

		// Wait up to 2 seconds for clean exit
		done := make(chan struct{})
		go func() { vm.Cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			// 3. SIGKILL as last resort
			syscall.Kill(-vm.Cmd.Process.Pid, syscall.SIGKILL)
			vm.Cmd.Wait()
		}
	}

	// Cleanup per-VM files
	removeSocketClient(vm.SocketPath)
	deleteTAP(vm.TapDevice)
	os.Remove(vm.SocketPath)
	os.Remove(vm.VsockPath)
	os.Remove(vm.CodeDrive)
	os.Remove(filepath.Join(m.config.SocketDir, vm.ID+".json"))
	m.releaseCID(vm.CID)
	m.releaseIP(vm.GuestIP)

	vm.State = VMStateStopped
	return nil
}

func (m *Manager) GetVM(vmID string) (*VM, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	vm, ok := m.vms[vmID]
	return vm, ok
}

func (m *Manager) ListVMs() []*VM {
	m.mu.RLock()
	defer m.mu.RUnlock()
	vms := make([]*VM, 0, len(m.vms))
	for _, vm := range m.vms {
		vms = append(vms, vm)
	}
	return vms
}

func (m *Manager) Shutdown() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.vms))
	for id := range m.vms {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	// Stop all VMs in parallel
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(vmID string) {
			defer wg.Done()
			m.StopVM(vmID)
		}(id)
	}
	wg.Wait()
}

// SnapshotDir returns the directory where snapshots are stored.
func (m *Manager) SnapshotDir() string {
	return m.config.SnapshotDir
}

// rootfsForRuntime maps runtime to rootfs image.
// Go/Rust: static binaries, minimal base is enough.
// Python: needs interpreter. WASM: needs wasmtime.
// Node/Deno/Bun: need JS runtime. Ruby: needs interpreter. Java: needs JVM.
func rootfsForRuntime(rt domain.Runtime) string {
	r := string(rt)
	switch {
	case r == string(domain.RuntimePython) || strings.HasPrefix(r, "python"):
		return "python.ext4"
	case r == string(domain.RuntimeWasm) || strings.HasPrefix(r, "wasm"):
		return "wasm.ext4"
	case r == string(domain.RuntimeNode) || strings.HasPrefix(r, "node"):
		return "node.ext4"
	case r == string(domain.RuntimeRuby) || strings.HasPrefix(r, "ruby"):
		return "ruby.ext4"
	case r == string(domain.RuntimeJava) || strings.HasPrefix(r, "java"):
		return "java.ext4"
	case r == string(domain.RuntimePHP) || strings.HasPrefix(r, "php"):
		return "php.ext4"
	case r == string(domain.RuntimeLua) || strings.HasPrefix(r, "lua"):
		return "lua.ext4"
	case r == string(domain.RuntimeDotnet) || strings.HasPrefix(r, "dotnet"):
		return "dotnet.ext4"
	case r == string(domain.RuntimeDeno) || strings.HasPrefix(r, "deno"):
		return "deno.ext4"
	case r == string(domain.RuntimeBun) || strings.HasPrefix(r, "bun"):
		return "bun.ext4"
	default:
		// Go, Rust use base
		return "base.ext4"
	}
}

// ─── Vsock protocol ─────────────────────────────────────

const (
	MsgTypeInit = 1
	MsgTypeExec = 2
	MsgTypeResp = 3
	MsgTypePing = 4
	MsgTypeStop = 5
)

type VsockMessage struct {
	Type    int             `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type InitPayload struct {
	Runtime   string            `json:"runtime"`
	Handler   string            `json:"handler"`
	EnvVars   map[string]string `json:"env_vars"`
	Command   []string          `json:"command,omitempty"`
	Extension string            `json:"extension,omitempty"`
}

type ExecPayload struct {
	RequestID   string          `json:"request_id"`
	Input       json.RawMessage `json:"input"`
	TimeoutS    int             `json:"timeout_s"`
	TraceParent string          `json:"traceparent,omitempty"` // W3C TraceContext
	TraceState  string          `json:"tracestate,omitempty"`  // W3C TraceContext
}

type RespPayload struct {
	RequestID  string          `json:"request_id"`
	Output     json.RawMessage `json:"output"`
	Error      string          `json:"error,omitempty"`
	DurationMs int64           `json:"duration_ms"`
	Stdout     string          `json:"stdout,omitempty"` // Captured stdout
	Stderr     string          `json:"stderr,omitempty"` // Captured stderr
}

type VsockClient struct {
	vm          *VM
	conn        net.Conn
	mu          sync.Mutex
	initPayload json.RawMessage
}

func NewVsockClient(vm *VM) (*VsockClient, error) {
	// Dial on demand. In practice, the underlying UDS-backed vsock connection may
	// be short-lived; keeping a long-lived connection is error-prone.
	return &VsockClient{vm: vm}, nil
}

func (c *VsockClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *VsockClient) closeLocked() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *VsockClient) dialLocked(timeout time.Duration) error {
	start := time.Now()
	conn, err := dialVsock(c.vm, timeout)
	if err != nil {
		return err
	}
	metrics.RecordVsockLatency("connect", float64(time.Since(start).Microseconds())/1000.0)
	c.conn = conn
	return nil
}

func (c *VsockClient) initLocked() error {
	if c.initPayload == nil {
		return errors.New("missing init payload")
	}
	if err := c.sendLocked(&VsockMessage{Type: MsgTypeInit, Payload: c.initPayload}); err != nil {
		return err
	}
	resp, err := c.receiveLocked()
	if err != nil {
		return err
	}
	if resp.Type != MsgTypeResp {
		return fmt.Errorf("unexpected response type: %d", resp.Type)
	}
	return nil
}

func (c *VsockClient) redialAndInitLocked(timeout time.Duration) error {
	hadConn := c.conn != nil
	_ = c.closeLocked()
	// Small delay after closing to let the vsock proxy clean up.
	if hadConn {
		time.Sleep(10 * time.Millisecond)
	}
	if err := c.dialLocked(timeout); err != nil {
		return err
	}
	if c.initPayload != nil {
		if err := c.initLocked(); err != nil {
			_ = c.closeLocked()
			return err
		}
	}
	return nil
}

func (c *VsockClient) Send(msg *VsockMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sendLocked(msg)
}

func (c *VsockClient) sendLocked(msg *VsockMessage) error {
	if c.conn == nil {
		return errors.New("vsock not connected")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// Batch length prefix and data into single write to reduce syscalls
	buf := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(data)))
	copy(buf[4:], data)

	start := time.Now()
	err = writeFull(c.conn, buf)
	metrics.RecordVsockLatency("send", float64(time.Since(start).Microseconds())/1000.0)
	return err
}

func (c *VsockClient) Receive() (*VsockMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.receiveLocked()
}

func (c *VsockClient) receiveLocked() (*VsockMessage, error) {
	if c.conn == nil {
		return nil, errors.New("vsock not connected")
	}

	start := time.Now()

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, lenBuf); err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint32(lenBuf)
	if msgLen > maxVsockMessageBytes {
		return nil, fmt.Errorf("vsock message too large: %d bytes", msgLen)
	}
	data := make([]byte, msgLen)
	if _, err := io.ReadFull(c.conn, data); err != nil {
		return nil, err
	}

	metrics.RecordVsockLatency("receive", float64(time.Since(start).Microseconds())/1000.0)

	var msg VsockMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (c *VsockClient) Init(fn *domain.Function) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, _ := json.Marshal(&InitPayload{
		Runtime:   string(fn.Runtime),
		Handler:   fn.Handler,
		EnvVars:   fn.EnvVars,
		Command:   fn.RuntimeCommand,
		Extension: fn.RuntimeExtension,
	})
	c.initPayload = payload
	if err := c.redialAndInitLocked(5 * time.Second); err != nil {
		return err
	}
	// Close connection after init. Execute() will establish a fresh connection.
	return c.closeLocked()
}

func (c *VsockClient) Execute(reqID string, input json.RawMessage, timeoutS int) (*RespPayload, error) {
	return c.ExecuteWithTrace(reqID, input, timeoutS, "", "")
}

// ExecuteWithTrace executes a request with optional W3C trace context propagation
func (c *VsockClient) ExecuteWithTrace(reqID string, input json.RawMessage, timeoutS int, traceParent, traceState string) (*RespPayload, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, _ := json.Marshal(&ExecPayload{
		RequestID:   reqID,
		Input:       input,
		TimeoutS:    timeoutS,
		TraceParent: traceParent,
		TraceState:  traceState,
	})

	execMsg := &VsockMessage{Type: MsgTypeExec, Payload: payload}

	// Exponential backoff: 10ms, 25ms, 50ms
	backoff := []time.Duration{10 * time.Millisecond, 25 * time.Millisecond, 50 * time.Millisecond}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := c.redialAndInitLocked(5 * time.Second); err != nil {
			lastErr = err
			if attempt < 2 {
				time.Sleep(backoff[attempt])
			}
			continue
		}

		deadline := time.Now().Add(time.Duration(timeoutS+5) * time.Second)
		_ = c.conn.SetDeadline(deadline)

		if err := c.sendLocked(execMsg); err != nil {
			lastErr = err
			_ = c.closeLocked()
			if isBrokenConnErr(err) && attempt < 2 {
				time.Sleep(backoff[attempt])
				continue
			}
			return nil, err
		}

		resp, err := c.receiveLocked()
		_ = c.conn.SetDeadline(time.Time{})
		if err != nil {
			lastErr = err
			_ = c.closeLocked()
			if isBrokenConnErr(err) && attempt < 2 {
				time.Sleep(backoff[attempt])
				continue
			}
			return nil, err
		}

		var result RespPayload
		if err := json.Unmarshal(resp.Payload, &result); err != nil {
			_ = c.closeLocked()
			return nil, err
		}

		_ = c.closeLocked()
		return &result, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("vsock execute failed")
}

func (c *VsockClient) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.redialAndInitLocked(5 * time.Second); err != nil {
		return err
	}
	defer c.closeLocked()

	_ = c.conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err := c.sendLocked(&VsockMessage{Type: MsgTypePing}); err != nil {
		return err
	}
	_, err := c.receiveLocked()
	_ = c.conn.SetDeadline(time.Time{})
	return err
}

func dialVsock(vm *VM, timeout time.Duration) (net.Conn, error) {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.Dial("unix", vm.VsockPath)
	if err != nil {
		return nil, err
	}
	if err := sendVsockConnect(conn, defaultVsockPort, timeout); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func sendVsockConnect(conn net.Conn, port int, timeout time.Duration) error {
	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}
	if _, err := fmt.Fprintf(conn, "CONNECT %d\n", port); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(line, "OK") {
		return fmt.Errorf("vsock connect failed: %s", strings.TrimSpace(line))
	}
	if timeout > 0 {
		_ = conn.SetDeadline(time.Time{})
	}
	return nil
}

func writeFull(conn net.Conn, b []byte) error {
	for len(b) > 0 {
		n, err := conn.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

func isBrokenConnErr(err error) bool {
	return err != nil && (errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.ENOTCONN))
}
