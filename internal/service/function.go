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
	Name                string                  `json:"name"`
	Runtime             string                  `json:"runtime"`
	Handler             string                  `json:"handler,omitempty"`
	Code                string                  `json:"code"` // Source code (required)
	MemoryMB            int                     `json:"memory_mb,omitempty"`
	TimeoutS            int                     `json:"timeout_s,omitempty"`
	MinReplicas         int                     `json:"min_replicas,omitempty"`
	MaxReplicas         int                     `json:"max_replicas,omitempty"`
	Mode                string                  `json:"mode,omitempty"`
	InstanceConcurrency int                     `json:"instance_concurrency,omitempty"`
	EnvVars             map[string]string       `json:"env_vars,omitempty"`
	Limits              *domain.ResourceLimits  `json:"limits,omitempty"`
	NetworkPolicy       *domain.NetworkPolicy   `json:"network_policy,omitempty"`
	RolloutPolicy       *domain.RolloutPolicy   `json:"rollout_policy,omitempty"`
	AutoScalePolicy     *domain.AutoScalePolicy `json:"auto_scale_policy,omitempty"`
	CapacityPolicy      *domain.CapacityPolicy  `json:"capacity_policy,omitempty"`
}

func (s *FunctionService) CreateFunction(ctx context.Context, req CreateFunctionRequest) (*domain.Function, string, error) {
	rt := domain.Runtime(req.Runtime)
	if rt == "" {
		return nil, "", fmt.Errorf("runtime is required")
	}
	if !rt.IsValid() {
		// Not a built-in runtime, check if it exists in DB
		if _, err := s.store.GetRuntime(ctx, string(rt)); err != nil {
			return nil, "", fmt.Errorf("invalid runtime: %s", req.Runtime)
		}
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
		ID:                  uuid.New().String(),
		Name:                req.Name,
		Runtime:             rt,
		Handler:             req.Handler,
		CodeHash:            codeHash,
		MemoryMB:            req.MemoryMB,
		TimeoutS:            req.TimeoutS,
		MinReplicas:         req.MinReplicas,
		MaxReplicas:         req.MaxReplicas,
		Mode:                domain.ExecutionMode(req.Mode),
		InstanceConcurrency: req.InstanceConcurrency,
		EnvVars:             req.EnvVars,
		Limits:              req.Limits,
		NetworkPolicy:       req.NetworkPolicy,
		RolloutPolicy:       req.RolloutPolicy,
		AutoScalePolicy:     req.AutoScalePolicy,
		CapacityPolicy:      req.CapacityPolicy,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	// Set default network policy to ensure isolation (NetNS)
	if fn.NetworkPolicy == nil {
		fn.NetworkPolicy = &domain.NetworkPolicy{
			IsolationMode: "egress-only",
		}
	} else if fn.NetworkPolicy.IsolationMode == "" {
		fn.NetworkPolicy.IsolationMode = "egress-only"
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
