import Link from "next/link"
import { getTranslations } from "next-intl/server"
import { DocsShell } from "@/components/docs/docs-shell"
import { CodeBlock } from "@/components/docs/code-block"
import { EndpointTable, type Endpoint } from "@/components/docs/endpoint-table"
import {
  apiDomainDocs,
  apiDomainHref,
  apiDomainOrder,
  apiEndpointHref,
  buildApiDocsNavGroups,
} from "@/lib/docs/api-reference"

const endpointIndex: Endpoint[] = apiDomainOrder.flatMap((domain) =>
  apiDomainDocs[domain].endpoints.map((endpoint) => ({
    method: endpoint.spec.method,
    path: endpoint.spec.path,
    description: endpoint.spec.summary,
    href: apiEndpointHref(domain, endpoint.slug),
  }))
)

export default async function DocsAPIPage() {
  const t = await getTranslations("docsApiOverviewPage")

  return (
    <DocsShell
      current="api"
      activeHref="/docs/api"
      title={t("title")}
      description={t("description")}
      navGroups={buildApiDocsNavGroups({
        guides: t("nav.guides"),
        introduction: t("nav.introduction"),
        architecture: t("nav.architecture"),
        installation: t("nav.installation"),
        reference: t("nav.reference"),
        apiOverview: t("nav.apiOverview"),
        orbitCli: t("nav.orbitCli"),
        atlasMcpServer: t("nav.atlasMcpServer"),
      })}
      toc={[
        { id: "api-basics", label: t("toc.apiBasics") },
        { id: "headers-auth", label: t("toc.headersAuth") },
        { id: "error-format", label: t("toc.errorFormat") },
        { id: "pagination-retries", label: t("toc.paginationRetries") },
        { id: "reference-sections", label: t("toc.referenceSections") },
        { id: "endpoint-index", label: t("toc.endpointIndex") },
      ]}
    >
      <section id="api-basics" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.apiBasics.title")}</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>{t("sections.apiBasics.items.baseUrl")} <code>http://&lt;nova-host&gt;:9000</code></li>
          <li>{t("sections.apiBasics.items.mediaType")} <code>application/json</code></li>
          <li>{t("sections.apiBasics.items.timeFormat")}</li>
          <li>{t("sections.apiBasics.items.tenantAware")}</li>
        </ul>
      </section>

      <section id="headers-auth" className="scroll-mt-24 space-y-2">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.headersAuth.title")}</h2>
        <p className="text-lg leading-8 text-muted-foreground">
          {t("sections.headersAuth.description")}
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
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.errorFormat.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.errorFormat.description")}
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
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.paginationRetries.title")}</h2>
        <ul className="mt-4 list-disc space-y-3 pl-6 text-lg leading-8">
          <li>{t("sections.paginationRetries.items.limitAndFilters")} <code>limit</code> {t("sections.paginationRetries.items.limitAndFiltersSuffix")}</li>
          <li>{t("sections.paginationRetries.items.asyncIdempotencyPrefix")} <code>idempotency_key</code> {t("sections.paginationRetries.items.asyncIdempotencySuffix")}</li>
          <li>{t("sections.paginationRetries.items.subscriptionReplay")}</li>
          <li>{t("sections.paginationRetries.items.deliveryRetry")}</li>
        </ul>
      </section>

      <section id="reference-sections" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.referenceSections.title")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">
          {t("sections.referenceSections.description")}
        </p>
        <div className="mt-4 grid gap-3 md:grid-cols-2">
          {apiDomainOrder.map((domain) => (
            <Link
              key={domain}
              href={apiDomainHref(domain)}
              className="rounded-lg border border-border p-4 hover:bg-muted/40"
            >
              <p className="text-sm font-medium">{apiDomainDocs[domain].title}</p>
              <p className="mt-1 text-sm text-muted-foreground">{apiDomainDocs[domain].description}</p>
            </Link>
          ))}
        </div>
      </section>

      <section id="endpoint-index" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("sections.endpointIndex.title")}</h2>
        <p className="mb-4 mt-3 text-lg leading-8 text-muted-foreground">
          {t("sections.endpointIndex.description")}
        </p>
        <EndpointTable endpoints={endpointIndex} />
      </section>
    </DocsShell>
  )
}
