package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/compiler"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/pkg/crypto"
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
	Code        string // Source code (required)
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

	if req.Code == "" {
		return nil, "", fmt.Errorf("code is required")
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

	codeHash := crypto.HashString(req.Code)

	fn := &domain.Function{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Runtime:     rt,
		Handler:     req.Handler,
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

	// Save source code to function_code table
	sourceHash := crypto.HashString(req.Code)
	if err := s.store.SaveFunctionCode(ctx, fn.ID, req.Code, sourceHash); err != nil {
		return nil, "", fmt.Errorf("save code: %w", err)
	}

	var compileStatus domain.CompileStatus = domain.CompileStatusNotRequired

	if s.compiler != nil {
		s.compiler.CompileAsync(ctx, fn, req.Code)
		if domain.NeedsCompilation(rt) {
			compileStatus = domain.CompileStatusCompiling
		}
	} else if domain.NeedsCompilation(rt) {
		compileStatus = domain.CompileStatusPending
	} else {
		// Interpreted language - store source as compiled artifact
		s.store.UpdateCompileResult(ctx, fn.ID, []byte(req.Code), sourceHash, domain.CompileStatusNotRequired, "")
	}

	return fn, string(compileStatus), nil
}
