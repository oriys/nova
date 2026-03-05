// Package invoke defines the invocation type system, payload constraints, and
// call-chain context for Nova's function-to-function invocation patterns.
//
// Nova supports three invocation semantics modelled after AWS Lambda:
//
//   - RequestResponse (synchronous): caller blocks until the target returns.
//     Payload limit is 6 MB. The caller is responsible for timeout and retry.
//   - Event (asynchronous): the request is enqueued and the caller returns
//     immediately. Nova handles retry with exponential backoff, DLQ, and
//     OnSuccess/OnFailure destinations. Payload limit is 1 MB.
//   - EventBridge (event-driven): the caller publishes to an event topic and
//     subscriptions fan out to target functions. This decouples producers from
//     consumers and is recommended for most inter-function communication.
//
// # Call-chain tracking
//
// Every invocation carries a CallChain context propagated via HTTP headers.
// This enables recursion detection (max depth), cycle detection (visited set),
// and distributed tracing of function-to-function call graphs.
package invoke

import (
	"context"
	"fmt"
	"strings"
)

// InvocationType determines the invocation semantics.
type InvocationType string

const (
	// InvocationTypeRequestResponse is a synchronous invocation: the caller
	// blocks until the target function returns a result.
	InvocationTypeRequestResponse InvocationType = "RequestResponse"

	// InvocationTypeEvent is an asynchronous invocation: the request is
	// enqueued and the caller receives an invocation ID immediately. Nova
	// handles retry, backoff, DLQ, and destination chaining.
	InvocationTypeEvent InvocationType = "Event"
)

// ParseInvocationType normalises and validates an invocation type string.
// An empty string defaults to RequestResponse (synchronous).
func ParseInvocationType(s string) (InvocationType, error) {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "", "requestresponse", "request_response", "sync":
		return InvocationTypeRequestResponse, nil
	case "event", "async":
		return InvocationTypeEvent, nil
	default:
		return "", fmt.Errorf("invalid invocation type: %q (valid: RequestResponse, Event)", s)
	}
}

// Payload size limits per invocation type (bytes).
const (
	// MaxSyncPayloadBytes is the maximum payload size for synchronous
	// (RequestResponse) invocations: 6 MB, matching AWS Lambda.
	MaxSyncPayloadBytes = 6 << 20 // 6 MB

	// MaxAsyncPayloadBytes is the maximum payload size for asynchronous
	// (Event) invocations: 1 MB, matching AWS Lambda.
	MaxAsyncPayloadBytes = 1 << 20 // 1 MB
)

// MaxPayloadBytes returns the payload size limit for the given invocation type.
func MaxPayloadBytes(t InvocationType) int {
	switch t {
	case InvocationTypeEvent:
		return MaxAsyncPayloadBytes
	default:
		return MaxSyncPayloadBytes
	}
}

// Header names used for call-chain propagation between functions.
const (
	// HeaderInvocationType selects sync vs async invocation semantics.
	// Values: "RequestResponse" (default), "Event".
	HeaderInvocationType = "X-Nova-Invocation-Type"

	// HeaderCallDepth carries the current call depth as a decimal integer.
	// The gateway increments it on each hop and rejects requests that exceed
	// MaxCallDepth.
	HeaderCallDepth = "X-Nova-Call-Depth"

	// HeaderCallChain carries a comma-separated list of function names
	// visited in the current call graph. Used for cycle detection.
	HeaderCallChain = "X-Nova-Call-Chain"

	// HeaderCallerFunction carries the name of the calling function.
	// Used for invoke-policy enforcement.
	HeaderCallerFunction = "X-Nova-Caller-Function"

	// HeaderQualifier selects a specific function alias or version.
	// Values: alias name (e.g. "stable", "canary") or version number.
	HeaderQualifier = "X-Nova-Qualifier"
)

// MaxCallDepth is the maximum allowed function call depth before recursion
// protection rejects the request. This prevents runaway recursive chains.
const MaxCallDepth = 32

// CallChain tracks the invocation call graph for recursion and cycle detection.
type CallChain struct {
	// Depth is the current call depth (0 = top-level external request).
	Depth int

	// Chain is the ordered list of function names visited in this call graph.
	Chain []string

	// CallerFunction is the name of the immediate calling function (empty for
	// top-level external requests).
	CallerFunction string
}

// Push returns a new CallChain with the given function appended and depth
// incremented. It does not mutate the receiver.
func (cc CallChain) Push(functionName string) CallChain {
	chain := make([]string, len(cc.Chain), len(cc.Chain)+1)
	copy(chain, cc.Chain)
	chain = append(chain, functionName)
	return CallChain{
		Depth:          cc.Depth + 1,
		Chain:          chain,
		CallerFunction: functionName,
	}
}

// HasCycle returns true if the given function name already appears in the chain.
func (cc CallChain) HasCycle(functionName string) bool {
	for _, name := range cc.Chain {
		if name == functionName {
			return true
		}
	}
	return false
}

// ChainString returns the chain as a comma-separated string for header propagation.
func (cc CallChain) ChainString() string {
	return strings.Join(cc.Chain, ",")
}

// ParseCallChain reconstructs a CallChain from HTTP header values.
func ParseCallChain(depthStr, chainStr, callerFunc string) CallChain {
	cc := CallChain{CallerFunction: callerFunc}

	if depthStr != "" {
		fmt.Sscanf(depthStr, "%d", &cc.Depth)
	}

	if chainStr != "" {
		parts := strings.Split(chainStr, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				cc.Chain = append(cc.Chain, p)
			}
		}
	}

	return cc
}

// callChainCtxKey is the context key for CallChain propagation across packages.
type callChainCtxKey struct{}

// WithCallChain returns a new context carrying the given CallChain.
func WithCallChain(ctx context.Context, cc CallChain) context.Context {
	return context.WithValue(ctx, callChainCtxKey{}, cc)
}

// CallChainFromContext extracts the CallChain from a context, if present.
func CallChainFromContext(ctx context.Context) (CallChain, bool) {
	cc, ok := ctx.Value(callChainCtxKey{}).(CallChain)
	return cc, ok
}
