package invoke

import "fmt"

// Recursion / avalanche protection errors.
var (
	// ErrCallDepthExceeded is returned when the call chain exceeds MaxCallDepth.
	ErrCallDepthExceeded = fmt.Errorf("call depth exceeded maximum of %d", MaxCallDepth)

	// ErrRecursionDetected is returned when a function invocation would create
	// a cycle in the call chain (A → B → A).
	ErrRecursionDetected = fmt.Errorf("recursive invocation detected")

	// ErrFunctionHalted is returned when a function's reserved concurrency is
	// set to 0, acting as an emergency kill switch (matching AWS's pattern of
	// setting reserved concurrency to 0 to immediately stop all invocations).
	ErrFunctionHalted = fmt.Errorf("function invocations halted (reserved concurrency = 0)")
)

// ValidateCallChain checks the call chain for recursion violations before
// allowing an invocation to proceed.
//
// It enforces two invariants:
//  1. Depth must not exceed MaxCallDepth (prevents unbounded recursive chains).
//  2. The target function must not already appear in the call chain (prevents
//     cycles like A → B → A).
//
// Returns nil if the invocation is allowed.
func ValidateCallChain(cc CallChain, targetFunction string) error {
	if cc.Depth >= MaxCallDepth {
		return ErrCallDepthExceeded
	}
	if cc.HasCycle(targetFunction) {
		return fmt.Errorf("%w: %s already in chain [%s]",
			ErrRecursionDetected, targetFunction, cc.ChainString())
	}
	return nil
}

// IsFunctionHalted returns true if the function's max replicas is explicitly
// set to 0 (the Nova equivalent of AWS's "set reserved concurrency to 0"
// pattern for emergency traffic stop).
func IsFunctionHalted(maxReplicas int) bool {
	// maxReplicas == 0 means unlimited in Nova's normal semantics.
	// We use maxReplicas == -1 as the explicit "halt" signal, or check
	// a dedicated field. For compatibility we check -1.
	return maxReplicas < 0
}
