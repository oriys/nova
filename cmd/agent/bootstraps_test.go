package main

import (
	"strings"
	"testing"
)

func TestBootstrapPHP_PersistentFlushesStdout(t *testing.T) {
	want := []string{
		`echo json_encode(['output' => $result]) . "\n";
            flush();`,
		`echo json_encode(['error' => $e->getMessage()]) . "\n";
            flush();`,
	}

	for _, w := range want {
		if !strings.Contains(bootstrapPHP, w) {
			t.Fatalf("bootstrapPHP missing persistent flush snippet: %q", w)
		}
	}
}
