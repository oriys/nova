import Link from "next/link"
import { DocsShell } from "@/components/docs/docs-shell"
import { CodeBlock } from "@/components/docs/code-block"
import { Badge } from "@/components/ui/badge"

type CoverageLevel = "full" | "partial" | "none"

type SurfaceComparisonRow = {
  capability: string
  lumen: CoverageLevel
  orbit: CoverageLevel
  mcp: CoverageLevel
  note: string
}

const surfaceComparisonRows: SurfaceComparisonRow[] = [
  {
    capability: "Functions lifecycle + invoke + logs",
    lumen: "full",
    orbit: "full",
    mcp: "full",
    note: "All three cover daily function development and operations.",
  },
  {
    capability: "Workflows (define/version/run)",
    lumen: "full",
    orbit: "full",
    mcp: "full",
    note: "Complete workflow lifecycle is available across all interfaces.",
  },
  {
    capability: "Events (topic/subscription/delivery/replay)",
    lumen: "full",
    orbit: "full",
    mcp: "full",
    note: "End-to-end event delivery controls are available in each surface.",
  },
  {
    capability: "Tenancy governance (quotas/usage)",
    lumen: "full",
    orbit: "full",
    mcp: "full",
    note: "Tenant and namespace governance is exposed everywhere.",
  },
  {
    capability: "Gateway route management",
    lumen: "none",
    orbit: "full",
    mcp: "full",
    note: "Gateway operations are currently CLI/MCP-first.",
  },
  {
    capability: "Layer management",
    lumen: "none",
    orbit: "full",
    mcp: "full",
    note: "Layer operations are currently CLI/MCP-first.",
  },
  {
    capability: "Scriptable batch automation",
    lumen: "partial",
    orbit: "full",
    mcp: "full",
    note: "Lumen can operate interactively; automation is stronger with Orbit/MCP.",
  },
  {
    capability: "AI-native tool calling",
    lumen: "none",
    orbit: "none",
    mcp: "full",
    note: "MCP is designed for agentic and assistant-driven operations.",
  },
]

function coverageBadge(level: CoverageLevel) {
  switch (level) {
    case "full":
      return (
        <Badge variant="secondary" className="border-0 bg-success/10 text-success">
          Full
        </Badge>
      )
    case "partial":
      return (
        <Badge variant="secondary" className="border-0 bg-amber-500/15 text-amber-700 dark:text-amber-400">
          Partial
        </Badge>
      )
    default:
      return (
        <Badge variant="secondary" className="border-0 bg-muted text-muted-foreground">
          Not Primary
        </Badge>
      )
  }
}

