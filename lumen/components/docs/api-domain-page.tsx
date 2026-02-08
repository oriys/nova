import Link from "next/link"

import { DocsShell } from "@/components/docs/docs-shell"
import { EndpointTable } from "@/components/docs/endpoint-table"
import { apiEndpointHref, apiDomainDocs, type ApiDomainKey } from "@/lib/docs/api-reference"

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

        <div className="mt-6 grid gap-3 sm:grid-cols-2">
          {domainDoc.endpoints.map((endpoint) => (
            <Link
              key={endpoint.slug}
              href={apiEndpointHref(domain, endpoint.slug)}
              className="rounded-lg border border-border p-4 hover:bg-muted/40"
            >
              <p className="text-sm font-medium text-foreground">{endpoint.spec.title}</p>
              <p className="mt-1 text-sm text-muted-foreground">
                <code className="rounded bg-muted px-1 py-0.5 text-xs">{endpoint.spec.method}</code>
                <span className="ml-2 font-mono text-xs">{endpoint.spec.path}</span>
              </p>
            </Link>
          ))}
        </div>
      </section>
    </DocsShell>
  )
}
