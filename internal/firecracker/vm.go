package firecracker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/google/uuid"
)

type VMState string

const (
	VMStateCreating VMState = "creating"
	VMStateRunning  VMState = "running"
	VMStateStopped  VMState = "stopped"

	// Fixed path inside VM where function code lives
	GuestCodeDir  = "/code"
	GuestCodePath = "/code/handler"

	// Code drive size (16MB, enough for any single function)
	codeDriveSizeMB = 16
)

type Config struct {
	FirecrackerBin string
	KernelPath     string
	RootfsDir      string
	SocketDir      string
	VsockDir       string
	LogDir         string
	BridgeName     string
	Subnet         string
	BootTimeout    time.Duration
}

func DefaultConfig() *Config {
	return &Config{
		FirecrackerBin: "/usr/local/bin/firecracker",
		KernelPath:     "/opt/nova/kernel/vmlinux",
		RootfsDir:      "/opt/nova/rootfs",
		SocketDir:      "/tmp/nova/sockets",
		VsockDir:       "/tmp/nova/vsock",
		LogDir:         "/tmp/nova/logs",
		BridgeName:     "novabr0",
		Subnet:         "172.30.0.0/24",
		BootTimeout:    10 * time.Second,
	}
}

type VM struct {
	ID         string
	Runtime    domain.Runtime
	State      VMState
	CID        uint32
	SocketPath string
	VsockPath  string
	CodeDrive  string // path to per-VM code drive
	Cmd        *exec.Cmd
	CreatedAt  time.Time
	LastUsed   time.Time
	mu         sync.RWMutex
}

type Manager struct {
	config  *Config
	vms     map[string]*VM
	mu      sync.RWMutex
	nextCID uint32
	cidMu   sync.Mutex
}

func NewManager(cfg *Config) (*Manager, error) {
	for _, dir := range []string{cfg.SocketDir, cfg.VsockDir, cfg.LogDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	return &Manager{
		config:  cfg,
		vms:     make(map[string]*VM),
		nextCID: 100,
	}, nil
}

func (m *Manager) allocateCID() uint32 {
	m.cidMu.Lock()
	defer m.cidMu.Unlock()
	cid := m.nextCID
	m.nextCID++
	return cid
}

// CreateVM boots a microVM for the given function.
// rootfs (drive 0) is shared read-only per runtime.
// code (drive 1) is a small per-VM ext4 with the function binary/script injected.
func (m *Manager) CreateVM(ctx context.Context, fn *domain.Function) (*VM, error) {
	vmID := uuid.New().String()[:8]
	cid := m.allocateCID()

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

	// Verify rootfs exists (shared, read-only)
	rootfsPath := filepath.Join(m.config.RootfsDir, rootfsForRuntime(fn.Runtime))
	if _, err := os.Stat(rootfsPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("rootfs not found: %s", rootfsPath)
	}

	// Create per-VM code drive with function code injected
	codeDrive := filepath.Join(m.config.SocketDir, vmID+"-code.ext4")
	if err := buildCodeDrive(codeDrive, fn.CodePath); err != nil {
		return nil, fmt.Errorf("build code drive: %w", err)
	}
	vm.CodeDrive = codeDrive

	// Write firecracker config
	fcConfig := m.buildConfig(vm, rootfsPath, codeDrive)
	configPath := filepath.Join(m.config.SocketDir, vmID+".json")
	configData, _ := json.MarshalIndent(fcConfig, "", "  ")
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}

	logFile, err := os.Create(filepath.Join(m.config.LogDir, vmID+".log"))
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	cmd := exec.CommandContext(ctx, m.config.FirecrackerBin,
		"--api-sock", vm.SocketPath,
		"--config-file", configPath,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("start firecracker: %w", err)
	}

	vm.Cmd = cmd
	vm.State = VMStateRunning

	m.mu.Lock()
	m.vms[vm.ID] = vm
	m.mu.Unlock()

	if err := m.waitForVsock(ctx, vm); err != nil {
		m.StopVM(vm.ID)
		return nil, fmt.Errorf("wait vsock: %w", err)
	}

	return vm, nil
}

// buildCodeDrive creates a small ext4 image and injects the function code at /handler.
// Uses mkfs.ext4 + debugfs, no root/mount required.
func buildCodeDrive(drivePath, codePath string) error {
	// Create empty image
	f, err := os.Create(drivePath)
	if err != nil {
		return err
	}
	if err := f.Truncate(int64(codeDriveSizeMB) * 1024 * 1024); err != nil {
		f.Close()
		return err
	}
	f.Close()

	// Format as ext4
	if out, err := exec.Command("mkfs.ext4", "-F", "-q", drivePath).CombinedOutput(); err != nil {
		return fmt.Errorf("mkfs.ext4: %s: %w", out, err)
	}

	// Inject function code using debugfs (no mount needed)
	debugfsCmd := fmt.Sprintf("write %s handler\n", codePath)
	cmd := exec.Command("debugfs", "-w", drivePath)
	cmd.Stdin = strings.NewReader(debugfsCmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("debugfs inject: %s: %w", out, err)
	}

	return nil
}

