// Package ebpf provides kernel-level observability via eBPF probes.
// It correlates kernel events (syscalls, scheduling, IO, page faults)
// with Nova function invocations via cgroup/pid mapping.
//
// The actual BPF CO-RE programs are compiled separately and loaded at runtime.
// This package provides the Go-side framework for:
//   - Loading and managing eBPF programs
//   - Reading perf ring buffers
//   - Correlating events with request IDs
//   - Aggregating metrics per function
package ebpf

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ProbeType identifies the kind of eBPF probe.
type ProbeType string

const (
	ProbeSyscall   ProbeType = "syscall"   // Syscall entry/exit latency
	ProbeScheduler ProbeType = "scheduler" // Off-CPU time (sched_switch/sched_wakeup)
	ProbeIO        ProbeType = "io"        // Block IO latency
	ProbePageFault ProbeType = "pagefault" // Major/minor page faults
	ProbeNetwork   ProbeType = "network"   // TCP send/recv latency
)

// Event represents a kernel event captured by an eBPF probe.
type Event struct {
	Type       ProbeType `json:"type"`
	Timestamp  uint64    `json:"timestamp_ns"`
	PID        uint32    `json:"pid"`
	TID        uint32    `json:"tid"`
	CgroupID   uint64    `json:"cgroup_id"`
	RequestID  string    `json:"request_id,omitempty"` // Populated by correlation
	FunctionID string    `json:"function_id,omitempty"`
	Duration   uint64    `json:"duration_ns"`

	// Probe-specific fields
	Syscall   string `json:"syscall,omitempty"`    // For ProbeSyscall
	IODevice  string `json:"io_device,omitempty"`  // For ProbeIO
	IOBytes   uint64 `json:"io_bytes,omitempty"`   // For ProbeIO
	FaultType string `json:"fault_type,omitempty"` // "major" or "minor" for ProbePageFault
	NetBytes  uint64 `json:"net_bytes,omitempty"`  // For ProbeNetwork
}

// CorrelationEntry maps a process to a Nova invocation.
type CorrelationEntry struct {
	PID        uint32
	CgroupID   uint64
	FunctionID string
	RequestID  string
	StartTime  time.Time
}

// Correlator maps kernel process/cgroup IDs to Nova request context.
type Correlator struct {
	mu       sync.RWMutex
	byPID    map[uint32]*CorrelationEntry
	byCgroup map[uint64]*CorrelationEntry
}

// NewCorrelator creates a new event correlator.
func NewCorrelator() *Correlator {
	return &Correlator{
		byPID:    make(map[uint32]*CorrelationEntry),
		byCgroup: make(map[uint64]*CorrelationEntry),
	}
}

// Register associates a PID/cgroup with a function invocation.
func (c *Correlator) Register(entry *CorrelationEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byPID[entry.PID] = entry
	if entry.CgroupID != 0 {
		c.byCgroup[entry.CgroupID] = entry
	}
}

// Unregister removes a PID/cgroup association.
func (c *Correlator) Unregister(pid uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.byPID[pid]; ok {
		delete(c.byPID, pid)
		if entry.CgroupID != 0 {
			delete(c.byCgroup, entry.CgroupID)
		}
	}
}

// Lookup finds the invocation context for a kernel event.
func (c *Correlator) Lookup(pid uint32, cgroupID uint64) (*CorrelationEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if entry, ok := c.byPID[pid]; ok {
		return entry, true
	}
	if cgroupID != 0 {
		if entry, ok := c.byCgroup[cgroupID]; ok {
			return entry, true
		}
	}
	return nil, false
}

// FunctionProfile aggregates kernel metrics for a single function.
type FunctionProfile struct {
	FunctionID string        `json:"function_id"`
	Window     time.Duration `json:"window"`

	// Syscall metrics
	SyscallCount    int64            `json:"syscall_count"`
	SyscallDuration time.Duration    `json:"syscall_duration_total"`
	TopSyscalls     map[string]int64 `json:"top_syscalls"` // syscall name -> count

	// Scheduler metrics
	OffCPUDuration time.Duration `json:"off_cpu_duration_total"`
	OffCPUCount    int64         `json:"off_cpu_count"`

	// IO metrics
	IOReadBytes    uint64        `json:"io_read_bytes"`
	IOWriteBytes   uint64        `json:"io_write_bytes"`
	IOLatencyTotal time.Duration `json:"io_latency_total"`
	IOCount        int64         `json:"io_count"`

	// Page fault metrics
	MajorFaults int64 `json:"major_faults"`
	MinorFaults int64 `json:"minor_faults"`

	// Network metrics
	NetTxBytes      uint64        `json:"net_tx_bytes"`
	NetRxBytes      uint64        `json:"net_rx_bytes"`
	NetLatencyTotal time.Duration `json:"net_latency_total"`
}

