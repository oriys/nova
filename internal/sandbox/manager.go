package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	fc "github.com/oriys/nova/internal/firecracker"
)

const (
	defaultMemoryMB  = 512
	defaultVCPUs     = 1
	defaultTimeoutS  = 3600 // 1 hour
	defaultOnIdleS   = 300  // 5 minutes
	cleanupInterval  = 30 * time.Second
)

// Manager manages sandbox lifecycle: create, destroy, exec, file ops.
type Manager struct {
	backend  backend.Backend
	mu       sync.RWMutex
	sandboxes map[string]*sandboxEntry
	stopCh   chan struct{}
}

type sandboxEntry struct {
	sandbox *domain.Sandbox
	vm      *backend.VM
	client  *Client
}

// NewManager creates a new sandbox manager.
func NewManager(b backend.Backend) *Manager {
	m := &Manager{
		backend:   b,
		sandboxes: make(map[string]*sandboxEntry),
		stopCh:    make(chan struct{}),
	}
	go m.cleanupLoop()
	return m
}

// Shutdown stops the cleanup loop and destroys all sandboxes.
func (m *Manager) Shutdown() {
	close(m.stopCh)
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, entry := range m.sandboxes {
		m.destroyLocked(entry)
		delete(m.sandboxes, id)
	}
}

// Create creates a new sandbox VM and returns its metadata.
func (m *Manager) Create(ctx context.Context, req *domain.CreateSandboxRequest) (*domain.Sandbox, error) {
	memoryMB := req.MemoryMB
	if memoryMB <= 0 {
		memoryMB = defaultMemoryMB
	}
	vcpus := req.VCPUs
	if vcpus <= 0 {
		vcpus = defaultVCPUs
	}
	timeoutS := req.TimeoutS
	if timeoutS <= 0 {
		timeoutS = defaultTimeoutS
	}
	onIdleS := req.OnIdleS
	if onIdleS <= 0 {
		onIdleS = defaultOnIdleS
	}
	template := req.Template
	if template == "" {
		template = "python"
	}
	networkPolicy := req.NetworkPolicy
	if networkPolicy == "" {
		networkPolicy = "restricted"
	}

	now := time.Now()
	sb := &domain.Sandbox{
		ID:            generateID(),
		Template:      template,
		Status:        domain.SandboxStatusCreating,
		MemoryMB:      memoryMB,
		VCPUs:         vcpus,
		TimeoutS:      timeoutS,
		OnIdleS:       onIdleS,
		NetworkPolicy: networkPolicy,
		EnvVars:       req.EnvVars,
		CreatedAt:     now,
		LastActiveAt:  now,
		ExpiresAt:     now.Add(time.Duration(timeoutS) * time.Second),
	}

	// Build a synthetic domain.Function to create the VM via the existing backend.
	fn := &domain.Function{
		ID:       sb.ID,
		Name:     "sandbox-" + sb.ID,
		Runtime:  domain.Runtime(template),
		Handler:  "handler",
		MemoryMB: memoryMB,
		Mode:     domain.ModeProcess,
		EnvVars:  req.EnvVars,
	}

	// Create VM with a minimal code drive (sandbox doesn't need pre-loaded code)
	vm, err := m.backend.CreateVM(ctx, fn, []byte("#!/bin/bash\necho sandbox\n"))
	if err != nil {
		sb.Status = domain.SandboxStatusError
		sb.Error = err.Error()
		return sb, fmt.Errorf("create sandbox VM: %w", err)
	}

	sb.VMID = vm.ID
	sb.Status = domain.SandboxStatusRunning

	// Create the vsock client for sandbox communication
	rawClient, err := m.backend.NewClient(vm)
	if err != nil {
		_ = m.backend.StopVM(vm.ID)
		sb.Status = domain.SandboxStatusError
		sb.Error = err.Error()
		return sb, fmt.Errorf("create sandbox client: %w", err)
	}

	client := NewClient(vm, rawClient)

	m.mu.Lock()
	m.sandboxes[sb.ID] = &sandboxEntry{
		sandbox: sb,
		vm:      vm,
		client:  client,
	}
	m.mu.Unlock()

	fmt.Printf("[sandbox] sandbox created id=%s template=%s memory_mb=%d\n", sb.ID, template, memoryMB)
	return sb, nil
}

// Get returns sandbox metadata.
func (m *Manager) Get(id string) (*domain.Sandbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.sandboxes[id]
	if !ok {
		return nil, fmt.Errorf("sandbox not found: %s", id)
	}
	return entry.sandbox, nil
}

// List returns all active sandboxes.
func (m *Manager) List() []*domain.Sandbox {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*domain.Sandbox, 0, len(m.sandboxes))
	for _, entry := range m.sandboxes {
		result = append(result, entry.sandbox)
	}
	return result
}

// Destroy stops and removes a sandbox.
func (m *Manager) Destroy(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.sandboxes[id]
	if !ok {
		return fmt.Errorf("sandbox not found: %s", id)
	}
	m.destroyLocked(entry)
	delete(m.sandboxes, id)
	fmt.Printf("[sandbox] sandbox destroyed id=%s\n", id)
	return nil
}

// Exec executes a shell command inside the sandbox.
func (m *Manager) Exec(id string, req *domain.SandboxExecRequest) (*domain.SandboxExecResponse, error) {
	entry, err := m.getEntry(id)
	if err != nil {
		return nil, err
	}

	m.touchActivity(id)

	resp, err := entry.client.ShellExec(req.Command, req.TimeoutS, req.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("shell exec: %w", err)
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("shell exec agent error: %s", resp.Error)
	}

	return &domain.SandboxExecResponse{
		ExitCode: resp.ExitCode,
		Stdout:   resp.Stdout,
		Stderr:   resp.Stderr,
	}, nil
}

