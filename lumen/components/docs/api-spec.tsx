import { Badge } from "@/components/ui/badge"
import { CodeBlock } from "@/components/docs/code-block"
import { SchemaTable, type SchemaField } from "@/components/docs/schema-table"

export type EndpointMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE"

export interface EndpointSpec {
  id: string
  title: string
  method: EndpointMethod
  path: string
  summary: string
  auth: string
  pathFields?: SchemaField[]
  queryFields?: SchemaField[]
  requestFields?: SchemaField[]
  responseFields: SchemaField[]
  successCodes: Array<{ code: number; meaning: string }>
  errorCodes: Array<{ code: number; meaning: string }>
  notes?: string[]
  requestExample?: string
  responseExample: string
  curlExample?: string
  orbitExample?: string
}

function methodBadgeVariant(method: EndpointMethod): "default" | "secondary" | "outline" | "destructive" {
  switch (method) {
    case "GET":
      return "secondary"
    case "POST":
      return "default"
    case "PUT":
      return "outline"
    case "PATCH":
      return "outline"
    case "DELETE":
      return "destructive"
    default:
      return "outline"
  }
}

export function EndpointSpecCard({ spec, showHeading = true }: { spec: EndpointSpec; showHeading?: boolean }) {
  return (
    <section id={spec.id} className="scroll-mt-24 space-y-5">
      <div className="space-y-2">
        {showHeading && <h2 className="text-3xl font-semibold tracking-tight">{spec.title}</h2>}
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={methodBadgeVariant(spec.method)}>{spec.method}</Badge>
          <code className="rounded bg-muted px-2 py-1 text-xs">{spec.path}</code>
        </div>
        {showHeading && <p className="text-lg leading-8 text-muted-foreground">{spec.summary}</p>}
      </div>

      <div className="rounded-lg border border-border bg-muted/20 p-4 text-sm text-muted-foreground">
        <p>
          <span className="font-medium text-foreground">Auth:</span> {spec.auth}
        </p>
      </div>

      {spec.pathFields && spec.pathFields.length > 0 && <SchemaTable title="Path Parameters" fields={spec.pathFields} />}
      {spec.queryFields && spec.queryFields.length > 0 && <SchemaTable title="Query Parameters" fields={spec.queryFields} />}
      {spec.requestFields && spec.requestFields.length > 0 && <SchemaTable title="Request Body" fields={spec.requestFields} />}
      <SchemaTable title="Response Body" fields={spec.responseFields} />

      <div className="grid gap-4 md:grid-cols-2">
        <div className="rounded-lg border border-border p-4">
          <p className="mb-2 text-sm font-medium text-foreground">Success Status</p>
          <ul className="space-y-1 text-sm text-muted-foreground">
            {spec.successCodes.map((item) => (
              <li key={`success-${item.code}`}>
                <code>{item.code}</code> {item.meaning}
              </li>
            ))}
          </ul>
        </div>
        <div className="rounded-lg border border-border p-4">
          <p className="mb-2 text-sm font-medium text-foreground">Error Status</p>
          <ul className="space-y-1 text-sm text-muted-foreground">
            {spec.errorCodes.length === 0 ? (
              <li>No endpoint-specific errors.</li>
            ) : (
              spec.errorCodes.map((item) => (
                <li key={`error-${item.code}`}>
                  <code>{item.code}</code> {item.meaning}
                </li>
              ))
            )}
          </ul>
        </div>
      </div>

      {spec.notes && spec.notes.length > 0 && (
        <div className="rounded-lg border border-border p-4">
          <p className="mb-2 text-sm font-medium text-foreground">Notes</p>
          <ul className="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
            {spec.notes.map((note) => (
              <li key={note}>{note}</li>
            ))}
          </ul>
        </div>
      )}

      {spec.requestExample && (
        <div className="space-y-2">
          <p className="text-sm font-medium text-foreground">Request Example</p>
          <CodeBlock code={spec.requestExample} />
        </div>
      )}

      <div className="space-y-2">
        <p className="text-sm font-medium text-foreground">Response Example</p>
        <CodeBlock code={spec.responseExample} />
      </div>

      {spec.curlExample && (
        <div className="space-y-2">
          <p className="text-sm font-medium text-foreground">cURL Example</p>
          <CodeBlock code={spec.curlExample} />
        </div>
      )}

      {spec.orbitExample && (
        <div className="space-y-2">
          <p className="text-sm font-medium text-foreground">Orbit Example</p>
          <CodeBlock code={spec.orbitExample} />
        </div>
      )}
    </section>
  )
}
