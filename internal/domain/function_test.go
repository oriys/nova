package domain

import (
	"encoding/json"
	"testing"
)

func TestNeedsCompilation(t *testing.T) {
	tests := []struct {
		runtime Runtime
		want    bool
	}{
		{RuntimeRust, true},
		{Runtime("rust1.84"), true},
		{RuntimeGo, true},
		{Runtime("go1.23"), true},
		{RuntimePython, false},
		{Runtime("python3.12"), false},
		{RuntimeJava, true},
		{RuntimeKotlin, true},
		{RuntimeSwift, true},
		{RuntimeZig, true},
		{RuntimeScala, true},
		{RuntimeNode, false},
		{RuntimeRuby, false},
		{RuntimeDeno, false},
		{RuntimeBun, false},
		{RuntimePHP, false},
		{RuntimeWasm, false},
	}

	for _, tt := range tests {
		got := NeedsCompilation(tt.runtime)
		if got != tt.want {
			t.Fatalf("NeedsCompilation(%q) = %v, want %v", tt.runtime, got, tt.want)
		}
	}
}

func TestRuntime_IsValid(t *testing.T) {
	tests := []struct {
		runtime Runtime
		want    bool
	}{
		{RuntimePython, true},
		{RuntimeGo, true},
		{RuntimeRust, true},
		{RuntimeNode, true},
		{RuntimeWasm, true},
		{RuntimeCustom, true},
		{RuntimeProvided, true},
		{Runtime("python3.12"), true},
		{Runtime("go1.24"), true},
		{Runtime("node20"), true},
		{Runtime("unknown"), false},
		{Runtime(""), false},
	}

	for _, tt := range tests {
		got := tt.runtime.IsValid()
		if got != tt.want {
			t.Errorf("Runtime(%q).IsValid() = %v, want %v", tt.runtime, got, tt.want)
		}
	}
}

func TestIsValidBackendType(t *testing.T) {
	tests := []struct {
		backend BackendType
		want    bool
	}{
		{BackendAuto, true},
		{BackendFirecracker, true},
		{BackendDocker, true},
		{BackendWasm, true},
		{BackendKubernetes, true},
		{BackendLibKrun, true},
		{BackendKata, true},
		{BackendType("qemu"), false},
		{BackendType(""), false},
	}

	for _, tt := range tests {
		got := IsValidBackendType(tt.backend)
		if got != tt.want {
			t.Errorf("IsValidBackendType(%q) = %v, want %v", tt.backend, got, tt.want)
		}
	}
}

func TestAllBackendTypes(t *testing.T) {
	types := AllBackendTypes()
	if len(types) != 7 {
		t.Errorf("AllBackendTypes() returned %d types, want 7", len(types))
	}
}

func TestExecutionMode_Values(t *testing.T) {
	if ModeProcess != "process" {
		t.Errorf("ModeProcess = %q, want %q", ModeProcess, "process")
	}
	if ModePersistent != "persistent" {
		t.Errorf("ModePersistent = %q, want %q", ModePersistent, "persistent")
	}
}

func TestCompileStatus_Values(t *testing.T) {
	tests := []struct {
		status CompileStatus
		want   string
	}{
		{CompileStatusPending, "pending"},
		{CompileStatusCompiling, "compiling"},
		{CompileStatusSuccess, "success"},
		{CompileStatusFailed, "failed"},
		{CompileStatusNotRequired, "not_required"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("CompileStatus = %q, want %q", tt.status, tt.want)
		}
	}
}

func TestFunction_MarshalBinary(t *testing.T) {
	fn := &Function{
		ID:       "fn-1",
		Name:     "hello",
		Runtime:  RuntimePython,
		Handler:  "main.handler",
		MemoryMB: 128,
		TimeoutS: 30,
	}
	data, err := fn.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("MarshalBinary() returned empty data")
	}

	var decoded Function
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if decoded.Name != "hello" {
		t.Errorf("decoded.Name = %q, want %q", decoded.Name, "hello")
	}
}

func TestFunction_UnmarshalBinary(t *testing.T) {
	data := []byte(`{"id":"fn-1","name":"hello","runtime":"python","handler":"main.handler","memory_mb":128,"timeout_s":30}`)
	var fn Function
	if err := fn.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary() error: %v", err)
	}
	if fn.Name != "hello" {
		t.Errorf("Name = %q, want %q", fn.Name, "hello")
	}
	if fn.Runtime != RuntimePython {
		t.Errorf("Runtime = %q, want %q", fn.Runtime, RuntimePython)
	}
	if fn.MemoryMB != 128 {
		t.Errorf("MemoryMB = %d, want 128", fn.MemoryMB)
	}
}

func TestFunction_TrafficSplitSumsTo100(t *testing.T) {
	fn := Function{
		TrafficSplit: map[int]int{1: 80, 2: 20},
	}
	sum := 0
	for _, pct := range fn.TrafficSplit {
		sum += pct
	}
	if sum != 100 {
		t.Errorf("TrafficSplit sum = %d, want 100", sum)
	}
}

func TestRolloutPolicy_CanaryRange(t *testing.T) {
	tests := []struct {
		pct   int
		valid bool
	}{
		{0, true},
		{1, true},
		{50, true},
		{100, true},
	}
	for _, tt := range tests {
		p := RolloutPolicy{Enabled: true, CanaryPercent: tt.pct}
		if tt.valid && (p.CanaryPercent < 0 || p.CanaryPercent > 100) {
			t.Errorf("CanaryPercent %d should be in range 0-100", tt.pct)
		}
	}
}
