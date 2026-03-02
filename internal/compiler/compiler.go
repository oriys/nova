package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	goRuntime "runtime"
	"strings"
	"sync"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/pkg/crypto"
	"github.com/oriys/nova/internal/pkg/safepath"
	"github.com/oriys/nova/internal/store"
)

// Compiler handles compilation of function source code using Docker containers.
type Compiler struct {
	store     store.MetadataStore
	tmpDir    string
	depsCache sync.Map // hash -> map[string][]byte (cached dependencies)
}

const compilerImagePullTimeout = 10 * time.Minute

type imageInspectFunc func(ctx context.Context, image string) (bool, error)
type imagePullFunc func(ctx context.Context, image string) error

// New creates a new Compiler instance.
func New(s store.MetadataStore) *Compiler {
	tmpDir := filepath.Join(os.TempDir(), "nova-compiler")
	os.MkdirAll(tmpDir, 0755)
	return &Compiler{
		store:  s,
		tmpDir: tmpDir,
	}
}

// EnsureDockerToolchainReady verifies Docker and pre-pulls compiler/deps images.
func EnsureDockerToolchainReady(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, compilerImagePullTimeout)
	defer cancel()

	if err := exec.CommandContext(checkCtx, "docker", "version").Run(); err != nil {
		return fmt.Errorf("docker not available for compiler: %w", err)
	}
	if err := ensureDockerImages(checkCtx, compilerDockerImages(), dockerImageExists, dockerPullImage); err != nil {
		return err
	}
	return nil
}

// CompileAsync starts an asynchronous compilation for a function.
// For interpreted languages, it stores the source code directly and sets status to not_required.
// For compiled languages, it runs Docker-based compilation in a goroutine.
func (c *Compiler) CompileAsync(ctx context.Context, fn *domain.Function, sourceCode string) {
	if !domain.NeedsCompilation(fn.Runtime) {
		// Interpreted language: store source as-is, write to CodePath
		c.handleInterpreted(ctx, fn, sourceCode)
		return
	}

	// Mark as compiling
	c.store.UpdateCompileResult(ctx, fn.ID, nil, "", domain.CompileStatusCompiling, "")

	// Run compilation in background
	go func() {
		bgCtx := context.Background()
		binary, err := c.compile(bgCtx, fn, sourceCode)
		if err != nil {
			logging.Op().Error("compilation failed", "function", fn.Name, "error", err)
			c.store.UpdateCompileResult(bgCtx, fn.ID, nil, "", domain.CompileStatusFailed, err.Error())
			return
		}

		binaryHash := hashBytes(binary)

		// Update code hash on function
		fn.CodeHash = binaryHash

		// Store result
		if err := c.store.UpdateCompileResult(bgCtx, fn.ID, binary, binaryHash, domain.CompileStatusSuccess, ""); err != nil {
			logging.Op().Error("failed to store compile result", "function", fn.Name, "error", err)
			return
		}

		// Update function's code hash in store
		c.store.SaveFunction(bgCtx, fn)

		logging.Op().Info("compilation succeeded", "function", fn.Name, "binary_size", len(binary))
	}()
}

func (c *Compiler) handleInterpreted(ctx context.Context, fn *domain.Function, sourceCode string) {
	// Update function CodeHash
	fn.CodeHash = crypto.HashString(sourceCode)
	c.store.SaveFunction(ctx, fn)

	// Store source as "binary" (it's the deployable artifact for interpreted langs)
	c.store.UpdateCompileResult(ctx, fn.ID, []byte(sourceCode), crypto.HashString(sourceCode), domain.CompileStatusNotRequired, "")
}

// CompileWithDeps compiles code with dependencies for multi-file functions.
// Returns the files map with dependencies added.
func (c *Compiler) CompileWithDeps(ctx context.Context, fn *domain.Function, files map[string][]byte) (map[string][]byte, error) {
	result := make(map[string][]byte)
	for k, v := range files {
		result[k] = v
	}

	baseRuntime := baseRuntimeID(fn.Runtime)

	switch baseRuntime {
	case domain.RuntimePython:
		if reqTxt, ok := files["requirements.txt"]; ok && len(reqTxt) > 0 {
			deps, err := c.installPythonDeps(ctx, reqTxt)
			if err != nil {
				logging.Op().Warn("failed to install Python deps", "function", fn.Name, "error", err)
			} else {
				for k, v := range deps {
					result["deps/"+k] = v
				}
				logging.Op().Info("installed Python deps", "function", fn.Name, "dep_files", len(deps))
			}
		}

	case domain.RuntimeNode:
		if pkgJson, ok := files["package.json"]; ok && len(pkgJson) > 0 {
			deps, err := c.installNodeDeps(ctx, pkgJson)
			if err != nil {
				logging.Op().Warn("failed to install Node deps", "function", fn.Name, "error", err)
			} else {
				for k, v := range deps {
					result["node_modules/"+k] = v
				}
				logging.Op().Info("installed Node deps", "function", fn.Name, "dep_files", len(deps))
			}
		}

	case domain.RuntimeRuby:
		if gemfile, ok := files["Gemfile"]; ok && len(gemfile) > 0 {
			deps, err := c.installRubyDeps(ctx, gemfile)
			if err != nil {
				logging.Op().Warn("failed to install Ruby deps", "function", fn.Name, "error", err)
			} else {
				for k, v := range deps {
					result["vendor/"+k] = v
				}
				logging.Op().Info("installed Ruby deps", "function", fn.Name, "dep_files", len(deps))
			}
		}

	case domain.RuntimePHP:
		if composerJson, ok := files["composer.json"]; ok && len(composerJson) > 0 {
			deps, err := c.installPHPDeps(ctx, composerJson)
			if err != nil {
				logging.Op().Warn("failed to install PHP deps", "function", fn.Name, "error", err)
			} else {
				for k, v := range deps {
					result["vendor/"+k] = v
				}
				logging.Op().Info("installed PHP deps", "function", fn.Name, "dep_files", len(deps))
			}
		}

	case domain.RuntimeBun:
		if pkgJson, ok := files["package.json"]; ok && len(pkgJson) > 0 {
			deps, err := c.installNodeDeps(ctx, pkgJson)
			if err != nil {
				logging.Op().Warn("failed to install Bun deps", "function", fn.Name, "error", err)
			} else {
				for k, v := range deps {
					result["node_modules/"+k] = v
				}
				logging.Op().Info("installed Bun deps", "function", fn.Name, "dep_files", len(deps))
			}
		}
	}

	return result, nil
}

// CompileAsyncWithFiles starts an asynchronous compilation for a multi-file function.
// It uses user-provided dependency files (go.mod, Cargo.toml, etc.) during compilation.
func (c *Compiler) CompileAsyncWithFiles(ctx context.Context, fn *domain.Function, files map[string][]byte) {
	if !domain.NeedsCompilation(fn.Runtime) {
		// Interpreted language: find entry point and store as-is
		entryPoint := findEntryPointFile(files, fn.Runtime, fn.Handler)
		if src, ok := files[entryPoint]; ok {
			c.handleInterpreted(ctx, fn, string(src))
		}
		return
	}

	// Mark as compiling
	c.store.UpdateCompileResult(ctx, fn.ID, nil, "", domain.CompileStatusCompiling, "")

	// Run compilation in background
	go func() {
		bgCtx := context.Background()
		binary, err := c.compileWithFiles(bgCtx, fn, files)
		if err != nil {
			logging.Op().Error("compilation failed", "function", fn.Name, "error", err)
			c.store.UpdateCompileResult(bgCtx, fn.ID, nil, "", domain.CompileStatusFailed, err.Error())
			return
		}

		binaryHash := hashBytes(binary)

		// Update code hash on function
		fn.CodeHash = binaryHash

		// Store result
		if err := c.store.UpdateCompileResult(bgCtx, fn.ID, binary, binaryHash, domain.CompileStatusSuccess, ""); err != nil {
			logging.Op().Error("failed to store compile result", "function", fn.Name, "error", err)
			return
		}

		// Update function's code hash in store
		c.store.SaveFunction(bgCtx, fn)

		logging.Op().Info("compilation succeeded", "function", fn.Name, "binary_size", len(binary))
	}()
}

