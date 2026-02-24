package metrics

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestMetrics creates an isolated Metrics instance for testing,
// avoiding interference with the package-level global singleton.
func newTestMetrics(t *testing.T) *Metrics {
	t.Helper()
	m := &Metrics{startTime: time.Now()}
	m.MinLatencyMs.Store(math.MaxInt64)
	m.tsChan = make(chan timeSeriesEvent, 8192)
	m.initTimeSeries()
	go m.processTimeSeriesLoop()
	t.Cleanup(func() { close(m.tsChan) })
	return m
}

func TestGlobal(t *testing.T) {
	t.Parallel()
	g := Global()
	if g == nil {
		t.Fatal("Global() returned nil")
	}
	if g2 := Global(); g2 != g {
		t.Error("Global() did not return the same instance")
	}
}

func TestNewTestMetrics(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)
	if m == nil {
		t.Fatal("newTestMetrics returned nil")
	}
	if m.startTime.IsZero() {
		t.Error("startTime is zero")
	}
	if m.MinLatencyMs.Load() != math.MaxInt64 {
		t.Error("MinLatencyMs not initialized to MaxInt64")
	}
}

func TestRecordInvocation_Success_Cold(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordInvocation("fn1", 42, true, true)

	if got := m.TotalInvocations.Load(); got != 1 {
		t.Errorf("TotalInvocations = %d, want 1", got)
	}
	if got := m.SuccessInvocations.Load(); got != 1 {
		t.Errorf("SuccessInvocations = %d, want 1", got)
	}
	if got := m.FailedInvocations.Load(); got != 0 {
		t.Errorf("FailedInvocations = %d, want 0", got)
	}
	if got := m.ColdStarts.Load(); got != 1 {
		t.Errorf("ColdStarts = %d, want 1", got)
	}
	if got := m.WarmStarts.Load(); got != 0 {
		t.Errorf("WarmStarts = %d, want 0", got)
	}
}

func TestRecordInvocation_Failure_Warm(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordInvocation("fn1", 100, false, false)

	if got := m.TotalInvocations.Load(); got != 1 {
		t.Errorf("TotalInvocations = %d, want 1", got)
	}
	if got := m.SuccessInvocations.Load(); got != 0 {
		t.Errorf("SuccessInvocations = %d, want 0", got)
	}
	if got := m.FailedInvocations.Load(); got != 1 {
		t.Errorf("FailedInvocations = %d, want 1", got)
	}
	if got := m.ColdStarts.Load(); got != 0 {
		t.Errorf("ColdStarts = %d, want 0", got)
	}
	if got := m.WarmStarts.Load(); got != 1 {
		t.Errorf("WarmStarts = %d, want 1", got)
	}
}

func TestRecordInvocation_Invariants(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordInvocation("fn1", 10, true, true)
	m.RecordInvocation("fn2", 20, false, false)
	m.RecordInvocation("fn3", 30, true, false)
	m.RecordInvocation("fn4", 40, false, true)

	total := m.TotalInvocations.Load()
	success := m.SuccessInvocations.Load()
	failed := m.FailedInvocations.Load()
	cold := m.ColdStarts.Load()
	warm := m.WarmStarts.Load()

	if total != success+failed {
		t.Errorf("invariant broken: total(%d) != success(%d) + failed(%d)", total, success, failed)
	}
	if total != cold+warm {
		t.Errorf("invariant broken: total(%d) != cold(%d) + warm(%d)", total, cold, warm)
	}
}

func TestRecordInvocationWithDetails(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordInvocationWithDetails("fn-abc", "myFunc", "python", 55, true, true)
	m.RecordInvocationWithDetails("fn-abc", "myFunc", "python", 105, false, false)

	if got := m.TotalInvocations.Load(); got != 2 {
		t.Errorf("TotalInvocations = %d, want 2", got)
	}
	if got := m.SuccessInvocations.Load(); got != 1 {
		t.Errorf("SuccessInvocations = %d, want 1", got)
	}
	if got := m.FailedInvocations.Load(); got != 1 {
		t.Errorf("FailedInvocations = %d, want 1", got)
	}
	if got := m.ColdStarts.Load(); got != 1 {
		t.Errorf("ColdStarts = %d, want 1", got)
	}
	if got := m.WarmStarts.Load(); got != 1 {
		t.Errorf("WarmStarts = %d, want 1", got)
	}
}

