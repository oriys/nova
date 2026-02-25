package ebpf

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ProfileResult represents a detailed execution profile for a request.
type ProfileResult struct {
	RequestID     string           `json:"request_id"`
	FunctionID    string           `json:"function_id"`
	TotalDuration time.Duration   `json:"total_duration_ns"`
	Breakdown     ProfileBreakdown `json:"breakdown"`
	SyscallStats  []SyscallStat    `json:"syscall_stats,omitempty"`
	Events        []*EnrichedEvent `json:"events,omitempty"`
	CollectedAt   time.Time        `json:"collected_at"`
}

// ProfileBreakdown breaks down where time was spent.
type ProfileBreakdown struct {
	OnCPU        time.Duration `json:"on_cpu_ns"`
	OffCPU       time.Duration `json:"off_cpu_ns"`
	IOWait       time.Duration `json:"io_wait_ns"`
	SyscallTime  time.Duration `json:"syscall_time_ns"`
	PageFaults   int64         `json:"page_faults"`
	MajorFaults  int64         `json:"major_faults"`
	MinorFaults  int64         `json:"minor_faults"`
	NetworkBytes int64         `json:"network_bytes"`
}

// SyscallStat tracks per-syscall statistics.
type SyscallStat struct {
	Name      string        `json:"name"`
	Count     int64         `json:"count"`
	TotalTime time.Duration `json:"total_time_ns"`
	MaxTime   time.Duration `json:"max_time_ns"`
	AvgTime   time.Duration `json:"avg_time_ns"`
}

// AuroraIntegration provides the /debug/profile endpoint.
type AuroraIntegration struct {
	mu       sync.RWMutex
	profiles map[string]*ProfileResult // requestID -> profile
	maxAge   time.Duration
	maxSize  int
}

// NewAuroraIntegration creates a new Aurora eBPF integration.
func NewAuroraIntegration() *AuroraIntegration {
	ai := &AuroraIntegration{
		profiles: make(map[string]*ProfileResult),
		maxAge:   15 * time.Minute,
		maxSize:  10000,
	}
	go ai.cleanupLoop()
	return ai
}

// RecordProfile stores a profile result for later retrieval.
func (ai *AuroraIntegration) RecordProfile(profile *ProfileResult) {
	ai.mu.Lock()
	defer ai.mu.Unlock()

	if len(ai.profiles) >= ai.maxSize {
		ai.evictOldest()
	}

	profile.CollectedAt = time.Now()
	ai.profiles[profile.RequestID] = profile
}

// GetProfile retrieves a profile by request ID.
func (ai *AuroraIntegration) GetProfile(requestID string) (*ProfileResult, bool) {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	p, ok := ai.profiles[requestID]
	return p, ok
}

// GetProfileJSON returns the profile as formatted JSON.
func (ai *AuroraIntegration) GetProfileJSON(requestID string) ([]byte, error) {
	p, ok := ai.GetProfile(requestID)
	if !ok {
		return nil, fmt.Errorf("profile not found for request %s", requestID)
	}
	return json.MarshalIndent(p, "", "  ")
}