// CodeExec executes a code snippet in the specified language.
func (m *Manager) CodeExec(id string, req *domain.SandboxCodeRequest) (*domain.SandboxExecResponse, error) {
	// Map language to interpreter command
	var cmd string
	switch req.Language {
	case "python", "python3":
		cmd = fmt.Sprintf("python3 -c %s", shellQuote(req.Code))
	case "javascript", "node", "nodejs":
		cmd = fmt.Sprintf("node -e %s", shellQuote(req.Code))
	case "bash", "sh":
		cmd = fmt.Sprintf("bash -c %s", shellQuote(req.Code))
	case "ruby":
		cmd = fmt.Sprintf("ruby -e %s", shellQuote(req.Code))
	case "php":
		cmd = fmt.Sprintf("php -r %s", shellQuote(req.Code))
	default:
		return nil, fmt.Errorf("unsupported language: %s", req.Language)
	}

	return m.Exec(id, &domain.SandboxExecRequest{
		Command:  cmd,
		TimeoutS: req.TimeoutS,
	})
}

// FileRead reads a file from the sandbox.
func (m *Manager) FileRead(id, path string) (*fc.FileRespPayload, error) {
	entry, err := m.getEntry(id)
	if err != nil {
		return nil, err
	}
	m.touchActivity(id)
	return entry.client.FileRead(path)
}

// FileWrite writes a file in the sandbox (content is base64-encoded).
func (m *Manager) FileWrite(id, path, content string, perm int) (*fc.FileRespPayload, error) {
	entry, err := m.getEntry(id)
	if err != nil {
		return nil, err
	}
	m.touchActivity(id)
	return entry.client.FileWrite(path, content, perm)
}

// FileList lists directory contents in the sandbox.
func (m *Manager) FileList(id, path string) (*fc.FileRespPayload, error) {
	entry, err := m.getEntry(id)
	if err != nil {
		return nil, err
	}
	m.touchActivity(id)
	return entry.client.FileList(path)
}

// FileDelete deletes a file or directory in the sandbox.
func (m *Manager) FileDelete(id, path string) (*fc.FileRespPayload, error) {
	entry, err := m.getEntry(id)
	if err != nil {
		return nil, err
	}
	m.touchActivity(id)
	return entry.client.FileDelete(path)
}

// ProcessList lists processes in the sandbox.
func (m *Manager) ProcessList(id string) (*fc.ProcessListRespPayload, error) {
	entry, err := m.getEntry(id)
	if err != nil {
		return nil, err
	}
	m.touchActivity(id)
	return entry.client.ProcessList()
}

// ProcessKill kills a process in the sandbox.
func (m *Manager) ProcessKill(id string, pid, signal int) (*fc.ProcessKillRespPayload, error) {
	entry, err := m.getEntry(id)
	if err != nil {
		return nil, err
	}
	m.touchActivity(id)
	return entry.client.ProcessKill(pid, signal)
}

// Keepalive extends the sandbox expiration.
func (m *Manager) Keepalive(id string) (*domain.Sandbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.sandboxes[id]
	if !ok {
		return nil, fmt.Errorf("sandbox not found: %s", id)
	}
	now := time.Now()
	entry.sandbox.LastActiveAt = now
	entry.sandbox.ExpiresAt = now.Add(time.Duration(entry.sandbox.TimeoutS) * time.Second)
	// Return a copy to avoid races on read
	sb := *entry.sandbox
	return &sb, nil
}

// ─── Internal helpers ───────────────────────────────────

func (m *Manager) getEntry(id string) (*sandboxEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.sandboxes[id]
	if !ok {
		return nil, fmt.Errorf("sandbox not found: %s", id)
	}
	if entry.sandbox.Status != domain.SandboxStatusRunning {
		return nil, fmt.Errorf("sandbox %s is not running (status: %s)", id, entry.sandbox.Status)
	}
	return entry, nil
}

func (m *Manager) touchActivity(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.sandboxes[id]; ok {
		entry.sandbox.LastActiveAt = time.Now()
	}
}

func (m *Manager) destroyLocked(entry *sandboxEntry) {
	entry.sandbox.Status = domain.SandboxStatusStopped
	if entry.client != nil {
		_ = entry.client.Close()
	}
	if entry.vm != nil {
		_ = m.backend.StopVM(entry.vm.ID)
	}
}

func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.cleanupExpired()
		}
	}
}

func (m *Manager) cleanupExpired() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, entry := range m.sandboxes {
		sb := entry.sandbox
		if sb.Status != domain.SandboxStatusRunning {
			continue
		}
		// Check absolute expiration
		if now.After(sb.ExpiresAt) {
			fmt.Printf("[sandbox] sandbox expired (timeout) id=%s\n", id)
			m.destroyLocked(entry)
			delete(m.sandboxes, id)
			continue
		}
		// Check idle expiration
		idleDeadline := sb.LastActiveAt.Add(time.Duration(sb.OnIdleS) * time.Second)
		if now.After(idleDeadline) {
			fmt.Printf("[sandbox] sandbox expired (idle) id=%s\n", id)
			m.destroyLocked(entry)
			delete(m.sandboxes, id)
		}
	}
}

func generateID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// Fallback: should never happen with crypto/rand
		b = []byte(fmt.Sprintf("%016x", time.Now().UnixNano()))
	}
	return "sb-" + hex.EncodeToString(b)
}

func shellQuote(s string) string {
	// Use single-quote wrapping with proper escaping:
	// replace each ' with '\'' (end quote, escaped quote, start quote)
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
