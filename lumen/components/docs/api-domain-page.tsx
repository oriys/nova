import { DocsShell } from "@/components/docs/docs-shell"
import { EndpointTable } from "@/components/docs/endpoint-table"
import { apiEndpointHref, apiDomainDocs, buildApiDocsNavGroups, type ApiDomainKey } from "@/lib/docs/api-reference"

interface ApiDomainPageProps {
  domain: ApiDomainKey
}

export function ApiDomainPage({ domain }: ApiDomainPageProps) {
  const domainDoc = apiDomainDocs[domain]

  return (
    <DocsShell
      current="api"
      activeHref={`/docs/api/${domain}`}
      title={domainDoc.title}
      description={domainDoc.description}
      navGroups={buildApiDocsNavGroups()}
      toc={[
        { id: "coverage", label: "Coverage" },
        { id: "endpoint-index", label: "Endpoint Index" },
      ]}
    >
      <section id="coverage" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Coverage</h2>
        <p className="mt-4 text-lg leading-8 text-muted-foreground">{domainDoc.coverage}</p>
      </section>

      <section id="endpoint-index" className="scroll-mt-24">
        <h2 className="text-3xl font-semibold tracking-tight">Endpoint Index</h2>
        <p className="mb-4 mt-3 text-lg leading-8 text-muted-foreground">
          Each endpoint has a dedicated contract page with request/response fields, status codes, and examples.
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
          Endpoint hierarchy is also available in the left sidebar for direct navigation.
        </p>
      </section>
    </DocsShell>
  )
}