// Aggregator collects and aggregates eBPF events per function.
type Aggregator struct {
	mu       sync.Mutex
	profiles map[string]*FunctionProfile // functionID -> profile
	window   time.Duration
}

// NewAggregator creates a new metrics aggregator.
func NewAggregator(window time.Duration) *Aggregator {
	return &Aggregator{
		profiles: make(map[string]*FunctionProfile),
		window:   window,
	}
}

// Record processes a single eBPF event.
func (a *Aggregator) Record(event *Event) {
	if event.FunctionID == "" {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	p, ok := a.profiles[event.FunctionID]
	if !ok {
		p = &FunctionProfile{
			FunctionID:  event.FunctionID,
			Window:      a.window,
			TopSyscalls: make(map[string]int64),
		}
		a.profiles[event.FunctionID] = p
	}

	dur := time.Duration(event.Duration)
	switch event.Type {
	case ProbeSyscall:
		p.SyscallCount++
		p.SyscallDuration += dur
		if event.Syscall != "" {
			p.TopSyscalls[event.Syscall]++
		}
	case ProbeScheduler:
		p.OffCPUCount++
		p.OffCPUDuration += dur
	case ProbeIO:
		p.IOCount++
		p.IOLatencyTotal += dur
		p.IOReadBytes += event.IOBytes // Simplified; real impl distinguishes R/W
	case ProbePageFault:
		if event.FaultType == "major" {
			p.MajorFaults++
		} else {
			p.MinorFaults++
		}
	case ProbeNetwork:
		p.NetLatencyTotal += dur
		p.NetTxBytes += event.NetBytes
	}
}

// GetProfile returns the aggregated profile for a function.
func (a *Aggregator) GetProfile(functionID string) (*FunctionProfile, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p, ok := a.profiles[functionID]
	if !ok {
		return nil, false
	}
	// Return a copy
	cp := *p
	cp.TopSyscalls = make(map[string]int64, len(p.TopSyscalls))
	for k, v := range p.TopSyscalls {
		cp.TopSyscalls[k] = v
	}
	return &cp, true
}

// Flush exports aggregated data and resets for the next window.
func (a *Aggregator) Flush() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.profiles = make(map[string]*FunctionProfile)
}

// Reset clears all aggregated data.
func (a *Aggregator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.profiles = make(map[string]*FunctionProfile)
}

// Config for the eBPF collector.
type Config struct {
	Enabled     bool          `json:"enabled"`
	Probes      []ProbeType   `json:"probes"`       // Which probes to enable
	SampleRate  float64       `json:"sample_rate"`   // 0.0-1.0, fraction of events to capture
	BufferPages int           `json:"buffer_pages"`  // Perf ring buffer size in pages
	Window      time.Duration `json:"window"`        // Aggregation window
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:     false,
		Probes:      []ProbeType{ProbeSyscall, ProbeScheduler, ProbeIO, ProbePageFault},
		SampleRate:  1.0,
		BufferPages: 64,
		Window:      time.Minute,
	}
}

// Collector is the main eBPF data collection daemon.
type Collector struct {
	cfg        Config
	correlator *Correlator
	aggregator *Aggregator
	eventCh    chan *Event
	running    atomic.Bool
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewCollector creates a new eBPF collector.
func NewCollector(cfg Config) *Collector {
	ctx, cancel := context.WithCancel(context.Background())
	return &Collector{
		cfg:        cfg,
		correlator: NewCorrelator(),
		aggregator: NewAggregator(cfg.Window),
		eventCh:    make(chan *Event, 8192),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start begins collecting eBPF events.
// In a real implementation, this would load BPF programs and start reading perf buffers.
func (c *Collector) Start() error {
	if !c.cfg.Enabled {
		return nil
	}
	c.running.Store(true)
	go c.processLoop()
	return nil
}

// Stop shuts down the collector.
func (c *Collector) Stop() {
	c.running.Store(false)
	c.cancel()
}

// Correlator returns the event correlator for registering process mappings.
func (c *Collector) Correlator() *Correlator {
	return c.correlator
}

// Aggregator returns the metrics aggregator.
func (c *Collector) Aggregator() *Aggregator {
	return c.aggregator
}

// Submit adds an event to the processing pipeline.
func (c *Collector) Submit(event *Event) {
	if !c.running.Load() {
		return
	}
	select {
	case c.eventCh <- event:
	default:
		// Drop event if channel full
	}
}

func (c *Collector) processLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case event := <-c.eventCh:
			// Correlate event with function context
			if entry, ok := c.correlator.Lookup(event.PID, event.CgroupID); ok {
				event.FunctionID = entry.FunctionID
				event.RequestID = entry.RequestID
			}
			c.aggregator.Record(event)
		}
	}
}
