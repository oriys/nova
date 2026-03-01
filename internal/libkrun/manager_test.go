package libkrun

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestWaitForAgent_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForAgent(ctx, "127.0.0.1:65534", 2*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestWaitForAgent_ReadyAddress(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	ready := make(chan struct{})
	go func() {
		close(ready)
		conn, err := l.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()
	<-ready

	if err := waitForAgent(context.Background(), l.Addr().String(), 2*time.Second); err != nil {
		t.Fatalf("waitForAgent() error = %v, want nil", err)
	}
}
