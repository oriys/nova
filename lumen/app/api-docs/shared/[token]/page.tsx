"use client"

import { useEffect, useState } from "react"
import { useParams } from "next/navigation"
import { Badge } from "@/components/ui/badge"
import { apiDocsApi, type APIDocShare } from "@/lib/api"
import { FileText, AlertTriangle, Loader2 } from "lucide-react"

export default function SharedDocPage() {
  const params = useParams()
  const token = params.token as string
  const [doc, setDoc] = useState<APIDocShare | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!token) return
    apiDocsApi
      .getSharedDoc(token)
      .then((data) => setDoc(data))
      .catch((err) => setError(err instanceof Error ? err.message : "Document not found or expired"))
      .finally(() => setLoading(false))
  }, [token])

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error || !doc) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <div className="text-center space-y-4">
          <AlertTriangle className="mx-auto h-12 w-12 text-destructive" />
          <h1 className="text-2xl font-bold">Document Not Available</h1>
          <p className="text-muted-foreground">{error || "The requested document was not found."}</p>
        </div>
      </div>
    )
  }

  const docs = doc.doc_content

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b bg-card px-6 py-4">
        <div className="max-w-4xl mx-auto flex items-center gap-3">
          <FileText className="h-6 w-6 text-primary" />
          <div>
            <h1 className="text-xl font-bold">{doc.title}</h1>
            <p className="text-sm text-muted-foreground">
              Shared API Documentation · {doc.function_name}
            </p>
          </div>
        </div>
      </header>

      <main className="max-w-4xl mx-auto px-6 py-8 space-y-8">
        {/* Meta */}
        <section className="space-y-4">
          <div className="flex items-center gap-2 flex-wrap">
            <h2 className="text-2xl font-bold">{docs.name}</h2>
            {docs.stability && <Badge variant="secondary">{docs.stability}</Badge>}
            {docs.version && <Badge variant="outline">{docs.version}</Badge>}
          </div>
          <p className="text-lg text-muted-foreground">{docs.summary}</p>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 rounded-lg border p-4">
            <div><span className="text-xs text-muted-foreground block">Protocol</span><span className="text-sm font-medium">{docs.protocol}</span></div>
            <div><span className="text-xs text-muted-foreground block">Auth</span><span className="text-sm font-medium">{docs.auth_method}</span></div>
            <div><span className="text-xs text-muted-foreground block">Rate Limit</span><span className="text-sm font-medium">{docs.rate_limit}</span></div>
            <div><span className="text-xs text-muted-foreground block">Timeout</span><span className="text-sm font-medium">{docs.timeout}</span></div>
          </div>
        </section>

        {/* Endpoint */}
        <section className="space-y-2">
          <h3 className="text-lg font-semibold">Endpoint</h3>
          <div className="flex items-center gap-2">
            <Badge>{docs.method}</Badge>
            <code className="rounded bg-muted px-2 py-1 text-sm">{docs.path}</code>
          </div>
          <p className="text-sm text-muted-foreground">Content-Type: {docs.content_type}</p>
        </section>

        {/* Request Fields */}
        {docs.request_fields && docs.request_fields.length > 0 && (
          <section className="space-y-2">
            <h3 className="text-lg font-semibold">Request Fields</h3>
            <div className="rounded-lg border overflow-hidden">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-3 py-2 text-left font-medium">Field</th>
                    <th className="px-3 py-2 text-left font-medium">Type</th>
                    <th className="px-3 py-2 text-left font-medium">Required</th>
                    <th className="px-3 py-2 text-left font-medium">Default</th>
                    <th className="px-3 py-2 text-left font-medium">Description</th>
                  </tr>
                </thead>
                <tbody>
                  {docs.request_fields.map((f, i) => (
                    <tr key={i} className="border-b">
                      <td className="px-3 py-2 font-mono text-xs">{f.name}</td>
                      <td className="px-3 py-2"><Badge variant="outline" className="text-xs">{f.type}</Badge></td>
                      <td className="px-3 py-2">{f.required ? <Badge className="text-xs">Required</Badge> : <span className="text-muted-foreground text-xs">Optional</span>}</td>
                      <td className="px-3 py-2 text-xs text-muted-foreground">{f.default || "-"}</td>
                      <td className="px-3 py-2 text-muted-foreground">{f.description}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        )}

        {/* Response Fields */}
        {docs.response_fields && docs.response_fields.length > 0 && (
          <section className="space-y-2">
            <h3 className="text-lg font-semibold">Response Fields</h3>
            <div className="rounded-lg border overflow-hidden">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-3 py-2 text-left font-medium">Field</th>
                    <th className="px-3 py-2 text-left font-medium">Type</th>
                    <th className="px-3 py-2 text-left font-medium">Description</th>
                  </tr>
                </thead>
                <tbody>
                  {docs.response_fields.map((f, i) => (
                    <tr key={i} className="border-b">
                      <td className="px-3 py-2 font-mono text-xs">{f.name}</td>
                      <td className="px-3 py-2"><Badge variant="outline" className="text-xs">{f.type}</Badge></td>
                      <td className="px-3 py-2 text-muted-foreground">{f.description}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        )}

        {/* Status Codes */}
        <section className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {docs.success_codes && docs.success_codes.length > 0 && (
            <div className="space-y-2">
              <h3 className="text-lg font-semibold">Success Codes</h3>
              <div className="space-y-1">
                {docs.success_codes.map((c, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <Badge variant="secondary" className="bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400 border-0">{c.code}</Badge>
                    <span className="text-sm text-muted-foreground">{c.meaning}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
          {docs.error_codes && docs.error_codes.length > 0 && (
            <div className="space-y-2">
              <h3 className="text-lg font-semibold">Error Codes</h3>
              <div className="space-y-1">
                {docs.error_codes.map((c, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <Badge variant="secondary" className="bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400 border-0">{c.code}</Badge>
                    <span className="text-sm text-muted-foreground">{c.meaning}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </section>

        {/* Error Model */}
        {docs.error_model && (
          <section className="space-y-2">
            <h3 className="text-lg font-semibold">Error Model</h3>
            <div className="rounded-lg border p-4 space-y-2 text-sm">
              <p><span className="font-medium">Format:</span> {docs.error_model.format}</p>
              <p><span className="font-medium">Retryable:</span> {docs.error_model.retryable}</p>
              <p><span className="font-medium">Description:</span> {docs.error_model.description}</p>
            </div>
          </section>
        )}

        {/* Auth & Idempotency */}
        <section className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="space-y-2">
            <h3 className="text-lg font-semibold">Authentication</h3>
            <div className="rounded-lg border p-4 space-y-1 text-sm">
              <p><span className="font-medium">Method:</span> {docs.auth_method}</p>
              {docs.roles_required && docs.roles_required.length > 0 && (
                <p><span className="font-medium">Roles:</span> {docs.roles_required.join(", ")}</p>
              )}
            </div>
          </div>
          <div className="space-y-2">
            <h3 className="text-lg font-semibold">Idempotency</h3>
            <div className="rounded-lg border p-4 space-y-1 text-sm">
              <p><span className="font-medium">Idempotent:</span> {docs.idempotent ? "Yes" : "No"}</p>
              {docs.idempotent_key && <p><span className="font-medium">Key:</span> {docs.idempotent_key}</p>}
            </div>
          </div>
        </section>

        {/* Observability */}
        <section className="space-y-2">
          <h3 className="text-lg font-semibold">Observability & Performance</h3>
          <div className="rounded-lg border p-4 grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
            <div><span className="font-medium">Tracing:</span> {docs.supports_tracing ? "Yes (X-Request-Id)" : "No"}</div>
            <div><span className="font-medium">Rate Limit:</span> {docs.rate_limit}</div>
            <div><span className="font-medium">Timeout:</span> {docs.timeout}</div>
            <div><span className="font-medium">Pagination:</span> {docs.pagination || "N/A"}</div>
          </div>
        </section>

        {/* Examples */}
        {docs.curl_example && (
          <section className="space-y-2">
            <h3 className="text-lg font-semibold">cURL Example</h3>
            <pre className="rounded-lg border bg-muted/30 p-4 text-sm overflow-x-auto"><code>{docs.curl_example}</code></pre>
          </section>
        )}
        {docs.request_example && (
          <section className="space-y-2">
            <h3 className="text-lg font-semibold">Request Example</h3>
            <pre className="rounded-lg border bg-muted/30 p-4 text-sm overflow-x-auto"><code>{docs.request_example}</code></pre>
          </section>
        )}
        {docs.response_example && (
          <section className="space-y-2">
            <h3 className="text-lg font-semibold">Response Example</h3>
            <pre className="rounded-lg border bg-muted/30 p-4 text-sm overflow-x-auto"><code>{docs.response_example}</code></pre>
          </section>
        )}
        {docs.error_example && (
          <section className="space-y-2">
            <h3 className="text-lg font-semibold">Error Response Example</h3>
            <pre className="rounded-lg border bg-muted/30 p-4 text-sm overflow-x-auto"><code>{docs.error_example}</code></pre>
          </section>
        )}

        {/* Notes */}
        {docs.notes && docs.notes.length > 0 && (
          <section className="space-y-2">
            <h3 className="text-lg font-semibold">Notes</h3>
            <ul className="list-disc list-inside space-y-1 text-sm text-muted-foreground">
              {docs.notes.map((n, i) => <li key={i}>{n}</li>)}
            </ul>
          </section>
        )}

        {/* Changelog */}
        {docs.changelog && docs.changelog.length > 0 && (
          <section className="space-y-2">
            <h3 className="text-lg font-semibold">Changelog</h3>
            <ul className="list-disc list-inside space-y-1 text-sm text-muted-foreground">
              {docs.changelog.map((c, i) => <li key={i}>{c}</li>)}
            </ul>
          </section>
        )}

        <footer className="border-t pt-6 text-center text-sm text-muted-foreground">
          <p>Generated by Nova AI · Shared on {new Date(doc.created_at).toLocaleDateString()}</p>
        </footer>
      </main>
    </div>
  )
}
