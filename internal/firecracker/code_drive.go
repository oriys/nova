package firecracker

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
)

// buildCodeDrive creates an ext4 image and injects the function code at /handler.
// Uses a cached template image for small functions to avoid repeated mkfs calls.
// For larger functions, creates a custom-sized drive.
func (m *Manager) buildCodeDrive(drivePath string, codeContent []byte) error {
	// Get code size
	codeSizeMB := float64(len(codeContent)) / (1024 * 1024)

	// Use config values with fallback to defaults
	defaultSize := m.config.CodeDriveSizeMB
	if defaultSize <= 0 {
		defaultSize = defaultCodeDriveSizeMB
	}
	minSize := m.config.MinCodeDriveSizeMB
	if minSize <= 0 {
		minSize = minCodeDriveSizeMB
	}

	// Calculate required drive size (code + ext4 overhead + buffer)
	requiredSizeMB := int(codeSizeMB/ext4OverheadFactor) + 2 // +2MB buffer for ext4 metadata

	// Determine if we can use the standard template
	useTemplate := requiredSizeMB <= defaultSize
	var driveSizeMB int

	if useTemplate {
		// Use cached template for small functions
		templatePath := filepath.Join(m.config.SocketDir, "template-code.ext4")

		// Retryable template creation using atomic bool + mutex
		if !m.templateReady.Load() {
			m.templateMu.Lock()
			if !m.templateReady.Load() {
				if err := createTemplateDrive(templatePath, defaultSize); err != nil {
					m.templateMu.Unlock()
					return err
				}
				m.templateReady.Store(true)
			}
			m.templateMu.Unlock()
		}

		// Buffered copy of template to new drive
		if err := copyFileBuffered(templatePath, drivePath); err != nil {
			return err
		}
		driveSizeMB = defaultSize
	} else {
		// Create custom-sized drive for large functions
		driveSizeMB = requiredSizeMB
		if driveSizeMB < minSize {
			driveSizeMB = minSize
		}
		logging.Op().Info("creating custom code drive",
			"size_mb", driveSizeMB,
			"code_size_mb", codeSizeMB)
		if err := createTemplateDrive(drivePath, driveSizeMB); err != nil {
			return err
		}
	}

	// Write code content to a temp file for debugfs
	tmpFile, err := os.CreateTemp("", "nova-code-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(codeContent); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	// Inject function code using debugfs (no mount needed)
	debugfsCmd := fmt.Sprintf("write %s handler\nsif handler mode 0100755\n", tmpPath)
	cmd := exec.Command("debugfs", "-w", drivePath)
	cmd.Stdin = strings.NewReader(debugfsCmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("debugfs inject (drive=%dMB, code=%.1fMB): %s: %w", driveSizeMB, codeSizeMB, out, err)
	}

	return nil
}

// buildCodeDriveMulti creates an ext4 image and injects multiple files.
// files is a map of relative path -> content.
func (m *Manager) buildCodeDriveMulti(drivePath string, files map[string][]byte) error {
	// Calculate total size
	var totalSize int64
	for _, content := range files {
		totalSize += int64(len(content))
	}
	totalSizeMB := float64(totalSize) / (1024 * 1024)

	// Use config values with fallback to defaults
	defaultSize := m.config.CodeDriveSizeMB
	if defaultSize <= 0 {
		defaultSize = defaultCodeDriveSizeMB
	}
	minSize := m.config.MinCodeDriveSizeMB
	if minSize <= 0 {
		minSize = minCodeDriveSizeMB
	}

	// Calculate required drive size:
	// - Add 50% headroom for future growth
	// - Account for ext4 overhead (15%)
	// - Add buffer for directory inodes and metadata
	contentWithHeadroom := totalSizeMB * 1.5
	requiredSizeMB := int(contentWithHeadroom/ext4OverheadFactor) + 4 // +4MB for metadata

	// Determine drive size with min/max limits
	driveSizeMB := defaultSize
	if requiredSizeMB > defaultSize {
		driveSizeMB = requiredSizeMB
	}
	if driveSizeMB < minSize {
		driveSizeMB = minSize
	}
	// Cap at 512MB to prevent excessive resource usage
	const maxCodeDriveSizeMB = 512
	if driveSizeMB > maxCodeDriveSizeMB {
		driveSizeMB = maxCodeDriveSizeMB
	}

	// Create the ext4 drive
	logging.Op().Info("creating multi-file code drive",
		"size_mb", driveSizeMB,
		"total_size_mb", totalSizeMB,
		"file_count", len(files))
	if err := createTemplateDrive(drivePath, driveSizeMB); err != nil {
		return err
	}

	// Collect all directories that need to be created
	dirs := make(map[string]bool)
	for path := range files {
		parts := strings.Split(path, "/")
		for i := 1; i < len(parts); i++ {
			dir := strings.Join(parts[:i], "/")
			if dir != "" {
				dirs[dir] = true
			}
		}
	}

	// Build debugfs commands
	var debugfsCmds strings.Builder

	// Create directories first (sorted to ensure parent dirs exist)
	sortedDirs := make([]string, 0, len(dirs))
	for dir := range dirs {
		sortedDirs = append(sortedDirs, dir)
	}
	// Simple sort by path depth then alphabetically
	for i := range sortedDirs {
		for j := i + 1; j < len(sortedDirs); j++ {
			iDepth := strings.Count(sortedDirs[i], "/")
			jDepth := strings.Count(sortedDirs[j], "/")
			if iDepth > jDepth || (iDepth == jDepth && sortedDirs[i] > sortedDirs[j]) {
				sortedDirs[i], sortedDirs[j] = sortedDirs[j], sortedDirs[i]
			}
		}
	}

	for _, dir := range sortedDirs {
		debugfsCmds.WriteString(fmt.Sprintf("mkdir %s\n", dir))
	}

	// Write files to temp dir and build injection commands
	tmpDir, err := os.MkdirTemp("", "nova-multifile-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	for path, content := range files {
		// Write to temp file
		tmpFile := filepath.Join(tmpDir, strings.ReplaceAll(path, "/", "_"))
		if err := os.WriteFile(tmpFile, content, 0644); err != nil {
			return fmt.Errorf("write temp file %s: %w", path, err)
		}

		// Add debugfs commands
		debugfsCmds.WriteString(fmt.Sprintf("write %s %s\n", tmpFile, path))
		// Make executable if it looks like an executable
		if isExecutable(path, content) {
			debugfsCmds.WriteString(fmt.Sprintf("sif %s mode 0100755\n", path))
		}
	}

	// Run debugfs to inject all files
	cmd := exec.Command("debugfs", "-w", drivePath)
	cmd.Stdin = strings.NewReader(debugfsCmds.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("debugfs inject (drive=%dMB, total=%.1fMB, files=%d): %s: %w",
			driveSizeMB, totalSizeMB, len(files), out, err)
	}

	return nil
}

// isExecutable determines if a file should be executable based on path and content
func isExecutable(path string, content []byte) bool {
	// Check extension
	ext := filepath.Ext(path)
	execExtensions := map[string]bool{
		".sh": true, ".bash": true, ".py": true, ".rb": true, ".pl": true,
	}
	if execExtensions[ext] {
		return true
	}

	// Check for shebang
	if len(content) >= 2 && content[0] == '#' && content[1] == '!' {
		return true
	}

	// Check for ELF binary (compiled Go/Rust/etc)
	if len(content) >= 4 && content[0] == 0x7f && content[1] == 'E' && content[2] == 'L' && content[3] == 'F' {
		return true
	}

	// Check for "handler" in path (main entry point)
	base := filepath.Base(path)
	if base == "handler" || strings.HasPrefix(base, "handler.") {
		return true
	}

	return false
}

func createTemplateDrive(path string, sizeMB int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := f.Truncate(int64(sizeMB) * 1024 * 1024); err != nil {
		f.Close()
		return err
	}
	f.Close()

	if out, err := exec.Command("mkfs.ext4", "-F", "-q", path).CombinedOutput(); err != nil {
		os.Remove(path)
		return fmt.Errorf("mkfs.ext4: %s: %w", out, err)
	}
	return nil
}

func copyFileBuffered(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, 256*1024) // 256KB buffer
	_, err = io.CopyBuffer(out, bufio.NewReaderSize(in, 256*1024), buf)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// rootfsForRuntime maps runtime to rootfs image.
// Go/Rust: static binaries, minimal base is enough.
// Python: needs interpreter. WASM: needs wasmtime.
// Node/Deno/Bun: need JS runtime. Ruby: needs interpreter. Java: needs JVM.
func rootfsForRuntime(rt domain.Runtime) string {
	r := string(rt)
	switch {
	case r == string(domain.RuntimePython) || strings.HasPrefix(r, "python"):
		return "python.ext4"
	case r == string(domain.RuntimeWasm) || strings.HasPrefix(r, "wasm"):
		return "wasm.ext4"
	case r == string(domain.RuntimeNode) || strings.HasPrefix(r, "node"):
		return "node.ext4"
	case r == string(domain.RuntimeRuby) || strings.HasPrefix(r, "ruby"):
		return "ruby.ext4"
	case r == string(domain.RuntimeJava) || strings.HasPrefix(r, "java") ||
		r == string(domain.RuntimeKotlin) || r == string(domain.RuntimeScala):
		return "java.ext4"
	case r == string(domain.RuntimePHP) || strings.HasPrefix(r, "php"):
		return "php.ext4"
	case r == string(domain.RuntimeLua) || strings.HasPrefix(r, "lua"):
		return "lua.ext4"
	case r == string(domain.RuntimeDeno) || strings.HasPrefix(r, "deno"):
		return "deno.ext4"
	case r == string(domain.RuntimeBun) || strings.HasPrefix(r, "bun"):
		return "bun.ext4"
	default:
		// Go, Rust, Zig, Swift use base. (Swift might need more, but base is current)
		return "base.ext4"
	}
}
