package store

import (
	"encoding/json"
	"testing"

	"github.com/oriys/nova/internal/domain"
)

func TestBuildSuccessorInputSinglePredecessorUsesDirectPayload(t *testing.T) {
	completed := &domain.RunNode{
		NodeKey: "step-a",
		Output:  json.RawMessage(`{"value":1}`),
	}

	input, err := buildSuccessorInput(completed, []workflowPredecessorState{
		{
			NodeKey:     "step-a",
			IsCompleted: true,
		},
	})
	if err != nil {
		t.Fatalf("buildSuccessorInput() error = %v", err)
	}

	if string(input) != `{"value":1}` {
		t.Fatalf("expected direct predecessor payload, got %s", string(input))
	}
}

func TestBuildSuccessorInputMultiPredecessorMergesCompletedAndSucceeded(t *testing.T) {
	completed := &domain.RunNode{
		NodeKey: "step-b",
		Output:  json.RawMessage(`{"b":2}`),
	}

	input, err := buildSuccessorInput(completed, []workflowPredecessorState{
		{
			NodeKey: "step-a",
			Status:  domain.NodeStatusSucceeded,
			Output:  json.RawMessage(`{"a":1}`),
		},
		{
			NodeKey:     "step-b",
			IsCompleted: true,
		},
		{
			NodeKey: "step-c",
			Status:  domain.NodeStatusRunning,
			Output:  json.RawMessage(`{"c":3}`),
		},
	})
	if err != nil {
		t.Fatalf("buildSuccessorInput() error = %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(input, &got); err != nil {
		t.Fatalf("unmarshal merged input: %v", err)
	}

	if string(got["step-a"]) != `{"a":1}` {
		t.Fatalf("expected step-a output to be preserved, got %s", string(got["step-a"]))
	}
	if string(got["step-b"]) != `{"b":2}` {
		t.Fatalf("expected completed output to be merged, got %s", string(got["step-b"]))
	}
	if _, ok := got["step-c"]; ok {
		t.Fatalf("expected non-succeeded predecessor to be excluded")
	}
}
