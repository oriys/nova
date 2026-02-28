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

// seedTenantSampleWorkflow inserts a starter DAG workflow that chains the
// seeded functions so new users can see the workflow engine in action.
func seedTenantSampleWorkflow(ctx context.Context, exec dbExecer, tenantID string) error {
	ns := DefaultNamespace
	now := time.Now()

	// ── Workflow: data-pipeline (hello-python → json-transform → hello-ruby) ──
	wfID := uuid.New().String()
	wfPrefix := tenantID + "/"
	_, err := exec.Exec(ctx, `
		INSERT INTO dag_workflows (id, tenant_id, namespace, name, description, status, current_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'active', 1, $6, $7)
		ON CONFLICT DO NOTHING
	`, wfID, tenantID, ns, wfPrefix+"data-pipeline",
		"Sample pipeline: greet → transform to uppercase → format report", now, now)
	if err != nil {
		return fmt.Errorf("seed workflow for tenant %s: %w", tenantID, err)
	}

	// Version
	vID := uuid.New().String()
	definition := `{"nodes":[` +
		`{"node_key":"greet","function_name":"hello-python"},` +
		`{"node_key":"transform","function_name":"json-transform"},` +
		`{"node_key":"report","function_name":"hello-ruby"}` +
		`],"edges":[` +
		`{"from":"greet","to":"transform"},` +
		`{"from":"transform","to":"report"}` +
		`]}`
	_, err = exec.Exec(ctx, `
		INSERT INTO dag_workflow_versions (id, workflow_id, version, definition, created_at)
		VALUES ($1, $2, 1, $3::jsonb, $4)
		ON CONFLICT DO NOTHING
	`, vID, wfID, definition, now)
	if err != nil {
		return fmt.Errorf("seed workflow version for tenant %s: %w", tenantID, err)
	}

	// Nodes
	type nodeSpec struct {
		key      string
		funcName string
		pos      int
	}
	nodes := []nodeSpec{
		{"greet", "hello-python", 0},
		{"transform", "json-transform", 1},
		{"report", "hello-ruby", 2},
	}
	nodeIDs := make(map[string]string, len(nodes))
	for _, n := range nodes {
		nID := uuid.New().String()
		nodeIDs[n.key] = nID
		_, err = exec.Exec(ctx, `
			INSERT INTO dag_workflow_nodes (id, version_id, node_key, node_type, function_name, timeout_s, position)
			VALUES ($1, $2, $3, 'function', $4, 30, $5)
			ON CONFLICT DO NOTHING
		`, nID, vID, n.key, n.funcName, n.pos)
		if err != nil {
			return fmt.Errorf("seed workflow node %s for tenant %s: %w", n.key, tenantID, err)
		}
	}

	// Edges
	edges := [][2]string{{"greet", "transform"}, {"transform", "report"}}
	for _, e := range edges {
		_, err = exec.Exec(ctx, `
			INSERT INTO dag_workflow_edges (id, version_id, from_node_id, to_node_id)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING
		`, uuid.New().String(), vID, nodeIDs[e[0]], nodeIDs[e[1]])
		if err != nil {
			return fmt.Errorf("seed workflow edge %s→%s for tenant %s: %w", e[0], e[1], tenantID, err)
		}
	}

	// ── Workflow: parallel-compute (fibonacci ⇉ hello-node, hello-ruby → hello-python) ──
	wf2ID := uuid.New().String()
	_, err = exec.Exec(ctx, `
		INSERT INTO dag_workflows (id, tenant_id, namespace, name, description, status, current_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'active', 1, $6, $7)
		ON CONFLICT DO NOTHING
	`, wf2ID, tenantID, ns, wfPrefix+"parallel-compute",
		"Sample fan-out: compute fibonacci, then run Node.js & Ruby in parallel, aggregate with Python", now, now)
	if err != nil {
		return fmt.Errorf("seed workflow parallel-compute for tenant %s: %w", tenantID, err)
	}

	v2ID := uuid.New().String()
	def2 := `{"nodes":[` +
		`{"node_key":"compute","function_name":"fibonacci"},` +
		`{"node_key":"format-node","function_name":"hello-node"},` +
		`{"node_key":"format-ruby","function_name":"hello-ruby"},` +
		`{"node_key":"aggregate","function_name":"hello-python"}` +
		`],"edges":[` +
		`{"from":"compute","to":"format-node"},` +
		`{"from":"compute","to":"format-ruby"},` +
		`{"from":"format-node","to":"aggregate"},` +
		`{"from":"format-ruby","to":"aggregate"}` +
		`]}`
	_, err = exec.Exec(ctx, `
		INSERT INTO dag_workflow_versions (id, workflow_id, version, definition, created_at)
		VALUES ($1, $2, 1, $3::jsonb, $4)
		ON CONFLICT DO NOTHING
	`, v2ID, wf2ID, def2, now)
	if err != nil {
		return fmt.Errorf("seed workflow version parallel-compute for tenant %s: %w", tenantID, err)
	}

	nodes2 := []nodeSpec{
		{"compute", "fibonacci", 0},
		{"format-node", "hello-node", 1},
		{"format-ruby", "hello-ruby", 1},
		{"aggregate", "hello-python", 2},
	}
	node2IDs := make(map[string]string, len(nodes2))
	for _, n := range nodes2 {
		nID := uuid.New().String()
		node2IDs[n.key] = nID
		_, err = exec.Exec(ctx, `
			INSERT INTO dag_workflow_nodes (id, version_id, node_key, node_type, function_name, timeout_s, position)
			VALUES ($1, $2, $3, 'function', $4, 30, $5)
			ON CONFLICT DO NOTHING
		`, nID, v2ID, n.key, n.funcName, n.pos)
		if err != nil {
			return fmt.Errorf("seed workflow node %s for tenant %s: %w", n.key, tenantID, err)
		}
	}

	edges2 := [][2]string{
		{"compute", "format-node"}, {"compute", "format-ruby"},
		{"format-node", "aggregate"}, {"format-ruby", "aggregate"},
	}
	for _, e := range edges2 {
		_, err = exec.Exec(ctx, `
			INSERT INTO dag_workflow_edges (id, version_id, from_node_id, to_node_id)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING
		`, uuid.New().String(), v2ID, node2IDs[e[0]], node2IDs[e[1]])
		if err != nil {
			return fmt.Errorf("seed workflow edge %s→%s for tenant %s: %w", e[0], e[1], tenantID, err)
		}
	}

	return nil
}

