//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func remountCodeDriveRW() error {
	if os.Getenv("NOVA_SKIP_MOUNT") == "true" {
		return nil // Docker mode: /code is a regular directory, no remount needed
	}
	return unix.Mount("", CodeMountPoint, "", unix.MS_REMOUNT, "")
}

func remountCodeDriveRO() error {
	if os.Getenv("NOVA_SKIP_MOUNT") == "true" {
		return nil
	}
	return unix.Mount("", CodeMountPoint, "", unix.MS_REMOUNT|unix.MS_RDONLY, "")
}

func mountCodeDrive() {
	if os.Getenv("NOVA_SKIP_MOUNT") == "true" {
		fmt.Println("[agent] NOVA_SKIP_MOUNT=true, skipping code drive mount")
		return
	}

	_ = os.MkdirAll(CodeMountPoint, 0755)

	// Minimal rootfs images may not include /bin/mount or udev. Mount devtmpfs
	// so /dev/vdb is available, then mount code drive via syscall.
	_ = os.MkdirAll("/dev", 0755)
	if err := unix.Mount("devtmpfs", "/dev", "devtmpfs", 0, ""); err != nil && !errors.Is(err, unix.EBUSY) {
		fmt.Fprintf(os.Stderr, "[agent] Mount devtmpfs: %v\n", err)
		os.Exit(1)
	}

	// Mount procfs at /proc. Some runtimes (e.g. Deno) call readlink("/proc/self/exe")
	// at startup to resolve their binary path, and will fail without it.
	_ = os.MkdirAll("/proc", 0755)
	if err := unix.Mount("proc", "/proc", "proc", 0, ""); err != nil && !errors.Is(err, unix.EBUSY) {
		fmt.Fprintf(os.Stderr, "[agent] Mount procfs: %v\n", err)
		os.Exit(1)
	}

	// Rootfs is mounted read-only for sharing; provide a writable tmpfs at /tmp.
	// This is needed because the agent writes request payloads to /tmp/input.json.
	if err := unix.Mount("tmpfs", "/tmp", "tmpfs", 0, "mode=1777,size=64m"); err != nil && !errors.Is(err, unix.EBUSY) {
		fmt.Fprintf(os.Stderr, "[agent] Mount tmpfs /tmp: %v\n", err)
		os.Exit(1)
	}

	var lastErr error
	for i := 0; i < 40; i++ { // ~2s total
		err := unix.Mount("/dev/vdb", CodeMountPoint, "ext4", unix.MS_RDONLY, "")
		if err == nil || errors.Is(err, unix.EBUSY) {
			fmt.Printf("[agent] Mounted code drive at %s\n", CodeMountPoint)
			return
		}
		lastErr = err
		if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ENXIO) {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		fmt.Fprintf(os.Stderr, "[agent] Mount /dev/vdb: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "[agent] Mount /dev/vdb timed out: %v\n", lastErr)
	os.Exit(1)
}

// mountLayerDrives mounts shared dependency layer drives at /layers/0, /layers/1, etc.
// Layers are on /dev/vdc through /dev/vdh (up to 6 layers).
func mountLayerDrives() {
	if os.Getenv("NOVA_SKIP_MOUNT") == "true" {
		return
	}

	// Probe /dev/vdc through /dev/vdh for layer drives
	devices := []string{"/dev/vdc", "/dev/vdd", "/dev/vde", "/dev/vdf", "/dev/vdg", "/dev/vdh"}
	mounted := 0

	for i, dev := range devices {
		if _, err := os.Stat(dev); err != nil {
			break // No more layer devices
		}

		mountPoint := fmt.Sprintf("/layers/%d", i)
		_ = os.MkdirAll(mountPoint, 0755)

		err := unix.Mount(dev, mountPoint, "ext4", unix.MS_RDONLY, "")
		if err != nil {
			if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ENXIO) {
				break // Device doesn't exist
			}
			fmt.Fprintf(os.Stderr, "[agent] Mount layer %s: %v\n", dev, err)
			continue
		}
		mounted++
		fmt.Printf("[agent] Mounted layer drive %s at %s\n", dev, mountPoint)
	}

	if mounted > 0 {
		fmt.Printf("[agent] Mounted %d layer drives\n", mounted)
	}
}

// mountLayerOverlay creates an overlayfs mount merging all layer directories
// into /layers/merged for runtimes that need a single unified directory.
func mountLayerOverlay(layerCount int) {
	if layerCount == 0 {
		return
	}

	// Build lower dirs string (in reverse order so layer 0 has highest priority)
	var lowerDirs []string
	for i := layerCount - 1; i >= 0; i-- {
		lowerDirs = append(lowerDirs, fmt.Sprintf("/layers/%d", i))
	}

	mergedDir := "/layers/merged"
	workDir := "/tmp/overlay-work"
	upperDir := "/tmp/overlay-upper"

	_ = os.MkdirAll(mergedDir, 0755)
	_ = os.MkdirAll(workDir, 0755)
	_ = os.MkdirAll(upperDir, 0755)

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		strings.Join(lowerDirs, ":"), upperDir, workDir)

	if err := unix.Mount("overlay", mergedDir, "overlay", 0, opts); err != nil {
		fmt.Fprintf(os.Stderr, "[agent] overlayfs mount failed: %v (falling back to separate mounts)\n", err)
		return
	}
	fmt.Printf("[agent] Mounted overlayfs at %s (layers: %d)\n", mergedDir, layerCount)
}

// mountVolumeDrives mounts persistent volume drives at the mount paths
// specified in the init payload. Volume drives are attached after layer
// drives, so the first volume device letter depends on the layer count.
// Drive layout: vda=rootfs, vdb=code, vdc..vdh=layers, then volumes.
func mountVolumeDrives(layerCount int, mounts []VolumeMountInfo) {
	if os.Getenv("NOVA_SKIP_MOUNT") == "true" {
		return
	}

	// Volume devices start after layers. The device letter offset is:
	// 'c' (index 2) + layerCount, where 'a'=0 (rootfs/vda), 'b'=1
	// (code/vdb), 'c'=2 (first layer/vdc). For example, with 0 layers
	// the first volume is /dev/vdc; with 2 layers it is /dev/vde.
	baseIdx := 2 + layerCount

	for i, m := range mounts {
		devLetter := rune('a' + baseIdx + i)
		dev := fmt.Sprintf("/dev/vd%c", devLetter)

		if _, err := os.Stat(dev); err != nil {
			fmt.Fprintf(os.Stderr, "[agent] Volume device %s not found, skipping mount to %s\n", dev, m.MountPath)
			continue
		}

		_ = os.MkdirAll(m.MountPath, 0755)

		var flags uintptr
		if m.ReadOnly {
			flags = unix.MS_RDONLY
		}

		err := unix.Mount(dev, m.MountPath, "ext4", flags, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[agent] Mount volume %s at %s: %v\n", dev, m.MountPath, err)
			continue
		}
		fmt.Printf("[agent] Mounted volume drive %s at %s (read_only=%v)\n", dev, m.MountPath, m.ReadOnly)
	}
}
