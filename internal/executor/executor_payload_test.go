package executor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
)

type captureSink struct {
	ch chan *store.InvocationLog
}

func (s *captureSink) Save(_ context.Context, log *store.InvocationLog) error {
	s.ch <- log
	return nil
}

func (s *captureSink) SaveBatch(_ context.Context, logs []*store.InvocationLog) error {
	for _, log := range logs {
		s.ch <- log
	}
	return nil
}

func (s *captureSink) Close() error { return nil }

func TestPersistInvocationLog_DropsPayloadsByDefault(t *testing.T) {
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil,
		WithLogSink(sink),
		WithLogBatcherConfig(LogBatcherConfig{BatchSize: 1, FlushInterval: time.Hour}),
	)
	defer e.logBatcher.Shutdown(time.Second)

	fn := &domain.Function{ID: "f1", Name: "hello", Runtime: domain.RuntimePython}
	e.persistInvocationLog("req1", fn, 123, false, true, "", 7, 8, json.RawMessage(`{"in":1}`), json.RawMessage(`{"out":2}`), "stdout", "stderr")

	select {
	case log := <-sink.ch:
		if log.Input != nil || log.Output != nil {
			t.Fatalf("expected payloads to be dropped by default")
		}
		if log.Stdout != "" || log.Stderr != "" {
			t.Fatalf("expected stdout/stderr to be dropped by default")
		}
		if log.InputSize != 7 || log.OutputSize != 8 {
			t.Fatalf("expected sizes to be retained, got input=%d output=%d", log.InputSize, log.OutputSize)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for invocation log")
	}
}

func TestPersistInvocationLog_PreservesPayloadsWhenEnabled(t *testing.T) {
	sink := &captureSink{ch: make(chan *store.InvocationLog, 1)}
	e := New(nil, nil,
		WithLogSink(sink),
		WithLogBatcherConfig(LogBatcherConfig{BatchSize: 1, FlushInterval: time.Hour}),
		WithPayloadPersistence(true),
	)
	defer e.logBatcher.Shutdown(time.Second)

	in := json.RawMessage(`{"in":1}`)
	out := json.RawMessage(`{"out":2}`)
	fn := &domain.Function{ID: "f1", Name: "hello", Runtime: domain.RuntimePython}
	e.persistInvocationLog("req2", fn, 123, false, true, "", 7, 8, in, out, "stdout", "stderr")

	select {
	case log := <-sink.ch:
		if string(log.Input) != string(in) || string(log.Output) != string(out) {
			t.Fatalf("expected payloads to be preserved when enabled")
		}
		if log.Stdout != "stdout" || log.Stderr != "stderr" {
			t.Fatalf("expected stdout/stderr to be preserved when enabled")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for invocation log")
	}
}