// compileWithFiles compiles a multi-file function using all user-provided files.
func (c *Compiler) compileWithFiles(ctx context.Context, fn *domain.Function, files map[string][]byte) ([]byte, error) {
	// Create temp work directory
	workDir, err := os.MkdirTemp(c.tmpDir, fmt.Sprintf("compile-%s-", fn.Name))
	if err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	// Write user files and generate any missing wrapper files
	if err := c.writeSourceFilesFromMap(workDir, fn.Runtime, files); err != nil {
		return nil, fmt.Errorf("write source: %w", err)
	}

	// Get Docker compile command
	image, buildCmd := dockerCompileCommand(fn.Runtime)
	if image == "" {
		return nil, fmt.Errorf("unsupported compiled runtime: %s", fn.Runtime)
	}

	containerName := fmt.Sprintf("nova-compile-%s-%d", fn.Name, os.Getpid())

	logging.Op().Info("starting multi-file compilation", "function", fn.Name, "runtime", fn.Runtime, "image", image, "files", len(files))

	createArgs := dockerCreateArgs(containerName, image, buildCmd, fn.Runtime)
	createCmd := exec.CommandContext(ctx, "docker", createArgs...)
	var createStderr bytes.Buffer
	createCmd.Stderr = &createStderr
	if err := createCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker create failed: %w: %s", err, createStderr.String())
	}

	defer func() {
		rmCmd := exec.Command("docker", "rm", "-f", containerName)
		rmCmd.Run()
	}()

	cpInArgs := []string{"cp", workDir + "/.", containerName + ":/work/"}
	cpInCmd := exec.CommandContext(ctx, "docker", cpInArgs...)
	var cpInStderr bytes.Buffer
	cpInCmd.Stderr = &cpInStderr
	if err := cpInCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker cp (in) failed: %w: %s", err, cpInStderr.String())
	}

	startArgs := []string{"start", "-a", containerName}
	startCmd := exec.CommandContext(ctx, "docker", startArgs...)
	var stdout, stderr bytes.Buffer
	startCmd.Stdout = &stdout
	startCmd.Stderr = &stderr
	if err := startCmd.Run(); err != nil {
		return nil, fmt.Errorf("compilation error: %s\n%s", err, stderr.String())
	}

	cpOutArgs := []string{"cp", containerName + ":/work/handler", workDir + "/handler"}
	cpOutCmd := exec.CommandContext(ctx, "docker", cpOutArgs...)
	var cpOutStderr bytes.Buffer
	cpOutCmd.Stderr = &cpOutStderr
	if err := cpOutCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker cp (out) failed: %w: %s", err, cpOutStderr.String())
	}

	binaryPath := filepath.Join(workDir, "handler")
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("read compiled binary: %w", err)
	}

	return binary, nil
}

// writeSourceFilesFromMap writes all user files to workDir and generates wrapper files if missing.
func (c *Compiler) writeSourceFilesFromMap(workDir string, runtime domain.Runtime, files map[string][]byte) error {
	// Write all user-provided files first
	for path, content := range files {
		fullPath, err := safepath.Join(workDir, path)
		if err != nil {
			return fmt.Errorf("unsafe file path %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fmt.Errorf("create dir for %s: %w", path, err)
		}
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			return fmt.Errorf("write file %s: %w", path, err)
		}
	}

	baseRuntime := baseRuntimeID(runtime)

	// Generate missing wrapper files based on runtime
	switch baseRuntime {
	case domain.RuntimeGo:
		if _, ok := files["main.go"]; !ok {
			if err := os.WriteFile(filepath.Join(workDir, "main.go"), []byte(goWrapperMain), 0644); err != nil {
				return err
			}
		}
		if _, ok := files["context.go"]; !ok {
			if err := os.WriteFile(filepath.Join(workDir, "context.go"), []byte(goWrapperContext), 0644); err != nil {
				return err
			}
		}
		if _, ok := files["go.mod"]; !ok {
			goMod := "module handler\n\ngo 1.23\n"
			if err := os.WriteFile(filepath.Join(workDir, "go.mod"), []byte(goMod), 0644); err != nil {
				return err
			}
		}

	case domain.RuntimeRust:
		srcDir := filepath.Join(workDir, "src")
		os.MkdirAll(srcDir, 0755)
		// Move .rs files to src/ if not already there
		for path, content := range files {
			if strings.HasSuffix(path, ".rs") && !strings.HasPrefix(path, "src/") {
				srcPath := filepath.Join(srcDir, path)
				if err := os.WriteFile(srcPath, content, 0644); err != nil {
					return err
				}
			}
		}
		if _, ok := files["src/main.rs"]; !ok {
			if _, ok := files["main.rs"]; !ok {
				if err := os.WriteFile(filepath.Join(srcDir, "main.rs"), []byte(rustWrapperMain), 0644); err != nil {
					return err
				}
			}
		}
		if _, ok := files["src/context.rs"]; !ok {
			if _, ok := files["context.rs"]; !ok {
				if err := os.WriteFile(filepath.Join(srcDir, "context.rs"), []byte(rustWrapperContext), 0644); err != nil {
					return err
				}
			}
		}
		if _, ok := files["Cargo.toml"]; !ok {
			cargoToml := `[package]
name = "handler"
version = "0.1.0"
edition = "2021"

[dependencies]
serde = { version = "1", features = ["derive"] }
serde_json = "1"

[profile.release]
lto = true
strip = true
`
			if err := os.WriteFile(filepath.Join(workDir, "Cargo.toml"), []byte(cargoToml), 0644); err != nil {
				return err
			}
		}
		// Create .cargo/config.toml if not provided
		if _, ok := files[".cargo/config.toml"]; !ok {
			cargoDir := filepath.Join(workDir, ".cargo")
			os.MkdirAll(cargoDir, 0755)
			rustTarget := resolveRustTarget()
			cargoConfig := fmt.Sprintf("[target.%s]\nrustflags = [\"-C\", \"target-feature=+crt-static\"]\n", rustTarget)
			if err := os.WriteFile(filepath.Join(cargoDir, "config.toml"), []byte(cargoConfig), 0644); err != nil {
				return err
			}
		}

	case domain.RuntimeJava:
		if _, ok := files["Main.java"]; !ok {
			if err := os.WriteFile(filepath.Join(workDir, "Main.java"), []byte(javaWrapperMain), 0644); err != nil {
				return err
			}
		}

	case domain.RuntimeKotlin:
		if _, ok := files["Main.kt"]; !ok {
			if err := os.WriteFile(filepath.Join(workDir, "Main.kt"), []byte(kotlinWrapperMain), 0644); err != nil {
				return err
			}
		}

	case domain.RuntimeSwift:
		if _, ok := files["main.swift"]; !ok {
			if err := os.WriteFile(filepath.Join(workDir, "main.swift"), []byte(swiftWrapperMain), 0644); err != nil {
				return err
			}
		}

	case domain.RuntimeZig:
		if _, ok := files["main.zig"]; !ok {
			if err := os.WriteFile(filepath.Join(workDir, "main.zig"), []byte(zigWrapperMain), 0644); err != nil {
				return err
			}
		}

	case domain.RuntimeScala:
		if _, ok := files["Main.scala"]; !ok {
			if err := os.WriteFile(filepath.Join(workDir, "Main.scala"), []byte(scalaWrapperMain), 0644); err != nil {
				return err
			}
		}

	case domain.RuntimeC:
		if _, ok := files["main.c"]; !ok {
			if err := os.WriteFile(filepath.Join(workDir, "main.c"), []byte(cWrapperMain), 0644); err != nil {
				return err
			}
		}

	case domain.RuntimeCpp:
		if _, ok := files["main.cpp"]; !ok {
			if err := os.WriteFile(filepath.Join(workDir, "main.cpp"), []byte(cppWrapperMain), 0644); err != nil {
				return err
			}
		}

	case domain.RuntimeGraalVM:
		if _, ok := files["Main.java"]; !ok {
			if err := os.WriteFile(filepath.Join(workDir, "Main.java"), []byte(javaWrapperMain), 0644); err != nil {
				return err
			}
		}
	}

	return nil
}

