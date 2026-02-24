package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromFile_FirecrackerPathFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nova.json")
	content := `{
  "firecracker": {
    "backend": "firecracker",
    "binary": "/custom/firecracker/bin/firecracker",
    "kernel": "/custom/firecracker/kernel/vmlinux",
    "rootfs_dir": "/custom/firecracker/rootfs",
    "snapshot_dir": "/custom/firecracker/snapshots",
    "socket_dir": "/custom/firecracker/sockets",
    "vsock_dir": "/custom/firecracker/vsock",
    "log_dir": "/custom/firecracker/logs"
  }
}`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got, want := cfg.Firecracker.Backend, "firecracker"; got != want {
		t.Fatalf("backend = %q, want %q", got, want)
	}
	if got, want := cfg.Firecracker.FirecrackerBin, "/custom/firecracker/bin/firecracker"; got != want {
		t.Fatalf("binary = %q, want %q", got, want)
	}
	if got, want := cfg.Firecracker.KernelPath, "/custom/firecracker/kernel/vmlinux"; got != want {
		t.Fatalf("kernel = %q, want %q", got, want)
	}
	if got, want := cfg.Firecracker.RootfsDir, "/custom/firecracker/rootfs"; got != want {
		t.Fatalf("rootfs_dir = %q, want %q", got, want)
	}
	if got, want := cfg.Firecracker.SnapshotDir, "/custom/firecracker/snapshots"; got != want {
		t.Fatalf("snapshot_dir = %q, want %q", got, want)
	}
	if got, want := cfg.Firecracker.SocketDir, "/custom/firecracker/sockets"; got != want {
		t.Fatalf("socket_dir = %q, want %q", got, want)
	}
	if got, want := cfg.Firecracker.VsockDir, "/custom/firecracker/vsock"; got != want {
		t.Fatalf("vsock_dir = %q, want %q", got, want)
	}
	if got, want := cfg.Firecracker.LogDir, "/custom/firecracker/logs"; got != want {
		t.Fatalf("log_dir = %q, want %q", got, want)
	}
}
