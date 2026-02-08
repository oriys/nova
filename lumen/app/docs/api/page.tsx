import Link from "next/link"
import { DocsShell } from "@/components/docs/docs-shell"
import { CodeBlock } from "@/components/docs/code-block"
import { EndpointTable, type Endpoint } from "@/components/docs/endpoint-table"

const endpointIndex: Endpoint[] = [
  { method: "POST", path: "/functions", description: "Create function" },
  { method: "POST", path: "/functions/{name}/invoke", description: "Sync invoke" },
  { method: "POST", path: "/functions/{name}/invoke-async", description: "Async invoke" },
  { method: "POST", path: "/workflows", description: "Create workflow" },
  { method: "POST", path: "/workflows/{name}/runs", description: "Trigger workflow run" },
  { method: "POST", path: "/topics", description: "Create topic" },
  { method: "POST", path: "/topics/{name}/subscriptions", description: "Create function/workflow subscription" },
  { method: "POST", path: "/topics/{name}/publish", description: "Publish event" },
  { method: "POST", path: "/subscriptions/{id}/replay", description: "Replay deliveries" },
  { method: "GET", path: "/metrics/timeseries", description: "Global timeseries metrics" },
  { method: "GET", path: "/health", description: "Service health detail" },
]

export default function DocsAPIPage() {
  return (
    <DocsShell
      current="api"
      activeHref="/docs/api"
      title="API Overview"
      description="Nova API is contract-first and tenant-scoped. Use this page for protocol rules, then jump to domain-specific reference pages for detailed endpoint contracts."
      toc={[
        { id: "api-basics", label: "API Basics" },
        { id: "headers-auth", label: "Headers & Auth" },
        { id: "error-format", label: "Error Format" },
        { id: "pagination-retries", label: "Pagination & Retries" },
        { id: "reference-sections", label: "Reference Sections" },
        { id: "endpoint-index", label: "Endpoint Index" },
      ]}
    >
      <section id="api-basics" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">API Basics</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>Base URL: <code>http://&lt;nova-host&gt;:9000</code></li>
          <li>Default media type: <code>application/json</code></li>
          <li>Time format: RFC3339 timestamps</li>
          <li>Tenant-aware routing and persistence on every request</li>
        </ul>
      </section>

      <section id="headers-auth" className="scroll-mt-24 space-y-2">
        <h2 className="text-3xl font-semibold tracking-tight">Headers &amp; Auth</h2>
        <p className="text-lg leading-8 text-muted-foreground">
          Explicit tenant headers are recommended for every write and operator workflow.
        </p>
        <CodeBlock
          code={`Content-Type: application/json
X-Nova-Tenant: <tenant-id>
X-Nova-Namespace: <namespace>

# when auth enabled
Authorization: Bearer <jwt>
X-API-Key: <api-key>`}
        />
      </section>

      <section id="error-format" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Error Format</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Most handlers return plain-text errors; some middleware returns structured JSON errors.
        </p>
        <CodeBlock
          code={`# Plain text example
HTTP/1.1 400 Bad Request
invalid JSON payload

# Structured middleware example
{
  "error": "tenant_scope_error",
  "message": "tenant scope is not allowed for this identity"
}`}
        />
      </section>

      <section id="pagination-retries" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Pagination &amp; Retries</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>List endpoints generally accept <code>limit</code> and optional status filters.</li>
          <li>Async invocation supports idempotency with <code>idempotency_key</code> and TTL.</li>
          <li>Event subscriptions support replay and seek from sequence/time anchors.</li>
          <li>Delivery pipelines use bounded retry with backoff and DLQ transitions.</li>
        </ul>
      </section>

      <section id="reference-sections" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Reference Sections</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Detailed endpoint contracts are split by domain for easier maintenance and faster lookup.
        </p>
        <div className="mt-4 grid gap-3 md:grid-cols-2">
          <Link href="/docs/api/functions" className="rounded-lg border border-border p-4 hover:bg-muted/40">
            <p className="text-sm font-medium">Functions API</p>
            <p className="mt-1 text-sm text-muted-foreground">Lifecycle, config patching, sync/async invoke.</p>
          </Link>
          <Link href="/docs/api/workflows" className="rounded-lg border border-border p-4 hover:bg-muted/40">
            <p className="text-sm font-medium">Workflows API</p>
            <p className="mt-1 text-sm text-muted-foreground">Workflow create, versions, runs, run detail.</p>
          </Link>
          <Link href="/docs/api/events" className="rounded-lg border border-border p-4 hover:bg-muted/40">
            <p className="text-sm font-medium">Events API</p>
            <p className="mt-1 text-sm text-muted-foreground">Topics, subscriptions, publishing, replay, deliveries.</p>
          </Link>
          <Link href="/docs/api/operations" className="rounded-lg border border-border p-4 hover:bg-muted/40">
            <p className="text-sm font-medium">Operations API</p>
            <p className="mt-1 text-sm text-muted-foreground">Health, metrics, logs, invocations.</p>
          </Link>
        </div>
      </section>

      <section id="endpoint-index" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Endpoint Index</h2>
        <p className="mb-4 mt-3 text-lg leading-8 text-muted-foreground">
          Frequently used endpoints across all API domains.
        </p>
        <EndpointTable endpoints={endpointIndex} />
      </section>
    </DocsShell>
  )
}
