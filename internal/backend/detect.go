package backend

import (
	"os"
	"os/exec"
	"runtime"

	"github.com/oriys/nova/internal/domain"
)

// BackendInfo describes an available backend and its detection status.
type BackendInfo struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// DetectAvailableBackends checks which execution backends are available on the current system.
func DetectAvailableBackends() []BackendInfo {
	return []BackendInfo{
		detectFirecracker(),
		detectDocker(),
		detectWasm(),
		detectKubernetes(),
		detectLibKrun(),
		detectKata(),
	}
}

// DetectDefaultBackend returns the best available backend for the current system.
func DetectDefaultBackend() domain.BackendType {
	if runtime.GOOS == "linux" {
		if _, err := os.Stat("/dev/kvm"); err == nil {
			if _, err := exec.LookPath("firecracker"); err == nil {
				return domain.BackendFirecracker
			}
		}
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return domain.BackendDocker
	}
	if _, err := exec.LookPath("wasmtime"); err == nil {
		return domain.BackendWasm
	}
	return domain.BackendDocker
}

func detectFirecracker() BackendInfo {
	info := BackendInfo{Name: "firecracker"}
	if runtime.GOOS != "linux" {
		info.Reason = "requires Linux"
		return info
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		info.Reason = "requires amd64 or arm64 architecture"
		return info
	}
	if _, err := os.Stat("/dev/kvm"); err != nil {
		info.Reason = "KVM not available (/dev/kvm not found)"
		return info
	}
	if _, err := exec.LookPath("firecracker"); err != nil {
		info.Reason = "firecracker binary not found in PATH"
		return info
	}
	info.Available = true
	return info
}

func detectDocker() BackendInfo {
	info := BackendInfo{Name: "docker"}
	if _, err := exec.LookPath("docker"); err != nil {
		info.Reason = "docker not found in PATH"
		return info
	}
	info.Available = true
	return info
}

func detectWasm() BackendInfo {
	info := BackendInfo{Name: "wasm"}
	if _, err := exec.LookPath("wasmtime"); err != nil {
		info.Reason = "wasmtime not found in PATH"
		return info
	}
	info.Available = true
	return info
}

func detectKubernetes() BackendInfo {
	info := BackendInfo{Name: "kubernetes"}
	if _, err := exec.LookPath("kubectl"); err != nil {
		info.Reason = "kubectl not found in PATH"
		return info
	}
	// Check for kubeconfig
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(home + "/.kube/config"); err != nil {
			if v := os.Getenv("KUBECONFIG"); v == "" {
				info.Reason = "no kubeconfig found"
				return info
			}
		}
	}
	info.Available = true
	return info
}

func detectLibKrun() BackendInfo {
	info := BackendInfo{Name: "libkrun"}
	if runtime.GOOS != "linux" {
		info.Reason = "requires Linux"
		return info
	}
	if _, err := exec.LookPath("krun"); err != nil {
		info.Reason = "krun binary not found in PATH"
		return info
	}
	info.Available = true
	return info
}

func detectKata() BackendInfo {
	info := BackendInfo{Name: "kata"}
	if runtime.GOOS != "linux" {
		info.Reason = "requires Linux"
		return info
	}
	if _, err := exec.LookPath("kata-runtime"); err != nil {
		info.Reason = "kata-runtime not found in PATH"
		return info
	}
	info.Available = true
	return info
}
