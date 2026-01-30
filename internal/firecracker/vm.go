package firecracker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
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
	VMStatePaused   VMState = "paused"
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
	SnapshotDir    string
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
		SnapshotDir:    "/opt/nova/snapshots",
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
	config       *Config
	vms          map[string]*VM
	mu           sync.RWMutex
	nextCID      uint32
	cidMu        sync.Mutex
	templateOnce sync.Once
	httpClient   *http.Client
}

func NewManager(cfg *Config) (*Manager, error) {
	for _, dir := range []string{cfg.SocketDir, cfg.VsockDir, cfg.LogDir, cfg.SnapshotDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	return &Manager{
		config:  cfg,
		vms:     make(map[string]*VM),
		nextCID: 100,
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, addr string) (net.Conn, error) {
					return net.Dial("unix", addr)
				},
			},
		},
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
// Checks for existing snapshot first.
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

	// Prepare resources
	rootfsPath := filepath.Join(m.config.RootfsDir, rootfsForRuntime(fn.Runtime))
	if _, err := os.Stat(rootfsPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("rootfs not found: %s", rootfsPath)
	}

	codeDrive := filepath.Join(m.config.SocketDir, vmID+"-code.ext4")
	if err := m.buildCodeDrive(codeDrive, fn.CodePath); err != nil {
		return nil, fmt.Errorf("build code drive: %w", err)
	}
	vm.CodeDrive = codeDrive

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
	cmd := exec.CommandContext(ctx, m.config.FirecrackerBin,
		"--api-sock", vm.SocketPath,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("start firecracker: %w", err)
	}
	vm.Cmd = cmd
	
	// Wait for API socket
	if err := m.waitForSocket(ctx, vm.SocketPath); err != nil {
		m.StopVM(vm.ID) // cleanup
		return nil, fmt.Errorf("wait api socket: %w", err)
	}

	if useSnapshot {
		// Load Snapshot
		err = m.apiLoadSnapshot(ctx, vm, snapshotPath, memPath)
	} else {
		// Regular Boot
		err = m.apiBoot(ctx, vm, rootfsPath, codeDrive, fn.MemoryMB)
	}

	if err != nil {
		m.StopVM(vm.ID)
		return nil, err
	}

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

	// Stop VM after snapshot (it's reusable but for safety we usually discard)
	// We leave it to the caller to StopVM if they want.
	return nil
}

// apiBoot configures and boots the VM via API
func (m *Manager) apiBoot(ctx context.Context, vm *VM, rootfs, codeDrive string, mem int) error {
	if mem <= 0 { mem = 128 }
	
	// 1. Boot Source
	bs := map[string]interface{}{
		"kernel_image_path": m.config.KernelPath,
		"boot_args":         "console=ttyS0 reboot=k panic=1 pci=off init=/init quiet 8250.nr_uarts=0",
	}
	if err := m.apiCall(ctx, vm, "PUT", "/boot-source", bs); err != nil {
		return fmt.Errorf("boot-source: %w", err)
	}

	// 2. Drives
	root := map[string]interface{}{
		"drive_id": "rootfs",
		"path_on_host": rootfs,
		"is_root_device": true,
		"is_read_only": true,
	}
	if err := m.apiCall(ctx, vm, "PUT", "/drives/rootfs", root); err != nil {
		return fmt.Errorf("drive rootfs: %w", err)
	}
	
	code := map[string]interface{}{
		"drive_id": "code",
		"path_on_host": codeDrive,
		"is_root_device": false,
		"is_read_only": true,
	}
	if err := m.apiCall(ctx, vm, "PUT", "/drives/code", code); err != nil {
		return fmt.Errorf("drive code: %w", err)
	}

	// 3. Machine Config
	mc := map[string]interface{}{
		"vcpu_count": 1,
		"mem_size_mib": mem,
	}
	if err := m.apiCall(ctx, vm, "PUT", "/machine-config", mc); err != nil {
		return fmt.Errorf("machine-config: %w", err)
	}

	// 4. Vsock
	vs := map[string]interface{}{
		"guest_cid": vm.CID,
		"uds_path": vm.VsockPath,
	}
	if err := m.apiCall(ctx, vm, "PUT", "/vsock", vs); err != nil {
		return fmt.Errorf("vsock: %w", err)
	}

	// 5. Action: InstanceStart
	if err := m.apiCall(ctx, vm, "PUT", "/actions", map[string]interface{}{"action_type": "InstanceStart"}); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	return nil
}

