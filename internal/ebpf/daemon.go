package ebpf

import (
	"context"
	"sync"
	"time"
)

// DaemonConfig configures the eBPF event processing daemon.
type DaemonConfig struct {
	BufferSize      int           // Ring buffer size for incoming events
	FlushInterval   time.Duration // How often to flush aggregated data
	MaxEventsPerSec int64         // Rate limit on processed events
	Enabled         bool          // Master switch
}

// DefaultDaemonConfig returns sensible defaults.
func DefaultDaemonConfig() DaemonConfig {
	return DaemonConfig{
		BufferSize:      65536,
		FlushInterval:   5 * time.Second,
		MaxEventsPerSec: 100000,
		Enabled:         true,
	}
}

// RawEvent represents an event from the eBPF perf ring buffer.
type RawEvent struct {
	ProbeType ProbeType `json:"probe_type"`
	Timestamp uint64    `json:"timestamp_ns"`
	PID       uint32    `json:"pid"`
	TID       uint32    `json:"tid"`
	CgroupID  uint64    `json:"cgroup_id"`
	Data      []byte    `json:"data"`
}

// EnrichedEvent is a RawEvent enriched with request context.
type EnrichedEvent struct {
	RawEvent
	RequestID  string `json:"request_id"`
	FunctionID string `json:"function_id"`
	TenantID   string `json:"tenant_id,omitempty"`
}

// EventSink receives enriched events for downstream processing.
type EventSink interface {
	HandleEvent(ctx context.Context, event *EnrichedEvent) error
}

// EventSinkFunc is an adapter for function-based sinks.
type EventSinkFunc func(ctx context.Context, event *EnrichedEvent) error

func (f EventSinkFunc) HandleEvent(ctx context.Context, event *EnrichedEvent) error {
	return f(ctx, event)
}

// Daemon is the user-space eBPF event processing daemon.
// It reads from the perf ring buffer, enriches events with request context,
// and dispatches to registered sinks.
type Daemon struct {
	mu         sync.RWMutex
	cfg        DaemonConfig
	correlator *Correlator
	aggregator *Aggregator
	sinks      []EventSink
	eventCh    chan *RawEvent
	ctx        context.Context
	cancel     context.CancelFunc

	// Stats
	eventsRead     uint64
	eventsEnriched uint64
	eventsDropped  uint64
}

// NewDaemon creates a new eBPF event processing daemon.
func NewDaemon(cfg DaemonConfig, correlator *Correlator, aggregator *Aggregator) *Daemon {
	ctx, cancel := context.WithCancel(context.Background())
	return &Daemon{
		cfg:        cfg,
		correlator: correlator,
		aggregator: aggregator,
		eventCh:    make(chan *RawEvent, cfg.BufferSize),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// AddSink registers an event sink.
func (d *Daemon) AddSink(sink EventSink) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sinks = append(d.sinks, sink)
}

// Start begins reading and processing events.
func (d *Daemon) Start() {
	if !d.cfg.Enabled {
		return
	}
	go d.processLoop()
	go d.flushLoop()
}

// Stop halts the daemon.
func (d *Daemon) Stop() {
	d.cancel()
}

// Submit enqueues a raw event for processing.
func (d *Daemon) Submit(event *RawEvent) {
	select {
	case d.eventCh <- event:
		d.eventsRead++
	default:
		d.eventsDropped++
	}
}

// Stats returns daemon statistics.
func (d *Daemon) Stats() DaemonStats {
	return DaemonStats{
		EventsRead:     d.eventsRead,
		EventsEnriched: d.eventsEnriched,
		EventsDropped:  d.eventsDropped,
		QueueLen:       len(d.eventCh),
		QueueCap:       cap(d.eventCh),
	}
}

// DaemonStats contains daemon operational statistics.
type DaemonStats struct {
	EventsRead     uint64 `json:"events_read"`
	EventsEnriched uint64 `json:"events_enriched"`
	EventsDropped  uint64 `json:"events_dropped"`
	QueueLen       int    `json:"queue_len"`
	QueueCap       int    `json:"queue_cap"`
}

func (d *Daemon) processLoop() {
	for {
		select {
		case <-d.ctx.Done():
			return
		case raw := <-d.eventCh:
			enriched := d.enrich(raw)
			if enriched != nil {
				d.dispatch(enriched)
				d.eventsEnriched++
			}
		}
	}
}

func (d *Daemon) enrich(raw *RawEvent) *EnrichedEvent {
	// Look up request context from correlator
	entry, ok := d.correlator.Lookup(raw.PID, raw.CgroupID)
	if !ok {
		return nil // Cannot correlate — drop
	}

	return &EnrichedEvent{
		RawEvent:   *raw,
		RequestID:  entry.RequestID,
		FunctionID: entry.FunctionID,
	}
}

func (d *Daemon) dispatch(event *EnrichedEvent) {
	d.mu.RLock()
	sinks := d.sinks
	d.mu.RUnlock()

	for _, sink := range sinks {
		sink.HandleEvent(d.ctx, event)
	}

	// Also feed to aggregator
	d.aggregator.Record(&Event{
		Type:       event.ProbeType,
		Timestamp:  event.Timestamp,
		PID:        event.PID,
		TID:        event.TID,
		CgroupID:   event.CgroupID,
		RequestID:  event.RequestID,
		FunctionID: event.FunctionID,
	})
}

func (d *Daemon) flushLoop() {
	ticker := time.NewTicker(d.cfg.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			// Flush aggregated metrics
			d.aggregator.Flush()
		}
	}
}
