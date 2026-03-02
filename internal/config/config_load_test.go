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

func TestLoadFromFile_ResolvesRelativePathsAgainstConfigDir(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	configDir := filepath.Join(rootDir, "configs")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}

	cfgPath := filepath.Join(configDir, "nova-native.json")
	content := `{
  "firecracker": {
    "rootfs_dir": "../assets/rootfs"
  },
  "applevz": {
    "kernel_path": "../assets/kernel/Image-alpine-arm64",
    "initrd_path": "../assets/kernel/initramfs-alpine-arm64-vsock",
    "rootfs_dir": "../assets/rootfs",
    "nova_vz_path": "../bin/nova-vz"
  }
}`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if got, want := cfg.Firecracker.RootfsDir, filepath.Join(rootDir, "assets", "rootfs"); got != want {
		t.Fatalf("firecracker rootfs_dir = %q, want %q", got, want)
	}
	if got, want := cfg.AppleVZ.KernelPath, filepath.Join(rootDir, "assets", "kernel", "Image-alpine-arm64"); got != want {
		t.Fatalf("applevz kernel_path = %q, want %q", got, want)
	}
	if got, want := cfg.AppleVZ.InitrdPath, filepath.Join(rootDir, "assets", "kernel", "initramfs-alpine-arm64-vsock"); got != want {
		t.Fatalf("applevz initrd_path = %q, want %q", got, want)
	}
	if got, want := cfg.AppleVZ.RootfsDir, filepath.Join(rootDir, "assets", "rootfs"); got != want {
		t.Fatalf("applevz rootfs_dir = %q, want %q", got, want)
	}
	if got, want := cfg.AppleVZ.NovaVZPath, filepath.Join(rootDir, "bin", "nova-vz"); got != want {
		t.Fatalf("applevz nova_vz_path = %q, want %q", got, want)
	}
}