func TestGetFunctionMetrics(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	t.Run("nil before any recording", func(t *testing.T) {
		if fm := m.GetFunctionMetrics("does-not-exist"); fm != nil {
			t.Error("expected nil for unrecorded function")
		}
	})

	t.Run("returns correct counters", func(t *testing.T) {
		m.RecordInvocation("fn-x", 10, true, true)
		m.RecordInvocation("fn-x", 20, false, false)
		m.RecordInvocation("fn-x", 30, true, true)

		fm := m.GetFunctionMetrics("fn-x")
		if fm == nil {
			t.Fatal("expected non-nil FunctionMetrics")
		}
		if got := fm.Invocations.Load(); got != 3 {
			t.Errorf("Invocations = %d, want 3", got)
		}
		if got := fm.Successes.Load(); got != 2 {
			t.Errorf("Successes = %d, want 2", got)
		}
		if got := fm.Failures.Load(); got != 1 {
			t.Errorf("Failures = %d, want 1", got)
		}
		if got := fm.ColdStarts.Load(); got != 2 {
			t.Errorf("ColdStarts = %d, want 2", got)
		}
		if got := fm.WarmStarts.Load(); got != 1 {
			t.Errorf("WarmStarts = %d, want 1", got)
		}
		if got := fm.TotalMs.Load(); got != 60 {
			t.Errorf("TotalMs = %d, want 60", got)
		}
		if got := fm.MinMs.Load(); got != 10 {
			t.Errorf("MinMs = %d, want 10", got)
		}
		if got := fm.MaxMs.Load(); got != 30 {
			t.Errorf("MaxMs = %d, want 30", got)
		}
	})

	t.Run("different functions are isolated", func(t *testing.T) {
		m.RecordInvocation("fn-a", 5, true, true)
		m.RecordInvocation("fn-b", 15, false, false)

		fmA := m.GetFunctionMetrics("fn-a")
		fmB := m.GetFunctionMetrics("fn-b")
		if fmA == nil || fmB == nil {
			t.Fatal("expected non-nil FunctionMetrics for both")
		}
		if fmA.Invocations.Load() < 1 {
			t.Error("fn-a should have at least 1 invocation")
		}
		if fmB.Failures.Load() < 1 {
			t.Error("fn-b should have at least 1 failure")
		}
	})
}

func TestRecordVM(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordVMCreated()
	m.RecordVMCreated()
	m.RecordVMStopped()
	m.RecordVMCrashed()
	m.RecordSnapshotHit()
	m.RecordSnapshotHit()
	m.RecordSnapshotHit()

	if got := m.VMsCreated.Load(); got != 2 {
		t.Errorf("VMsCreated = %d, want 2", got)
	}
	if got := m.VMsStopped.Load(); got != 1 {
		t.Errorf("VMsStopped = %d, want 1", got)
	}
	if got := m.VMsCrashed.Load(); got != 1 {
		t.Errorf("VMsCrashed = %d, want 1", got)
	}
	if got := m.SnapshotsHit.Load(); got != 3 {
		t.Errorf("SnapshotsHit = %d, want 3", got)
	}
}

func TestSnapshot(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordInvocation("fn1", 100, true, true)
	m.RecordInvocation("fn1", 200, false, false)
	m.RecordVMCreated()

	snap := m.Snapshot()

	assertSnapshotKey := func(key string) {
		t.Helper()
		if _, ok := snap[key]; !ok {
			t.Errorf("Snapshot missing key %q", key)
		}
	}
	assertSnapshotKey("uptime_seconds")
	assertSnapshotKey("invocations")
	assertSnapshotKey("latency_ms")
	assertSnapshotKey("vms")
	assertSnapshotKey("ts_dropped_events")

	inv, ok := snap["invocations"].(map[string]interface{})
	if !ok {
		t.Fatal("invocations is not a map")
	}
	if got, want := inv["total"].(int64), int64(2); got != want {
		t.Errorf("invocations.total = %d, want %d", got, want)
	}
	if got, want := inv["success"].(int64), int64(1); got != want {
		t.Errorf("invocations.success = %d, want %d", got, want)
	}
	if got, want := inv["failed"].(int64), int64(1); got != want {
		t.Errorf("invocations.failed = %d, want %d", got, want)
	}
	if got, want := inv["cold"].(int64), int64(1); got != want {
		t.Errorf("invocations.cold = %d, want %d", got, want)
	}
	if got, want := inv["warm"].(int64), int64(1); got != want {
		t.Errorf("invocations.warm = %d, want %d", got, want)
	}

	latency, ok := snap["latency_ms"].(map[string]interface{})
	if !ok {
		t.Fatal("latency_ms is not a map")
	}
	if got := latency["min"].(int64); got != 100 {
		t.Errorf("latency_ms.min = %d, want 100", got)
	}
	if got := latency["max"].(int64); got != 200 {
		t.Errorf("latency_ms.max = %d, want 200", got)
	}
	if got := latency["avg"].(float64); got != 150 {
		t.Errorf("latency_ms.avg = %f, want 150", got)
	}

	vms, ok := snap["vms"].(map[string]interface{})
	if !ok {
		t.Fatal("vms is not a map")
	}
	if got := vms["created"].(int64); got != 1 {
		t.Errorf("vms.created = %d, want 1", got)
	}
}

