package firecracker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
)

// snapshotMeta stores metadata needed for snapshot restore.
type snapshotMeta struct {
	VsockPath       string `json:"vsock_path"`
	VsockCID        uint32 `json:"vsock_cid"`
	CodeDrive       string `json:"code_drive,omitempty"`        // path Firecracker expects (may be in /tmp)
	CodeDriveBackup string `json:"code_drive_backup,omitempty"` // persistent copy in SnapshotDir
	GuestIP         string `json:"guest_ip,omitempty"`
	GuestMAC        string `json:"guest_mac,omitempty"`
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

	// Firecracker's snapshot state internally records the code drive path
	// that was configured at snapshot creation time. On restore, it expects
	// the backing file at that exact path. Since the code drive lives in
	// /tmp (SocketDir), it may be cleaned up by systemd-tmpfiles or lost
	// on reboot. We keep a persistent copy in SnapshotDir so we can
	// restore the file on demand during snapshot load.
	persistentCodeDrive := filepath.Join(m.config.SnapshotDir, funcID+"-code.ext4")
	if err := copyFile(vm.CodeDrive, persistentCodeDrive); err != nil {
		return fmt.Errorf("persist code drive for snapshot: %w", err)
	}

	// Save metadata for snapshot restore (vsock path, CID, network, etc.)
	// CodeDrive stores the path Firecracker expects (the original /tmp path).
	// CodeDriveBackup stores the persistent copy that survives reboots.
	meta := snapshotMeta{
		VsockPath:       vm.VsockPath,
		VsockCID:        vm.CID,
		CodeDrive:       vm.CodeDrive,
		CodeDriveBackup: persistentCodeDrive,
		GuestIP:         vm.GuestIP,
		GuestMAC:        vm.GuestMAC,
	}
	metaData, _ := json.Marshal(meta)
	metaPath := filepath.Join(m.config.SnapshotDir, funcID+".meta")
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		return errors.New("write snapshot metadata: " + err.Error())
	}

	// Snapshot state references the original code drive backing file,
	// so keep it on disk after this VM is stopped.
	vm.PreserveCodeDrive = true

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
		if vm.NetNS != "" {
			CleanupNetNS(vm.ID)
		} else {
			deleteTAP(vm.TapDevice)
		}
		os.Remove(vm.SocketPath)
		os.Remove(vm.VsockPath)
		if !vm.PreserveCodeDrive {
			os.Remove(vm.CodeDrive)
		}
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
	if vm.NetNS != "" {
		CleanupNetNS(vm.ID)
	} else {
		deleteTAP(vm.TapDevice)
	}
	os.Remove(vm.SocketPath)
	os.Remove(vm.VsockPath)
	if !vm.PreserveCodeDrive {
		os.Remove(vm.CodeDrive)
	}
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
