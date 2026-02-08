import Link from "next/link"
import { Badge } from "@/components/ui/badge"

export interface Endpoint {
  method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE"
  path: string
  description: string
  href?: string
}

function methodBadgeVariant(method: Endpoint["method"]): "default" | "secondary" | "outline" | "destructive" {
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

interface EndpointTableProps {
  endpoints: Endpoint[]
}

export function EndpointTable({ endpoints }: EndpointTableProps) {
  const hasLinks = endpoints.some((endpoint) => !!endpoint.href)

  return (
    <div className="overflow-x-auto rounded-lg border border-border">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border bg-muted/30">
            <th className="px-3 py-2 text-left font-medium text-muted-foreground">Method</th>
            <th className="px-3 py-2 text-left font-medium text-muted-foreground">Path</th>
            <th className="px-3 py-2 text-left font-medium text-muted-foreground">Description</th>
            {hasLinks && <th className="px-3 py-2 text-left font-medium text-muted-foreground">Doc</th>}
          </tr>
        </thead>
        <tbody>
          {endpoints.map((endpoint) => (
            <tr key={`${endpoint.method}-${endpoint.path}`} className="border-b border-border last:border-0">
              <td className="px-3 py-2">
                <Badge variant={methodBadgeVariant(endpoint.method)}>{endpoint.method}</Badge>
              </td>
              <td className="px-3 py-2 font-mono text-xs text-foreground">{endpoint.path}</td>
              <td className="px-3 py-2 text-muted-foreground">{endpoint.description}</td>
              {hasLinks && (
                <td className="px-3 py-2 text-sm">
                  {endpoint.href ? (
                    <Link
                      href={endpoint.href}
                      className="text-foreground underline underline-offset-4 hover:text-foreground/80"
                    >
                      Open
                    </Link>
                  ) : (
                    <span className="text-muted-foreground">-</span>
                  )}
                </td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
