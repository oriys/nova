package main

import "testing"

func TestNormalizeRuntime(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"rust1.84", "rust"},
		{"go1.23", "go"},
		{"dotnet8", "dotnet"},
		{"python3.12", "python"},
		{"node24", "node"},
		{"wasm", "wasm"},
	}

	for _, tt := range tests {
		if got := normalizeRuntime(tt.in); got != tt.want {
			t.Fatalf("normalizeRuntime(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
