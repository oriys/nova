package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/compiler"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/pkg/crypto"
	"github.com/oriys/nova/internal/pkg/fsutil"
	"github.com/oriys/nova/internal/store"
)

type FunctionService struct {
	store    *store.Store
	compiler *compiler.Compiler
}

func NewFunctionService(s *store.Store, c *compiler.Compiler) *FunctionService {
	return &FunctionService{
		store:    s,
		compiler: c,
	}
}

type CreateFunctionRequest struct {
	Name        string
	Runtime     string
	Handler     string
	CodePath    string
	Code        string
	MemoryMB    int
	TimeoutS    int
	MinReplicas int
	MaxReplicas int
	Mode        string
	EnvVars     map[string]string
	Limits      *domain.ResourceLimits
}

func (s *FunctionService) CreateFunction(ctx context.Context, req CreateFunctionRequest) (*domain.Function, string, error) {
	rt := domain.Runtime(req.Runtime)
	if !rt.IsValid() {
		return nil, "", fmt.Errorf("invalid runtime: %s", req.Runtime)
	}

	// Check if function name already exists
	if existing, _ := s.store.GetFunctionByName(ctx, req.Name); existing != nil {
		return nil, "", fmt.Errorf("function '%s' already exists", req.Name)
	}

	// Set defaults
	if req.Handler == "" {
		req.Handler = "main.handler"
	}
	if req.MemoryMB == 0 {
		req.MemoryMB = 128
	}
	if req.TimeoutS == 0 {
		req.TimeoutS = 30
	}
	if req.Mode == "" {
		req.Mode = string(domain.ModeProcess)
	}

	codePath := req.CodePath
	var codeHash string

	if req.Code != "" {
		codeHash = crypto.HashString(req.Code)
		funcDir := filepath.Join(os.TempDir(), "nova-functions")
		os.MkdirAll(funcDir, 0755)
		ext := runtimeExtension(rt)
		codePath = filepath.Join(funcDir, req.Name+ext)
	} else {
		if _, err := os.Stat(req.CodePath); os.IsNotExist(err) {
			return nil, "", fmt.Errorf("code path not found: %s", req.CodePath)
		}
		var err error
		codeHash, err = fsutil.HashFile(codePath)
		if err != nil {
			return nil, "", fmt.Errorf("hash code file: %w", err)
		}
	}

	fn := &domain.Function{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Runtime:     rt,
		Handler:     req.Handler,
		CodePath:    codePath,
		CodeHash:    codeHash,
		MemoryMB:    req.MemoryMB,
		TimeoutS:    req.TimeoutS,
		MinReplicas: req.MinReplicas,
		MaxReplicas: req.MaxReplicas,
		Mode:        domain.ExecutionMode(req.Mode),
		EnvVars:     req.EnvVars,
		Limits:      req.Limits,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.store.SaveFunction(ctx, fn); err != nil {
		return nil, "", err
	}

	var compileStatus domain.CompileStatus = domain.CompileStatusNotRequired

	if req.Code != "" {
		sourceHash := crypto.HashString(req.Code)
		if err := s.store.SaveFunctionCode(ctx, fn.ID, req.Code, sourceHash); err != nil {
			return nil, "", fmt.Errorf("save code: %w", err)
		}

		if s.compiler != nil {
			s.compiler.CompileAsync(ctx, fn, req.Code)
			if domain.NeedsCompilation(rt) {
				compileStatus = domain.CompileStatusCompiling
			}
		} else if !domain.NeedsCompilation(rt) {
			if err := os.WriteFile(codePath, []byte(req.Code), 0644); err != nil {
				return nil, "", fmt.Errorf("write code: %w", err)
			}
			s.store.UpdateCompileResult(ctx, fn.ID, []byte(req.Code), sourceHash, domain.CompileStatusNotRequired, "")
		} else {
			compileStatus = domain.CompileStatusPending
		}
	}

	return fn, string(compileStatus), nil
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
