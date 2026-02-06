package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/pkg/crypto"
	"github.com/oriys/nova/internal/store"
)

// Compiler handles compilation of function source code using Docker containers.
type Compiler struct {
	store    store.MetadataStore
	tmpDir   string
	depsCache sync.Map // hash -> map[string][]byte (cached dependencies)
}

// New creates a new Compiler instance.
func New(s store.MetadataStore) *Compiler {
	tmpDir := filepath.Join(os.TempDir(), "nova-compiler")
	os.MkdirAll(tmpDir, 0755)
	return &Compiler{
		store:  s,
		tmpDir: tmpDir,
	}
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

	switch fn.Runtime {
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
	}

	return result, nil
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

	// Run pip install in Docker
	args := []string{
		"run", "--rm",
		"-v", workDir + ":/work",
		"python:3.12-slim",
		"sh", "-c", "pip install --no-cache-dir -r /work/requirements.txt -t /work/deps 2>&1",
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	logging.Op().Debug("installing Python deps", "requirements_size", len(requirements))

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pip install failed: %w: %s", err, stderr.String())
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

	// Run npm install in Docker
	args := []string{
		"run", "--rm",
		"-v", workDir + ":/work",
		"node:20-slim",
		"sh", "-c", "cd /work && npm install --production --no-audit --no-fund 2>&1",
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	logging.Op().Debug("installing Node deps", "package_json_size", len(packageJson))

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("npm install failed: %w: %s", err, stderr.String())
	}

	// Collect installed files
	nodeModulesDir := filepath.Join(workDir, "node_modules")
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
	createArgs := []string{"create", "--name", containerName, image, "sh", "-c", buildCmd}
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
		// Write main.go
		if err := os.WriteFile(filepath.Join(workDir, "main.go"), []byte(sourceCode), 0644); err != nil {
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
		if err := os.WriteFile(filepath.Join(srcDir, "main.rs"), []byte(sourceCode), 0644); err != nil {
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
		cargoConfig := `[target.x86_64-unknown-linux-musl]
rustflags = ["-C", "target-feature=+crt-static"]
`
		if err := os.WriteFile(filepath.Join(cargoDir, "config.toml"), []byte(cargoConfig), 0644); err != nil {
			return err
		}
	case domain.RuntimeJava:
		if err := os.WriteFile(filepath.Join(workDir, "Handler.java"), []byte(sourceCode), 0644); err != nil {
			return err
		}
	case domain.RuntimeKotlin:
		if err := os.WriteFile(filepath.Join(workDir, "Handler.kt"), []byte(sourceCode), 0644); err != nil {
			return err
		}
	case domain.RuntimeSwift:
		if err := os.WriteFile(filepath.Join(workDir, "main.swift"), []byte(sourceCode), 0644); err != nil {
			return err
		}
	case domain.RuntimeZig:
		if err := os.WriteFile(filepath.Join(workDir, "main.zig"), []byte(sourceCode), 0644); err != nil {
			return err
		}
	case domain.RuntimeDotnet:
		if err := os.WriteFile(filepath.Join(workDir, "Program.cs"), []byte(sourceCode), 0644); err != nil {
			return err
		}
		csproj := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <OutputType>Exe</OutputType>
    <TargetFramework>net8.0</TargetFramework>
    <RuntimeIdentifier>linux-musl-x64</RuntimeIdentifier>
    <PublishSingleFile>true</PublishSingleFile>
    <SelfContained>true</SelfContained>
  </PropertyGroup>
</Project>`
		if err := os.WriteFile(filepath.Join(workDir, "handler.csproj"), []byte(csproj), 0644); err != nil {
			return err
		}
	case domain.RuntimeScala:
		if err := os.WriteFile(filepath.Join(workDir, "Handler.scala"), []byte(sourceCode), 0644); err != nil {
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
		return "golang:1.23-alpine", "cd /work && go mod tidy && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o handler ."
	case domain.RuntimeRust:
		// Use musl target for static linking; install target first, then build
		return "rust:1.84-alpine", "rustup target add x86_64-unknown-linux-musl && cd /work && cargo build --release --target x86_64-unknown-linux-musl && cp target/x86_64-unknown-linux-musl/release/handler /work/handler"
	case domain.RuntimeJava:
		return "eclipse-temurin:21-jdk", "cd /work && javac Handler.java && jar cfe handler.jar Handler *.class && cp handler.jar handler"
	case domain.RuntimeKotlin:
		return "gradle:8-jdk21", "cd /work && kotlinc Handler.kt -include-runtime -d handler.jar && cp handler.jar handler"
	case domain.RuntimeSwift:
		// Use -static-executable to produce a fully static binary for base.ext4 (no libc)
		return "swift:5.10", "cd /work && swiftc -o handler -static-executable main.swift"
	case domain.RuntimeZig:
		return "euantorano/zig:0.13.0", "cd /work && zig build-exe main.zig -name handler -target x86_64-linux-musl"
	case domain.RuntimeDotnet:
		return "mcr.microsoft.com/dotnet/sdk:8.0", "cd /work && dotnet publish -c Release -r linux-musl-x64 -o out && cp out/handler /work/handler"
	case domain.RuntimeScala:
		// Build a fat JAR including the Scala standard library so it runs standalone on java.ext4
		return "sbtscala/scala-sbt:eclipse-temurin-21.0.2_13_1.10.1_3.5.1",
			`cd /work && scalac Handler.scala && ` +
				`SCALA_LIB=$(find / -name "scala-library*.jar" 2>/dev/null | head -1) && ` +
				`mkdir -p /tmp/fatjar && cd /tmp/fatjar && ` +
				`jar xf "$SCALA_LIB" && ` +
				`cp /work/*.class . && ` +
				`jar cfe /work/handler.jar Handler -C . . && ` +
				`cp /work/handler.jar /work/handler`
	default:
		return "", ""
	}
}

func runtimeExtension(runtime domain.Runtime) string {
	exts := map[domain.Runtime]string{
		domain.RuntimePython: ".py",
		domain.RuntimeGo:     ".go",
		domain.RuntimeRust:   ".rs",
		domain.RuntimeNode:   ".js",
		domain.RuntimeRuby:   ".rb",
		domain.RuntimeJava:   ".java",
		domain.RuntimeDeno:   ".ts",
		domain.RuntimeBun:    ".ts",
		domain.RuntimeWasm:   ".wasm",
		domain.RuntimePHP:    ".php",
		domain.RuntimeDotnet: ".cs",
		domain.RuntimeElixir: ".exs",
		domain.RuntimeKotlin: ".kt",
		domain.RuntimeSwift:  ".swift",
		domain.RuntimeZig:    ".zig",
		domain.RuntimeLua:    ".lua",
		domain.RuntimePerl:   ".pl",
		domain.RuntimeR:      ".R",
		domain.RuntimeJulia:  ".jl",
		domain.RuntimeScala:  ".scala",
	}
	if ext, ok := exts[runtime]; ok {
		return ext
	}
	return ".txt"
}

func hashBytes(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))[:16]
}
