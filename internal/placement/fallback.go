package placement

import (
	"fmt"

	"github.com/oriys/nova/internal/domain"
)

// FallbackPolicy controls behavior when the preferred architecture is unavailable.
type FallbackPolicy int

const (
	// FallbackReject rejects the request with a clear error.
	FallbackReject FallbackPolicy = iota
	// FallbackEmulate allows running on a different arch via QEMU user-static.
	FallbackEmulate
	// FallbackAny selects any available node regardless of architecture.
	FallbackAny
)

// FallbackConfig configures architecture fallback behavior.
type FallbackConfig struct {
	Policy          FallbackPolicy `json:"policy"`
	EmulationBinary string         `json:"emulation_binary"` // e.g., "/usr/bin/qemu-aarch64-static"
	WarnOnFallback  bool           `json:"warn_on_fallback"` // Log warning when fallback triggers
}

// DefaultFallbackConfig returns the default fallback configuration.
func DefaultFallbackConfig() FallbackConfig {
	return FallbackConfig{
		Policy:         FallbackReject,
		WarnOnFallback: true,
	}
}

// FallbackResult describes the outcome of a fallback decision.
type FallbackResult struct {
	OriginalArch domain.Arch `json:"original_arch"`
	ResolvedArch domain.Arch `json:"resolved_arch"`
	Emulated     bool        `json:"emulated"`
	EmulationCmd string      `json:"emulation_cmd,omitempty"`
	Warning      string      `json:"warning,omitempty"`
}

// ResolveFallbackArch determines what architecture to use when the preferred one is unavailable.
func ResolveFallbackArch(preferred domain.Arch, available []domain.Arch, cfg FallbackConfig) (*FallbackResult, error) {
	// Check if preferred arch is directly available
	for _, a := range available {
		if a == preferred {
			return &FallbackResult{
				OriginalArch: preferred,
				ResolvedArch: preferred,
			}, nil
		}
	}

	// Preferred not available, apply fallback policy
	switch cfg.Policy {
	case FallbackReject:
		return nil, &ArchNotAvailableError{
			Requested: preferred,
			Available: available,
		}

	case FallbackEmulate:
		if len(available) == 0 {
			return nil, &ArchNotAvailableError{Requested: preferred}
		}
		// Prefer amd64 for emulation (most widely supported)
		target := available[0]
		for _, a := range available {
			if a == domain.ArchAMD64 {
				target = a
				break
			}
		}
		return &FallbackResult{
			OriginalArch: preferred,
			ResolvedArch: target,
			Emulated:     true,
			EmulationCmd: cfg.EmulationBinary,
			Warning:      fmt.Sprintf("running %s workload on %s via emulation (expect ~5-10x performance penalty)", preferred, target),
		}, nil

	case FallbackAny:
		if len(available) == 0 {
			return nil, &ArchNotAvailableError{Requested: preferred}
		}
		target := available[0]
		result := &FallbackResult{
			OriginalArch: preferred,
			ResolvedArch: target,
		}
		if target != preferred && cfg.WarnOnFallback {
			result.Warning = fmt.Sprintf("requested arch %s unavailable, using %s (native)", preferred, target)
		}
		return result, nil

	default:
		return nil, fmt.Errorf("unknown fallback policy: %d", cfg.Policy)
	}
}

// ArchNotAvailableError is returned when the requested architecture is not available.
type ArchNotAvailableError struct {
	Requested domain.Arch
	Available []domain.Arch
}

func (e *ArchNotAvailableError) Error() string {
	if len(e.Available) == 0 {
		return fmt.Sprintf("architecture %s not available, no nodes registered", e.Requested)
	}
	return fmt.Sprintf("architecture %s not available, available: %v", e.Requested, e.Available)
}
