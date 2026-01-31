//go:build linux

package firecracker

import (
	"context"
	"fmt"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// waitForFileInotify waits for a file to be created in a directory using inotify.
// Returns nil when the file is created, or an error on timeout/cancellation.
func waitForFileInotify(ctx context.Context, dir, filename string, deadline time.Time) error {
	// Initialize inotify
	fd, err := syscall.InotifyInit1(syscall.IN_NONBLOCK | syscall.IN_CLOEXEC)
	if err != nil {
		return fmt.Errorf("inotify init: %w", err)
	}
	defer syscall.Close(fd)

	// Watch the directory for file creation
	wd, err := syscall.InotifyAddWatch(fd, dir, syscall.IN_CREATE)
	if err != nil {
		return fmt.Errorf("inotify add watch: %w", err)
	}
	defer syscall.InotifyRmWatch(fd, uint32(wd))

	// Create a polling file descriptor for epoll
	epfd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		return fmt.Errorf("epoll create: %w", err)
	}
	defer syscall.Close(epfd)

	event := syscall.EpollEvent{Events: syscall.EPOLLIN, Fd: int32(fd)}
	if err := syscall.EpollCtl(epfd, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
		return fmt.Errorf("epoll ctl: %w", err)
	}

	buf := make([]byte, 4096)
	events := make([]syscall.EpollEvent, 1)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Wait with a short timeout to allow context cancellation checks
		timeout := int(time.Until(deadline).Milliseconds())
		if timeout > 100 {
			timeout = 100 // Check context every 100ms
		}
		if timeout <= 0 {
			break
		}

		n, err := syscall.EpollWait(epfd, events, timeout)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			return fmt.Errorf("epoll wait: %w", err)
		}

		if n > 0 {
			// Read inotify events
			nbytes, err := syscall.Read(fd, buf)
			if err != nil {
				if err == syscall.EAGAIN {
					continue
				}
				return fmt.Errorf("inotify read: %w", err)
			}

			// Parse inotify events
			offset := 0
			for offset < nbytes {
				// Parse the inotify_event structure
				if offset+16 > nbytes {
					break
				}
				nameLen := *(*uint32)(unsafe.Pointer(&buf[offset+12]))
				if offset+16+int(nameLen) > nbytes {
					break
				}

				// Extract filename (null-terminated)
				name := string(buf[offset+16 : offset+16+int(nameLen)])
				name = strings.TrimRight(name, "\x00")

				if name == filename {
					return nil // File was created
				}

				offset += 16 + int(nameLen)
			}
		}
	}

	return fmt.Errorf("timeout waiting for %s", filename)
}
