/* eslint-disable i18next/no-literal-string */
import { getTranslations } from "next-intl/server"
import { DocsShell } from "@/components/docs/docs-shell"
import { CodeBlock } from "@/components/docs/code-block"

export default async function DocsArchitecturePage() {
  const t = await getTranslations("docsArchitecturePage")

  return (
    <DocsShell
      current="architecture"
      title={t("title")}
      description={t("description")}
      toc={[
        { id: "principles", label: t("toc.principles") },
        { id: "component-map", label: t("toc.componentMap") },
        { id: "sync-invocation-flow", label: t("toc.syncInvocationFlow") },
        { id: "event-workflow-delivery", label: t("toc.eventWorkflowDelivery") },
        { id: "tenancy-isolation", label: t("toc.tenancyIsolation") },
        { id: "reliability-model", label: t("toc.reliabilityModel") },
        { id: "observability", label: t("toc.observability") },
      ]}
    >
      <section id="principles" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.principles.title")}</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>
            <strong>{t("sections.principles.items.backendFirst.label")}:</strong>{" "}
            {t("sections.principles.items.backendFirst.value")}
          </li>
          <li>
            <strong>{t("sections.principles.items.oneControlPlane.label")}:</strong>{" "}
            {t("sections.principles.items.oneControlPlane.value")}
          </li>
          <li>
            <strong>{t("sections.principles.items.durability.label")}:</strong>{" "}
            {t("sections.principles.items.durability.value")}
          </li>
          <li>
            <strong>{t("sections.principles.items.transparency.label")}:</strong>{" "}
            {t("sections.principles.items.transparency.value")}
          </li>
        </ul>
      </section>

      <section id="component-map" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.componentMap.title")}</h2>
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
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.syncInvocationFlow.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.syncInvocationFlow.description")}
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
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.eventWorkflowDelivery.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.eventWorkflowDelivery.description")}
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
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.tenancyIsolation.title")}</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          {/* i18n-ignore-next-line */}
          <li>{t("sections.tenancyIsolation.items.item1")} <code>X-Nova-Tenant</code> {t("sections.tenancyIsolation.items.item1Middle")} <code>X-Nova-Namespace</code>.</li>
          <li>{t("sections.tenancyIsolation.items.item2")}</li>
          <li>{t("sections.tenancyIsolation.items.item3")}</li>
          <li>{t("sections.tenancyIsolation.items.item4")}</li>
        </ul>
      </section>

      <section id="reliability-model" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.reliabilityModel.title")}</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>
            <strong>{t("sections.reliabilityModel.items.retries.label")}:</strong>{" "}
            {t("sections.reliabilityModel.items.retries.value")}
          </li>
          <li>
            <strong>{t("sections.reliabilityModel.items.dlq.label")}:</strong>{" "}
            {t("sections.reliabilityModel.items.dlq.value")}
          </li>
          <li>
            <strong>{t("sections.reliabilityModel.items.replay.label")}:</strong>{" "}
            {t("sections.reliabilityModel.items.replay.value")}
          </li>
          <li>
            <strong>{t("sections.reliabilityModel.items.idempotency.label")}:</strong>{" "}
            {t("sections.reliabilityModel.items.idempotency.value")}
          </li>
        </ul>
      </section>

      <section id="observability" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.observability.title")}</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          {/* i18n-ignore-next-line */}
          <li>{t("sections.observability.items.item1")} <code>/health</code>, <code>/health/live</code>, <code>/health/ready</code>, <code>/health/startup</code>.</li>
          {/* i18n-ignore-next-line */}
          <li>{t("sections.observability.items.item2")} <code>/metrics</code>, <code>/metrics/timeseries</code>, <code>/metrics/heatmap</code>.</li>
          {/* i18n-ignore-next-line */}
          <li>{t("sections.observability.items.item3")} <code>/functions/{"{"}name{"}"}/logs</code>, <code>/functions/{"{"}name{"}"}/metrics</code>.</li>
          {/* i18n-ignore-next-line */}
          <li>{t("sections.observability.items.item4")} <code>/stats</code>, {t("sections.observability.items.item4Suffix")} <code>/invocations</code>.</li>
        </ul>
      </section>
    </DocsShell>
  )
}
