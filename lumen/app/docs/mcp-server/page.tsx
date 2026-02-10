import { DocsShell } from "@/components/docs/docs-shell"
import { CodeBlock } from "@/components/docs/code-block"

export default function DocsMCPServerPage() {
  return (
    <DocsShell
      current="mcp"
      title="Atlas MCP Server"
      description="Atlas exposes Nova capabilities as MCP tools so AI agents can operate Nova through a typed stdio tool interface."
      toc={[
        { id: "overview", label: "Overview" },
        { id: "when-to-use", label: "When To Use" },
        { id: "build-and-run", label: "Build & Run" },
        { id: "connection-config", label: "Connection Config" },
        { id: "tool-surface", label: "Tool Surface" },
        { id: "operational-guidance", label: "Operational Guidance" },
      ]}
    >
      <section id="overview" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Overview</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Atlas is implemented in Go and runs over MCP stdio transport. It maps Nova HTTP APIs to MCP tools with a
          consistent <code>nova_*</code> naming convention.
        </p>
        <CodeBlock
          code={`Component: atlas
Protocol: MCP
Transport: stdio
Tool naming: nova_*
Target backend: ZENITH_URL (default http://localhost:9000)`}
        />
      </section>

      <section id="when-to-use" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">When To Use</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>Automate operational runbooks from AI coding/ops assistants.</li>
          <li>Drive function, workflow, and event operations from natural language requests.</li>
          <li>Keep the same tenant/namespace scoping model as Orbit and HTTP clients.</li>
        </ul>
      </section>

      <section id="build-and-run" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Build &amp; Run</h2>
        <CodeBlock
          code={`# Build Atlas
make atlas

# Run Atlas MCP server
./bin/atlas

# Linux artifact
make atlas-linux`}
        />
      </section>

      <section id="connection-config" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Connection Config</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Atlas reads backend context from environment variables and forwards them as Nova request headers.
        </p>
        <CodeBlock
          code={`Environment variables
ZENITH_URL=http://localhost:9000
NOVA_API_KEY=<api-key>
NOVA_TENANT=default
NOVA_NAMESPACE=default

Forwarded headers
X-API-Key: <NOVA_API_KEY>
X-Nova-Tenant: <NOVA_TENANT>
X-Nova-Namespace: <NOVA_NAMESPACE>`}
        />
        <CodeBlock
          code={`{
  "mcpServers": {
    "atlas": {
      "command": "/absolute/path/to/bin/atlas",
      "env": {
        "ZENITH_URL": "http://localhost:9000",
        "NOVA_API_KEY": "<api-key>",
        "NOVA_TENANT": "default",
        "NOVA_NAMESPACE": "default"
      }
    }
  }
}`}
        />
      </section>

      <section id="tool-surface" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Tool Surface</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Atlas tools are grouped by domain to mirror Nova APIs.
        </p>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>Functions: lifecycle, code, versions, invocation, async invocation.</li>
          <li>Workflows: create/update/list, versions, runs.</li>
          <li>Events: topics, subscriptions, deliveries, replay, outbox.</li>
          <li>Governance: tenants, API keys, secrets, config.</li>
          <li>Operations: health, stats, metrics, logs, invocations.</li>
        </ul>
        <CodeBlock
          code={`Example tools
nova_list_functions
nova_create_function
nova_invoke_function
nova_create_topic
nova_create_subscription
nova_trigger_workflow_run
nova_health
nova_get_metrics`}
        />
      </section>

      <section id="operational-guidance" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Operational Guidance</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>Always set tenant and namespace explicitly in shared environments.</li>
          <li>Prefer least-privilege API keys per automation/agent workflow.</li>
          <li>Treat MCP tool responses as source-of-truth runtime state, not cached assumptions.</li>
          <li>Use replay/seek/dlq tools for controlled recovery instead of ad-hoc DB changes.</li>
        </ul>
      </section>
    </DocsShell>
  )
}
