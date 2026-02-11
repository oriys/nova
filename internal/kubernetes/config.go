// Package kubernetes provides the Kubernetes backend for function execution.
// It deploys functions as Kubernetes Pods with a nova-agent sidecar and supports
// scale-to-zero inspired by Knative and kata-container patterns.
package kubernetes

import (
	"os"
	"time"
)

// Config holds Kubernetes backend configuration.
type Config struct {
	Kubeconfig       string        `json:"kubeconfig"`        // Path to kubeconfig file (empty for in-cluster)
	Namespace        string        `json:"namespace"`         // Kubernetes namespace for function pods
	ImagePrefix      string        `json:"image_prefix"`      // Container image prefix (e.g., "nova-runtime")
	AgentImage       string        `json:"agent_image"`       // Nova agent sidecar image
	ServiceAccount   string        `json:"service_account"`   // ServiceAccount for function pods
	NodeSelector     string        `json:"node_selector"`     // Node selector label (e.g., "nova.dev/role=function")
	RuntimeClassName string        `json:"runtime_class_name"` // RuntimeClass for pod sandboxing (e.g., "kata" for kata-containers)
	AgentPort        int           `json:"agent_port"`        // Agent port inside the pod (default: 9999)
	DefaultTimeout   time.Duration `json:"default_timeout"`   // Default operation timeout (default: 30s)
	AgentTimeout     time.Duration `json:"agent_timeout"`     // Agent startup timeout (default: 30s)

	// Scale-to-zero settings (Knative-inspired)
	ScaleToZeroGracePeriod time.Duration `json:"scale_to_zero_grace_period"` // Grace period before scaling to zero (default: 60s)
	StableWindow           time.Duration `json:"stable_window"`              // Observation window for stable metrics (default: 60s)
}

// DefaultConfig returns sensible defaults for Kubernetes backend.
func DefaultConfig() *Config {
	kubeconfig := os.Getenv("KUBECONFIG")
	namespace := os.Getenv("NOVA_K8S_NAMESPACE")
	if namespace == "" {
		namespace = "nova-functions"
	}
	imagePrefix := os.Getenv("NOVA_K8S_IMAGE_PREFIX")
	if imagePrefix == "" {
		imagePrefix = "nova-runtime"
	}
	agentImage := os.Getenv("NOVA_K8S_AGENT_IMAGE")
	if agentImage == "" {
		agentImage = "nova-agent:latest"
	}

	return &Config{
		Kubeconfig:             kubeconfig,
		Namespace:              namespace,
		ImagePrefix:            imagePrefix,
		AgentImage:             agentImage,
		ServiceAccount:         os.Getenv("NOVA_K8S_SERVICE_ACCOUNT"),
		NodeSelector:           os.Getenv("NOVA_K8S_NODE_SELECTOR"),
		RuntimeClassName:       os.Getenv("NOVA_K8S_RUNTIME_CLASS"),
		AgentPort:              9999,
		DefaultTimeout:         30 * time.Second,
		AgentTimeout:           30 * time.Second,
		ScaleToZeroGracePeriod: 60 * time.Second,
		StableWindow:           60 * time.Second,
	}
}
