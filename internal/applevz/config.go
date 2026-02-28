// Package applevz provides a backend that uses Apple's Virtualization.framework
// for running functions in lightweight Linux VMs on macOS.
//
// Two VM managers are supported:
//   - nova-vz (preferred): Custom Swift tool with full snapshot save/restore support
//   - vfkit (fallback): Third-party CLI, no snapshot support
//
// The backend boots Linux VMs with direct kernel boot, shares function code
// via VirtioFS, and communicates with the nova-agent over virtio-vsock
// (exposed as a UNIX socket on the host).
//
// Prerequisites:
//   - macOS 13+ (Ventura or later; macOS 14+ for snapshots)
//   - nova-vz (built from tools/nova-vz/) or vfkit (brew install vfkit)
//   - Linux kernel image (arm64 for Apple Silicon, x86_64 for Intel)
//   - Runtime rootfs images (ext4 or raw disk images)
package applevz

import (
	"os"
	"runtime"
	"time"
)

// Config holds Apple Virtualization backend configuration.
type Config struct {
	KernelPath     string        `json:"kernel_path"`      // Path to Linux kernel image (uncompressed Image for arm64)
	InitrdPath     string        `json:"initrd_path"`      // Path to initrd/initramfs (optional)
	RootfsDir      string        `json:"rootfs_dir"`       // Directory containing per-runtime rootfs images
	CodeDir        string        `json:"code_dir"`         // Base directory for function code (shared via VirtioFS)
	SocketDir      string        `json:"socket_dir"`       // Directory for vsock UNIX sockets
	VfkitPath      string        `json:"vfkit_path"`       // Path to vfkit binary (auto-detected if empty)
	NovaVZPath     string        `json:"nova_vz_path"`     // Path to nova-vz binary (preferred over vfkit)
	SnapshotDirVal string        `json:"snapshot_dir"`     // Directory for snapshot state files (empty = disabled)
	DefaultMemMB   int           `json:"default_mem_mb"`   // Default memory in MB per VM (default: 256)
	DefaultCPUs    int           `json:"default_cpus"`     // Default CPU count per VM (default: 1)
	AgentTimeout   time.Duration `json:"agent_timeout"`    // Agent startup timeout (default: 15s)
	KernelCmdline  string        `json:"kernel_cmdline"`   // Extra kernel command-line parameters
}

// DefaultConfig returns sensible defaults for the Apple VZ backend.
func DefaultConfig() *Config {
	codeDir := os.Getenv("NOVA_APPLEVZ_CODE_DIR")
	if codeDir == "" {
		codeDir = "/tmp/nova/applevz-code"
	}
	rootfsDir := os.Getenv("NOVA_APPLEVZ_ROOTFS_DIR")
	if rootfsDir == "" {
		rootfsDir = "/opt/nova/rootfs"
	}
	kernelPath := os.Getenv("NOVA_APPLEVZ_KERNEL")
	if kernelPath == "" {
		kernelPath = "/opt/nova/assets/vmlinux"
	}
	initrdPath := os.Getenv("NOVA_APPLEVZ_INITRD")
	socketDir := os.Getenv("NOVA_APPLEVZ_SOCKET_DIR")
	if socketDir == "" {
		socketDir = "/tmp/nova/applevz-socks"
	}
	vfkitPath := os.Getenv("NOVA_APPLEVZ_VFKIT_PATH")
	novaVZPath := os.Getenv("NOVA_APPLEVZ_NOVA_VZ_PATH")
	snapshotDir := os.Getenv("NOVA_APPLEVZ_SNAPSHOT_DIR")

	return &Config{
		KernelPath:     kernelPath,
		InitrdPath:     initrdPath,
		RootfsDir:      rootfsDir,
		CodeDir:        codeDir,
		SocketDir:      socketDir,
		VfkitPath:      vfkitPath,
		NovaVZPath:     novaVZPath,
		SnapshotDirVal: snapshotDir,
		DefaultMemMB:   256,
		DefaultCPUs:    1,
		AgentTimeout:   15 * time.Second,
	}
}

// IsSupported returns true if the current platform supports Apple Virtualization.
func IsSupported() bool {
	return runtime.GOOS == "darwin"
}