// ListProfiles returns profiles matching the filter.
func (ai *AuroraIntegration) ListProfiles(functionID string, limit int) []*ProfileResult {
	ai.mu.RLock()
	defer ai.mu.RUnlock()

	var results []*ProfileResult
	for _, p := range ai.profiles {
		if functionID == "" || p.FunctionID == functionID {
			results = append(results, p)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CollectedAt.After(results[j].CollectedAt)
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func (ai *AuroraIntegration) evictOldest() {
	var oldest string
	var oldestTime time.Time
	for id, p := range ai.profiles {
		if oldest == "" || p.CollectedAt.Before(oldestTime) {
			oldest = id
			oldestTime = p.CollectedAt
		}
	}
	if oldest != "" {
		delete(ai.profiles, oldest)
	}
}

func (ai *AuroraIntegration) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		ai.mu.Lock()
		cutoff := time.Now().Add(-ai.maxAge)
		for id, p := range ai.profiles {
			if p.CollectedAt.Before(cutoff) {
				delete(ai.profiles, id)
			}
		}
		ai.mu.Unlock()
	}
}

// HandleEvent implements EventSink for the daemon pipeline.
func (ai *AuroraIntegration) HandleEvent(_ context.Context, event *EnrichedEvent) error {
	ai.mu.Lock()
	defer ai.mu.Unlock()

	profile, ok := ai.profiles[event.RequestID]
	if !ok {
		profile = &ProfileResult{
			RequestID:  event.RequestID,
			FunctionID: event.FunctionID,
		}
		ai.profiles[event.RequestID] = profile
	}
	profile.Events = append(profile.Events, event)
	return nil
}

// PrometheusExporter exports eBPF metrics in Prometheus text format.
type PrometheusExporter struct {
	aurora *AuroraIntegration
}

// NewPrometheusExporter creates a Prometheus exporter.
func NewPrometheusExporter(aurora *AuroraIntegration) *PrometheusExporter {
	return &PrometheusExporter{aurora: aurora}
}

// Export returns Prometheus-formatted metrics.
func (pe *PrometheusExporter) Export() string {
	var b strings.Builder

	b.WriteString("# HELP nova_ebpf_syscall_duration_seconds Syscall duration by function\n")
	b.WriteString("# TYPE nova_ebpf_syscall_duration_seconds summary\n")

	b.WriteString("# HELP nova_ebpf_offcpu_seconds Off-CPU time by function\n")
	b.WriteString("# TYPE nova_ebpf_offcpu_seconds summary\n")

	b.WriteString("# HELP nova_ebpf_page_faults_total Page faults by function\n")
	b.WriteString("# TYPE nova_ebpf_page_faults_total counter\n")

	b.WriteString("# HELP nova_ebpf_io_wait_seconds IO wait time by function\n")
	b.WriteString("# TYPE nova_ebpf_io_wait_seconds summary\n")

	b.WriteString("# HELP nova_ebpf_network_bytes_total Network bytes by function\n")
	b.WriteString("# TYPE nova_ebpf_network_bytes_total counter\n")

	pe.aurora.mu.RLock()
	funcStats := make(map[string]*ProfileBreakdown)
	funcCounts := make(map[string]int)
	for _, p := range pe.aurora.profiles {
		stats, ok := funcStats[p.FunctionID]
		if !ok {
			stats = &ProfileBreakdown{}
			funcStats[p.FunctionID] = stats
		}
		stats.OffCPU += p.Breakdown.OffCPU
		stats.IOWait += p.Breakdown.IOWait
		stats.SyscallTime += p.Breakdown.SyscallTime
		stats.PageFaults += p.Breakdown.PageFaults
		stats.MajorFaults += p.Breakdown.MajorFaults
		stats.MinorFaults += p.Breakdown.MinorFaults
		stats.NetworkBytes += p.Breakdown.NetworkBytes
		funcCounts[p.FunctionID]++
	}
	pe.aurora.mu.RUnlock()

	for funcID, stats := range funcStats {
		count := funcCounts[funcID]
		if count > 0 {
			avgSyscall := float64(stats.SyscallTime.Nanoseconds()) / float64(count) / 1e9
			avgOffCPU := float64(stats.OffCPU.Nanoseconds()) / float64(count) / 1e9
			avgIOWait := float64(stats.IOWait.Nanoseconds()) / float64(count) / 1e9
			fmt.Fprintf(&b, "nova_ebpf_syscall_duration_seconds{function=%q,quantile=\"avg\"} %.9f\n", funcID, avgSyscall)
			fmt.Fprintf(&b, "nova_ebpf_offcpu_seconds{function=%q,quantile=\"avg\"} %.9f\n", funcID, avgOffCPU)
			fmt.Fprintf(&b, "nova_ebpf_io_wait_seconds{function=%q,quantile=\"avg\"} %.9f\n", funcID, avgIOWait)
		}
		fmt.Fprintf(&b, "nova_ebpf_page_faults_total{function=%q,type=\"major\"} %d\n", funcID, stats.MajorFaults)
		fmt.Fprintf(&b, "nova_ebpf_page_faults_total{function=%q,type=\"minor\"} %d\n", funcID, stats.MinorFaults)
		fmt.Fprintf(&b, "nova_ebpf_network_bytes_total{function=%q} %d\n", funcID, stats.NetworkBytes)
	}

	b.WriteString("# HELP nova_ebpf_profiles_total Total profiles collected\n")
	b.WriteString("# TYPE nova_ebpf_profiles_total gauge\n")
	pe.aurora.mu.RLock()
	fmt.Fprintf(&b, "nova_ebpf_profiles_total %d\n", len(pe.aurora.profiles))
	pe.aurora.mu.RUnlock()

	return b.String()
}
