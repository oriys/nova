package domain

import "testing"

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
	}

	for _, tt := range tests {
		got := NeedsCompilation(tt.runtime)
		if got != tt.want {
			t.Fatalf("NeedsCompilation(%q) = %v, want %v", tt.runtime, got, tt.want)
		}
	}
}