// findEntryPointFile determines the entry point file from a files map.
func findEntryPointFile(files map[string][]byte, runtime domain.Runtime, handler string) string {
	// Check if handler matches a file
	if handler != "" {
		if _, ok := files[handler]; ok {
			return handler
		}
	}

	baseRuntime := baseRuntimeID(runtime)

	// Runtime-specific entry points
	entryPoints := map[domain.Runtime][]string{
		domain.RuntimePython: {"handler.py", "main.py", "app.py", "index.py"},
		domain.RuntimeNode:   {"handler.js", "index.js", "main.js", "app.js"},
		domain.RuntimeGo:     {"handler.go", "main.go"},
		domain.RuntimeRust:   {"handler.rs", "src/handler.rs", "main.rs", "src/main.rs"},
		domain.RuntimeRuby:   {"handler.rb", "main.rb", "app.rb"},
		domain.RuntimeJava:   {"Handler.java", "Main.java"},
		domain.RuntimePHP:    {"handler.php", "index.php", "main.php"},
		domain.RuntimeDeno:   {"handler.ts", "main.ts", "index.ts"},
		domain.RuntimeBun:    {"handler.ts", "handler.js", "index.ts", "index.js"},
		domain.RuntimeC:      {"handler.c", "main.c"},
		domain.RuntimeCpp:    {"handler.cpp", "main.cpp"},
	}

	if candidates, ok := entryPoints[baseRuntime]; ok {
		for _, candidate := range candidates {
			if _, exists := files[candidate]; exists {
				return candidate
			}
		}
	}

	// Fallback: return first file
	for path := range files {
		return path
	}
	return "handler"
}

// baseRuntimeID extracts the base runtime from a versioned runtime ID
// (e.g., "python3.11" -> RuntimePython, "go1.21" -> RuntimeGo)
func baseRuntimeID(runtime domain.Runtime) domain.Runtime {
	rt := string(runtime)
	// Check exact match first to avoid prefix collisions (e.g. "c" matching "cpp")
	exactMap := map[string]domain.Runtime{
		"python":  domain.RuntimePython,
		"node":    domain.RuntimeNode,
		"go":      domain.RuntimeGo,
		"rust":    domain.RuntimeRust,
		"ruby":    domain.RuntimeRuby,
		"java":    domain.RuntimeJava,
		"php":     domain.RuntimePHP,
		"deno":    domain.RuntimeDeno,
		"bun":     domain.RuntimeBun,
		"kotlin":  domain.RuntimeKotlin,
		"swift":   domain.RuntimeSwift,
		"zig":     domain.RuntimeZig,
		"scala":   domain.RuntimeScala,
		"c":       domain.RuntimeC,
		"cpp":     domain.RuntimeCpp,
		"graalvm": domain.RuntimeGraalVM,
	}
	if base, ok := exactMap[rt]; ok {
		return base
	}
	// Then try prefix matching (longest prefix first to avoid collisions)
	prefixes := []struct {
		prefix string
		base   domain.Runtime
	}{
		{"python", domain.RuntimePython},
		{"node", domain.RuntimeNode},
		{"kotlin", domain.RuntimeKotlin},
		{"graalvm", domain.RuntimeGraalVM},
		{"swift", domain.RuntimeSwift},
		{"scala", domain.RuntimeScala},
		{"rust", domain.RuntimeRust},
		{"ruby", domain.RuntimeRuby},
		{"java", domain.RuntimeJava},
		{"deno", domain.RuntimeDeno},
		{"php", domain.RuntimePHP},
		{"bun", domain.RuntimeBun},
		{"cpp", domain.RuntimeCpp},
		{"zig", domain.RuntimeZig},
		{"go", domain.RuntimeGo},
		{"c", domain.RuntimeC},
	}
	for _, p := range prefixes {
		if strings.HasPrefix(rt, p.prefix) {
			return p.base
		}
	}
	return runtime
}

// HasDependencyFiles checks if the files map contains any dependency manifest files.
func HasDependencyFiles(files map[string][]byte) bool {
	depFiles := []string{
		"go.mod", "go.sum",
		"package.json", "package-lock.json",
		"requirements.txt", "Pipfile", "pyproject.toml",
		"Cargo.toml", "Cargo.lock",
		"Gemfile", "Gemfile.lock",
		"composer.json", "composer.lock",
	}
	for _, name := range depFiles {
		if _, ok := files[name]; ok {
			return true
		}
	}
	return false
}

func compilerDockerImages() []string {
	compiledRuntimes := []domain.Runtime{
		domain.RuntimeGo,
		domain.RuntimeRust,
		domain.RuntimeJava,
		domain.RuntimeKotlin,
		domain.RuntimeSwift,
		domain.RuntimeZig,
		domain.RuntimeScala,
		domain.RuntimeC,
		domain.RuntimeCpp,
		domain.RuntimeGraalVM,
	}

	set := make(map[string]struct{}, len(compiledRuntimes)+4)
	for _, rt := range compiledRuntimes {
		image, _ := dockerCompileCommand(rt)
		if image != "" {
			set[image] = struct{}{}
		}
	}

	for _, depImage := range []string{
		"python:3.12-slim",
		"node:20-slim",
		"ruby:3.3-slim",
		"composer:2",
	} {
		set[depImage] = struct{}{}
	}

	images := make([]string, 0, len(set))
	for image := range set {
		images = append(images, image)
	}
	sort.Strings(images)
	return images
}

func ensureDockerImages(ctx context.Context, images []string, inspect imageInspectFunc, pull imagePullFunc) error {
	for _, image := range images {
		exists, err := inspect(ctx, image)
		if err != nil {
			return fmt.Errorf("inspect docker image %s: %w", image, err)
		}
		if exists {
			continue
		}
		logging.Op().Info("compiler image missing, pulling", "image", image)
		if err := pull(ctx, image); err != nil {
			return err
		}
		logging.Op().Info("compiler image ready", "image", image)
	}
	return nil
}

