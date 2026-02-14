package executor

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPersistentVsockStream_Execute(t *testing.T) {
	dialCount := 0
	p := NewPersistentVsockStream(
		func() error { dialCount++; return nil },
		func(msg interface{}) error { return nil },
		func() (interface{}, error) { return "response", nil },
		func() error { return nil },
	)
	defer p.Close()

	resp, err := p.Execute("request")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if resp != "response" {
		t.Fatalf("unexpected response: %v", resp)
	}
	if dialCount != 1 {
		t.Fatalf("expected 1 dial, got %d", dialCount)
	}

	// Second call should reuse connection (no redial)
	resp, err = p.Execute("request2")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if dialCount != 1 {
		t.Fatalf("expected 1 dial (reused), got %d", dialCount)
	}
}

func TestPersistentVsockStream_Reconnect(t *testing.T) {
	sendCount := 0
	dialCount := 0
	p := NewPersistentVsockStream(
		func() error { dialCount++; return nil },
		func(msg interface{}) error {
			sendCount++
			if sendCount == 1 {
				return errors.New("broken pipe")
			}
			return nil
		},
		func() (interface{}, error) { return "ok", nil },
		func() error { return nil },
	)
	defer p.Close()

	// First send fails, triggering reconnect
	resp, err := p.Execute("request")
	if err != nil {
		t.Fatalf("Execute should succeed after reconnect: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("unexpected response: %v", resp)
	}
	// Should have dialed twice: initial + reconnect
	if dialCount != 2 {
		t.Fatalf("expected 2 dials (1 initial + 1 reconnect), got %d", dialCount)
	}
}

func TestPersistentVsockStream_DialError(t *testing.T) {
	p := NewPersistentVsockStream(
		func() error { return errors.New("connection refused") },
		func(msg interface{}) error { return nil },
		func() (interface{}, error) { return nil, nil },
		func() error { return nil },
	)
	defer p.Close()

	_, err := p.Execute("request")
	if err == nil {
		t.Fatal("expected error on dial failure")
	}
}

func TestPersistentVsockStream_Close(t *testing.T) {
	closed := false
	p := NewPersistentVsockStream(
		func() error { return nil },
		func(msg interface{}) error { return nil },
		func() (interface{}, error) { return nil, nil },
		func() error { closed = true; return nil },
	)

	p.Execute("init")
	if err := p.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !closed {
		t.Fatal("closer should have been called")
	}
}

func TestPersistentVsockStream_ConcurrentAccess(t *testing.T) {
	var callCount atomic.Int64
	p := NewPersistentVsockStream(
		func() error { return nil },
		func(msg interface{}) error { return nil },
		func() (interface{}, error) {
			callCount.Add(1)
			return "ok", nil
		},
		func() error { return nil },
	)
	defer p.Close()

	const goroutines = 20
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := p.Execute("request")
			if err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent execute failed: %v", err)
	}

	if callCount.Load() != goroutines {
		t.Fatalf("expected %d calls, got %d", goroutines, callCount.Load())
	}
}

func TestBalancedRemoteInvoker_InterfaceCompliance(t *testing.T) {
	// Verify BalancedRemoteInvoker implements Invoker
	var _ Invoker = (*BalancedRemoteInvoker)(nil)
}
