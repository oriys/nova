package service

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	errValidation = errors.New("validation")
	errConflict   = errors.New("conflict")

	awsFunctionNamePattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	awsModuleHandlerPattern  = regexp.MustCompile(`^[A-Za-z0-9_./-]+\.[A-Za-z0-9_$][A-Za-z0-9_$.]*$`)
	awsJavaHandlerPattern    = regexp.MustCompile(`^[A-Za-z0-9_$.]+::[A-Za-z0-9_$]+$`)
	awsDotnetHandlerPattern  = regexp.MustCompile(`^[A-Za-z0-9_.-]+::[A-Za-z0-9_.$+]+::[A-Za-z0-9_$]+$`)
	awsExecutableNamePattern = regexp.MustCompile(`^[A-Za-z0-9_/-]{1,128}$`)
	awsGenericHandlerPattern = regexp.MustCompile(`^[^\s]{1,128}$`)
)

type classifiedError struct {
	kind error
	msg  string
}

func (e *classifiedError) Error() string { return e.msg }

func (e *classifiedError) Unwrap() error { return e.kind }

func validationErrorf(format string, args ...any) error {
	return &classifiedError{
		kind: errValidation,
		msg:  fmt.Sprintf(format, args...),
	}
}

func conflictErrorf(format string, args ...any) error {
	return &classifiedError{
		kind: errConflict,
		msg:  fmt.Sprintf(format, args...),
	}
}

func IsValidationError(err error) bool {
	return errors.Is(err, errValidation)
}

func IsConflictError(err error) bool {
	return errors.Is(err, errConflict)
}

func validateCreateFunctionRequest(req *CreateFunctionRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Runtime = strings.TrimSpace(req.Runtime)
	req.Handler = strings.TrimSpace(req.Handler)

	if req.Name == "" {
		return validationErrorf("name is required")
	}
	if !awsFunctionNamePattern.MatchString(req.Name) {
		return validationErrorf("invalid function name: must match AWS Lambda format ([A-Za-z0-9_-], length 1-64)")
	}
	if req.Runtime == "" {
		return validationErrorf("runtime is required")
	}
	if strings.TrimSpace(req.Code) == "" {
		return validationErrorf("code is required")
	}
	if req.MemoryMB != 0 && (req.MemoryMB < 128 || req.MemoryMB > 10240) {
		return validationErrorf("memory_mb must be between 128 and 10240 (AWS Lambda range)")
	}
	if req.TimeoutS != 0 && (req.TimeoutS < 1 || req.TimeoutS > 900) {
		return validationErrorf("timeout_s must be between 1 and 900 seconds (AWS Lambda range)")
	}

	if req.Handler == "" {
		req.Handler = defaultHandlerForRuntime(req.Runtime)
	}
	if err := validateAWSHandlerFormat(req.Runtime, req.Handler); err != nil {
		return err
	}
	return nil
}

func defaultHandlerForRuntime(runtime string) string {
	switch runtimeFamily(runtime) {
	case "java", "kotlin", "scala":
		return "example.Handler::handleRequest"
	case "dotnet":
		return "Assembly::Namespace.Function::FunctionHandler"
	case "go", "rust", "swift", "zig", "wasm", "provided", "custom":
		return "handler"
	default:
		return "main.handler"
	}
}

func validateAWSHandlerFormat(runtime, handler string) error {
	if handler == "" {
		return validationErrorf("handler is required")
	}

	switch runtimeFamily(runtime) {
	case "python", "node", "ruby", "php", "deno", "bun", "elixir", "lua", "perl", "r", "julia":
		if !awsModuleHandlerPattern.MatchString(handler) {
			return validationErrorf("invalid handler for %s: expected AWS style '<module>.<function>' (example: 'main.handler')", runtime)
		}
	case "java", "kotlin", "scala":
		if !awsJavaHandlerPattern.MatchString(handler) {
			return validationErrorf("invalid handler for %s: expected AWS Java style '<package>.<Class>::<method>'", runtime)
		}
	case "dotnet":
		if !awsDotnetHandlerPattern.MatchString(handler) {
			return validationErrorf("invalid handler for %s: expected AWS .NET style '<Assembly>::<Namespace.Class>::<Method>'", runtime)
		}
	case "go", "rust", "swift", "zig", "wasm", "provided", "custom":
		if !awsExecutableNamePattern.MatchString(handler) {
			return validationErrorf("invalid handler for %s: expected executable entry name (letters/numbers/_-/)", runtime)
		}
	default:
		if !awsGenericHandlerPattern.MatchString(handler) {
			return validationErrorf("invalid handler format")
		}
	}

	return nil
}

func runtimeFamily(runtime string) string {
	rt := strings.ToLower(strings.TrimSpace(runtime))
	switch {
	case strings.HasPrefix(rt, "python"):
		return "python"
	case strings.HasPrefix(rt, "node"):
		return "node"
	case strings.HasPrefix(rt, "go"):
		return "go"
	case strings.HasPrefix(rt, "rust"):
		return "rust"
	case strings.HasPrefix(rt, "java"):
		return "java"
	case strings.HasPrefix(rt, "ruby"):
		return "ruby"
	case strings.HasPrefix(rt, "php"):
		return "php"
	case strings.HasPrefix(rt, "dotnet"):
		return "dotnet"
	case strings.HasPrefix(rt, "deno"):
		return "deno"
	case strings.HasPrefix(rt, "bun"):
		return "bun"
	case strings.HasPrefix(rt, "elixir"):
		return "elixir"
	case strings.HasPrefix(rt, "kotlin"):
		return "kotlin"
	case strings.HasPrefix(rt, "swift"):
		return "swift"
	case strings.HasPrefix(rt, "zig"):
		return "zig"
	case strings.HasPrefix(rt, "lua"):
		return "lua"
	case strings.HasPrefix(rt, "perl"):
		return "perl"
	case strings.HasPrefix(rt, "julia"):
		return "julia"
	case strings.HasPrefix(rt, "scala"):
		return "scala"
	case strings.HasPrefix(rt, "wasm"):
		return "wasm"
	case strings.HasPrefix(rt, "provided"):
		return "provided"
	case strings.HasPrefix(rt, "custom"):
		return "custom"
	case rt == "r":
		return "r"
	default:
		return rt
	}
}
