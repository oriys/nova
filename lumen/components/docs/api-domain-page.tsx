import { useTranslations } from "next-intl"
import { DocsShell } from "@/components/docs/docs-shell"
import { EndpointTable } from "@/components/docs/endpoint-table"
import { apiEndpointHref, apiDomainDocs, buildApiDocsNavGroups, type ApiDomainKey } from "@/lib/docs/api-reference"

interface ApiDomainPageProps {
  domain: ApiDomainKey
}

export function ApiDomainPage({ domain }: ApiDomainPageProps) {
  const t = useTranslations("docsApiDomainPage")
  const domainDoc = apiDomainDocs[domain]

  return (
    <DocsShell
      current="api"
      activeHref={`/docs/api/${domain}`}
      title={domainDoc.title}
      description={domainDoc.description}
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
        { id: "coverage", label: t("toc.coverage") },
        { id: "endpoint-index", label: t("toc.endpointIndex") },
      ]}
    >
      <section id="coverage" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("coverageTitle")}</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">{domainDoc.coverage}</p>
      </section>

      <section id="endpoint-index" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("endpointIndexTitle")}</h2>
        <p className="mb-4 mt-3 text-lg leading-8 text-muted-foreground">
          {t("endpointIndexDescription")}
        </p>

        <EndpointTable
          endpoints={domainDoc.endpoints.map((endpoint) => ({
            method: endpoint.spec.method,
            path: endpoint.spec.path,
            description: endpoint.spec.summary,
            href: apiEndpointHref(domain, endpoint.slug),
          }))}
        />
        <p className="mt-4 text-sm text-muted-foreground">
          {t("leftSidebarHint")}
        </p>
      </section>
    </DocsShell>
  )
}
