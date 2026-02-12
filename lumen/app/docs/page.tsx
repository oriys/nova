import Link from "next/link"
import { getTranslations } from "next-intl/server"
import { DocsShell } from "@/components/docs/docs-shell"
import { CodeBlock } from "@/components/docs/code-block"
import { Badge } from "@/components/ui/badge"

type CoverageLevel = "full" | "partial" | "none"
type SurfaceComparisonRowKey =
  | "functionsLifecycle"
  | "workflowsLifecycle"
  | "eventsLifecycle"
  | "tenancyGovernance"
  | "gatewayRoutes"
  | "layerManagement"
  | "batchAutomation"
  | "aiToolCalling"

type SurfaceComparisonRow = {
  key: SurfaceComparisonRowKey
  lumen: CoverageLevel
  orbit: CoverageLevel
  mcp: CoverageLevel
}

const surfaceComparisonRows: SurfaceComparisonRow[] = [
  {
    key: "functionsLifecycle",
    lumen: "full",
    orbit: "full",
    mcp: "full",
  },
  {
    key: "workflowsLifecycle",
    lumen: "full",
    orbit: "full",
    mcp: "full",
  },
  {
    key: "eventsLifecycle",
    lumen: "full",
    orbit: "full",
    mcp: "full",
  },
  {
    key: "tenancyGovernance",
    lumen: "full",
    orbit: "full",
    mcp: "full",
  },
  {
    key: "gatewayRoutes",
    lumen: "none",
    orbit: "full",
    mcp: "full",
  },
  {
    key: "layerManagement",
    lumen: "none",
    orbit: "full",
    mcp: "full",
  },
  {
    key: "batchAutomation",
    lumen: "partial",
    orbit: "full",
    mcp: "full",
  },
  {
    key: "aiToolCalling",
    lumen: "none",
    orbit: "none",
    mcp: "full",
  },
]

function coverageBadge(level: CoverageLevel, t: Awaited<ReturnType<typeof getTranslations>>) {
  switch (level) {
    case "full":
      return (
        <Badge variant="secondary" className="border-0 bg-success/10 text-success">
          {t("comparison.badges.full")}
        </Badge>
      )
    case "partial":
      return (
        <Badge variant="secondary" className="border-0 bg-amber-500/15 text-amber-700 dark:text-amber-400">
          {t("comparison.badges.partial")}
        </Badge>
      )
    default:
      return (
        <Badge variant="secondary" className="border-0 bg-muted text-muted-foreground">
          {t("comparison.badges.notPrimary")}
        </Badge>
      )
  }
}

