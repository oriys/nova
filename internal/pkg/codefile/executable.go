package codefile

import (
	"path/filepath"
	"strings"
)

// ShouldBeExecutable reports whether a code file should be marked executable.
func ShouldBeExecutable(path string, content []byte) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".sh", ".bash", ".py", ".rb", ".pl":
		return true
	}

	// Shebang script
	if len(content) >= 2 && content[0] == '#' && content[1] == '!' {
		return true
	}

	// ELF binary
	if len(content) >= 4 && content[0] == 0x7f && content[1] == 'E' && content[2] == 'L' && content[3] == 'F' {
		return true
	}

	base := strings.ToLower(filepath.Base(path))
	if base == "handler" || strings.HasPrefix(base, "handler.") {
		return true
	}
	if base == "bootstrap" || strings.HasPrefix(base, "bootstrap.") {
		return true
	}

	return false
}
