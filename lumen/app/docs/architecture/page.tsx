import { DocsShell } from "@/components/docs/docs-shell"
import { CodeBlock } from "@/components/docs/code-block"

export default function DocsArchitecturePage() {
  return (
    <DocsShell
      current="architecture"
      title="Architecture"
      description="Nova is designed as a backend service with clear control-plane and data-plane boundaries, durable state, and asynchronous workers for reliable delivery."
      toc={[
        { id: "principles", label: "Design Principles" },
        { id: "component-map", label: "Component Map" },
        { id: "sync-invocation-flow", label: "Sync Invocation Flow" },
        { id: "event-workflow-delivery", label: "Event + Workflow Delivery" },
        { id: "tenancy-isolation", label: "Tenancy & Isolation" },
        { id: "reliability-model", label: "Reliability Model" },
        { id: "observability", label: "Observability" },
      ]}
    >
      <section id="principles" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Design Principles</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>
            <strong>Backend-first:</strong> Nova is the execution and control backend; Orbit and Lumen are clients.
          </li>
          <li>
            <strong>One control plane:</strong> functions, workflows, events, and governance share the same API surface.
          </li>
          <li>
            <strong>Durability by default:</strong> metadata and delivery state are persisted in Postgres.
          </li>
          <li>
            <strong>Operational transparency:</strong> health, metrics, logs, and replay APIs are first-class endpoints.
          </li>
        </ul>
      </section>

      <section id="component-map" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Component Map</h2>
        <CodeBlock
          code={`[Clients]
  Orbit CLI, Lumen UI, MCP clients
      |
      v
[Nova HTTP API :9000]
  - Control plane handlers (functions/workflows/events/tenants/...)
  - Data plane handlers (invoke, async invoke, logs, metrics, health)
      |
      +--> [Executor] --> [Pool] --> [Runtime backend]
      |                    |           - Firecracker
      |                    |           - Docker
      |
      +--> [Store/Postgres]
      |      - functions, code, versions
      |      - workflows and runs
      |      - topics, subscriptions, deliveries, outbox
      |      - quotas, API keys, secrets
      |
      +--> [Background workers]
             - async invocation workers
             - event delivery workers
             - outbox relay
             - scheduler`}
        />
      </section>

      <section id="sync-invocation-flow" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Sync Invocation Flow</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          The synchronous path is optimized for predictable latency with pool reuse and clear backpressure behavior.
        </p>
        <CodeBlock
          code={`POST /functions/{name}/invoke
  -> resolve function metadata from store
  -> enforce tenant quota and capacity limits
  -> acquire warm instance from pool (or cold start)
  -> execute function payload
  -> persist logs/metrics
  -> return invocation response`}
        />
      </section>

      <section id="event-workflow-delivery" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Event + Workflow Delivery</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Event subscriptions can target functions or workflows. For workflow subscriptions, a webhook may optionally
          push final outputs externally.
        </p>
        <CodeBlock
          code={`POST /topics/{name}/publish
  -> message appended to topic stream
  -> fanout delivery records created per subscription
  -> delivery worker dequeues due records
     - type=function: invoke target function
     - type=workflow: trigger workflow run
         - if webhook_url set: push final result to webhook
         - else: persist result internally only
  -> success advances cursor; failures retry with backoff; terminal goes to DLQ`}
        />
      </section>

      <section id="tenancy-isolation" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Tenancy &amp; Isolation</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>Request scope is resolved from <code>X-Nova-Tenant</code> and <code>X-Nova-Namespace</code>.</li>
          <li>Auth-bound identities can only access allowed scopes.</li>
          <li>Store queries are tenant/namespace filtered to prevent data bleed.</li>
          <li>Quotas are enforced per tenant for invocations, queue depth, and event publish dimensions.</li>
        </ul>
      </section>

      <section id="reliability-model" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Reliability Model</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>
            <strong>Retries with backoff:</strong> async invocations, event deliveries, and outbox jobs all use bounded
            attempts and exponential-style backoff.
          </li>
          <li>
            <strong>DLQ semantics:</strong> terminal failures move to DLQ where operators can replay or retry.
          </li>
          <li>
            <strong>Replay & seek:</strong> subscriptions support replay from sequence/time and cursor reset.
          </li>
          <li>
            <strong>Idempotency:</strong> async invocation API supports idempotency key + TTL to avoid duplicate work.
          </li>
        </ul>
      </section>

      <section id="observability" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Observability</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>Health endpoints: <code>/health</code>, <code>/health/live</code>, <code>/health/ready</code>, <code>/health/startup</code>.</li>
          <li>Operational metrics: <code>/metrics</code>, <code>/metrics/timeseries</code>, <code>/metrics/heatmap</code>.</li>
          <li>Function-level diagnostics: <code>/functions/{"{"}name{"}"}/logs</code>, <code>/functions/{"{"}name{"}"}/metrics</code>.</li>
          <li>Pool and system status: <code>/stats</code>, invocation history via <code>/invocations</code>.</li>
        </ul>
      </section>
    </DocsShell>
  )
}
