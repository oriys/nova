package service

import (
	"strings"
	"testing"
)

func TestValidateCreateFunctionRequest_AWSNameValidation(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{name: "hello_world-01", wantErr: false},
		{name: "bad.name", wantErr: true},
		{name: "bad name", wantErr: true},
		{name: strings.Repeat("a", 64), wantErr: false},
		{name: strings.Repeat("a", 65), wantErr: true},
	}

	for _, tt := range tests {
		req := &CreateFunctionRequest{
			Name:    tt.name,
			Runtime: "python",
			Handler: "main.handler",
			Code:    "def handler(event, context):\n  return event\n",
		}
		err := validateCreateFunctionRequest(req)
		if tt.wantErr && err == nil {
			t.Fatalf("expected validation error for name %q", tt.name)
		}
		if !tt.wantErr && err != nil {
			t.Fatalf("unexpected validation error for name %q: %v", tt.name, err)
		}
	}
}

func TestValidateCreateFunctionRequest_HandlerByRuntime(t *testing.T) {
	tests := []struct {
		runtime string
		handler string
		valid   bool
	}{
		{runtime: "python", handler: "main.handler", valid: true},
		{runtime: "python", handler: "handler", valid: false},
		{runtime: "node20", handler: "index.handler", valid: true},
		{runtime: "java", handler: "com.example.Handler::handleRequest", valid: true},
		{runtime: "java", handler: "main.handler", valid: false},
		{runtime: "go1.24", handler: "handler", valid: true},
		{runtime: "go1.24", handler: "main.handler", valid: false},
	}

	for _, tt := range tests {
		req := &CreateFunctionRequest{
			Name:    "valid_name",
			Runtime: tt.runtime,
			Handler: tt.handler,
			Code:    "dummy",
		}
		err := validateCreateFunctionRequest(req)
		if tt.valid && err != nil {
			t.Fatalf("expected handler %q for runtime %q to be valid, got %v", tt.handler, tt.runtime, err)
		}
		if !tt.valid && err == nil {
			t.Fatalf("expected handler %q for runtime %q to be invalid", tt.handler, tt.runtime)
		}
	}
}

func TestValidateCreateFunctionRequest_DefaultHandler(t *testing.T) {
	tests := []struct {
		runtime       string
		expectHandler string
	}{
		{runtime: "python", expectHandler: "main.handler"},
		{runtime: "go", expectHandler: "handler"},
		{runtime: "java", expectHandler: "example.Handler::handleRequest"},
	}

	for _, tt := range tests {
		req := &CreateFunctionRequest{
			Name:    "valid_name",
			Runtime: tt.runtime,
			Code:    "dummy",
		}
		if err := validateCreateFunctionRequest(req); err != nil {
			t.Fatalf("unexpected validation error for runtime %q: %v", tt.runtime, err)
		}
		if req.Handler != tt.expectHandler {
			t.Fatalf("runtime %q default handler = %q, want %q", tt.runtime, req.Handler, tt.expectHandler)
		}
	}
}

func TestValidateCreateFunctionRequest_AWSRanges(t *testing.T) {
	tests := []struct {
		name      string
		memoryMB  int
		timeoutS  int
		shouldErr bool
	}{
		{name: "valid", memoryMB: 128, timeoutS: 1, shouldErr: false},
		{name: "valid-max", memoryMB: 10240, timeoutS: 900, shouldErr: false},
		{name: "bad-memory-low", memoryMB: 64, timeoutS: 30, shouldErr: true},
		{name: "bad-memory-high", memoryMB: 11000, timeoutS: 30, shouldErr: true},
		{name: "bad-timeout-low", memoryMB: 128, timeoutS: 0, shouldErr: false}, // 0 means use default
		{name: "bad-timeout-high", memoryMB: 128, timeoutS: 901, shouldErr: true},
	}

	for _, tt := range tests {
		req := &CreateFunctionRequest{
			Name:     "valid_name",
			Runtime:  "python",
			Handler:  "main.handler",
			Code:     "dummy",
			MemoryMB: tt.memoryMB,
			TimeoutS: tt.timeoutS,
		}
		err := validateCreateFunctionRequest(req)
		if tt.shouldErr && err == nil {
			t.Fatalf("%s: expected error", tt.name)
		}
		if !tt.shouldErr && err != nil {
			t.Fatalf("%s: unexpected error: %v", tt.name, err)
		}
	}
}