func dockerImageExists(ctx context.Context, image string) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func dockerPullImage(ctx context.Context, image string) error {
	cmd := exec.CommandContext(ctx, "docker", "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pull docker image %s: %w: %s", image, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// installRubyDeps installs Ruby dependencies from Gemfile
func (c *Compiler) installRubyDeps(ctx context.Context, gemfile []byte) (map[string][]byte, error) {
	hash := hashBytes(gemfile)
	if cached, ok := c.depsCache.Load(hash); ok {
		return cached.(map[string][]byte), nil
	}

	workDir, err := os.MkdirTemp(c.tmpDir, "rubydeps-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workDir)

	if err := os.WriteFile(filepath.Join(workDir, "Gemfile"), gemfile, 0644); err != nil {
		return nil, err
	}

	depsDir := filepath.Join(workDir, "vendor")
	os.MkdirAll(depsDir, 0755)

	containerName := fmt.Sprintf("nova-rubydeps-%s", hash[:12])
	image := "ruby:3.3-slim"
	buildCmd := "cd /work && bundle config set --local path vendor/bundle && bundle install --jobs=4 2>&1"

	createCmd := exec.CommandContext(ctx, "docker", "create", "--network", "host", "--name", containerName, image, "sh", "-c", buildCmd)
	var createStderr bytes.Buffer
	createCmd.Stderr = &createStderr
	if err := createCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker create failed: %w: %s", err, createStderr.String())
	}
	defer func() {
		exec.Command("docker", "rm", "-f", containerName).Run()
	}()

	cpInCmd := exec.CommandContext(ctx, "docker", "cp", workDir+"/.", containerName+":/work/")
	if out, err := cpInCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker cp (in) failed: %w: %s", err, out)
	}

	startCmd := exec.CommandContext(ctx, "docker", "start", "-a", containerName)
	var stderr bytes.Buffer
	startCmd.Stderr = &stderr

	logging.Op().Debug("installing Ruby deps", "gemfile_size", len(gemfile))

	if err := startCmd.Run(); err != nil {
		return nil, fmt.Errorf("bundle install failed: %w: %s", err, stderr.String())
	}

	cpOutCmd := exec.CommandContext(ctx, "docker", "cp", containerName+":/work/vendor/.", depsDir+"/")
	if out, err := cpOutCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker cp (out) failed: %w: %s", err, out)
	}

	deps := make(map[string][]byte)
	err = filepath.Walk(depsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		relPath, _ := filepath.Rel(depsDir, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		deps[relPath] = content
		return nil
	})
	if err != nil {
		return nil, err
	}

	c.depsCache.Store(hash, deps)
	return deps, nil
}

// installPHPDeps installs PHP dependencies from composer.json
func (c *Compiler) installPHPDeps(ctx context.Context, composerJson []byte) (map[string][]byte, error) {
	hash := hashBytes(composerJson)
	if cached, ok := c.depsCache.Load(hash); ok {
		return cached.(map[string][]byte), nil
	}

	workDir, err := os.MkdirTemp(c.tmpDir, "phpdeps-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workDir)

	if err := os.WriteFile(filepath.Join(workDir, "composer.json"), composerJson, 0644); err != nil {
		return nil, err
	}

	vendorDir := filepath.Join(workDir, "vendor")
	os.MkdirAll(vendorDir, 0755)

	containerName := fmt.Sprintf("nova-phpdeps-%s", hash[:12])
	image := "composer:2"
	buildCmd := "cd /work && composer install --no-dev --no-interaction --optimize-autoloader 2>&1"

	createCmd := exec.CommandContext(ctx, "docker", "create", "--network", "host", "--name", containerName, image, "sh", "-c", buildCmd)
	var createStderr bytes.Buffer
	createCmd.Stderr = &createStderr
	if err := createCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker create failed: %w: %s", err, createStderr.String())
	}
	defer func() {
		exec.Command("docker", "rm", "-f", containerName).Run()
	}()

	cpInCmd := exec.CommandContext(ctx, "docker", "cp", workDir+"/.", containerName+":/work/")
	if out, err := cpInCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker cp (in) failed: %w: %s", err, out)
	}

	startCmd := exec.CommandContext(ctx, "docker", "start", "-a", containerName)
	var stderr bytes.Buffer
	startCmd.Stderr = &stderr

	logging.Op().Debug("installing PHP deps", "composer_json_size", len(composerJson))

	if err := startCmd.Run(); err != nil {
		return nil, fmt.Errorf("composer install failed: %w: %s", err, stderr.String())
	}

	cpOutCmd := exec.CommandContext(ctx, "docker", "cp", containerName+":/work/vendor/.", vendorDir+"/")
	if out, err := cpOutCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker cp (out) failed: %w: %s", err, out)
	}

	deps := make(map[string][]byte)
	err = filepath.Walk(vendorDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		relPath, _ := filepath.Rel(vendorDir, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		deps[relPath] = content
		return nil
	})
	if err != nil {
		return nil, err
	}

	c.depsCache.Store(hash, deps)
	return deps, nil
}

