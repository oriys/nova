//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func mountCodeDrive() {
	_ = os.MkdirAll(CodeMountPoint, 0755)

	// Minimal rootfs images may not include /bin/mount or udev. Mount devtmpfs
	// so /dev/vdb is available, then mount code drive via syscall.
	_ = os.MkdirAll("/dev", 0755)
	if err := unix.Mount("devtmpfs", "/dev", "devtmpfs", 0, ""); err != nil && !errors.Is(err, unix.EBUSY) {
		fmt.Fprintf(os.Stderr, "[agent] Mount devtmpfs: %v\n", err)
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
