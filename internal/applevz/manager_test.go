package applevz

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitForVsockAgent_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForVsockAgent(ctx, filepath.Join(t.TempDir(), "missing.sock"), 2*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestWaitForVsockAgent_ReadySocket(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "apvz-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(dir)
	socketPath := filepath.Join(dir, "a.sock")
	ready := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		l, err := net.Listen("unix", socketPath)
		if err != nil {
			errCh <- err
			close(ready)
			return
		}
		close(ready)
		defer l.Close()
		conn, err := l.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("socket listener did not start")
	}
	select {
	case err := <-errCh:
		t.Fatalf("listen unix: %v", err)
	default:
	}
	defer os.Remove(socketPath)

	if err := waitForVsockAgent(context.Background(), socketPath, 2*time.Second); err != nil {
		t.Fatalf("waitForVsockAgent() error = %v, want nil", err)
	}
}