// installPythonDeps installs Python dependencies from requirements.txt
func (c *Compiler) installPythonDeps(ctx context.Context, requirements []byte) (map[string][]byte, error) {
	// Check cache
	hash := hashBytes(requirements)
	if cached, ok := c.depsCache.Load(hash); ok {
		return cached.(map[string][]byte), nil
	}

	// Create temp directory
	workDir, err := os.MkdirTemp(c.tmpDir, "pydeps-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workDir)

	// Write requirements.txt
	reqPath := filepath.Join(workDir, "requirements.txt")
	if err := os.WriteFile(reqPath, requirements, 0644); err != nil {
		return nil, err
	}

	// Create deps directory
	depsDir := filepath.Join(workDir, "deps")
	os.MkdirAll(depsDir, 0755)

	// Use docker create + docker cp pattern (works in Docker-in-Docker)
	containerName := fmt.Sprintf("nova-pydeps-%s", hash[:12])
	image := "python:3.12-slim"
	buildCmd := "pip install --no-cache-dir -r /work/requirements.txt -t /work/deps 2>&1"

	createCmd := exec.CommandContext(ctx, "docker", "create", "--network", "host", "--name", containerName, image, "sh", "-c", buildCmd)
	var createStderr bytes.Buffer
	createCmd.Stderr = &createStderr
	if err := createCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker create failed: %w: %s", err, createStderr.String())
	}
	defer func() {
		exec.Command("docker", "rm", "-f", containerName).Run()
	}()

	cpInCmd := exec.CommandContext(ctx, "docker", "cp", workDir+"/.", containerName+":/work/")
	if out, err := cpInCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker cp (in) failed: %w: %s", err, out)
	}

	startCmd := exec.CommandContext(ctx, "docker", "start", "-a", containerName)
	var stderr bytes.Buffer
	startCmd.Stderr = &stderr

	logging.Op().Debug("installing Python deps", "requirements_size", len(requirements))

	if err := startCmd.Run(); err != nil {
		return nil, fmt.Errorf("pip install failed: %w: %s", err, stderr.String())
	}

	// Copy deps directory out of container
	cpOutCmd := exec.CommandContext(ctx, "docker", "cp", containerName+":/work/deps/.", depsDir+"/")
	if out, err := cpOutCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker cp (out) failed: %w: %s", err, out)
	}

	// Collect installed files
	deps := make(map[string][]byte)
	err = filepath.Walk(depsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		relPath, _ := filepath.Rel(depsDir, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		deps[relPath] = content
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Cache result
	c.depsCache.Store(hash, deps)

	return deps, nil
}

// installNodeDeps installs Node.js dependencies from package.json
func (c *Compiler) installNodeDeps(ctx context.Context, packageJson []byte) (map[string][]byte, error) {
	// Check cache
	hash := hashBytes(packageJson)
	if cached, ok := c.depsCache.Load(hash); ok {
		return cached.(map[string][]byte), nil
	}

	// Create temp directory
	workDir, err := os.MkdirTemp(c.tmpDir, "nodedeps-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workDir)

	// Write package.json
	pkgPath := filepath.Join(workDir, "package.json")
	if err := os.WriteFile(pkgPath, packageJson, 0644); err != nil {
		return nil, err
	}

	// Use docker create + docker cp pattern (works in Docker-in-Docker)
	containerName := fmt.Sprintf("nova-nodedeps-%s", hash[:12])
	image := "node:20-slim"
	buildCmd := "cd /work && npm install --production --no-audit --no-fund 2>&1"

	createCmd := exec.CommandContext(ctx, "docker", "create", "--network", "host", "--name", containerName, image, "sh", "-c", buildCmd)
	var createStderr bytes.Buffer
	createCmd.Stderr = &createStderr
	if err := createCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker create failed: %w: %s", err, createStderr.String())
	}
	defer func() {
		exec.Command("docker", "rm", "-f", containerName).Run()
	}()

	cpInCmd := exec.CommandContext(ctx, "docker", "cp", workDir+"/.", containerName+":/work/")
	if out, err := cpInCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker cp (in) failed: %w: %s", err, out)
	}

	startCmd := exec.CommandContext(ctx, "docker", "start", "-a", containerName)
	var stderr bytes.Buffer
	startCmd.Stderr = &stderr

	logging.Op().Debug("installing Node deps", "package_json_size", len(packageJson))

	if err := startCmd.Run(); err != nil {
		return nil, fmt.Errorf("npm install failed: %w: %s", err, stderr.String())
	}

	// Copy node_modules out of container
	nodeModulesDir := filepath.Join(workDir, "node_modules")
	cpOutCmd := exec.CommandContext(ctx, "docker", "cp", containerName+":/work/node_modules/.", nodeModulesDir+"/")
	if out, err := cpOutCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker cp (out) failed: %w: %s", err, out)
	}

	// Collect installed files
	deps := make(map[string][]byte)
	err = filepath.Walk(nodeModulesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		relPath, _ := filepath.Rel(nodeModulesDir, path)
		// Skip .bin directory and unnecessary files
		if strings.HasPrefix(relPath, ".bin/") || strings.HasSuffix(relPath, ".md") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		deps[relPath] = content
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Cache result
	c.depsCache.Store(hash, deps)

	return deps, nil
}

func (c *Compiler) compile(ctx context.Context, fn *domain.Function, sourceCode string) ([]byte, error) {
	// Create temp work directory
	workDir, err := os.MkdirTemp(c.tmpDir, fmt.Sprintf("compile-%s-", fn.Name))
	if err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	// Write source code to work directory
	if err := c.writeSourceFiles(workDir, fn.Runtime, sourceCode); err != nil {
		return nil, fmt.Errorf("write source: %w", err)
	}

	// Get Docker compile command
	image, buildCmd := dockerCompileCommand(fn.Runtime)
	if image == "" {
		return nil, fmt.Errorf("unsupported compiled runtime: %s", fn.Runtime)
	}

	// Use docker create + docker cp pattern instead of bind mounts (-v).
	// This works in Docker-in-Docker (e.g. Docker Compose with socket sharing)
	// where the host daemon can't see paths inside the nova container.
	containerName := fmt.Sprintf("nova-compile-%s-%d", fn.Name, os.Getpid())

	logging.Op().Info("starting compilation", "function", fn.Name, "runtime", fn.Runtime, "image", image)

	// Step 1: Create container (not started)
	// Force linux/amd64 platform — compiled binaries must run in x86_64 VMs/containers.
	// Without this, ARM hosts pull ARM images and cross-compilation may fail
	// (e.g., Rust proc-macros need host-native toolchain).
	createArgs := dockerCreateArgs(containerName, image, buildCmd, fn.Runtime)
	createCmd := exec.CommandContext(ctx, "docker", createArgs...)
	var createStderr bytes.Buffer
	createCmd.Stderr = &createStderr
	if err := createCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker create failed: %w: %s", err, createStderr.String())
	}

	// Ensure container is removed on exit
	defer func() {
		rmCmd := exec.Command("docker", "rm", "-f", containerName)
		rmCmd.Run()
	}()

	// Step 2: Copy source files into container
	cpInArgs := []string{"cp", workDir + "/.", containerName + ":/work/"}
	cpInCmd := exec.CommandContext(ctx, "docker", cpInArgs...)
	var cpInStderr bytes.Buffer
	cpInCmd.Stderr = &cpInStderr
	if err := cpInCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker cp (in) failed: %w: %s", err, cpInStderr.String())
	}

	// Step 3: Start container and wait for compilation
	startArgs := []string{"start", "-a", containerName}
	startCmd := exec.CommandContext(ctx, "docker", startArgs...)
	var stdout, stderr bytes.Buffer
	startCmd.Stdout = &stdout
	startCmd.Stderr = &stderr
	if err := startCmd.Run(); err != nil {
		return nil, fmt.Errorf("compilation error: %s\n%s", err, stderr.String())
	}

	// Step 4: Copy compiled binary out of container
	cpOutArgs := []string{"cp", containerName + ":/work/handler", workDir + "/handler"}
	cpOutCmd := exec.CommandContext(ctx, "docker", cpOutArgs...)
	var cpOutStderr bytes.Buffer
	cpOutCmd.Stderr = &cpOutStderr
	if err := cpOutCmd.Run(); err != nil {
		return nil, fmt.Errorf("docker cp (out) failed: %w: %s", err, cpOutStderr.String())
	}

	// Read compiled binary
	binaryPath := filepath.Join(workDir, "handler")
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("read compiled binary: %w", err)
	}

	return binary, nil
}