export default function DocsPage() {
  return (
    <DocsShell
      current="introduction"
      title="Introduction"
      description="Nova is a backend-first serverless control plane. It provides function execution, workflow orchestration, event delivery, and tenant-aware operations behind one API."
      toc={[
        { id: "who-this-is-for", label: "Who This Is For" },
        { id: "what-you-get", label: "What You Get" },
        { id: "control-surface-comparison", label: "Control Surface Comparison" },
        { id: "docs-map", label: "Docs Map" },
        { id: "recommended-learning-path", label: "Recommended Learning Path" },
        { id: "first-smoke-test", label: "First Smoke Test" },
        { id: "next-steps", label: "Next Steps" },
      ]}
    >
      <div className="rounded-lg border border-border bg-muted/30 px-4 py-3">
        <p className="text-sm text-muted-foreground">
          Looking for capability differences? Jump to{" "}
          <Link href="#control-surface-comparison" className="font-medium text-foreground underline underline-offset-4">
            Control Surface Comparison
          </Link>
          .
        </p>
      </div>

      <section id="who-this-is-for" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Who This Is For</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          This documentation is for platform engineers and backend developers who want to run Nova as a
          multi-tenant serverless backend and operate it with Orbit CLI.
        </p>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>You need one backend API for functions, workflows, and event-driven delivery.</li>
          <li>You want tenant and namespace isolation at request level.</li>
          <li>You need local development parity with production-like operational primitives.</li>
        </ul>
      </section>

      <section id="what-you-get" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">What You Get</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>
            <strong>Function runtime control:</strong> create/update/invoke functions, observe logs and metrics.
          </li>
          <li>
            <strong>Workflow orchestration:</strong> publish workflow definitions and trigger runs.
          </li>
          <li>
            <strong>Event bus:</strong> topics, subscriptions, delivery retries, replay, outbox relay.
          </li>
          <li>
            <strong>Platform governance:</strong> tenants, namespaces, quotas, API keys, secrets.
          </li>
          <li>
            <strong>Operations tooling:</strong> Orbit CLI for operators and Atlas MCP server for AI tooling.
          </li>
        </ul>
      </section>

      <section id="control-surface-comparison" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Control Surface Comparison</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          Nova exposes the same backend through three operation surfaces. They overlap heavily but are not identical.
          Use this matrix to pick the right entry point for each task.
        </p>

        <div className="mt-5 overflow-x-auto rounded-lg border border-border">
          <table className="w-full min-w-[980px] text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/30">
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">Capability</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">Lumen UI</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">Orbit CLI</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">Atlas MCP</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">Notes</th>
              </tr>
            </thead>
            <tbody>
              {surfaceComparisonRows.map((row) => (
                <tr key={row.capability} className="border-b border-border last:border-0">
                  <td className="px-3 py-2 font-medium text-foreground">{row.capability}</td>
                  <td className="px-3 py-2">{coverageBadge(row.lumen)}</td>
                  <td className="px-3 py-2">{coverageBadge(row.orbit)}</td>
                  <td className="px-3 py-2">{coverageBadge(row.mcp)}</td>
                  <td className="px-3 py-2 text-muted-foreground">{row.note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="mt-5 grid gap-3 md:grid-cols-3">
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm font-medium text-foreground">Choose Lumen UI when</p>
            <p className="mt-2 text-sm text-muted-foreground">
              you need visual inspection, quick interactive edits, and dashboard-driven troubleshooting.
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm font-medium text-foreground">Choose Orbit CLI when</p>
            <p className="mt-2 text-sm text-muted-foreground">
              you need full API coverage, repeatable scripts, CI/CD integration, and bulk operations.
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm font-medium text-foreground">Choose Atlas MCP when</p>
            <p className="mt-2 text-sm text-muted-foreground">
              you want AI agents to call typed Nova tools directly with tenant-scoped context.
            </p>
          </div>
        </div>
      </section>

      <section id="docs-map" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Docs Map</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          The docs are split into guides and reference pages to match common engineering workflows.
        </p>
        <CodeBlock
          code={`Guides
- /docs                  Introduction and orientation
- /docs/architecture     System model and runtime flow
- /docs/installation     Local and production setup

Reference
- /docs/api              HTTP API contracts (inputs/outputs/errors/examples)
- /docs/cli              Orbit CLI operations reference
- /docs/mcp-server       Atlas MCP server reference`}
        />
      </section>

      <section id="recommended-learning-path" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Recommended Learning Path</h2>
        <ol className="mt-4 list-decimal space-y-3 pl-6 text-lg leading-8">
          <li>
            Read <Link href="/docs/architecture" className="underline underline-offset-4">Architecture</Link> to
            understand control plane vs data plane and delivery pipelines.
          </li>
          <li>
            Follow <Link href="/docs/installation" className="underline underline-offset-4">Installation</Link> to
            stand up Nova + Lumen + Orbit in your environment.
          </li>
          <li>
            Use <Link href="/docs/api" className="underline underline-offset-4">API Reference</Link> for request and
            response schemas while integrating clients.
          </li>
          <li>
            Keep <Link href="/docs/cli" className="underline underline-offset-4">Orbit CLI</Link> open for operational
            tasks and day-2 operations.
          </li>
        </ol>
      </section>

      <section id="first-smoke-test" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">First Smoke Test</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          After deployment, run this minimal end-to-end test to verify API reachability, function lifecycle, and
          execution path.
        </p>
        <CodeBlock
          code={`# 1) Verify backend health (Zenith entrypoint)
curl -s http://localhost:9000/health | jq

# 2) Configure Orbit once
orbit config set server http://localhost:9000
orbit config set tenant default
orbit config set namespace default

# 3) Create and invoke a function
orbit functions create \
  --name hello-docs \
  --runtime python \
  --handler handler \
  --code 'def handler(event, context):\n    return {"ok": True, "echo": event}'

orbit functions invoke hello-docs --payload '{"source":"docs"}'`}
        />
      </section>

      <section id="next-steps" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Next Steps</h2>
        <div className="mt-4 flex flex-wrap gap-3">
          <Link href="/docs/architecture" className="rounded-md border border-border px-3 py-2 text-sm hover:bg-muted/60">
            Read Architecture
          </Link>
          <Link href="/docs/installation" className="rounded-md border border-border px-3 py-2 text-sm hover:bg-muted/60">
            Run Installation
          </Link>
          <Link href="/docs/api" className="rounded-md border border-border px-3 py-2 text-sm hover:bg-muted/60">
            Open API Reference
          </Link>
        </div>
      </section>
    </DocsShell>
  )
}