func TestSnapshot_ZeroInvocations(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	snap := m.Snapshot()
	latency := snap["latency_ms"].(map[string]interface{})
	if got := latency["min"].(int64); got != 0 {
		t.Errorf("latency_ms.min = %d, want 0 when no invocations", got)
	}
	if got := latency["avg"].(float64); got != 0 {
		t.Errorf("latency_ms.avg = %f, want 0 when no invocations", got)
	}
}

func TestFunctionStats(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordInvocation("fn-alpha", 10, true, true)
	m.RecordInvocation("fn-alpha", 50, false, true)
	m.RecordInvocation("fn-beta", 100, true, false)

	stats := m.FunctionStats()
	if len(stats) < 2 {
		t.Fatalf("expected at least 2 functions in stats, got %d", len(stats))
	}

	alpha, ok := stats["fn-alpha"].(map[string]interface{})
	if !ok {
		t.Fatal("fn-alpha not found in FunctionStats")
	}
	if got := alpha["invocations"].(int64); got != 2 {
		t.Errorf("fn-alpha invocations = %d, want 2", got)
	}
	if got := alpha["successes"].(int64); got != 2 {
		t.Errorf("fn-alpha successes = %d, want 2", got)
	}
	if got := alpha["min_ms"].(int64); got != 10 {
		t.Errorf("fn-alpha min_ms = %d, want 10", got)
	}
	if got := alpha["max_ms"].(int64); got != 50 {
		t.Errorf("fn-alpha max_ms = %d, want 50", got)
	}

	beta, ok := stats["fn-beta"].(map[string]interface{})
	if !ok {
		t.Fatal("fn-beta not found in FunctionStats")
	}
	if got := beta["failures"].(int64); got != 1 {
		t.Errorf("fn-beta failures = %d, want 1", got)
	}
}

func TestFunctionStats_NoInvocations(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	stats := m.FunctionStats()
	if len(stats) != 0 {
		t.Errorf("expected empty FunctionStats, got %d entries", len(stats))
	}
}

func TestJSONHandler(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordInvocation("fn1", 42, true, true)
	m.RecordVMCreated()

	handler := m.JSONHandler()
	if handler == nil {
		t.Fatal("JSONHandler returned nil")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	for _, key := range []string{"uptime_seconds", "invocations", "latency_ms", "vms", "functions"} {
		if _, ok := result[key]; !ok {
			t.Errorf("JSON response missing key %q", key)
		}
	}
}

func TestTimeSeriesHandler(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordInvocation("fn-tsh", 50, true, true)

	handler := m.TimeSeriesHandler()
	if handler == nil {
		t.Fatal("TimeSeriesHandler returned nil")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/timeseries", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(result) != timeSeriesBucketCount {
		t.Errorf("time series length = %d, want %d", len(result), timeSeriesBucketCount)
	}
	// Each entry must have the expected keys.
	for _, key := range []string{"timestamp", "invocations", "errors", "avg_duration"} {
		if _, ok := result[0][key]; !ok {
			t.Errorf("time series entry missing key %q", key)
		}
	}
}

func TestLatencyTracking(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordInvocation("fn1", 500, true, true)
	m.RecordInvocation("fn1", 100, false, true)
	m.RecordInvocation("fn1", 300, false, true)

	if got := m.MinLatencyMs.Load(); got != 100 {
		t.Errorf("MinLatencyMs = %d, want 100", got)
	}
	if got := m.MaxLatencyMs.Load(); got != 500 {
		t.Errorf("MaxLatencyMs = %d, want 500", got)
	}
	if got := m.TotalLatencyMs.Load(); got != 900 {
		t.Errorf("TotalLatencyMs = %d, want 900", got)
	}
}

func TestUpdateMinMax_CAS(t *testing.T) {
	t.Parallel()

	t.Run("updateMin", func(t *testing.T) {
		var v atomic.Int64
		v.Store(50)
		updateMin(&v, 30)
		if got := v.Load(); got != 30 {
			t.Errorf("updateMin: got %d, want 30", got)
		}
		updateMin(&v, 40)
		if got := v.Load(); got != 30 {
			t.Errorf("updateMin should not change: got %d, want 30", got)
		}
		updateMin(&v, 30)
		if got := v.Load(); got != 30 {
			t.Errorf("updateMin with equal value: got %d, want 30", got)
		}
	})

	t.Run("updateMax", func(t *testing.T) {
		var v atomic.Int64
		v.Store(50)
		updateMax(&v, 80)
		if got := v.Load(); got != 80 {
			t.Errorf("updateMax: got %d, want 80", got)
		}
		updateMax(&v, 60)
		if got := v.Load(); got != 80 {
			t.Errorf("updateMax should not change: got %d, want 80", got)
		}
		updateMax(&v, 80)
		if got := v.Load(); got != 80 {
			t.Errorf("updateMax with equal value: got %d, want 80", got)
		}
	})
}

func TestColdStartPercentage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cold  int64
		total int64
		want  float64
	}{
		{"zero total returns 0", 0, 0, 0},
		{"all cold", 10, 10, 100},
		{"none cold", 0, 10, 0},
		{"half cold", 5, 10, 50},
		{"one third cold", 1, 3, 100.0 / 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := coldStartPercentage(tt.cold, tt.total)
			if math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("coldStartPercentage(%d, %d) = %f, want %f", tt.cold, tt.total, got, tt.want)
			}
		})
	}
}

