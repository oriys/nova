package domain

import "testing"

func TestNodeType_Constants(t *testing.T) {
	tests := []struct {
		nt   NodeType
		want string
	}{
		{NodeTypeFunction, "function"},
		{NodeTypeSubWorkflow, "sub_workflow"},
	}
	for _, tt := range tests {
		if string(tt.nt) != tt.want {
			t.Errorf("NodeType = %q, want %q", tt.nt, tt.want)
		}
	}
}

func TestWorkflowStatus_Values(t *testing.T) {
	tests := []struct {
		status WorkflowStatus
		want   string
	}{
		{WorkflowStatusActive, "active"},
		{WorkflowStatusInactive, "inactive"},
		{WorkflowStatusDeleted, "deleted"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("WorkflowStatus = %q, want %q", tt.status, tt.want)
		}
	}
}

func TestRunStatus_Values(t *testing.T) {
	tests := []struct {
		status RunStatus
		want   string
	}{
		{RunStatusPending, "pending"},
		{RunStatusRunning, "running"},
		{RunStatusSucceeded, "succeeded"},
		{RunStatusFailed, "failed"},
		{RunStatusCancelled, "cancelled"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("RunStatus = %q, want %q", tt.status, tt.want)
		}
	}
}

func TestNodeStatus_Values(t *testing.T) {
	tests := []struct {
		status NodeStatus
		want   string
	}{
		{NodeStatusPending, "pending"},
		{NodeStatusReady, "ready"},
		{NodeStatusRunning, "running"},
		{NodeStatusSucceeded, "succeeded"},
		{NodeStatusFailed, "failed"},
		{NodeStatusSkipped, "skipped"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("NodeStatus = %q, want %q", tt.status, tt.want)
		}
	}
}

func TestRetryPolicy_Defaults(t *testing.T) {
	p := &RetryPolicy{
		MaxAttempts:  3,
		BaseMS:       100,
		MaxBackoffMS: 5000,
	}
	if p.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", p.MaxAttempts)
	}
	if p.BaseMS != 100 {
		t.Errorf("BaseMS = %d, want 100", p.BaseMS)
	}
	if p.MaxBackoffMS != 5000 {
		t.Errorf("MaxBackoffMS = %d, want 5000", p.MaxBackoffMS)
	}
}

func TestWorkflowDefinition_EmptyIsValid(t *testing.T) {
	def := WorkflowDefinition{}
	if def.Nodes != nil {
		t.Error("empty definition should have nil Nodes")
	}
	if def.Edges != nil {
		t.Error("empty definition should have nil Edges")
	}
}

func TestNodeDefinition_FunctionNode(t *testing.T) {
	node := NodeDefinition{
		NodeKey:      "step-1",
		NodeType:     NodeTypeFunction,
		FunctionName: "hello-python",
		TimeoutS:     30,
	}
	if node.NodeType != NodeTypeFunction {
		t.Errorf("NodeType = %q, want %q", node.NodeType, NodeTypeFunction)
	}
	if node.FunctionName != "hello-python" {
		t.Errorf("FunctionName = %q, want %q", node.FunctionName, "hello-python")
	}
	if node.WorkflowName != "" {
		t.Error("function node should not have WorkflowName")
	}
}

func TestNodeDefinition_SubWorkflowNode(t *testing.T) {
	node := NodeDefinition{
		NodeKey:      "step-2",
		NodeType:     NodeTypeSubWorkflow,
		WorkflowName: "data-pipeline",
	}
	if node.NodeType != NodeTypeSubWorkflow {
		t.Errorf("NodeType = %q, want %q", node.NodeType, NodeTypeSubWorkflow)
	}
	if node.WorkflowName != "data-pipeline" {
		t.Errorf("WorkflowName = %q, want %q", node.WorkflowName, "data-pipeline")
	}
	if node.FunctionName != "" {
		t.Error("sub-workflow node should not have FunctionName")
	}
}
