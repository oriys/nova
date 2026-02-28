package applevz

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/oriys/nova/internal/logging"
)

// controlRequest is a JSON command sent to the nova-vz control socket.
type controlRequest struct {
	Action string `json:"action"`
	Path   string `json:"path,omitempty"`
}

// controlResponse is the JSON reply from nova-vz.
type controlResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	State string `json:"state,omitempty"`
}

// snapshotMeta stores metadata for snapshot restore.
type snapshotMeta struct {
	SocketPath        string `json:"socket_path"`
	ControlSocketPath string `json:"control_socket_path"`
	CodeDir           string `json:"code_dir"`
	Runtime           string `json:"runtime"`
}

// sendControlCommand sends a command to a nova-vz control socket and returns the response.
func sendControlCommand(socketPath string, req controlRequest) (*controlResponse, error) {
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to control socket %s: %w", socketPath, err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(30 * time.Second))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("send control command: %w", err)
	}

	var resp controlResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("read control response: %w", err)
	}

	if !resp.OK {
		return &resp, fmt.Errorf("control command %q failed: %s", req.Action, resp.Error)
	}
	return &resp, nil
}

// CreateSnapshot pauses the VM, saves state to disk, and resumes the VM.
// The snapshot state file is saved at <SnapshotDir>/<funcID>.vzsave
// and metadata at <SnapshotDir>/<funcID>.meta.
func (m *Manager) CreateSnapshot(ctx context.Context, vmID string, funcID string) error {
	if !m.useNovaVZ {
		return fmt.Errorf("snapshots require nova-vz (vfkit does not support save/restore)")
	}
	if m.config.SnapshotDirVal == "" {
		return fmt.Errorf("snapshot_dir not configured")
	}

	m.mu.RLock()
	info, ok := m.vms[vmID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("VM not found: %s", vmID)
	}
	if info.controlSocketPath == "" {
		return fmt.Errorf("VM %s has no control socket", vmID)
	}

	statePath := filepath.Join(m.config.SnapshotDirVal, funcID+".vzsave")
	metaPath := filepath.Join(m.config.SnapshotDirVal, funcID+".meta")

	logging.Op().Info("creating Apple VZ snapshot",
		"vmID", vmID,
		"funcID", funcID,
		"statePath", statePath,
	)

	// Pause the VM
	if _, err := sendControlCommand(info.controlSocketPath, controlRequest{Action: "pause"}); err != nil {
		return fmt.Errorf("pause VM: %w", err)
	}

	// Save state
	if _, err := sendControlCommand(info.controlSocketPath, controlRequest{Action: "save", Path: statePath}); err != nil {
		// Try to resume on save failure
		sendControlCommand(info.controlSocketPath, controlRequest{Action: "resume"})
		return fmt.Errorf("save VM state: %w", err)
	}

	// Save metadata
	meta := snapshotMeta{
		SocketPath:        info.socketPath,
		ControlSocketPath: info.controlSocketPath,
		CodeDir:           info.codeDir,
		Runtime:           string(info.vm.Runtime),
	}
	metaData, _ := json.Marshal(meta)
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		return fmt.Errorf("write snapshot metadata: %w", err)
	}

	logging.Op().Info("Apple VZ snapshot created", "funcID", funcID, "statePath", statePath)
	return nil
}

// ResumeVM resumes a paused VM (e.g., after snapshot creation).
func (m *Manager) ResumeVM(ctx context.Context, vmID string) error {
	m.mu.RLock()
	info, ok := m.vms[vmID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("VM not found: %s", vmID)
	}
	if info.controlSocketPath == "" {
		return fmt.Errorf("VM %s has no control socket", vmID)
	}

	_, err := sendControlCommand(info.controlSocketPath, controlRequest{Action: "resume"})
	return err
}

// HasSnapshot checks if a snapshot exists for the given function.
func (m *Manager) HasSnapshot(funcID string) bool {
	if m.config.SnapshotDirVal == "" {
		return false
	}
	statePath := filepath.Join(m.config.SnapshotDirVal, funcID+".vzsave")
	metaPath := filepath.Join(m.config.SnapshotDirVal, funcID+".meta")
	_, err1 := os.Stat(statePath)
	_, err2 := os.Stat(metaPath)
	return err1 == nil && err2 == nil
}
