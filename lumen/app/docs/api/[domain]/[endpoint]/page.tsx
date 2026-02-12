import Link from "next/link"
import { getTranslations } from "next-intl/server"
import { notFound } from "next/navigation"

import { EndpointSpecCard } from "@/components/docs/api-spec"
import { DocsShell } from "@/components/docs/docs-shell"
import {
  apiDomainDocs,
  apiDomainHref,
  apiDomainOrder,
  apiEndpointHref,
  buildApiDocsNavGroups,
  getApiEndpointDoc,
  type ApiDomainKey,
} from "@/lib/docs/api-reference"

interface EndpointPageProps {
  params: Promise<{
    domain: string
    endpoint: string
  }>
}

export function generateStaticParams() {
  return apiDomainOrder.flatMap((domain) =>
    apiDomainDocs[domain].endpoints.map((endpoint) => ({
      domain,
      endpoint: endpoint.slug,
    }))
  )
}

export default async function DocsApiEndpointPage({ params }: EndpointPageProps) {
  const t = await getTranslations("docsApiEndpointPage")
  const { domain, endpoint } = await params

  if (!(domain in apiDomainDocs)) {
    notFound()
  }

  const domainKey = domain as ApiDomainKey
  const hit = getApiEndpointDoc(domainKey, endpoint)

  if (!hit) {
    notFound()
  }

  const { domainDoc, endpointDoc, index } = hit
  const prev = index > 0 ? domainDoc.endpoints[index - 1] : null
  const next = index < domainDoc.endpoints.length - 1 ? domainDoc.endpoints[index + 1] : null

  return (
    <DocsShell
      current="api"
      activeHref={apiEndpointHref(domainKey, endpointDoc.slug)}
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
      title={endpointDoc.spec.title}
      description={endpointDoc.spec.summary}
      toc={[
        { id: endpointDoc.spec.id, label: t("toc.contract") },
        { id: "navigation", label: t("toc.navigation") },
      ]}
    >
      <EndpointSpecCard spec={endpointDoc.spec} showHeading={false} />

      <section id="navigation" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">{t("navigationTitle")}</h2>
        <div className="mt-4 flex flex-wrap items-center gap-2">
          <Link
            href={apiDomainHref(domainKey)}
            className="rounded-md border border-border px-3 py-2 text-sm hover:bg-muted/60"
          >
            {t("backTo", { domain: domainDoc.title })}
          </Link>
          {prev && (
            <Link
              href={apiEndpointHref(domainKey, prev.slug)}
              className="rounded-md border border-border px-3 py-2 text-sm hover:bg-muted/60"
            >
              {t("previous", { title: prev.spec.title })}
            </Link>
          )}
          {next && (
            <Link
              href={apiEndpointHref(domainKey, next.slug)}
              className="rounded-md border border-border px-3 py-2 text-sm hover:bg-muted/60"
            >
              {t("next", { title: next.spec.title })}
            </Link>
          )}
        </div>
      </section>
    </DocsShell>
  )
}