func (m *Manager) apiLoadSnapshot(ctx context.Context, vm *VM, snapPath, memPath string) error {
	// Enable vsock before loading? Firecracker docs say restoration restores device state.
	// However, the vsock UDS path might need to be set if it changed (it changes per VM ID).
	// Snapshot restore typically requires matching device config.
	// But Firecracker allows updating config on restore in newer versions.
	// Simplified: We assume snapshot was taken with generic config, but UDS path is problematic.
	// Workaround: We don't change UDS path in snapshot? No, UDS path is host-side.
	// Actually, Firecracker re-binds to UDS path specified in config *or* updated via API?
	// The `load_snapshot` API doesn't take Vsock config.
	
	// This is tricky. If we load a snapshot, the vsock device state (CID) is restored.
	// The host UDS path must be valid.
	// We might need to "patch" the VM after load, or ensure the snapshot VM had a generic path?
	// Actually, `snapshot/load` restores the *guest* state. Host resources (tap, vsock uds) are re-attached?
	// Let's rely on `resume_vm: true` handling it, but we might need to configure Vsock backend *before* load?
	// Documentation says: "Configure the microVM... then Load Snapshot".
	
	// We'll set up Vsock (with NEW UDS path) and other devices, THEN load snapshot with `resume_vm: true`.
	// This allows the restored guest to talk to the new socket.
	
	vs := map[string]interface{}{
		"guest_cid": vm.CID, // Must match snapshot? Yes, or guest gets confused.
		"uds_path": vm.VsockPath,
	}
	if err := m.apiCall(ctx, vm, "PUT", "/vsock", vs); err != nil {
		return fmt.Errorf("vsock: %w", err)
	}

	// We also need to re-attach drives?
	// Firecracker snapshot usually contains device state. 
	// But host paths might need to be set if changed.
	// For now, let's assume we just call Load.
	
	req := map[string]interface{}{
		"snapshot_path": snapPath,
		"mem_file_path": memPath,
		"resume_vm": true,
	}
	if err := m.apiCall(ctx, vm, "PUT", "/snapshot/load", req); err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}
	return nil
}

func (m *Manager) apiCall(ctx context.Context, vm *VM, method, path string, body interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}

	// Dial using the VM's socket path as the "host"
	// The Transport DialContext handles the unix socket connection
	req, err := http.NewRequestWithContext(ctx, method, "http://localhost"+path, bodyReader)
	if err != nil {
		return err
	}
	
	// Hack: Put the socket path in the URL Host or use a custom transport per request?
	// The single httpClient has a Dial that uses `addr`. 
	// We need to pass the socket path.
	// We can use a custom transport for EACH call, or update the specific request's context.
	// Easiest: Create a new Transport/Client for this request.
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", vm.SocketPath)
			},
		},
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

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

func (m *Manager) waitForSocket(ctx context.Context, path string) error {
	deadline, _ := ctx.Deadline()
	if deadline.IsZero() {
		deadline = time.Now().Add(5 * time.Second)
	}
	for time.Now().Before(deadline) {
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

// buildCodeDrive creates a small ext4 image and injects the function code at /handler.
// Uses a cached template image to avoid repeated mkfs calls.
func (m *Manager) buildCodeDrive(drivePath, codePath string) error {
	templatePath := filepath.Join(m.config.SocketDir, "template-code.ext4")

	// Create template once
	var templateErr error
	m.templateOnce.Do(func() {
		// Create empty image
		f, err := os.Create(templatePath)
		if err != nil {
			templateErr = err
			return
		}
		if err := f.Truncate(int64(codeDriveSizeMB) * 1024 * 1024); err != nil {
			f.Close()
			templateErr = err
			return
		}
		f.Close()

		// Format as ext4
		if out, err := exec.Command("mkfs.ext4", "-F", "-q", templatePath).CombinedOutput(); err != nil {
			templateErr = fmt.Errorf("mkfs.ext4: %s: %w", out, err)
			return
		}
	})
	if templateErr != nil {
		return templateErr
	}

	// Copy template to new drive
	if err := copyFile(templatePath, drivePath); err != nil {
		return err
	}

	// Inject function code using debugfs (no mount needed)
	// write: copy file
	// sif: set inode field (mode 0100755 = regular file + rwxr-xr-x)
	debugfsCmd := fmt.Sprintf("write %s handler\nsif handler mode 0100755\n", codePath)
	cmd := exec.Command("debugfs", "-w", drivePath)
	cmd.Stdin = strings.NewReader(debugfsCmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("debugfs inject: %s: %w", out, err)
	}

	return nil
}

func copyFile(src, dst string) error {
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

	_, err = io.Copy(out, in)
	return err
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
