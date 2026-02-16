package firecracker

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	Backend             string // "firecracker", "docker", or "wasm"
	FirecrackerBin      string
	KernelPath          string
	RootfsDir           string
	SnapshotDir         string
	SocketDir           string
	VsockDir            string
	LogDir              string
	BridgeName          string
	Subnet              string
	BootTimeout         time.Duration
	LogLevel            string // Firecracker log level: Error, Warning, Info, Debug
	CodeDriveSizeMB     int    // Default code drive size in MB (default: 16)
	MinCodeDriveSizeMB  int    // Minimum code drive size in MB (default: 4)
	VsockPort           int    // Vsock port for guest agent (default: 9999)
	MaxVsockMessageMB   int    // Maximum vsock message size in MB (default: 8)
}

// NovaDir is the base installation directory for nova
const NovaDir = "/opt/nova"

func DefaultConfig() *Config {
	backend := "firecracker"
	if v := os.Getenv("NOVA_BACKEND"); v != "" {
		backend = v
	}
	return &Config{
		Backend:            backend,
		FirecrackerBin:     NovaDir + "/bin/firecracker",
		KernelPath:         NovaDir + "/kernel/vmlinux",
		RootfsDir:          NovaDir + "/rootfs",
		SnapshotDir:        NovaDir + "/snapshots",
		SocketDir:          "/tmp/nova/sockets",
		VsockDir:           "/tmp/nova/vsock",
		LogDir:             "/tmp/nova/logs",
		BridgeName:         "novabr0",
		Subnet:             "172.30.0.0/24",
		BootTimeout:        10 * time.Second,
		LogLevel:           "Warning",
		CodeDriveSizeMB:    defaultCodeDriveSizeMB,
		MinCodeDriveSizeMB: minCodeDriveSizeMB,
		VsockPort:          defaultVsockPort,
		MaxVsockMessageMB:  8,
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
	PreserveCodeDrive bool   // whether code drive should survive VM stop (needed for snapshot restore)
	TapDevice         string // TAP device name (e.g., "nova-abc123")
	GuestIP           string // IP assigned to guest (e.g., "172.30.0.2")
	GuestMAC          string // MAC address for guest
	NetNS             string // Network namespace name (empty = no netns isolation)
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
	var rootfsFile string
	if fn.RuntimeImageName != "" {
		rootfsFile = fn.RuntimeImageName
	} else {
		rootfsFile = rootfsForRuntime(fn.Runtime)
	}
	rootfsPath := filepath.Join(m.config.RootfsDir, rootfsFile)
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

	useNetNS := fn.NetworkPolicy != nil && fn.NetworkPolicy.IsolationMode != "" && fn.NetworkPolicy.IsolationMode != "none"
	if useNetNS {
		ip, err := m.allocateIP()
		if err != nil {
			vm.State = VMStateStopped
			return nil, err
		}
		vm.GuestIP = ip
		vm.GuestMAC = generateMAC(vmID)
		if err := m.SetupNetNS(vm, m.bridgeGatewayIP()); err != nil {
			vm.State = VMStateStopped
			m.releaseIP(vm.GuestIP)
			return nil, fmt.Errorf("setup netns: %w", err)
		}
		if err := m.ApplyEgressRules(vm.NetNS, fn.NetworkPolicy); err != nil {
			vm.State = VMStateStopped
			CleanupNetNS(vm.ID)
			m.releaseIP(vm.GuestIP)
			return nil, fmt.Errorf("apply egress rules: %w", err)
		}
	} else {
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
	}

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
	var cmd *exec.Cmd
	if vm.NetNS != "" {
		cmd = exec.Command("ip", "netns", "exec", vm.NetNS,
			m.config.FirecrackerBin,
			"--api-sock", vm.SocketPath,
		)
	} else {
		cmd = exec.Command(m.config.FirecrackerBin,
			"--api-sock", vm.SocketPath,
		)
	}
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
		// Load Snapshot (pass funcID for metadata lookup and original CID for resource management)
		err = m.apiLoadSnapshot(ctx, vm, snapshotPath, memPath, fn.ID, cid)
		if err != nil {
			logging.Op().Warn("snapshot load failed, falling back to fresh boot",
				"func_id", fn.ID,
				"error", err)

			// Delete broken snapshot files so we don't hit this again
			os.Remove(snapshotPath)
			os.Remove(memPath)
			metaPath := filepath.Join(m.config.SnapshotDir, fn.ID+".meta")
			os.Remove(metaPath)

			// After a failed /snapshot/load the Firecracker process is in
			// an undefined state. Kill it and start a fresh one.
			if vm.Cmd != nil && vm.Cmd.Process != nil {
				syscall.Kill(-vm.Cmd.Process.Pid, syscall.SIGKILL)
				vm.Cmd.Wait()
			}
			removeSocketClient(vm.SocketPath)
			os.Remove(vm.SocketPath)

			logFile2, err2 := os.Create(filepath.Join(m.config.LogDir, vmID+".log"))
			if err2 != nil {
				vm.State = VMStateStopped
				return nil, fmt.Errorf("create log file for fresh boot: %w", err2)
			}
			var cmd2 *exec.Cmd
			if vm.NetNS != "" {
				cmd2 = exec.Command("ip", "netns", "exec", vm.NetNS, m.config.FirecrackerBin, "--api-sock", vm.SocketPath)
			} else {
				cmd2 = exec.Command(m.config.FirecrackerBin, "--api-sock", vm.SocketPath)
			}
			cmd2.Stdout = logFile2
			cmd2.Stderr = logFile2
			cmd2.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err2 = cmd2.Start(); err2 != nil {
				logFile2.Close()
				vm.State = VMStateStopped
				return nil, fmt.Errorf("restart firecracker for fresh boot: %w", err2)
			}
			logFile2.Close()
			vm.Cmd = cmd2

			if err2 = m.waitForSocket(ctx, vm.SocketPath, cmd2.Process, m.config.BootTimeout); err2 != nil {
				m.StopVM(vm.ID)
				return nil, fmt.Errorf("wait api socket (fresh boot): %w", err2)
			}

			// Fall back to fresh boot with the original code drive
			useSnapshot = false
			err = m.apiBoot(ctx, vm, rootfsPath, codeDrive, fn)
		} else {
			// Snapshot loaded successfully - apiLoadSnapshot already released the original CID
			cidAllocated = false
		}
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

// CreateVMWithFiles boots a microVM with multiple code files.
// files is a map of relative path -> content.
func (m *Manager) CreateVMWithFiles(ctx context.Context, fn *domain.Function, files map[string][]byte) (*VM, error) {
	// If single file, use the standard CreateVM path
	if len(files) == 1 {
		for _, content := range files {
			return m.CreateVM(ctx, fn, content)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files provided")
	}

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

	// Clean up any stale sockets
	_ = os.Remove(vm.SocketPath)
	_ = os.Remove(vm.VsockPath)

	// Prepare rootfs
	var rootfsFile string
	if fn.RuntimeImageName != "" {
		rootfsFile = fn.RuntimeImageName
	} else {
		rootfsFile = rootfsForRuntime(fn.Runtime)
	}
	rootfsPath := filepath.Join(m.config.RootfsDir, rootfsFile)
	if _, err := os.Stat(rootfsPath); os.IsNotExist(err) {
		vm.State = VMStateStopped
		return nil, fmt.Errorf("rootfs not found: %s", rootfsPath)
	}

	// Build code drive with multiple files
	codeDrive := filepath.Join(m.config.SocketDir, vmID+"-code.ext4")
	if err := m.buildCodeDriveMulti(codeDrive, files); err != nil {
		vm.State = VMStateStopped
		return nil, fmt.Errorf("build code drive: %w", err)
	}
	vm.CodeDrive = codeDrive

	// Setup network
	if err := m.ensureBridge(); err != nil {
		vm.State = VMStateStopped
		return nil, fmt.Errorf("ensure bridge: %w", err)
	}

	useNetNS := fn.NetworkPolicy != nil && fn.NetworkPolicy.IsolationMode != "" && fn.NetworkPolicy.IsolationMode != "none"
	if useNetNS {
		ip, err := m.allocateIP()
		if err != nil {
			vm.State = VMStateStopped
			return nil, err
		}
		vm.GuestIP = ip
		vm.GuestMAC = generateMAC(vmID)
		if err := m.SetupNetNS(vm, m.bridgeGatewayIP()); err != nil {
			vm.State = VMStateStopped
			m.releaseIP(vm.GuestIP)
			return nil, fmt.Errorf("setup netns: %w", err)
		}
		if err := m.ApplyEgressRules(vm.NetNS, fn.NetworkPolicy); err != nil {
			vm.State = VMStateStopped
			CleanupNetNS(vm.ID)
			m.releaseIP(vm.GuestIP)
			return nil, fmt.Errorf("apply egress rules: %w", err)
		}
	} else {
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
	}

	// Note: snapshots not supported for multi-file VMs initially

	// Start Firecracker process
	logFile, err := os.Create(filepath.Join(m.config.LogDir, vmID+".log"))
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	var cmd *exec.Cmd
	if vm.NetNS != "" {
		cmd = exec.Command("ip", "netns", "exec", vm.NetNS,
			m.config.FirecrackerBin,
			"--api-sock", vm.SocketPath,
		)
	} else {
		cmd = exec.Command(m.config.FirecrackerBin,
			"--api-sock", vm.SocketPath,
		)
	}
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
		m.StopVM(vm.ID)
		return nil, fmt.Errorf("wait api socket: %w", err)
	}

	// Regular Boot (no snapshot support for multi-file yet)
	if err := m.apiBoot(ctx, vm, rootfsPath, codeDrive, fn); err != nil {
		m.StopVM(vm.ID)
		return nil, err
	}

	vm.State = VMStateRunning
	m.mu.Lock()
	m.vms[vm.ID] = vm
	m.mu.Unlock()

	metrics.Global().RecordVMCreated()

	go m.monitorProcess(vm)

	if err := m.waitForVsock(ctx, vm); err != nil {
		m.StopVM(vm.ID)
		return nil, fmt.Errorf("wait vsock: %w", err)
	}

	return vm, nil
}



