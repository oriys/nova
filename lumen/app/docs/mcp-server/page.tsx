/* eslint-disable i18next/no-literal-string */
import { getTranslations } from "next-intl/server"
import { DocsShell } from "@/components/docs/docs-shell"
import { CodeBlock } from "@/components/docs/code-block"

export default async function DocsMCPServerPage() {
  const t = await getTranslations("docsMcpServerPage")

  return (
    <DocsShell
      current="mcp"
      title={t("title")}
      description={t("description")}
      toc={[
        { id: "overview", label: t("toc.overview") },
        { id: "when-to-use", label: t("toc.whenToUse") },
        { id: "build-and-run", label: t("toc.buildAndRun") },
        { id: "connection-config", label: t("toc.connectionConfig") },
        { id: "tool-surface", label: t("toc.toolSurface") },
        { id: "operational-guidance", label: t("toc.operationalGuidance") },
      ]}
    >
      <section id="overview" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.overview.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {/* i18n-ignore-next-line */}
          {t("sections.overview.descriptionPrefix")} <code>nova_*</code> {t("sections.overview.descriptionSuffix")}
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
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.whenToUse.title")}</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>{t("sections.whenToUse.items.item1")}</li>
          <li>{t("sections.whenToUse.items.item2")}</li>
          <li>{t("sections.whenToUse.items.item3")}</li>
        </ul>
      </section>

      <section id="build-and-run" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.buildAndRun.title")}</h2>
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
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.connectionConfig.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.connectionConfig.description")}
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
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.toolSurface.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.toolSurface.description")}
        </p>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>{t("sections.toolSurface.items.functions")}</li>
          <li>{t("sections.toolSurface.items.workflows")}</li>
          <li>{t("sections.toolSurface.items.events")}</li>
          <li>{t("sections.toolSurface.items.governance")}</li>
          <li>{t("sections.toolSurface.items.operations")}</li>
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
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.operationalGuidance.title")}</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>{t("sections.operationalGuidance.items.item1")}</li>
          <li>{t("sections.operationalGuidance.items.item2")}</li>
          <li>{t("sections.operationalGuidance.items.item3")}</li>
          <li>{t("sections.operationalGuidance.items.item4")}</li>
        </ul>
      </section>
    </DocsShell>
  )
}
