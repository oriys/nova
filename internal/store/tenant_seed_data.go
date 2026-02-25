package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/domain"
)

// sampleFunction describes a starter function seeded for new tenants.
type sampleFunction struct {
	Name    string
	Runtime domain.Runtime
	Handler string
	Code    string
}

// tenantSampleFunctions returns the starter functions for new tenants.
// Only interpreted runtimes are included so they work immediately without compilation.
func tenantSampleFunctions() []sampleFunction {
	return []sampleFunction{
		{
			Name:    "hello-python",
			Runtime: domain.RuntimePython,
			Handler: "main.handler",
			Code: `def handler(event, context):
    name = event.get("name", "World")
    return {
        "message": f"Hello, {name}!",
        "runtime": "python",
        "event": event,
    }`,
		},
		{
			Name:    "hello-node",
			Runtime: domain.RuntimeNode,
			Handler: "main.handler",
			Code: `function handler(event, context) {
  const name = event.name || "World";
  return {
    message: "Hello, " + name + "!",
    runtime: "node",
    event: event,
  };
}

module.exports = { handler };`,
		},
		{
			Name:    "fibonacci",
			Runtime: domain.RuntimePython,
			Handler: "main.handler",
			Code: `def handler(event, context):
    def fib(n):
        if n <= 1:
            return n
        a, b = 0, 1
        for _ in range(2, n + 1):
            a, b = b, a + b
        return b
    n = event.get("n", 10)
    return {"n": n, "fibonacci": fib(n)}`,
		},
		{
			Name:    "json-transform",
			Runtime: domain.RuntimeNode,
			Handler: "main.handler",
			Code: `function handler(event, context) {
  const data = event.data || {};
  const op = event.operation || "keys";
  let result;
  if (op === "uppercase") {
    result = JSON.parse(JSON.stringify(data), (k, v) => typeof v === "string" ? v.toUpperCase() : v);
  } else if (op === "lowercase") {
    result = JSON.parse(JSON.stringify(data), (k, v) => typeof v === "string" ? v.toLowerCase() : v);
  } else if (op === "keys") {
    result = Object.keys(data);
  } else if (op === "values") {
    result = Object.values(data);
  } else {
    result = data;
  }
  return { operation: op, result };
}

module.exports = { handler };`,
		},
		{
			Name:    "hello-ruby",
			Runtime: domain.RuntimeRuby,
			Handler: "main.handler",
			Code: `def handler(event, context)
  name = event["name"] || "World"
  {
    message: "Hello, #{name}!",
    runtime: "ruby",
    event: event,
  }
end`,
		},
	}
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8])
}

// seedTenantSampleFunctions inserts starter functions for a newly created
// tenant so the dashboard is populated and users can invoke immediately.
func seedTenantSampleFunctions(ctx context.Context, exec dbExecer, tenantID string) error {
	ns := DefaultNamespace
	now := time.Now()

	for _, sf := range tenantSampleFunctions() {
		funcID := uuid.New().String()
		codeHash := hashString(sf.Code)

		fn := &domain.Function{
			ID:        funcID,
			TenantID:  tenantID,
			Namespace: ns,
			Name:      sf.Name,
			Runtime:   sf.Runtime,
			Handler:   sf.Handler,
			CodeHash:  codeHash,
			MemoryMB:  128,
			TimeoutS:  30,
			Mode:      domain.ModeProcess,
			NetworkPolicy: &domain.NetworkPolicy{
				IsolationMode: "egress-only",
			},
			CreatedAt: now,
			UpdatedAt: now,
		}

		data, err := json.Marshal(fn)
		if err != nil {
			return fmt.Errorf("marshal sample function %s: %w", sf.Name, err)
		}

		// Insert function metadata
		_, err = exec.Exec(ctx, `
			INSERT INTO functions (id, tenant_id, namespace, name, data, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
			ON CONFLICT (tenant_id, namespace, name) DO NOTHING
		`, funcID, tenantID, ns, sf.Name, data, now, now)
		if err != nil {
			return fmt.Errorf("seed function %s for tenant %s: %w", sf.Name, tenantID, err)
		}

		// Insert function code with source as compiled artifact (interpreted = ready to run)
		_, err = exec.Exec(ctx, `
			INSERT INTO function_code (function_id, source_code, source_hash, compiled_binary, binary_hash, compile_status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'not_required', $6, $7)
			ON CONFLICT (function_id) DO NOTHING
		`, funcID, sf.Code, codeHash, []byte(sf.Code), codeHash, now, now)
		if err != nil {
			return fmt.Errorf("seed function code %s for tenant %s: %w", sf.Name, tenantID, err)
		}
	}

	return nil
}