func TestConcurrentRecordInvocation(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	const goroutines = 100
	const perGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				cold := j%2 == 0
				success := j%3 != 0
				m.RecordInvocation("concurrent-fn", int64(j+1), cold, success)
			}
		}(i)
	}
	wg.Wait()

	total := m.TotalInvocations.Load()
	success := m.SuccessInvocations.Load()
	failed := m.FailedInvocations.Load()
	cold := m.ColdStarts.Load()
	warm := m.WarmStarts.Load()

	wantTotal := int64(goroutines * perGoroutine)
	if total != wantTotal {
		t.Errorf("TotalInvocations = %d, want %d", total, wantTotal)
	}
	if total != success+failed {
		t.Errorf("invariant broken: total(%d) != success(%d) + failed(%d)", total, success, failed)
	}
	if total != cold+warm {
		t.Errorf("invariant broken: total(%d) != cold(%d) + warm(%d)", total, cold, warm)
	}

	fm := m.GetFunctionMetrics("concurrent-fn")
	if fm == nil {
		t.Fatal("expected non-nil FunctionMetrics for concurrent-fn")
	}
	if fm.Invocations.Load() != wantTotal {
		t.Errorf("per-function invocations = %d, want %d", fm.Invocations.Load(), wantTotal)
	}
	fmTotal := fm.Invocations.Load()
	if fmTotal != fm.Successes.Load()+fm.Failures.Load() {
		t.Error("per-function invariant broken: invocations != successes + failures")
	}
	if fmTotal != fm.ColdStarts.Load()+fm.WarmStarts.Load() {
		t.Error("per-function invariant broken: invocations != cold + warm")
	}
}

func TestConcurrentVMCounters(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	const n = 100
	var wg sync.WaitGroup
	wg.Add(4 * n)
	for i := 0; i < n; i++ {
		go func() { defer wg.Done(); m.RecordVMCreated() }()
		go func() { defer wg.Done(); m.RecordVMStopped() }()
		go func() { defer wg.Done(); m.RecordVMCrashed() }()
		go func() { defer wg.Done(); m.RecordSnapshotHit() }()
	}
	wg.Wait()

	if got := m.VMsCreated.Load(); got != int64(n) {
		t.Errorf("VMsCreated = %d, want %d", got, n)
	}
	if got := m.VMsStopped.Load(); got != int64(n) {
		t.Errorf("VMsStopped = %d, want %d", got, n)
	}
	if got := m.VMsCrashed.Load(); got != int64(n) {
		t.Errorf("VMsCrashed = %d, want %d", got, n)
	}
	if got := m.SnapshotsHit.Load(); got != int64(n) {
		t.Errorf("SnapshotsHit = %d, want %d", got, n)
	}
}