func (m *Manager) buildConfig(vm *VM, rootfsPath, codeDrivePath string) map[string]interface{} {
	return map[string]interface{}{
		"boot-source": map[string]interface{}{
			"kernel_image_path": m.config.KernelPath,
			"boot_args":         "console=ttyS0 reboot=k panic=1 pci=off init=/init",
		},
		"drives": []map[string]interface{}{
			{
				"drive_id":       "rootfs",
				"path_on_host":   rootfsPath,
				"is_root_device": true,
				"is_read_only":   true,
			},
			{
				"drive_id":       "code",
				"path_on_host":   codeDrivePath,
				"is_root_device": false,
				"is_read_only":   true,
			},
		},
		"machine-config": map[string]interface{}{
			"vcpu_count":  1,
			"mem_size_mib": 128,
		},
		"vsock": map[string]interface{}{
			"guest_cid": vm.CID,
			"uds_path":  vm.VsockPath,
		},
	}
}

func (m *Manager) waitForVsock(ctx context.Context, vm *VM) error {
	deadline := time.Now().Add(m.config.BootTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(vm.VsockPath); err == nil {
			conn, err := net.DialTimeout("unix", vm.VsockPath, time.Second)
			if err == nil {
				conn.Close()
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("vsock timeout")
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

	vm.mu.Lock()
	defer vm.mu.Unlock()

	// 1. Try graceful shutdown via vsock
	if conn, err := net.DialTimeout("unix", vm.VsockPath, time.Second); err == nil {
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
	os.Remove(vm.SocketPath)
	os.Remove(vm.VsockPath)
	os.Remove(vm.CodeDrive)
	os.Remove(filepath.Join(m.config.SocketDir, vm.ID+".json"))

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

	for _, id := range ids {
		m.StopVM(id)
	}
}

// rootfsForRuntime maps runtime to rootfs image.
// Go/Rust: static binaries, minimal base is enough.
// Python: needs interpreter. WASM: needs wasmtime.
func rootfsForRuntime(rt domain.Runtime) string {
	switch rt {
	case domain.RuntimePython:
		return "python.ext4"
	case domain.RuntimeWasm:
		return "wasm.ext4"
	default:
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
	Runtime string            `json:"runtime"`
	Handler string            `json:"handler"`
	EnvVars map[string]string `json:"env_vars"`
}

type ExecPayload struct {
	RequestID string          `json:"request_id"`
	Input     json.RawMessage `json:"input"`
	TimeoutS  int             `json:"timeout_s"`
}

type RespPayload struct {
	RequestID  string          `json:"request_id"`
	Output     json.RawMessage `json:"output"`
	Error      string          `json:"error,omitempty"`
	DurationMs int64           `json:"duration_ms"`
}

type VsockClient struct {
	vm   *VM
	conn net.Conn
}

func NewVsockClient(vm *VM) (*VsockClient, error) {
	conn, err := net.DialTimeout("unix", vm.VsockPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial vsock: %w", err)
	}
	return &VsockClient{vm: vm, conn: conn}, nil
}

func (c *VsockClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *VsockClient) Send(msg *VsockMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))

	if _, err := c.conn.Write(lenBuf); err != nil {
		return err
	}
	_, err = c.conn.Write(data)
	return err
}

func (c *VsockClient) Receive() (*VsockMessage, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, lenBuf); err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint32(lenBuf)
	data := make([]byte, msgLen)
	if _, err := io.ReadFull(c.conn, data); err != nil {
		return nil, err
	}

	var msg VsockMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (c *VsockClient) Init(fn *domain.Function) error {
	payload, _ := json.Marshal(&InitPayload{
		Runtime: string(fn.Runtime),
		Handler: fn.Handler,
		EnvVars: fn.EnvVars,
	})

	if err := c.Send(&VsockMessage{Type: MsgTypeInit, Payload: payload}); err != nil {
		return err
	}

	resp, err := c.Receive()
	if err != nil {
		return err
	}

	if resp.Type != MsgTypeResp {
		return fmt.Errorf("unexpected response type: %d", resp.Type)
	}
	return nil
}

func (c *VsockClient) Execute(reqID string, input json.RawMessage, timeoutS int) (*RespPayload, error) {
	payload, _ := json.Marshal(&ExecPayload{
		RequestID: reqID,
		Input:     input,
		TimeoutS:  timeoutS,
	})

	c.conn.SetDeadline(time.Now().Add(time.Duration(timeoutS+5) * time.Second))

	if err := c.Send(&VsockMessage{Type: MsgTypeExec, Payload: payload}); err != nil {
		return nil, err
	}

	resp, err := c.Receive()
	if err != nil {
		return nil, err
	}

	var result RespPayload
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *VsockClient) Ping() error {
	c.conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err := c.Send(&VsockMessage{Type: MsgTypePing}); err != nil {
		return err
	}
	_, err := c.Receive()
	return err
}