func (c *Compiler) writeSourceFiles(workDir string, runtime domain.Runtime, sourceCode string) error {
	switch runtime {
	case domain.RuntimeGo:
		// Save user code as handler.go (must export Handler function)
		if err := os.WriteFile(filepath.Join(workDir, "handler.go"), []byte(sourceCode), 0644); err != nil {
			return err
		}
		// Generate wrapper main.go that calls Handler(event, ctx)
		if err := os.WriteFile(filepath.Join(workDir, "main.go"), []byte(goWrapperMain), 0644); err != nil {
			return err
		}
		// Generate context.go with Context type definition
		if err := os.WriteFile(filepath.Join(workDir, "context.go"), []byte(goWrapperContext), 0644); err != nil {
			return err
		}
		// Always write go.mod so `go build` and `go mod tidy` work correctly
		goMod := "module handler\n\ngo 1.23\n"
		if err := os.WriteFile(filepath.Join(workDir, "go.mod"), []byte(goMod), 0644); err != nil {
			return err
		}
	case domain.RuntimeRust:
		// Create Cargo project structure
		srcDir := filepath.Join(workDir, "src")
		os.MkdirAll(srcDir, 0755)
		// Save user code as handler.rs
		if err := os.WriteFile(filepath.Join(srcDir, "handler.rs"), []byte(sourceCode), 0644); err != nil {
			return err
		}
		// Generate wrapper main.rs
		if err := os.WriteFile(filepath.Join(srcDir, "main.rs"), []byte(rustWrapperMain), 0644); err != nil {
			return err
		}
		// Generate context.rs
		if err := os.WriteFile(filepath.Join(srcDir, "context.rs"), []byte(rustWrapperContext), 0644); err != nil {
			return err
		}
		// Configure static linking via Cargo.toml
		cargoToml := `[package]
name = "handler"
version = "0.1.0"
edition = "2021"

[dependencies]
serde = { version = "1", features = ["derive"] }
serde_json = "1"

[profile.release]
lto = true
strip = true
`
		if err := os.WriteFile(filepath.Join(workDir, "Cargo.toml"), []byte(cargoToml), 0644); err != nil {
			return err
		}
		// Create .cargo/config.toml for static musl linking
		cargoDir := filepath.Join(workDir, ".cargo")
		os.MkdirAll(cargoDir, 0755)
		rustTarget := resolveRustTarget()
		cargoConfig := fmt.Sprintf("[target.%s]\nrustflags = [\"-C\", \"target-feature=+crt-static\"]\n", rustTarget)
		if err := os.WriteFile(filepath.Join(cargoDir, "config.toml"), []byte(cargoConfig), 0644); err != nil {
			return err
		}
	case domain.RuntimeJava:
		// Save user code as Handler.java
		if err := os.WriteFile(filepath.Join(workDir, "Handler.java"), []byte(sourceCode), 0644); err != nil {
			return err
		}
		// Generate wrapper Main.java
		if err := os.WriteFile(filepath.Join(workDir, "Main.java"), []byte(javaWrapperMain), 0644); err != nil {
			return err
		}
	case domain.RuntimeKotlin:
		if err := os.WriteFile(filepath.Join(workDir, "Handler.kt"), []byte(sourceCode), 0644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(workDir, "Main.kt"), []byte(kotlinWrapperMain), 0644); err != nil {
			return err
		}
	case domain.RuntimeSwift:
		// Save user code as Handler.swift
		if err := os.WriteFile(filepath.Join(workDir, "Handler.swift"), []byte(sourceCode), 0644); err != nil {
			return err
		}
		// Generate wrapper main.swift
		if err := os.WriteFile(filepath.Join(workDir, "main.swift"), []byte(swiftWrapperMain), 0644); err != nil {
			return err
		}
	case domain.RuntimeZig:
		// Save user code as handler.zig
		if err := os.WriteFile(filepath.Join(workDir, "handler.zig"), []byte(sourceCode), 0644); err != nil {
			return err
		}
		// Generate wrapper main.zig
		if err := os.WriteFile(filepath.Join(workDir, "main.zig"), []byte(zigWrapperMain), 0644); err != nil {
			return err
		}
	case domain.RuntimeScala:
		if err := os.WriteFile(filepath.Join(workDir, "Handler.scala"), []byte(sourceCode), 0644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(workDir, "Main.scala"), []byte(scalaWrapperMain), 0644); err != nil {
			return err
		}
	case domain.RuntimeC:
		// Save user code as handler.c
		if err := os.WriteFile(filepath.Join(workDir, "handler.c"), []byte(sourceCode), 0644); err != nil {
			return err
		}
		// Generate wrapper main.c
		if err := os.WriteFile(filepath.Join(workDir, "main.c"), []byte(cWrapperMain), 0644); err != nil {
			return err
		}
	case domain.RuntimeCpp:
		// Save user code as handler.cpp
		if err := os.WriteFile(filepath.Join(workDir, "handler.cpp"), []byte(sourceCode), 0644); err != nil {
			return err
		}
		// Generate wrapper main.cpp
		if err := os.WriteFile(filepath.Join(workDir, "main.cpp"), []byte(cppWrapperMain), 0644); err != nil {
			return err
		}
	case domain.RuntimeGraalVM:
		// Save user code as Handler.java (same convention as Java)
		if err := os.WriteFile(filepath.Join(workDir, "Handler.java"), []byte(sourceCode), 0644); err != nil {
			return err
		}
		// Generate wrapper Main.java
		if err := os.WriteFile(filepath.Join(workDir, "Main.java"), []byte(javaWrapperMain), 0644); err != nil {
			return err
		}
	default:
		ext := runtimeExtension(runtime)
		if err := os.WriteFile(filepath.Join(workDir, "handler"+ext), []byte(sourceCode), 0644); err != nil {
			return err
		}
	}
	return nil
}

func dockerCompileCommand(runtime domain.Runtime) (image, cmd string) {
	switch runtime {
	case domain.RuntimeGo:
		goarch := "amd64"
		if p := resolveCompilePlatform(runtime); strings.Contains(p, "arm64") {
			goarch = "arm64"
		}
		return "golang:1.23-alpine", fmt.Sprintf("cd /work && go mod tidy && CGO_ENABLED=0 GOOS=linux GOARCH=%s go build -o handler .", goarch)
	case domain.RuntimeRust:
		rustTarget := resolveRustTarget()
		return "rust:1.84-alpine", fmt.Sprintf("apk add --no-cache musl-dev gcc && cd /work && RUSTFLAGS='-C target-feature=+crt-static' cargo build --release --target %s && cp target/%s/release/handler /work/handler", rustTarget, rustTarget)
	case domain.RuntimeJava:
		return "eclipse-temurin:21-jdk", "cd /work && javac Main.java Handler.java && jar cfe handler.jar Main *.class && cp handler.jar handler"
	case domain.RuntimeKotlin:
		return "eclipse-temurin:21-jdk", "DEBIAN_FRONTEND=noninteractive apt-get update && apt-get install -y --no-install-recommends kotlin && cd /work && kotlinc *.kt -include-runtime -d handler.jar && cp handler.jar handler"
	case domain.RuntimeSwift:
		return "swift:5.10", "cd /work && swiftc -o handler -static-executable Handler.swift main.swift"
	case domain.RuntimeZig:
		zigTarget := resolveZigTarget()
		return "euantorano/zig:0.13.0", fmt.Sprintf("cd /work && zig build-exe main.zig -name handler -target %s", zigTarget)
	case domain.RuntimeScala:
		return "eclipse-temurin:21-jdk",
			`DEBIAN_FRONTEND=noninteractive apt-get update && apt-get install -y --no-install-recommends scala && cd /work && scalac *.scala && ` +
				`SCALA_LIB=$(find / -name "scala-library*.jar" 2>/dev/null | head -1) && ` +
				`mkdir -p /tmp/fatjar && cd /tmp/fatjar && ` +
				`jar xf "$SCALA_LIB" && ` +
				`cp /work/*.class . && ` +
				`jar cfe /work/handler.jar Main -C . . && ` +
				`cp /work/handler.jar /work/handler`
	case domain.RuntimeC:
		return "gcc:14", "cd /work && gcc -std=c11 -O2 -static -o handler *.c"
	case domain.RuntimeCpp:
		return "gcc:14", "cd /work && g++ -std=c++17 -O2 -static -o handler *.cpp"
	case domain.RuntimeGraalVM:
		return "ghcr.io/graalvm/native-image-community:21",
			"cd /work && javac *.java && native-image --static --no-fallback -o handler Main"
	default:
		return "", ""
	}
}