func TestTimeSeries(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordInvocation("fn-ts", 100, true, true)
	m.RecordInvocation("fn-ts", 200, false, false)

	// Drain channel to ensure events are processed.
	time.Sleep(50 * time.Millisecond)

	ts := m.TimeSeries()
	if len(ts) != timeSeriesBucketCount {
		t.Errorf("TimeSeries length = %d, want %d", len(ts), timeSeriesBucketCount)
	}

	// The last bucket should have our invocations.
	last := ts[len(ts)-1]
	invCount, _ := last["invocations"].(int64)
	errCount, _ := last["errors"].(int64)
	if invCount < 2 {
		t.Errorf("last bucket invocations = %d, want >= 2", invCount)
	}
	if errCount < 1 {
		t.Errorf("last bucket errors = %d, want >= 1", errCount)
	}
}

func TestSnapshotColdStartPercentage(t *testing.T) {
	t.Parallel()
	m := newTestMetrics(t)

	m.RecordInvocation("fn1", 10, true, true)
	m.RecordInvocation("fn1", 10, true, true)
	m.RecordInvocation("fn1", 10, false, true)
	m.RecordInvocation("fn1", 10, false, true)

	snap := m.Snapshot()
	inv := snap["invocations"].(map[string]interface{})
	pct := inv["cold_pct"].(float64)
	if pct != 50 {
		t.Errorf("cold_pct = %f, want 50", pct)
	}
}

func TestStartTime(t *testing.T) {
	t.Parallel()
	st := StartTime()
	if st.IsZero() {
		t.Error("StartTime() returned zero time")
	}
	if time.Since(st) < 0 {
		t.Error("StartTime() is in the future")
	}
}

// --- Prometheus tests ---

// initPromForTest saves/restores the package-level promMetrics and initialises fresh.
func initPromForTest(t *testing.T) {
	t.Helper()
	old := promMetrics
	t.Cleanup(func() { promMetrics = old })
	promMetrics = nil
	InitPrometheus("test", nil)
}

func TestInitPrometheus(t *testing.T) {
	old := promMetrics
	defer func() { promMetrics = old }()
	promMetrics = nil

	InitPrometheus("test", nil)
	if promMetrics == nil {
		t.Fatal("expected promMetrics to be initialized")
	}
	if PrometheusRegistry() == nil {
		t.Fatal("expected registry to be non-nil")
	}
}

func TestInitPrometheus_CustomBuckets(t *testing.T) {
	old := promMetrics
	defer func() { promMetrics = old }()
	promMetrics = nil

	InitPrometheus("test", []float64{1, 10, 100})
	if promMetrics == nil {
		t.Fatal("expected promMetrics to be initialized with custom buckets")
	}
	if PrometheusRegistry() == nil {
		t.Fatal("expected registry to be non-nil")
	}
}

func TestRecordPrometheusInvocation(t *testing.T) {
	initPromForTest(t)
	// cold start + success
	RecordPrometheusInvocation("fn1", "python", 100, true, true)
	// warm start + failure
	RecordPrometheusInvocation("fn1", "python", 50, false, false)
}

func TestRecordPrometheusInvocation_NilSafe(t *testing.T) {
	old := promMetrics
	defer func() { promMetrics = old }()
	promMetrics = nil

	// All public prometheus functions must be safe to call with nil promMetrics.
	RecordPrometheusInvocation("fn1", "python", 100, true, true)
	RecordPrometheusVMCreated()
	RecordPrometheusVMStopped()
	RecordPrometheusVMCrashed()
	RecordPrometheusSnapshotHit()
	SetVMPoolSize("fn1", 1, 1)
	RecordVMBootDuration("fn1", "python", 100, true)
	RecordSnapshotRestoreTime("fn1", 50)
	RecordVsockLatency("connect", 1.5)
	IncActiveRequests()
	DecActiveRequests()
	SetActiveVMs(5)
	SetAutoscaleDesiredReplicas("fn1", 3)
	RecordAutoscaleDecision("fn1", "up")
	RecordAdmissionResult("fn1", "admit", "ok")
	RecordShed("fn1", "overload")
	SetQueueDepth("fn1", 10)
	SetQueueWaitMs("fn1", 200)
	SetCircuitBreakerState("fn1", 1)
	RecordCircuitBreakerTrip("fn1", "open")
	RecordLogBatcherDrop()
	RecordLogBatcherFlushFailed()
}

func TestPrometheusVMFunctions(t *testing.T) {
	initPromForTest(t)
	RecordPrometheusVMCreated()
	RecordPrometheusVMStopped()
	RecordPrometheusVMCrashed()
	RecordPrometheusSnapshotHit()
}

