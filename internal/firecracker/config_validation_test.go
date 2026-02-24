package firecracker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/domain"
)

func writeFile(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func validManagerConfig(t *testing.T) *Config {
	t.Helper()
	tmp := t.TempDir()

	cfg := DefaultConfig()
	cfg.SocketDir = filepath.Join(tmp, "sockets")
	cfg.VsockDir = filepath.Join(tmp, "vsock")
	cfg.LogDir = filepath.Join(tmp, "logs")
	cfg.SnapshotDir = filepath.Join(tmp, "snapshots")
	cfg.FirecrackerBin = filepath.Join(tmp, "bin", "firecracker")
	cfg.KernelPath = filepath.Join(tmp, "kernel", "vmlinux")
	cfg.RootfsDir = filepath.Join(tmp, "rootfs")

	writeFile(t, cfg.FirecrackerBin, []byte("#!/bin/sh\nexit 0\n"), 0755)
	writeFile(t, cfg.KernelPath, []byte("kernel"), 0644)
	writeFile(t, filepath.Join(cfg.RootfsDir, "base.ext4"), []byte("ext4"), 0644)

	return cfg
}

func TestNewManager_ValidatesPaths(t *testing.T) {
	t.Parallel()

	t.Run("missing firecracker binary", func(t *testing.T) {
		cfg := validManagerConfig(t)
		cfg.FirecrackerBin = filepath.Join(t.TempDir(), "missing-firecracker")

		_, err := NewManager(cfg)
		if err == nil || !strings.Contains(err.Error(), "firecracker binary not found") {
			t.Fatalf("expected firecracker binary validation error, got: %v", err)
		}
	})

	t.Run("missing rootfs images", func(t *testing.T) {
		cfg := validManagerConfig(t)
		if err := os.Remove(filepath.Join(cfg.RootfsDir, "base.ext4")); err != nil {
			t.Fatalf("remove base.ext4: %v", err)
		}

		_, err := NewManager(cfg)
		if err == nil || !strings.Contains(err.Error(), "no rootfs images") {
			t.Fatalf("expected rootfs image validation error, got: %v", err)
		}
	})
}

func TestRootfsForRuntime_VersionedKotlinScala(t *testing.T) {
	t.Parallel()

	cases := []struct {
		runtime domain.Runtime
		want    string
	}{
		{runtime: domain.Runtime("kotlin2.0"), want: "java.ext4"},
		{runtime: domain.Runtime("scala3.5"), want: "java.ext4"},
	}

	for _, tc := range cases {
		if got := rootfsForRuntime(tc.runtime); got != tc.want {
			t.Fatalf("rootfsForRuntime(%q) = %q, want %q", tc.runtime, got, tc.want)
		}
	}
}
