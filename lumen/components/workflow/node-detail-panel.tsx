"use client"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import type { RunNode } from "@/lib/api"
import { X } from "lucide-react"

interface NodeDetailPanelProps {
  node: RunNode
  onClose: () => void
}

export function NodeDetailPanel({ node, onClose }: NodeDetailPanelProps) {
  const statusColor = (s: string): "default" | "secondary" | "destructive" | "outline" => {
    switch (s) {
      case "succeeded": return "default"
      case "running": return "secondary"
      case "failed": return "destructive"
      default: return "outline"
    }
  }

  return (
    <div className="w-80 border-l border-border bg-card h-full overflow-y-auto">
      <div className="flex items-center justify-between px-4 py-3 border-b border-border">
        <h3 className="font-medium text-sm text-foreground">Node Detail</h3>
        <Button variant="ghost" size="icon" className="h-7 w-7" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      <div className="p-4 space-y-4">
        <div>
          <p className="text-xs text-muted-foreground mb-1">Node Key</p>
          <p className="font-mono text-sm font-bold">{node.node_key}</p>
        </div>

        <div>
          <p className="text-xs text-muted-foreground mb-1">Function</p>
          <p className="text-sm">{node.function_name}</p>
        </div>

        <div>
          <p className="text-xs text-muted-foreground mb-1">Status</p>
          <Badge variant={statusColor(node.status)}>{node.status}</Badge>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div>
            <p className="text-xs text-muted-foreground mb-1">Attempt</p>
            <p className="text-sm">{node.attempt}</p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground mb-1">Deps</p>
            <p className="text-sm">{node.unresolved_deps}</p>
          </div>
        </div>

        {node.started_at && (
          <div>
            <p className="text-xs text-muted-foreground mb-1">Started</p>
            <p className="text-sm">{new Date(node.started_at).toLocaleString()}</p>
          </div>
        )}

        {node.finished_at && (
          <div>
            <p className="text-xs text-muted-foreground mb-1">Finished</p>
            <p className="text-sm">{new Date(node.finished_at).toLocaleString()}</p>
          </div>
        )}

        {node.error_message && (
          <div>
            <p className="text-xs text-muted-foreground mb-1">Error</p>
            <pre className="text-xs font-mono bg-destructive/10 text-destructive rounded p-2 whitespace-pre-wrap break-all">
              {node.error_message}
            </pre>
          </div>
        )}

        {node.input !== undefined && node.input !== null && (
          <div>
            <p className="text-xs text-muted-foreground mb-1">Input</p>
            <pre className="text-xs font-mono bg-muted rounded p-2 overflow-x-auto max-h-48">
              {typeof node.input === "string" ? node.input : JSON.stringify(node.input, null, 2)}
            </pre>
          </div>
        )}

        {node.output !== undefined && node.output !== null && (
          <div>
            <p className="text-xs text-muted-foreground mb-1">Output</p>
            <pre className="text-xs font-mono bg-muted rounded p-2 overflow-x-auto max-h-48">
              {typeof node.output === "string" ? node.output : JSON.stringify(node.output, null, 2)}
            </pre>
          </div>
        )}
      </div>
    </div>
  )
}