// seedTenantSampleGatewayRoutes inserts starter gateway routes mapping
// REST-style paths to the seeded functions.
func seedTenantSampleGatewayRoutes(ctx context.Context, exec dbExecer, tenantID string) error {
	now := time.Now()

	type routeSpec struct {
		path         string
		methods      []string
		functionName string
		description  string
		mapping      []domain.ParamMapping
	}

	// Use /t/<tenantID>/... paths to avoid cross-tenant collisions
	prefix := "/t/" + tenantID
	routes := []routeSpec{
		{prefix + "/hello", []string{"GET", "POST"}, "hello-python", "Greeting endpoint", nil},
		{prefix + "/fibonacci", []string{"GET", "POST"}, "fibonacci", "Compute Fibonacci numbers", []domain.ParamMapping{
			{Source: domain.ParamSourceQuery, Name: "n", Target: "n", Type: domain.ParamTypeInteger},
		}},
		{prefix + "/transform", []string{"POST"}, "json-transform", "JSON data transformations", []domain.ParamMapping{
			{Source: domain.ParamSourceBody, Name: "user_name", Target: "userName", Transform: domain.ParamTransformCamelCase},
			{Source: domain.ParamSourceHeader, Name: "X-Request-ID", Target: "requestId"},
			{Source: domain.ParamSourceQuery, Name: "format", Target: "outputFormat", Default: "json"},
		}},
	}

	for _, r := range routes {
		route := &domain.GatewayRoute{
			ID:           uuid.New().String()[:8],
			Path:         r.path,
			Methods:      r.methods,
			FunctionName: r.functionName,
			AuthStrategy: "none",
			ParamMapping: r.mapping,
			RateLimit: &domain.RouteRateLimit{
				RequestsPerSecond: 100,
				BurstSize:         200,
			},
			CORS: &domain.CORSConfig{
				AllowOrigins: []string{"*"},
				AllowMethods: r.methods,
				AllowHeaders: []string{"Content-Type", "Authorization"},
				MaxAge:       3600,
			},
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		}

		data, err := json.Marshal(route)
		if err != nil {
			return fmt.Errorf("marshal gateway route %s: %w", r.path, err)
		}

		_, err = exec.Exec(ctx, `
			INSERT INTO gateway_routes (id, domain, path, function_name, data, enabled, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (domain, path) DO NOTHING
		`, route.ID, route.Domain, route.Path, route.FunctionName, data, route.Enabled, now, now)
		if err != nil {
			return fmt.Errorf("seed gateway route %s for tenant %s: %w", r.path, tenantID, err)
		}
	}

	return nil
}

