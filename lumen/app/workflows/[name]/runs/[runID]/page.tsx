"use client"

import { useEffect, useState, useCallback } from "react"
import { useParams } from "next/navigation"
import Link from "next/link"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { workflowsApi, functionsApi, type WorkflowRun, type WorkflowVersion, type RunNode } from "@/lib/api"
import { DagRunViewer } from "@/components/workflow/dag-run-viewer"
import { CodeDisplay } from "@/components/code-editor"
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog"
import { RefreshCw, ArrowLeft, X, ExternalLink, Loader2 } from "lucide-react"

export default function RunDetailPage() {
  const params = useParams()
  const name = decodeURIComponent(params.name as string)
  const runID = params.runID as string

  const [run, setRun] = useState<WorkflowRun | null>(null)
  const [version, setVersion] = useState<WorkflowVersion | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Function code viewer
  const [codeViewFn, setCodeViewFn] = useState<string | null>(null)
  const [codeViewData, setCodeViewData] = useState<{ code: string; runtime: string } | null>(null)
  const [codeViewLoading, setCodeViewLoading] = useState(false)
  const [codeViewError, setCodeViewError] = useState<string | null>(null)

  const handleFunctionClick = useCallback(async (fnName: string) => {
    setCodeViewFn(fnName)
    setCodeViewData(null)
    setCodeViewError(null)
    setCodeViewLoading(true)
    try {
      const [fn, codeResp] = await Promise.all([
        functionsApi.get(fnName),
        functionsApi.getCode(fnName),
      ])
      setCodeViewData({
        code: codeResp.source_code || "// No source code available",
        runtime: fn.runtime,
      })
    } catch (err) {
      setCodeViewError(err instanceof Error ? err.message : "Failed to load source code")
    } finally {
      setCodeViewLoading(false)
    }
  }, [])

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const r = await workflowsApi.getRun(name, runID)
      setRun(r)
      // Fetch version for DAG edge structure (only once or when version changes)
      if (r.version && (!version || version.version !== r.version)) {
        try {
          const v = await workflowsApi.getVersion(name, r.version)
          setVersion(v)
        } catch {
          // Version fetch is optional — viewer degrades gracefully
        }
      }
    } catch (err) {
      console.error("Failed to fetch run:", err)
      setError(err instanceof Error ? err.message : "Failed to load run")
    } finally {
      setLoading(false)
    }
  }, [name, runID, version])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  // Auto-refresh while running
  useEffect(() => {
    if (!run || run.status !== "running") return
    const interval = setInterval(fetchData, 2000)
    return () => clearInterval(interval)
  }, [run, fetchData])

  const statusColor = (s: string): "default" | "secondary" | "destructive" | "outline" => {
    switch (s) {
      case "succeeded": return "default"
      case "running": return "secondary"
      case "failed": return "destructive"
      default: return "outline"
    }
  }

  const nodeStatusColor = (s: string): "default" | "secondary" | "destructive" | "outline" => {
    switch (s) {
      case "succeeded": return "default"
      case "running": return "secondary"
      case "failed": return "destructive"
      case "ready": return "outline"
      default: return "outline"
    }
  }

  if (error) {
    return (
      <DashboardLayout>
        <Header title={`Run: ${runID.substring(0, 8)}...`} />
        <div className="p-6">
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive">
            <p className="font-medium">Failed to load run</p>
            <p className="text-sm mt-1">{error}</p>
          </div>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header
        title={`Run: ${runID.substring(0, 8)}...`}
        description={run ? `Workflow: ${name} | Version: v${run.version}` : undefined}
      />

      <div className="p-6 space-y-6">
        <div className="flex items-center justify-between">
          <Link href={`/workflows/${encodeURIComponent(name)}`}>
            <Button variant="ghost" size="sm">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to {name}
            </Button>
          </Link>
          <Button variant="outline" onClick={fetchData} disabled={loading}>
            <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
            Refresh
          </Button>
        </div>

        {run && (
          <>
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-5">
              <div className="rounded-lg border border-border bg-card p-4">
                <p className="text-sm text-muted-foreground">Status</p>
                <Badge variant={statusColor(run.status)} className="mt-1">
                  {run.status}
                </Badge>
              </div>
              <div className="rounded-lg border border-border bg-card p-4">
                <p className="text-sm text-muted-foreground">Trigger</p>
                <p className="text-lg font-semibold">{run.trigger_type}</p>
              </div>
              <div className="rounded-lg border border-border bg-card p-4">
                <p className="text-sm text-muted-foreground">Nodes</p>
                <p className="text-lg font-semibold">{run.nodes?.length ?? 0}</p>
              </div>
              <div className="rounded-lg border border-border bg-card p-4">
                <p className="text-sm text-muted-foreground">Started</p>
                <p className="text-sm font-medium">{new Date(run.started_at).toLocaleString()}</p>
              </div>
              <div className="rounded-lg border border-border bg-card p-4">
                <p className="text-sm text-muted-foreground">Finished</p>
                <p className="text-sm font-medium">
                  {run.finished_at ? new Date(run.finished_at).toLocaleString() : "-"}
                </p>
              </div>
            </div>

            {run.error_message && (
              <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive">
                <p className="font-medium">Error</p>
                <p className="text-sm mt-1">{run.error_message}</p>
              </div>
            )}

            {version && (
              <div>
                <div className="mb-2">
                  <h3 className="font-medium text-foreground">DAG Visualization</h3>
                  {run.status === "running" && (
                    <p className="text-xs text-muted-foreground">Auto-refreshing every 2s</p>
                  )}
                </div>
                <DagRunViewer version={version} run={run} onFunctionClick={handleFunctionClick} />
              </div>
            )}

            <div className="rounded-lg border border-border bg-card">
              <div className="px-4 py-3 border-b border-border">
                <h3 className="font-medium text-foreground">Node Status</h3>
                {run.status === "running" && (
                  <p className="text-xs text-muted-foreground">Auto-refreshing every 2s</p>
                )}
              </div>
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Node</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Function</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Status</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Attempt</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Deps</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Started</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Finished</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Error</th>
                  </tr>
                </thead>
                <tbody>
                  {(!run.nodes || run.nodes.length === 0) ? (
                    <tr>
                      <td colSpan={8} className="px-4 py-8 text-center text-muted-foreground">
                        No nodes.
                      </td>
                    </tr>
                  ) : (
                    run.nodes.map((node: RunNode) => (
                      <tr key={node.id} className="border-b border-border last:border-0">
                        <td className="px-4 py-3 font-mono text-sm">{node.node_key}</td>
                        <td className="px-4 py-3 text-sm">{node.function_name}</td>
                        <td className="px-4 py-3">
                          <Badge variant={nodeStatusColor(node.status)}>
                            {node.status}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">{node.attempt}</td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">{node.unresolved_deps}</td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">
                          {node.started_at ? new Date(node.started_at).toLocaleTimeString() : "-"}
                        </td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">
                          {node.finished_at ? new Date(node.finished_at).toLocaleTimeString() : "-"}
                        </td>
                        <td className="px-4 py-3 text-sm text-destructive max-w-xs truncate">
                          {node.error_message || "-"}
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>

            {run.output && (
              <div className="rounded-lg border border-border bg-card p-4">
                <h3 className="font-medium text-foreground mb-2">Run Output</h3>
                <pre className="text-sm font-mono bg-muted p-3 rounded overflow-x-auto">
                  {typeof run.output === "string" ? run.output : JSON.stringify(run.output, null, 2)}
                </pre>
              </div>
            )}

            {run.input && (
              <div className="rounded-lg border border-border bg-card p-4">
                <h3 className="font-medium text-foreground mb-2">Run Input</h3>
                <pre className="text-sm font-mono bg-muted p-3 rounded overflow-x-auto">
                  {typeof run.input === "string" ? run.input : JSON.stringify(run.input, null, 2)}
                </pre>
              </div>
            )}
          </>
        )}

        {loading && !run && (
          <div className="text-center text-muted-foreground py-8">Loading...</div>
        )}

        {/* Function Code Viewer Dialog */}
        <Dialog open={!!codeViewFn} onOpenChange={(open) => { if (!open) setCodeViewFn(null) }}>
          <DialogContent className="max-w-3xl w-full max-h-[80vh] p-0 gap-0 flex flex-col">
            <div className="flex items-center justify-between px-4 py-3 border-b border-border shrink-0">
              <div className="flex items-center gap-2 min-w-0">
                <DialogTitle className="font-mono text-sm truncate">{codeViewFn}</DialogTitle>
                {codeViewData?.runtime && (
                  <Badge variant="secondary" className="shrink-0">{codeViewData.runtime}</Badge>
                )}
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <Link href={`/functions/${encodeURIComponent(codeViewFn || "")}`} target="_blank">
                  <Button variant="ghost" size="sm">
                    <ExternalLink className="h-3.5 w-3.5" />
                  </Button>
                </Link>
                <Button variant="ghost" size="sm" onClick={() => setCodeViewFn(null)}>
                  <X className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
            <div className="flex-1 min-h-0 overflow-auto">
              {codeViewLoading && (
                <div className="flex items-center justify-center py-12 text-muted-foreground">
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Loading source code...
                </div>
              )}
              {codeViewError && (
                <div className="px-4 py-8 text-center text-destructive text-sm">{codeViewError}</div>
              )}
              {codeViewData && (
                <CodeDisplay
                  code={codeViewData.code}
                  runtime={codeViewData.runtime}
                  maxHeight="calc(80vh - 56px)"
                  showLineNumbers
                />
              )}
            </div>
          </DialogContent>
        </Dialog>
      </div>
    </DashboardLayout>
  )
}
