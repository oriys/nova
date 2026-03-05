package invoke

import (
	"fmt"

	"github.com/oriys/nova/internal/domain"
)

// ErrInvokeNotAllowed is returned when the caller is not permitted to invoke
// the target function.
var ErrInvokeNotAllowed = fmt.Errorf("caller is not allowed to invoke this function")

// CheckInvokePolicy validates that the given caller function is permitted to
// invoke the target function according to its InvokePolicy.
//
// If policy is nil, invocation is allowed (open by default).
// If callerFunction is empty, this is a top-level external request (always allowed).
func CheckInvokePolicy(policy *domain.InvokePolicy, callerFunction string) error {
	// No policy = open access
	if policy == nil {
		return nil
	}

	// Top-level external requests are always allowed
	if callerFunction == "" {
		return nil
	}

	// Check deny list first (deny takes precedence)
	for _, pattern := range policy.DenyCallers {
		if matchPattern(pattern, callerFunction) {
			return fmt.Errorf("%w: %q is denied by policy", ErrInvokeNotAllowed, callerFunction)
		}
	}

	// Allow-all permits everything not explicitly denied
	if policy.AllowAll {
		return nil
	}

	// Check allow list
	for _, pattern := range policy.AllowedCallers {
		if matchPattern(pattern, callerFunction) {
			return nil
		}
	}

	return fmt.Errorf("%w: %q is not in the allowed callers list", ErrInvokeNotAllowed, callerFunction)
}

// matchPattern matches a function name against a pattern that supports
// trailing wildcard (e.g. "order-*" matches "order-processor").
func matchPattern(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(name) >= len(prefix) && name[:len(prefix)] == prefix
	}
	return pattern == name
}