func dockerCreateArgs(containerName, image, buildCmd string, runtime domain.Runtime) []string {
	platform := resolveCompilePlatform(runtime)

	// Force shell entrypoint so images with custom ENTRYPOINT (e.g. GraalVM native-image)
	// run our build script instead of interpreting "cd" as tool arguments.
	args := []string{
		"create",
		"--network", "host",
		"--name", containerName,
		"--entrypoint", "/bin/sh",
		image,
		"-c", buildCmd,
	}
	if strings.TrimSpace(platform) != "" {
		args = append([]string{"create", "--platform", platform}, args[1:]...)
	}
	return args
}

// resolveRustTarget returns the Rust target triple matching the compile platform.
func resolveRustTarget() string {
	if p := resolveCompilePlatform(domain.RuntimeRust); strings.Contains(p, "arm64") || strings.Contains(p, "aarch64") {
		return "aarch64-unknown-linux-musl"
	}
	return "x86_64-unknown-linux-musl"
}

// resolveZigTarget returns the Zig target triple matching the compile platform.
func resolveZigTarget() string {
	if p := resolveCompilePlatform(domain.RuntimeZig); strings.Contains(p, "arm64") || strings.Contains(p, "aarch64") {
		return "aarch64-linux-musl"
	}
	return "x86_64-linux-musl"
}

func resolveCompilePlatform(runtime domain.Runtime) string {
	// Auto-detect from host architecture; override via NOVA_COMPILE_PLATFORM.
	platform := strings.TrimSpace(os.Getenv("NOVA_COMPILE_PLATFORM"))
	if platform == "" {
		platform = "linux/" + goRuntime.GOARCH
	}
	// Allow GraalVM to be compiled for a different platform in Docker backend
	// to avoid cross-arch emulation startup overhead on ARM hosts.
	if runtime == domain.RuntimeGraalVM {
		if graal := strings.TrimSpace(os.Getenv("NOVA_GRAALVM_COMPILE_PLATFORM")); graal != "" {
			platform = graal
		}
	}
	return platform
}

func runtimeExtension(runtime domain.Runtime) string {
	return RuntimeExtension(runtime)
}

// RuntimeExtension returns the file extension for a runtime (e.g., ".py" for Python).
func RuntimeExtension(runtime domain.Runtime) string {
	rt := baseRuntimeID(runtime)
	exts := map[domain.Runtime]string{
		domain.RuntimePython:  ".py",
		domain.RuntimeGo:      ".go",
		domain.RuntimeRust:    ".rs",
		domain.RuntimeNode:    ".js",
		domain.RuntimeRuby:    ".rb",
		domain.RuntimeJava:    ".java",
		domain.RuntimeDeno:    ".ts",
		domain.RuntimeBun:     ".ts",
		domain.RuntimeWasm:    ".wasm",
		domain.RuntimePHP:     ".php",
		domain.RuntimeElixir:  ".exs",
		domain.RuntimeKotlin:  ".kt",
		domain.RuntimeSwift:   ".swift",
		domain.RuntimeZig:     ".zig",
		domain.RuntimeLua:     ".lua",
		domain.RuntimePerl:    ".pl",
		domain.RuntimeR:       ".R",
		domain.RuntimeJulia:   ".jl",
		domain.RuntimeScala:   ".scala",
		domain.RuntimeC:       ".c",
		domain.RuntimeCpp:     ".cpp",
		domain.RuntimeGraalVM: ".java",
	}
	if ext, ok := exts[rt]; ok {
		return ext
	}
	return ".txt"
}

