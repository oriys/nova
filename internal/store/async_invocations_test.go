package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type captureExec struct {
	query string
	args  []any
}

func (c *captureExec) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	c.query = query
	c.args = append([]any(nil), args...)
	return pgconn.CommandTag{}, nil
}

func TestInsertAsyncInvocation_UsesEmptyWorkflowStrings(t *testing.T) {
	inv := NewAsyncInvocation("fn-1", "hello", json.RawMessage(`{"ok":true}`))
	inv.TenantID = DefaultTenantID
	inv.Namespace = DefaultNamespace

	exec := &captureExec{}
	if err := insertAsyncInvocation(context.Background(), exec, inv); err != nil {
		t.Fatalf("insertAsyncInvocation returned error: %v", err)
	}

	if len(exec.args) != 25 {
		t.Fatalf("expected 25 SQL args, got %d", len(exec.args))
	}
	if workflowID, ok := exec.args[5].(string); !ok || workflowID != "" {
		t.Fatalf("workflow_id arg = %#v, want empty string", exec.args[5])
	}
	if workflowName, ok := exec.args[6].(string); !ok || workflowName != "" {
		t.Fatalf("workflow_name arg = %#v, want empty string", exec.args[6])
	}
}