export default async function DocsPage() {
  const t = await getTranslations("docsIntroPage")

  return (
    <DocsShell
      current="introduction"
      title={t("title")}
      description={t("description")}
      toc={[
        { id: "who-this-is-for", label: t("toc.whoThisIsFor") },
        { id: "what-you-get", label: t("toc.whatYouGet") },
        { id: "control-surface-comparison", label: t("toc.controlSurfaceComparison") },
        { id: "docs-map", label: t("toc.docsMap") },
        { id: "recommended-learning-path", label: t("toc.recommendedLearningPath") },
        { id: "first-smoke-test", label: t("toc.firstSmokeTest") },
        { id: "next-steps", label: t("toc.nextSteps") },
      ]}
    >
      <div className="rounded-lg border border-border bg-muted/30 px-4 py-3">
        <p className="text-sm text-muted-foreground">
          {t("jumpToComparison.prefix")}{" "}
          <Link href="#control-surface-comparison" className="font-medium text-foreground underline underline-offset-4">
            {t("toc.controlSurfaceComparison")}
          </Link>
          {t("jumpToComparison.suffix")}
        </p>
      </div>

      <section id="who-this-is-for" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.whoThisIsFor.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.whoThisIsFor.description")}
        </p>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>{t("sections.whoThisIsFor.items.item1")}</li>
          <li>{t("sections.whoThisIsFor.items.item2")}</li>
          <li>{t("sections.whoThisIsFor.items.item3")}</li>
        </ul>
      </section>

      <section id="what-you-get" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.whatYouGet.title")}</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>
            <strong>{t("sections.whatYouGet.items.functionControl.label")}:</strong>{" "}
            {t("sections.whatYouGet.items.functionControl.value")}
          </li>
          <li>
            <strong>{t("sections.whatYouGet.items.workflowOrchestration.label")}:</strong>{" "}
            {t("sections.whatYouGet.items.workflowOrchestration.value")}
          </li>
          <li>
            <strong>{t("sections.whatYouGet.items.eventBus.label")}:</strong>{" "}
            {t("sections.whatYouGet.items.eventBus.value")}
          </li>
          <li>
            <strong>{t("sections.whatYouGet.items.platformGovernance.label")}:</strong>{" "}
            {t("sections.whatYouGet.items.platformGovernance.value")}
          </li>
          <li>
            <strong>{t("sections.whatYouGet.items.operationsTooling.label")}:</strong>{" "}
            {t("sections.whatYouGet.items.operationsTooling.value")}
          </li>
        </ul>
      </section>

      <section id="control-surface-comparison" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.comparison.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.comparison.description")}
        </p>

        <div className="mt-5 overflow-x-auto rounded-lg border border-border">
          <table className="w-full min-w-[980px] text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/30">
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("sections.comparison.columns.capability")}</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("sections.comparison.columns.lumenUi")}</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("sections.comparison.columns.orbitCli")}</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("sections.comparison.columns.atlasMcp")}</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">{t("sections.comparison.columns.notes")}</th>
              </tr>
            </thead>
            <tbody>
              {surfaceComparisonRows.map((row) => (
                <tr key={row.key} className="border-b border-border last:border-0">
                  <td className="px-3 py-2 font-medium text-foreground">{t(`sections.comparison.rows.${row.key}.capability`)}</td>
                  <td className="px-3 py-2">{coverageBadge(row.lumen, t)}</td>
                  <td className="px-3 py-2">{coverageBadge(row.orbit, t)}</td>
                  <td className="px-3 py-2">{coverageBadge(row.mcp, t)}</td>
                  <td className="px-3 py-2 text-muted-foreground">{t(`sections.comparison.rows.${row.key}.note`)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="mt-5 grid gap-3 md:grid-cols-3">
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm font-medium text-foreground">{t("sections.comparison.choose.lumenTitle")}</p>
            <p className="mt-2 text-sm text-muted-foreground">
              {t("sections.comparison.choose.lumenDescription")}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm font-medium text-foreground">{t("sections.comparison.choose.orbitTitle")}</p>
            <p className="mt-2 text-sm text-muted-foreground">
              {t("sections.comparison.choose.orbitDescription")}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm font-medium text-foreground">{t("sections.comparison.choose.atlasTitle")}</p>
            <p className="mt-2 text-sm text-muted-foreground">
              {t("sections.comparison.choose.atlasDescription")}
            </p>
          </div>
        </div>
      </section>

      <section id="docs-map" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.docsMap.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.docsMap.description")}
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
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.learningPath.title")}</h2>
        <ol className="mt-4 list-decimal space-y-3 pl-6 text-lg leading-8">
          <li>
            {t("sections.learningPath.items.architecture.prefix")}{" "}
            <Link href="/docs/architecture" className="underline underline-offset-4">
              {t("sections.learningPath.items.architecture.link")}
            </Link>{" "}
            {t("sections.learningPath.items.architecture.suffix")}
          </li>
          <li>
            {t("sections.learningPath.items.installation.prefix")}{" "}
            <Link href="/docs/installation" className="underline underline-offset-4">
              {t("sections.learningPath.items.installation.link")}
            </Link>{" "}
            {t("sections.learningPath.items.installation.suffix")}
          </li>
          <li>
            {t("sections.learningPath.items.api.prefix")}{" "}
            <Link href="/docs/api" className="underline underline-offset-4">
              {t("sections.learningPath.items.api.link")}
            </Link>{" "}
            {t("sections.learningPath.items.api.suffix")}
          </li>
          <li>
            {t("sections.learningPath.items.cli.prefix")}{" "}
            <Link href="/docs/cli" className="underline underline-offset-4">
              {t("sections.learningPath.items.cli.link")}
            </Link>{" "}
            {t("sections.learningPath.items.cli.suffix")}
          </li>
        </ol>
      </section>

      <section id="first-smoke-test" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.firstSmokeTest.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.firstSmokeTest.description")}
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
  --handler main.handler \
  --code 'def handler(event, context):\n    return {"ok": True, "echo": event}'

orbit functions invoke hello-docs --payload '{"source":"docs"}'`}
        />
      </section>

      <section id="next-steps" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.nextSteps.title")}</h2>
        <div className="mt-4 flex flex-wrap gap-3">
          <Link href="/docs/architecture" className="rounded-md border border-border px-3 py-2 text-sm hover:bg-muted/60">
            {t("sections.nextSteps.actions.architecture")}
          </Link>
          <Link href="/docs/installation" className="rounded-md border border-border px-3 py-2 text-sm hover:bg-muted/60">
            {t("sections.nextSteps.actions.installation")}
          </Link>
          <Link href="/docs/api" className="rounded-md border border-border px-3 py-2 text-sm hover:bg-muted/60">
            {t("sections.nextSteps.actions.api")}
          </Link>
        </div>
      </section>
    </DocsShell>
  )
}
