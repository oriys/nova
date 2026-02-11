package kubernetes

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/backend"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
)

// Manager manages Kubernetes pods for function execution.
// It implements the backend.Backend interface and supports scale-to-zero
// by tracking pod idle time and deleting pods that exceed the grace period.
type Manager struct {
	config *Config
	pods   map[string]*podInfo
	mu     sync.RWMutex
}

// podInfo tracks a running function pod.
type podInfo struct {
	vm      *backend.VM
	podName string
}

// NewManager creates a new Kubernetes backend manager.
func NewManager(cfg *Config) (*Manager, error) {
	// Verify kubectl is available
	if err := exec.Command("kubectl", "version", "--client", "--output=yaml").Run(); err != nil {
		return nil, fmt.Errorf("kubectl not available: %w", err)
	}

	// Ensure namespace exists (create if missing)
	if err := ensureNamespace(cfg.Namespace); err != nil {
		return nil, fmt.Errorf("ensure namespace %s: %w", cfg.Namespace, err)
	}

	return &Manager{
		config: cfg,
		pods:   make(map[string]*podInfo),
	}, nil
}

// ensureNamespace creates the namespace if it doesn't exist.
func ensureNamespace(ns string) error {
	// Check if namespace exists
	check := exec.Command("kubectl", "get", "namespace", ns, "--no-headers", "--ignore-not-found")
	out, err := check.Output()
	if err != nil {
		return fmt.Errorf("check namespace: %w", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		return nil // namespace exists
	}

	// Create namespace
	create := exec.Command("kubectl", "create", "namespace", ns)
	if output, err := create.CombinedOutput(); err != nil {
		return fmt.Errorf("create namespace: %w: %s", err, output)
	}
	return nil
}

// CreateVM creates a new Kubernetes pod for the given function.
func (m *Manager) CreateVM(ctx context.Context, fn *domain.Function, codeContent []byte) (*backend.VM, error) {
	return m.createPod(ctx, fn, map[string][]byte{"handler": codeContent})
}

// CreateVMWithFiles creates a new Kubernetes pod with multiple code files.
func (m *Manager) CreateVMWithFiles(ctx context.Context, fn *domain.Function, files map[string][]byte) (*backend.VM, error) {
	return m.createPod(ctx, fn, files)
}

// createPod creates a Kubernetes pod with the nova-agent and function code.
func (m *Manager) createPod(ctx context.Context, fn *domain.Function, files map[string][]byte) (*backend.VM, error) {
	vmID := uuid.New().String()[:12]
	podName := fmt.Sprintf("nova-fn-%s-%s", sanitizeName(fn.Name), vmID)

	// Determine the runtime image
	var image string
	if fn.RuntimeImageName != "" {
		image = fn.RuntimeImageName
	} else {
		image = imageForRuntime(fn.Runtime, m.config.ImagePrefix)
	}

	// Build pod manifest
	manifest := m.buildPodManifest(podName, fn, image)

	// Apply the pod manifest
	applyCmd := exec.CommandContext(ctx, "kubectl", "apply", "-n", m.config.Namespace, "-f", "-")
	applyCmd.Stdin = strings.NewReader(manifest)
	if output, err := applyCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("kubectl apply pod failed: %w: %s", err, output)
	}

	logging.Op().Debug("creating K8s pod", "pod", podName, "image", image, "namespace", m.config.Namespace)

	// Wait for the pod to be running
	if err := waitForPodRunning(ctx, m.config.Namespace, podName, m.config.AgentTimeout); err != nil {
		m.deletePod(podName)
		return nil, fmt.Errorf("pod not ready: %w", err)
	}

	// Get the pod IP
	podIP, err := getPodIP(ctx, m.config.Namespace, podName)
	if err != nil {
		m.deletePod(podName)
		return nil, fmt.Errorf("get pod IP: %w", err)
	}

	agentAddr := fmt.Sprintf("%s:%d", podIP, m.config.AgentPort)

	// Copy code files into the running pod
	if err := m.copyCodeToPod(ctx, podName, files); err != nil {
		m.deletePod(podName)
		return nil, fmt.Errorf("copy code to pod: %w", err)
	}

	// Wait for the agent inside the pod to be ready
	if err := waitForAgent(agentAddr, m.config.AgentTimeout); err != nil {
		m.deletePod(podName)
		return nil, fmt.Errorf("agent not ready: %w", err)
	}

	vm := &backend.VM{
		ID:        vmID,
		Runtime:   fn.Runtime,
		State:     backend.VMStateRunning,
		GuestIP:   agentAddr,
		KubePod:   podName,
		KubeNS:    m.config.Namespace,
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
	}

	metrics.Global().RecordVMCreated()

	m.mu.Lock()
	m.pods[vmID] = &podInfo{vm: vm, podName: podName}
	m.mu.Unlock()

	logging.Op().Info("K8s pod ready", "pod", podName, "addr", agentAddr, "namespace", m.config.Namespace)
	return vm, nil
}