// seedTenantSampleEvents inserts sample event topics and subscriptions so the
// events dashboard is populated for new tenants.
func seedTenantSampleEvents(ctx context.Context, exec dbExecer, tenantID string) error {
	ns := DefaultNamespace
	now := time.Now()

	type topicSpec struct {
		name           string
		description    string
		retentionHours int
	}

	topics := []topicSpec{
		{"user.events", "User lifecycle events (signup, login, profile updates)", 168},
		{"order.events", "Order processing events (created, paid, shipped)", 720},
		{"system.alerts", "System alerts and notifications", 72},
	}

	topicIDs := make(map[string]string, len(topics))
	for _, t := range topics {
		id := uuid.New().String()
		topicIDs[t.name] = id
		_, err := exec.Exec(ctx, `
			INSERT INTO event_topics (id, tenant_id, namespace, name, description, retention_hours, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT DO NOTHING
		`, id, tenantID, ns, t.name, t.description, t.retentionHours, now, now)
		if err != nil {
			return fmt.Errorf("seed event topic %s for tenant %s: %w", t.name, tenantID, err)
		}
	}

	type subSpec struct {
		topicName     string
		name          string
		consumerGroup string
		functionName  string
	}

	subs := []subSpec{
		{"user.events", "greet-new-users", "user-greet-group", "hello-python"},
		{"order.events", "transform-orders", "order-transform-group", "json-transform"},
		{"system.alerts", "alert-handler", "alert-handler-group", "hello-node"},
	}

	for _, s := range subs {
		topicID, ok := topicIDs[s.topicName]
		if !ok {
			continue
		}
		subID := uuid.New().String()
		_, err := exec.Exec(ctx, `
			INSERT INTO event_subscriptions (
				id, tenant_id, namespace, topic_id, name, consumer_group,
				function_id, function_name, workflow_id, workflow_name,
				enabled, max_attempts, backoff_base_ms, backoff_max_ms,
				max_inflight, rate_limit_per_sec, last_acked_sequence,
				type, webhook_url, webhook_method, webhook_headers, webhook_signing_secret, webhook_timeout_ms,
				created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6,
				'', $7, '', '',
				true, 3, 1000, 60000,
				10, 100, 0,
				'function', '', '', '{}', '', 30000,
				$8, $9
			) ON CONFLICT DO NOTHING
		`, subID, tenantID, ns, topicID, s.name, s.consumerGroup,
			s.functionName, now, now)
		if err != nil {
			return fmt.Errorf("seed event subscription %s for tenant %s: %w", s.name, tenantID, err)
		}
	}

	return nil
}

// seedTenantSampleTriggers inserts sample triggers so the triggers dashboard
// is populated for new tenants.
func seedTenantSampleTriggers(ctx context.Context, exec dbExecer, tenantID string) error {
	ns := DefaultNamespace
	now := time.Now()

	type triggerSpec struct {
		name         string
		triggerType  string
		functionName string
		config       map[string]interface{}
	}

	triggers := []triggerSpec{
		{
			name:         "webhook-greeter",
			triggerType:  "webhook",
			functionName: "hello-python",
			config: map[string]interface{}{
				"listen_addr": ":8090",
				"path":        "/hook/" + tenantID + "/greet",
			},
		},
		{
			name:         "kafka-orders",
			triggerType:  "kafka",
			functionName: "json-transform",
			config: map[string]interface{}{
				"brokers": "localhost:9092",
				"topic":   "orders",
				"group":   tenantID + "-order-group",
			},
		},
		{
			name:         "fs-watcher",
			triggerType:  "filesystem",
			functionName: "hello-node",
			config: map[string]interface{}{
				"path":          "/var/data/incoming",
				"pattern":       "*.json",
				"poll_interval": 60,
			},
		},
	}

	for _, t := range triggers {
		id := uuid.New().String()
		configJSON, err := json.Marshal(t.config)
		if err != nil {
			return fmt.Errorf("marshal trigger config %s: %w", t.name, err)
		}

		_, err = exec.Exec(ctx, `
			INSERT INTO triggers (id, tenant_id, namespace, name, type, function_id, function_name, enabled, config, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, '', $6, $7, $8, $9, $10)
			ON CONFLICT DO NOTHING
		`, id, tenantID, ns, t.name, t.triggerType, t.functionName, true, configJSON, now, now)
		if err != nil {
			return fmt.Errorf("seed trigger %s for tenant %s: %w", t.name, tenantID, err)
		}
	}

	return nil
}
