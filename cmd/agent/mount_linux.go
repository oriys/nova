//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
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