func hashBytes(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ─── Wrapper templates for compiled runtimes ────────────────────────

// Go: user writes Handler(event json.RawMessage, ctx Context) (interface{}, error)
const goWrapperMain = `package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read input: %v\n", err)
		os.Exit(1)
	}
	ctx := BuildContext()
	result, err := Handler(json.RawMessage(data), ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "handler error: %v\n", err)
		os.Exit(1)
	}
	output, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal output: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(output))
}
`

const goWrapperContext = `package main

import (
	"os"
	"strconv"
)

type Context struct {
	RequestID       string
	FunctionName    string
	FunctionVersion string
	MemoryLimitMB   int
	TimeoutS        int
	Runtime         string
}

func BuildContext() Context {
	memMB, _ := strconv.Atoi(os.Getenv("NOVA_MEMORY_LIMIT_MB"))
	timeoutS, _ := strconv.Atoi(os.Getenv("NOVA_TIMEOUT_S"))
	return Context{
		RequestID:       os.Getenv("NOVA_REQUEST_ID"),
		FunctionName:    os.Getenv("NOVA_FUNCTION_NAME"),
		FunctionVersion: os.Getenv("NOVA_FUNCTION_VERSION"),
		MemoryLimitMB:   memMB,
		TimeoutS:        timeoutS,
		Runtime:         os.Getenv("NOVA_RUNTIME"),
	}
}
`

// Rust: user writes pub fn handler(event: serde_json::Value, ctx: Context) -> Result<serde_json::Value, String>
const rustWrapperMain = `mod handler;
mod context;

pub use context::Context;

use std::env;
use std::fs;

fn main() {
    let args: Vec<String> = env::args().collect();
    let data = fs::read_to_string(&args[1]).expect("read input file");
    let event: serde_json::Value = serde_json::from_str(&data).expect("parse input JSON");
    let ctx = context::build_context();
    match handler::handler(event, ctx) {
        Ok(result) => println!("{}", serde_json::to_string(&result).expect("serialize output")),
        Err(e) => {
            eprintln!("handler error: {}", e);
            std::process::exit(1);
        }
    }
}
`

const rustWrapperContext = `use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Context {
    pub request_id: String,
    pub function_name: String,
    pub function_version: String,
    pub memory_limit_mb: i32,
    pub timeout_s: i32,
    pub runtime: String,
}

pub fn build_context() -> Context {
    Context {
        request_id: std::env::var("NOVA_REQUEST_ID").unwrap_or_default(),
        function_name: std::env::var("NOVA_FUNCTION_NAME").unwrap_or_default(),
        function_version: std::env::var("NOVA_FUNCTION_VERSION").unwrap_or_default(),
        memory_limit_mb: std::env::var("NOVA_MEMORY_LIMIT_MB")
            .unwrap_or_default().parse().unwrap_or(0),
        timeout_s: std::env::var("NOVA_TIMEOUT_S")
            .unwrap_or_default().parse().unwrap_or(0),
        runtime: std::env::var("NOVA_RUNTIME").unwrap_or_default(),
    }
}
`

// Java: user writes public class Handler { public static Object handler(String event, Map<String, Object> context) }
const javaWrapperMain = `import java.nio.file.*;
import java.util.*;

public class Main {
    public static void main(String[] args) throws Exception {
        String input = Files.readString(Path.of(args[0]));
        Map<String, Object> context = new HashMap<>();
        context.put("request_id", System.getenv().getOrDefault("NOVA_REQUEST_ID", ""));
        context.put("function_name", System.getenv().getOrDefault("NOVA_FUNCTION_NAME", ""));
        context.put("function_version", System.getenv().getOrDefault("NOVA_FUNCTION_VERSION", ""));
        context.put("memory_limit_mb", System.getenv().getOrDefault("NOVA_MEMORY_LIMIT_MB", "0"));
        context.put("timeout_s", System.getenv().getOrDefault("NOVA_TIMEOUT_S", "0"));
        context.put("runtime", System.getenv().getOrDefault("NOVA_RUNTIME", ""));
        Object result = Handler.handler(input, context);
        System.out.println(result);
    }
}
`

// Kotlin: user writes object Handler { fun handler(event: String, context: Map<String, Any>): Any }
const kotlinWrapperMain = `fun main(args: Array<String>) {
    val input = java.io.File(args[0]).readText()
    val context = mapOf(
        "request_id" to (System.getenv("NOVA_REQUEST_ID") ?: ""),
        "function_name" to (System.getenv("NOVA_FUNCTION_NAME") ?: ""),
        "function_version" to (System.getenv("NOVA_FUNCTION_VERSION") ?: ""),
        "memory_limit_mb" to (System.getenv("NOVA_MEMORY_LIMIT_MB") ?: "0"),
        "timeout_s" to (System.getenv("NOVA_TIMEOUT_S") ?: "0"),
        "runtime" to (System.getenv("NOVA_RUNTIME") ?: "")
    )
    val result = Handler.handler(input, context)
    println(result)
}
`

// Swift: user writes func handler(event: String, context: [String: Any]) -> Any
const swiftWrapperMain = `import Foundation

struct NovaContext {
    let requestId: String
    let functionName: String
    let functionVersion: String
    let memoryLimitMB: Int
    let timeoutS: Int
    let runtime: String
}

func buildContext() -> NovaContext {
    return NovaContext(
        requestId: ProcessInfo.processInfo.environment["NOVA_REQUEST_ID"] ?? "",
        functionName: ProcessInfo.processInfo.environment["NOVA_FUNCTION_NAME"] ?? "",
        functionVersion: ProcessInfo.processInfo.environment["NOVA_FUNCTION_VERSION"] ?? "",
        memoryLimitMB: Int(ProcessInfo.processInfo.environment["NOVA_MEMORY_LIMIT_MB"] ?? "0") ?? 0,
        timeoutS: Int(ProcessInfo.processInfo.environment["NOVA_TIMEOUT_S"] ?? "0") ?? 0,
        runtime: ProcessInfo.processInfo.environment["NOVA_RUNTIME"] ?? ""
    )
}

let inputPath = CommandLine.arguments[1]
let data = try! String(contentsOfFile: inputPath, encoding: .utf8)
let ctx = buildContext()
let result = handler(event: data, context: ctx)
if let jsonData = try? JSONSerialization.data(withJSONObject: result, options: []),
   let jsonString = String(data: jsonData, encoding: .utf8) {
    print(jsonString)
} else {
    print(result)
}
`

// Zig: user writes pub fn handler(event: []const u8, ctx: Context) ![]const u8
const zigWrapperMain = `const std = @import("std");
const handler_mod = @import("handler.zig");

pub fn main() !void {
    var arena = std.heap.ArenaAllocator.init(std.heap.page_allocator);
    defer arena.deinit();
    const allocator = arena.allocator();

    const args = try std.process.argsAlloc(allocator);
    const input = try std.fs.cwd().readFileAlloc(allocator, args[1], 1024 * 1024);

    const result = try handler_mod.handler(input, allocator);
    const stdout = std.io.getStdOut().writer();
    try stdout.writeAll(result);
    try stdout.writeAll("\n");
}
`

// Scala: user writes object Handler { def handler(event: String, context: Map[String, Any]): Any }
const scalaWrapperMain = `object Main {
  def main(args: Array[String]): Unit = {
    val input = scala.io.Source.fromFile(args(0)).mkString
    val context = Map(
      "request_id" -> sys.env.getOrElse("NOVA_REQUEST_ID", ""),
      "function_name" -> sys.env.getOrElse("NOVA_FUNCTION_NAME", ""),
      "function_version" -> sys.env.getOrElse("NOVA_FUNCTION_VERSION", ""),
      "memory_limit_mb" -> sys.env.getOrElse("NOVA_MEMORY_LIMIT_MB", "0"),
      "timeout_s" -> sys.env.getOrElse("NOVA_TIMEOUT_S", "0"),
      "runtime" -> sys.env.getOrElse("NOVA_RUNTIME", "")
    )
    val result = Handler.handler(input, context)
    println(result)
  }
}
`

// C: user writes const char* handler(const char* event, const char* context)
const cWrapperMain = `#include <stdio.h>
#include <stdlib.h>
#include <string.h>

extern const char* handler(const char* event, const char* context);

static char* read_file(const char* path) {
    FILE* f = fopen(path, "rb");
    if (!f) { perror("open input"); exit(1); }
    fseek(f, 0, SEEK_END);
    long len = ftell(f);
    fseek(f, 0, SEEK_SET);
    char* buf = (char*)malloc(len + 1);
    if (!buf) { perror("malloc"); exit(1); }
    fread(buf, 1, len, f);
    buf[len] = '\0';
    fclose(f);
    return buf;
}

static char* build_context(void) {
    const char* rid = getenv("NOVA_REQUEST_ID");
    const char* fname = getenv("NOVA_FUNCTION_NAME");
    const char* fver = getenv("NOVA_FUNCTION_VERSION");
    const char* mem = getenv("NOVA_MEMORY_LIMIT_MB");
    const char* tout = getenv("NOVA_TIMEOUT_S");
    const char* rt = getenv("NOVA_RUNTIME");
    char* ctx = (char*)malloc(1024);
    if (!ctx) { perror("malloc"); exit(1); }
    snprintf(ctx, 1024,
        "{\"request_id\":\"%s\",\"function_name\":\"%s\","
        "\"function_version\":\"%s\",\"memory_limit_mb\":\"%s\","
        "\"timeout_s\":\"%s\",\"runtime\":\"%s\"}",
        rid ? rid : "", fname ? fname : "", fver ? fver : "",
        mem ? mem : "0", tout ? tout : "0", rt ? rt : "");
    return ctx;
}

int main(int argc, char* argv[]) {
    if (argc < 2) { fprintf(stderr, "usage: handler <input.json>\n"); return 1; }
    char* input = read_file(argv[1]);
    char* ctx = build_context();
    const char* result = handler(input, ctx);
    printf("%s\n", result);
    free(input);
    free(ctx);
    return 0;
}
`

// C++: user writes std::string handler(const std::string& event, const std::string& context)
const cppWrapperMain = `#include <iostream>
#include <fstream>
#include <sstream>
#include <string>
#include <cstdlib>

extern std::string handler(const std::string& event, const std::string& context);

static std::string read_file(const char* path) {
    std::ifstream f(path);
    if (!f) { std::cerr << "open input: " << path << std::endl; std::exit(1); }
    std::ostringstream ss;
    ss << f.rdbuf();
    return ss.str();
}

static std::string build_context() {
    auto env = [](const char* k) -> std::string {
        const char* v = std::getenv(k);
        return v ? v : "";
    };
    return std::string("{\"request_id\":\"") + env("NOVA_REQUEST_ID") +
        "\",\"function_name\":\"" + env("NOVA_FUNCTION_NAME") +
        "\",\"function_version\":\"" + env("NOVA_FUNCTION_VERSION") +
        "\",\"memory_limit_mb\":\"" + (env("NOVA_MEMORY_LIMIT_MB").empty() ? "0" : env("NOVA_MEMORY_LIMIT_MB")) +
        "\",\"timeout_s\":\"" + (env("NOVA_TIMEOUT_S").empty() ? "0" : env("NOVA_TIMEOUT_S")) +
        "\",\"runtime\":\"" + env("NOVA_RUNTIME") + "\"}";
}

int main(int argc, char* argv[]) {
    if (argc < 2) { std::cerr << "usage: handler <input.json>" << std::endl; return 1; }
    std::string input = read_file(argv[1]);
    std::string ctx = build_context();
    std::string result = handler(input, ctx);
    std::cout << result << std::endl;
    return 0;
}
`
