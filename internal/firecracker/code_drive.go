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

// buildCodeDrive creates an ext4 image containing the function code at /handler.
// Uses mke2fs -d to build the image directly from a staging directory.
func (m *Manager) buildCodeDrive(drivePath string, codeContent []byte) error {
	codeSizeMB := float64(len(codeContent)) / (1024 * 1024)

	// Stage the code into a temp directory
	stageDir, err := os.MkdirTemp("", "nova-code-*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(stageDir)

	handlerPath := filepath.Join(stageDir, "handler")
	if err := os.WriteFile(handlerPath, codeContent, 0755); err != nil {
		return fmt.Errorf("write handler: %w", err)
	}

	driveSizeMB := m.calcDriveSizeMB(codeSizeMB, 2)

	return mke2fsFromDir(stageDir, drivePath, driveSizeMB)
}

// buildCodeDriveMulti creates an ext4 image containing multiple files.
// files is a map of relative path -> content.
func (m *Manager) buildCodeDriveMulti(drivePath string, files map[string][]byte) error {
	var totalSize int64
	for _, content := range files {
		totalSize += int64(len(content))
	}
	totalSizeMB := float64(totalSize) / (1024 * 1024)

	// Stage all files into a temp directory
	stageDir, err := os.MkdirTemp("", "nova-multifile-*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(stageDir)

	for relPath, content := range files {
		dst := filepath.Join(stageDir, relPath)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
		}
		perm := os.FileMode(0644)
		if isExecutable(relPath, content) {
			perm = 0755
		}
		if err := os.WriteFile(dst, content, perm); err != nil {
			return fmt.Errorf("write %s: %w", relPath, err)
		}
	}

	driveSizeMB := m.calcDriveSizeMB(totalSizeMB*1.5, 4)

	logging.Op().Info("creating multi-file code drive",
		"size_mb", driveSizeMB,
		"total_size_mb", totalSizeMB,
		"file_count", len(files))

	return mke2fsFromDir(stageDir, drivePath, driveSizeMB)
}

// buildCodeDriveFromDir creates an ext4 image directly from an existing directory.
func (m *Manager) buildCodeDriveFromDir(drivePath string, srcDir string) error {
	dirSizeMB, err := dirSizeInMB(srcDir)
	if err != nil {
		return fmt.Errorf("calculate dir size: %w", err)
	}

	driveSizeMB := m.calcDriveSizeMB(dirSizeMB*1.5, 4)

	logging.Op().Info("creating code drive from directory",
		"size_mb", driveSizeMB,
		"dir_size_mb", dirSizeMB,
		"src", srcDir)

	return mke2fsFromDir(srcDir, drivePath, driveSizeMB)
}

// calcDriveSizeMB computes the ext4 image size given content size and overhead buffer.
func (m *Manager) calcDriveSizeMB(contentMB float64, metadataBufferMB int) int {
	defaultSize := m.config.CodeDriveSizeMB
	if defaultSize <= 0 {
		defaultSize = defaultCodeDriveSizeMB
	}
	minSize := m.config.MinCodeDriveSizeMB
	if minSize <= 0 {
		minSize = minCodeDriveSizeMB
	}

	requiredMB := int(contentMB/ext4OverheadFactor) + metadataBufferMB

	sizeMB := defaultSize
	if requiredMB > sizeMB {
		sizeMB = requiredMB
	}
	if sizeMB < minSize {
		sizeMB = minSize
	}
	const maxCodeDriveSizeMB = 512
	if sizeMB > maxCodeDriveSizeMB {
		sizeMB = maxCodeDriveSizeMB
	}
	return sizeMB
}

// mke2fsFromDir builds an ext4 image from a host directory using mke2fs -d.
// Preserves file permissions; no root or mount required.
// Uses -m 0 (no reserved blocks) and -O ^has_journal (no journal) because
// code drives are ephemeral and read-only inside the guest VM.
func mke2fsFromDir(srcDir, drivePath string, sizeMB int) error {
	sizeArg := fmt.Sprintf("%dM", sizeMB)
	cmd := exec.Command("mke2fs", "-d", srcDir, "-t", "ext4", "-m", "0", "-O", "^has_journal", "-q", drivePath, sizeArg)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(drivePath)
		return fmt.Errorf("mke2fs -d (size=%s, src=%s): %s: %w", sizeArg, srcDir, out, err)
	}
	return nil
}