func TestSetVMPoolSize(t *testing.T) {
	initPromForTest(t)
	SetVMPoolSize("fn1", 3, 2)
}

func TestSetVMPoolSize_ZeroTotal(t *testing.T) {
	initPromForTest(t)
	SetVMPoolSize("fn1", 0, 0)
}

func TestRecordVMBootDuration(t *testing.T) {
	initPromForTest(t)
	RecordVMBootDuration("fn1", "python", 200, true)
	RecordVMBootDuration("fn1", "python", 300, false)
}

func TestRecordSnapshotRestoreTime(t *testing.T) {
	initPromForTest(t)
	RecordSnapshotRestoreTime("fn1", 150)
}

func TestRecordVsockLatency(t *testing.T) {
	initPromForTest(t)
	RecordVsockLatency("connect", 2.5)
}

func TestActiveRequests(t *testing.T) {
	initPromForTest(t)
	IncActiveRequests()
	DecActiveRequests()
}

func TestSetActiveVMs(t *testing.T) {
	initPromForTest(t)
	SetActiveVMs(5)
}

func TestPrometheusHandler(t *testing.T) {
	t.Run("nil returns 503", func(t *testing.T) {
		old := promMetrics
		defer func() { promMetrics = old }()
		promMetrics = nil

		h := PrometheusHandler()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("initialized returns 200", func(t *testing.T) {
		initPromForTest(t)

		h := PrometheusHandler()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})
}

func TestPrometheusRegistry_Nil(t *testing.T) {
	old := promMetrics
	defer func() { promMetrics = old }()
	promMetrics = nil

	if reg := PrometheusRegistry(); reg != nil {
		t.Error("expected nil registry when promMetrics is nil")
	}
}

func TestAutoscaleFunctions(t *testing.T) {
	initPromForTest(t)
	SetAutoscaleDesiredReplicas("fn1", 5)
	RecordAutoscaleDecision("fn1", "up")
	RecordAutoscaleDecision("fn1", "down")
}

func TestAdmissionFunctions(t *testing.T) {
	initPromForTest(t)
	RecordAdmissionResult("fn1", "admit", "ok")
	RecordAdmissionResult("fn1", "reject", "overload")
	RecordShed("fn1", "overload")
}

func TestQueueFunctions(t *testing.T) {
	initPromForTest(t)
	SetQueueDepth("fn1", 10)
	SetQueueWaitMs("fn1", 200)
}

func TestCircuitBreakerFunctions(t *testing.T) {
	initPromForTest(t)
	SetCircuitBreakerState("fn1", 0)
	SetCircuitBreakerState("fn1", 1)
	RecordCircuitBreakerTrip("fn1", "open")
	RecordCircuitBreakerTrip("fn1", "half_open")
}

func TestLogBatcherFunctions(t *testing.T) {
	initPromForTest(t)
	RecordLogBatcherDrop()
	RecordLogBatcherFlushFailed()
}

func TestAllPrometheusNilSafe(t *testing.T) {
	old := promMetrics
	defer func() { promMetrics = old }()
	promMetrics = nil

	RecordPrometheusInvocation("f", "r", 1, true, true)
	RecordPrometheusInvocation("f", "r", 1, false, false)
	RecordPrometheusVMCreated()
	RecordPrometheusVMStopped()
	RecordPrometheusVMCrashed()
	RecordPrometheusSnapshotHit()
	SetVMPoolSize("f", 1, 1)
	SetVMPoolSize("f", 0, 0)
	RecordVMBootDuration("f", "r", 1, true)
	RecordVMBootDuration("f", "r", 1, false)
	RecordSnapshotRestoreTime("f", 1)
	RecordVsockLatency("connect", 1.0)
	IncActiveRequests()
	DecActiveRequests()
	SetActiveVMs(0)
	SetAutoscaleDesiredReplicas("f", 0)
	RecordAutoscaleDecision("f", "up")
	RecordAdmissionResult("f", "admit", "ok")
	RecordShed("f", "overload")
	SetQueueDepth("f", 0)
	SetQueueWaitMs("f", 0)
	SetCircuitBreakerState("f", 0)
	RecordCircuitBreakerTrip("f", "open")
	RecordLogBatcherDrop()
	RecordLogBatcherFlushFailed()

	if reg := PrometheusRegistry(); reg != nil {
		t.Error("expected nil registry")
	}

	h := PrometheusHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
