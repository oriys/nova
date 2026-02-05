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

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/pkg/crypto"
	"github.com/oriys/nova/internal/store"
)

// Compiler handles compilation of function source code using Docker containers.
type Compiler struct {
	store  store.MetadataStore
	tmpDir string
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

	// Run Docker compilation
	args := []string{
		"run", "--rm",
		"-v", workDir + ":/work",
		image,
		"sh", "-c", buildCmd,
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	logging.Op().Info("starting compilation", "function", fn.Name, "runtime", fn.Runtime, "image", image)

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("compilation error: %s\n%s", err, stderr.String())
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
		// Write go.mod if not present in source
		if !bytes.Contains([]byte(sourceCode), []byte("module")) {
			goMod := "module handler\n\ngo 1.23\n"
			if err := os.WriteFile(filepath.Join(workDir, "go.mod"), []byte(goMod), 0644); err != nil {
				return err
			}
		}
	case domain.RuntimeRust:
		// Create Cargo project structure
		srcDir := filepath.Join(workDir, "src")
		os.MkdirAll(srcDir, 0755)
		if err := os.WriteFile(filepath.Join(srcDir, "main.rs"), []byte(sourceCode), 0644); err != nil {
			return err
		}
		cargoToml := `[package]
name = "handler"
version = "0.1.0"
edition = "2021"

[dependencies]
serde = { version = "1", features = ["derive"] }
serde_json = "1"
`
		if err := os.WriteFile(filepath.Join(workDir, "Cargo.toml"), []byte(cargoToml), 0644); err != nil {
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
		return "golang:1.23-alpine", "cd /work && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o handler ."
	case domain.RuntimeRust:
		return "rust:1.84-alpine", "cd /work && cargo build --release && cp target/release/handler /work/handler"
	case domain.RuntimeJava:
		return "eclipse-temurin:21-jdk", "cd /work && javac Handler.java && jar cfe handler.jar Handler *.class && cp handler.jar handler"
	case domain.RuntimeKotlin:
		return "gradle:8-jdk21", "cd /work && kotlinc Handler.kt -include-runtime -d handler.jar && cp handler.jar handler"
	case domain.RuntimeSwift:
		return "swift:5.10", "cd /work && swiftc -o handler main.swift"
	case domain.RuntimeZig:
		return "euantorano/zig:0.13.0", "cd /work && zig build-exe main.zig -name handler -target x86_64-linux-musl"
	case domain.RuntimeDotnet:
		return "mcr.microsoft.com/dotnet/sdk:8.0", "cd /work && dotnet publish -c Release -r linux-musl-x64 -o out && cp out/handler /work/handler"
	case domain.RuntimeScala:
		return "sbtscala/scala-sbt:eclipse-temurin-21.0.2_13_1.10.1_3.5.1", "cd /work && scalac Handler.scala && jar cfe handler.jar Handler *.class && cp handler.jar handler"
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