// dirSizeInMB calculates the total size of files in a directory tree.
func dirSizeInMB(dir string) (float64, error) {
	var total int64
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return float64(total) / (1024 * 1024), nil
}

// isExecutable determines if a file should be executable based on path and content.
func isExecutable(path string, content []byte) bool {
	ext := filepath.Ext(path)
	execExtensions := map[string]bool{
		".sh": true, ".bash": true, ".py": true, ".rb": true, ".pl": true,
	}
	if execExtensions[ext] {
		return true
	}

	if len(content) >= 2 && content[0] == '#' && content[1] == '!' {
		return true
	}

	if len(content) >= 4 && content[0] == 0x7f && content[1] == 'E' && content[2] == 'L' && content[3] == 'F' {
		return true
	}

	base := filepath.Base(path)
	if base == "handler" || strings.HasPrefix(base, "handler.") {
		return true
	}

	return false
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
func rootfsForRuntime(rt domain.Runtime, arch domain.Arch) string {
	r := string(rt)
	var base string
	switch {
	case r == string(domain.RuntimePython) || strings.HasPrefix(r, "python"):
		base = "python.ext4"
	case r == string(domain.RuntimeWasm) || strings.HasPrefix(r, "wasm"):
		base = "wasm.ext4"
	case r == string(domain.RuntimeNode) || strings.HasPrefix(r, "node"):
		base = "node.ext4"
	case r == string(domain.RuntimeRuby) || strings.HasPrefix(r, "ruby"):
		base = "ruby.ext4"
	case r == string(domain.RuntimeJava) || strings.HasPrefix(r, "java") ||
		r == string(domain.RuntimeKotlin) || strings.HasPrefix(r, "kotlin") ||
		r == string(domain.RuntimeScala) || strings.HasPrefix(r, "scala"):
		base = "java.ext4"
	case r == string(domain.RuntimePHP) || strings.HasPrefix(r, "php"):
		base = "php.ext4"
	case r == string(domain.RuntimeLua) || strings.HasPrefix(r, "lua"):
		base = "lua.ext4"
	case r == string(domain.RuntimeDeno) || strings.HasPrefix(r, "deno"):
		base = "deno.ext4"
	case r == string(domain.RuntimeBun) || strings.HasPrefix(r, "bun"):
		base = "bun.ext4"
	case r == string(domain.RuntimeGraalVM) || strings.HasPrefix(r, "graalvm"):
		base = "graalvm.ext4"
	case r == string(domain.RuntimeElixir) || strings.HasPrefix(r, "elixir"):
		base = "elixir.ext4"
	case r == string(domain.RuntimePerl) || strings.HasPrefix(r, "perl"):
		base = "perl.ext4"
	case r == string(domain.RuntimeR):
		base = "r.ext4"
	case r == string(domain.RuntimeJulia) || strings.HasPrefix(r, "julia"):
		base = "julia.ext4"
	case r == string(domain.RuntimeSwift) || strings.HasPrefix(r, "swift"):
		base = "swift.ext4"
	default:
		// Go, Rust, Zig, C, C++ use base. (Static native binaries)
		base = "base.ext4"
	}
	return archSuffixedRootfs(base, arch)
}

func archSuffixedRootfs(base string, arch domain.Arch) string {
	if arch == "" || arch == domain.ArchAMD64 {
		return base // backward compatible: no suffix for amd64
	}
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	return name + "-" + string(arch) + ext
}