// buildPodManifest generates a Kubernetes pod YAML manifest.
func (m *Manager) buildPodManifest(podName string, fn *domain.Function, image string) string {
	memoryMB := fn.MemoryMB
	if memoryMB <= 0 {
		memoryMB = 128
	}

	// Build environment variables
	var envLines string
	for k, v := range fn.EnvVars {
		envLines += fmt.Sprintf("        - name: %q\n          value: %q\n", k, v)
	}

	// Optional RuntimeClassName for kata-container or gVisor sandboxing
	var runtimeClassLine string
	if m.config.RuntimeClassName != "" {
		runtimeClassLine = fmt.Sprintf("  runtimeClassName: %q\n", m.config.RuntimeClassName)
	}

	// Optional node selector
	var nodeSelectorLines string
	if m.config.NodeSelector != "" {
		parts := strings.SplitN(m.config.NodeSelector, "=", 2)
		if len(parts) == 2 {
			nodeSelectorLines = fmt.Sprintf("  nodeSelector:\n    %s: %q\n", parts[0], parts[1])
		}
	}

	// Optional service account
	var serviceAccountLine string
	if m.config.ServiceAccount != "" {
		serviceAccountLine = fmt.Sprintf("  serviceAccountName: %q\n", m.config.ServiceAccount)
	}

	manifest := fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %q
  namespace: %q
  labels:
    app: nova-function
    nova.dev/function: %q
    nova.dev/runtime: %q
  annotations:
    nova.dev/managed-by: nova
spec:
%s%s%s  terminationGracePeriodSeconds: 5
  containers:
  - name: agent
    image: %q
    ports:
    - containerPort: %d
      protocol: TCP
    env:
    - name: NOVA_AGENT_MODE
      value: "tcp"
    - name: NOVA_SKIP_MOUNT
      value: "true"
%s    resources:
      requests:
        memory: %q
      limits:
        memory: %q
    volumeMounts:
    - name: code
      mountPath: /code
  - name: runtime
    image: %q
    command: ["sleep", "infinity"]
    volumeMounts:
    - name: code
      mountPath: /code
    resources:
      requests:
        memory: %q
      limits:
        memory: %q
  volumes:
  - name: code
    emptyDir:
      sizeLimit: "64Mi"
`,
		podName,
		m.config.Namespace,
		sanitizeName(fn.Name),
		string(fn.Runtime),
		runtimeClassLine,
		nodeSelectorLines,
		serviceAccountLine,
		m.config.AgentImage,
		m.config.AgentPort,
		envLines,
		fmt.Sprintf("%dMi", memoryMB/4), // agent gets 1/4 of memory
		fmt.Sprintf("%dMi", memoryMB/4),
		image,
		fmt.Sprintf("%dMi", memoryMB),
		fmt.Sprintf("%dMi", memoryMB),
	)
	return manifest
}

// copyCodeToPod copies code files into the pod's /code directory.
func (m *Manager) copyCodeToPod(ctx context.Context, podName string, files map[string][]byte) error {
	for path, content := range files {
		// Write content to a temp file, then kubectl cp into pod
		tmpFile, err := os.CreateTemp("", "nova-code-*")
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		if _, err := tmpFile.Write(content); err != nil {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			return fmt.Errorf("write temp file: %w", err)
		}
		tmpFile.Close()

		// Copy file into the agent container
		dest := fmt.Sprintf("%s/%s:%s", m.config.Namespace, podName, "/code/"+path)
		cpCmd := exec.CommandContext(ctx, "kubectl", "cp", tmpFile.Name(), dest, "-c", "agent", "-n", m.config.Namespace)
		if output, err := cpCmd.CombinedOutput(); err != nil {
			os.Remove(tmpFile.Name())
			return fmt.Errorf("kubectl cp %s: %w: %s", path, err, output)
		}
		os.Remove(tmpFile.Name())

		// Make the file executable (best-effort: some runtimes like Python don't need it)
		chmodCmd := exec.CommandContext(ctx, "kubectl", "exec", "-n", m.config.Namespace, podName, "-c", "agent", "--",
			"chmod", "+x", "/code/"+path)
		if chmodOut, chmodErr := chmodCmd.CombinedOutput(); chmodErr != nil {
			logging.Op().Debug("chmod failed (non-fatal)", "path", path, "error", chmodErr, "output", string(chmodOut))
		}
	}
	return nil
}

// StopVM stops and deletes a Kubernetes pod.
func (m *Manager) StopVM(vmID string) error {
	m.mu.Lock()
	info, ok := m.pods[vmID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("pod not found: %s", vmID)
	}
	delete(m.pods, vmID)
	m.mu.Unlock()

	metrics.Global().RecordVMStopped()
	return m.deletePod(info.podName)
}

// deletePod removes a pod from the cluster.
func (m *Manager) deletePod(podName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "delete", "pod", podName,
		"-n", m.config.Namespace, "--grace-period=5", "--ignore-not-found")
	if output, err := cmd.CombinedOutput(); err != nil {
		logging.Op().Warn("failed to delete pod", "pod", podName, "error", err, "output", string(output))
		return err
	}
	return nil
}

// NewClient creates a TCP client for the pod's agent.
func (m *Manager) NewClient(vm *backend.VM) (backend.Client, error) {
	return NewClient(vm), nil
}

// Shutdown stops all managed pods.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.pods))
	for id := range m.pods {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(vmID string) {
			defer wg.Done()
			m.StopVM(vmID)
		}(id)
	}
	wg.Wait()
}

// SnapshotDir returns empty string - K8s backend doesn't support VM snapshots.
func (m *Manager) SnapshotDir() string {
	return ""
}

// waitForPodRunning waits until the pod is in Running phase.
func waitForPodRunning(ctx context.Context, namespace, podName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.CommandContext(ctx, "kubectl", "get", "pod", podName,
			"-n", namespace, "-o", "jsonpath={.status.phase}")
		out, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(out)) == "Running" {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for pod %s to be running", podName)
}

// getPodIP retrieves the pod's cluster IP.
func getPodIP(ctx context.Context, namespace, podName string) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "pod", podName,
		"-n", namespace, "-o", "jsonpath={.status.podIP}")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get pod IP: %w", err)
	}
	ip := strings.TrimSpace(string(out))
	if ip == "" {
		return "", fmt.Errorf("pod %s has no IP", podName)
	}
	return ip, nil
}

// waitForAgent polls the agent TCP port until it's ready.
func waitForAgent(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for agent on %s", addr)
}

// sanitizeName converts a function name to a valid Kubernetes label value.
func sanitizeName(name string) string {
	s := strings.ToLower(name)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, s)
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.Trim(s, "-")
}

// imageForRuntime maps a runtime to a container image name.
func imageForRuntime(rt domain.Runtime, prefix string) string {
	r := string(rt)
	switch {
	case r == string(domain.RuntimePython) || strings.HasPrefix(r, "python"):
		return prefix + "-python"
	case r == string(domain.RuntimeNode) || strings.HasPrefix(r, "node"):
		return prefix + "-node"
	case r == string(domain.RuntimeGo) || strings.HasPrefix(r, "go"):
		return prefix + "-base"
	case r == string(domain.RuntimeRust):
		return prefix + "-base"
	case r == string(domain.RuntimeRuby) || strings.HasPrefix(r, "ruby"):
		return prefix + "-ruby"
	case r == string(domain.RuntimeJava) || strings.HasPrefix(r, "java") ||
		r == string(domain.RuntimeKotlin) || r == string(domain.RuntimeScala):
		return prefix + "-java"
	case r == string(domain.RuntimePHP) || strings.HasPrefix(r, "php"):
		return prefix + "-php"
	case r == string(domain.RuntimeLua) || strings.HasPrefix(r, "lua"):
		return prefix + "-lua"
	case r == string(domain.RuntimeDotnet) || strings.HasPrefix(r, "dotnet"):
		return prefix + "-dotnet"
	case r == string(domain.RuntimeDeno) || strings.HasPrefix(r, "deno"):
		return prefix + "-deno"
	case r == string(domain.RuntimeBun) || strings.HasPrefix(r, "bun"):
		return prefix + "-bun"
	case r == string(domain.RuntimeWasm) || strings.HasPrefix(r, "wasm"):
		return prefix + "-wasm"
	default:
		return prefix + "-base"
	}
}

// Client communicates with the nova-agent inside a Kubernetes pod via TCP.
type Client struct {
	vm          *backend.VM
	conn        net.Conn
	mu          sync.Mutex
	initPayload json.RawMessage
}

// NewClient creates a new TCP client for the pod's agent.
func NewClient(vm *backend.VM) *Client {
	return &Client{vm: vm}
}

// Init sends the init message to the agent.
func (c *Client) Init(fn *domain.Function) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, _ := json.Marshal(&backend.InitPayload{
		Runtime:         string(fn.Runtime),
		Handler:         fn.Handler,
		EnvVars:         fn.EnvVars,
		Command:         fn.RuntimeCommand,
		Extension:       fn.RuntimeExtension,
		Mode:            string(fn.Mode),
		FunctionName:    fn.Name,
		FunctionVersion: fn.Version,
		MemoryMB:        fn.MemoryMB,
		TimeoutS:        fn.TimeoutS,
	})
	c.initPayload = payload

	if err := c.redialAndInit(5 * time.Second); err != nil {
		return err
	}
	return c.closeLocked()
}

// Execute runs a function invocation.
func (c *Client) Execute(reqID string, input json.RawMessage, timeoutS int) (*backend.RespPayload, error) {
	return c.ExecuteWithTrace(reqID, input, timeoutS, "", "")
}

// ExecuteWithTrace runs a function with W3C trace context.
func (c *Client) ExecuteWithTrace(reqID string, input json.RawMessage, timeoutS int, traceParent, traceState string) (*backend.RespPayload, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, _ := json.Marshal(&backend.ExecPayload{
		RequestID:   reqID,
		Input:       input,
		TimeoutS:    timeoutS,
		TraceParent: traceParent,
		TraceState:  traceState,
	})

	execMsg := &backend.VsockMessage{Type: backend.MsgTypeExec, Payload: payload}

	backoff := []time.Duration{10 * time.Millisecond, 25 * time.Millisecond, 50 * time.Millisecond}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := c.redialAndInit(5 * time.Second); err != nil {
			lastErr = err
			if attempt < 2 {
				time.Sleep(backoff[attempt])
			}
			continue
		}

		deadline := time.Now().Add(time.Duration(timeoutS+5) * time.Second)
		_ = c.conn.SetDeadline(deadline)

		if err := c.sendLocked(execMsg); err != nil {
			lastErr = err
			_ = c.closeLocked()
			if isBrokenConnErr(err) && attempt < 2 {
				time.Sleep(backoff[attempt])
				continue
			}
			return nil, err
		}

		resp, err := c.receiveLocked()
		_ = c.conn.SetDeadline(time.Time{})
		if err != nil {
			lastErr = err
			_ = c.closeLocked()
			if isBrokenConnErr(err) && attempt < 2 {
				time.Sleep(backoff[attempt])
				continue
			}
			return nil, err
		}

		var result backend.RespPayload
		if err := json.Unmarshal(resp.Payload, &result); err != nil {
			_ = c.closeLocked()
			return nil, err
		}

		_ = c.closeLocked()
		return &result, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("execute failed")
}

// ExecuteStream runs a function in streaming mode.
func (c *Client) ExecuteStream(reqID string, input json.RawMessage, timeoutS int, traceParent, traceState string, callback func(chunk []byte, isLast bool, err error) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, _ := json.Marshal(&backend.ExecPayload{
		RequestID:   reqID,
		Input:       input,
		TimeoutS:    timeoutS,
		TraceParent: traceParent,
		TraceState:  traceState,
		Stream:      true,
	})

	execMsg := &backend.VsockMessage{Type: backend.MsgTypeExec, Payload: payload}

	if err := c.redialAndInit(5 * time.Second); err != nil {
		return err
	}

	deadline := time.Now().Add(time.Duration(timeoutS+5) * time.Second)
	_ = c.conn.SetDeadline(deadline)

	if err := c.sendLocked(execMsg); err != nil {
		_ = c.closeLocked()
		return err
	}

	for {
		resp, err := c.receiveLocked()
		if err != nil {
			_ = c.closeLocked()
			return err
		}

		if resp.Type != backend.MsgTypeStream {
			_ = c.closeLocked()
			return fmt.Errorf("unexpected message type: %d (expected stream)", resp.Type)
		}

		var chunk backend.StreamChunkPayload
		if err := json.Unmarshal(resp.Payload, &chunk); err != nil {
			_ = c.closeLocked()
			return err
		}

		var chunkErr error
		if chunk.Error != "" {
			chunkErr = fmt.Errorf("%s", chunk.Error)
		}
		if err := callback(chunk.Data, chunk.IsLast, chunkErr); err != nil {
			_ = c.closeLocked()
			return err
		}

		if chunk.IsLast {
			break
		}
	}

	_ = c.conn.SetDeadline(time.Time{})
	_ = c.closeLocked()
	return nil
}

// Ping checks if the agent is responsive.
func (c *Client) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.redialAndInit(5 * time.Second); err != nil {
		return err
	}
	defer c.closeLocked()

	_ = c.conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err := c.sendLocked(&backend.VsockMessage{Type: backend.MsgTypePing}); err != nil {
		return err
	}
	_, err := c.receiveLocked()
	_ = c.conn.SetDeadline(time.Time{})
	return err
}

// Reload sends new code files to the agent for hot reload.
func (c *Client) Reload(files map[string][]byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload, err := json.Marshal(&backend.ReloadPayload{Files: files})
	if err != nil {
		return err
	}

	if err := c.redialAndInit(5 * time.Second); err != nil {
		return err
	}
	defer c.closeLocked()

	_ = c.conn.SetDeadline(time.Now().Add(30 * time.Second))
	if err := c.sendLocked(&backend.VsockMessage{Type: backend.MsgTypeReload, Payload: payload}); err != nil {
		return err
	}

	resp, err := c.receiveLocked()
	_ = c.conn.SetDeadline(time.Time{})
	if err != nil {
		return err
	}

	if resp.Type != backend.MsgTypeResp {
		return fmt.Errorf("unexpected response type: %d", resp.Type)
	}
	return nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *Client) closeLocked() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) dialLocked(timeout time.Duration) error {
	addr := c.vm.GuestIP
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

func (c *Client) initLocked() error {
	if c.initPayload == nil {
		return errors.New("missing init payload")
	}
	if err := c.sendLocked(&backend.VsockMessage{Type: backend.MsgTypeInit, Payload: c.initPayload}); err != nil {
		return err
	}
	resp, err := c.receiveLocked()
	if err != nil {
		return err
	}
	if resp.Type != backend.MsgTypeResp {
		return fmt.Errorf("unexpected response type: %d", resp.Type)
	}
	return nil
}

func (c *Client) redialAndInit(timeout time.Duration) error {
	hadConn := c.conn != nil
	_ = c.closeLocked()
	if hadConn {
		time.Sleep(10 * time.Millisecond)
	}
	if err := c.dialLocked(timeout); err != nil {
		return err
	}
	if c.initPayload != nil {
		if err := c.initLocked(); err != nil {
			_ = c.closeLocked()
			return err
		}
	}
	return nil
}

func (c *Client) sendLocked(msg *backend.VsockMessage) error {
	if c.conn == nil {
		return errors.New("not connected")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	buf := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(data)))
	copy(buf[4:], data)

	return writeFull(c.conn, buf)
}

func (c *Client) receiveLocked() (*backend.VsockMessage, error) {
	if c.conn == nil {
		return nil, errors.New("not connected")
	}

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, lenBuf); err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint32(lenBuf)
	data := make([]byte, msgLen)
	if _, err := io.ReadFull(c.conn, data); err != nil {
		return nil, err
	}

	var msg backend.VsockMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func writeFull(conn net.Conn, b []byte) error {
	for len(b) > 0 {
		n, err := conn.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

func isBrokenConnErr(err error) bool {
	return err != nil && (errors.Is(err, io.EOF) ||
		errors.Is(err, net.ErrClosed) ||
		strings.Contains(err.Error(), "connection reset") ||
		strings.Contains(err.Error(), "broken pipe"))
}
