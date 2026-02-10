"use client"

import { useEffect, useState, useCallback } from "react"
import { useParams } from "next/navigation"
import Link from "next/link"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import {
  workflowsApi,
  functionsApi,
  type Workflow,
  type WorkflowVersion,
  type WorkflowRun,
  type WorkflowNode as WFNode,
  type WorkflowEdge as WFEdge,
  type PublishVersionRequest,
  type NodeDefinition,
  type EdgeDefinition,
} from "@/lib/api"
import { DagViewer } from "@/components/workflow/dag-viewer"
import { DagEditor } from "@/components/workflow/dag-editor"
import { CodeDisplay } from "@/components/code-editor"
import type { LayoutMap } from "@/components/workflow/dag-layout"
import { Play, RefreshCw, ArrowLeft, Pencil, X, ExternalLink, Loader2 } from "lucide-react"

type Notice = {
  kind: "success" | "error" | "info"
  text: string
}

export default function WorkflowDetailPage() {
  const params = useParams()
  const name = decodeURIComponent(params.name as string)

  const [workflow, setWorkflow] = useState<Workflow | null>(null)
  const [versions, setVersions] = useState<WorkflowVersion[]>([])
  const [runs, setRuns] = useState<WorkflowRun[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<Notice | null>(null)

  const [functionNames, setFunctionNames] = useState<string[]>([])
  const [publishing, setPublishing] = useState(false)

  const [isTriggerOpen, setIsTriggerOpen] = useState(false)
  const [triggerInput, setTriggerInput] = useState("{}")
  const [triggering, setTriggering] = useState(false)

  const [currentVersionDetail, setCurrentVersionDetail] = useState<WorkflowVersion | null>(null)

  // Graph tab: view vs edit mode
  const [graphEditing, setGraphEditing] = useState(false)
  const [editorInitialDef, setEditorInitialDef] = useState<(PublishVersionRequest & { layout?: LayoutMap }) | undefined>(undefined)
  const [editorKey, setEditorKey] = useState(0)

  // Function code viewer
  const [codeViewFn, setCodeViewFn] = useState<string | null>(null)
  const [codeViewData, setCodeViewData] = useState<{ code: string; runtime: string } | null>(null)
  const [codeViewLoading, setCodeViewLoading] = useState(false)
  const [codeViewError, setCodeViewError] = useState<string | null>(null)

  /** Convert a WorkflowVersion into the format DagEditor expects */
  function versionToEditorDef(v: WorkflowVersion): PublishVersionRequest & { layout?: LayoutMap } {
    const vNodes = v.nodes || []
    const vEdges = v.edges || []
    const nodeIdMap: Record<string, string> = {}
    for (const n of vNodes) nodeIdMap[n.id] = n.node_key

    const nodes: NodeDefinition[] = vNodes.map((n: WFNode) => ({
      node_key: n.node_key,
      function_name: n.function_name,
      timeout_s: n.timeout_s,
      ...(n.retry_policy ? { retry_policy: n.retry_policy } : {}),
      ...(n.input_mapping ? { input_mapping: n.input_mapping } : {}),
    }))

    const edges: EdgeDefinition[] = vEdges.map((e: WFEdge) => ({
      from: nodeIdMap[e.from_node_id] || e.from_node_id,
      to: nodeIdMap[e.to_node_id] || e.to_node_id,
    }))

    const layout = (v.definition as { layout?: LayoutMap })?.layout
    return { nodes, edges, ...(layout ? { layout } : {}) }
  }

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const [wf, vers, rns, fns] = await Promise.all([
        workflowsApi.get(name),
        workflowsApi.listVersions(name),
        workflowsApi.listRuns(name),
        functionsApi.list().catch(() => []),
      ])
      setWorkflow(wf)
      setVersions(vers)
      setRuns(rns)
      setFunctionNames(fns.map((f) => f.name))
      if (wf.current_version > 0) {
        try {
          const vd = await workflowsApi.getVersion(name, wf.current_version)
          setCurrentVersionDetail(vd)
        } catch {
          // non-critical
        }
      }
    } catch (err) {
      console.error("Failed to fetch workflow:", err)
      setError(err instanceof Error ? err.message : "Failed to load workflow")
    } finally {
      setLoading(false)
    }
  }, [name])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const handlePublish = async (def: PublishVersionRequest & { layout?: LayoutMap }) => {
    try {
      setPublishing(true)
      await workflowsApi.publishVersion(name, def)
      setGraphEditing(false)
      fetchData()
      setNotice({ kind: "success", text: "Workflow version published" })
    } catch (err) {
      console.error("Failed to publish version:", err)
      setNotice({ kind: "error", text: err instanceof Error ? err.message : "Failed to publish" })
    } finally {
      setPublishing(false)
    }
  }

  const handleTrigger = async () => {
    try {
      setTriggering(true)
      const input = JSON.parse(triggerInput)
      await workflowsApi.triggerRun(name, input)
      setIsTriggerOpen(false)
      fetchData()
      setNotice({ kind: "success", text: "Workflow run triggered" })
    } catch (err) {
      console.error("Failed to trigger run:", err)
      setNotice({ kind: "error", text: err instanceof Error ? err.message : "Failed to trigger" })
    } finally {
      setTriggering(false)
    }
  }

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

  /** Switch to edit mode, optionally loading current version */
  const enterEdit = useCallback((fromVersion?: WorkflowVersion) => {
    setEditorInitialDef(fromVersion ? versionToEditorDef(fromVersion) : undefined)
    setEditorKey((k) => k + 1)
    setGraphEditing(true)
  }, [])

  const exitEdit = useCallback(() => {
    setGraphEditing(false)
  }, [])

  if (error) {
    return (
      <DashboardLayout>
        <Header title={`Workflow: ${name}`} />
        <div className="p-6">
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive">
            <p className="font-medium">Failed to load workflow</p>
            <p className="text-sm mt-1">{error}</p>
          </div>
        </div>
      </DashboardLayout>
    )
  }

  const statusColor = (s: string) => {
    switch (s) {
      case "succeeded": return "default"
      case "running": return "secondary"
      case "failed": return "destructive"
      default: return "outline"
    }
  }

  return (
    <DashboardLayout>
      <Header title={`Workflow: ${name}`} description={workflow?.description} />

      <div className="p-6 space-y-6">
        {notice && (
          <div
            className={`rounded-lg border p-4 text-sm ${
              notice.kind === "success"
                ? "border-success/50 bg-success/10 text-success"
                : notice.kind === "error"
                  ? "border-destructive/50 bg-destructive/10 text-destructive"
                  : "border-primary/40 bg-primary/10 text-primary"
            }`}
          >
            <div className="flex items-center justify-between gap-3">
              <p>{notice.text}</p>
              <Button variant="ghost" size="sm" onClick={() => setNotice(null)}>
                Dismiss
              </Button>
            </div>
          </div>
        )}

        <div className="flex items-center justify-between">
          <Link href="/workflows">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Workflows
            </Button>
          </Link>
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={fetchData} disabled={loading}>
              <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
              Refresh
            </Button>
            <Button
              onClick={() => setIsTriggerOpen(true)}
              disabled={!workflow || workflow.current_version === 0}
            >
              <Play className="mr-2 h-4 w-4" />
              Trigger Run
            </Button>
          </div>
        </div>

        {workflow && (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            <div className="rounded-lg border border-border bg-card p-4">
              <p className="text-sm text-muted-foreground">Status</p>
              <Badge variant={workflow.status === "active" ? "default" : "secondary"}>
                {workflow.status}
              </Badge>
            </div>
            <div className="rounded-lg border border-border bg-card p-4">
              <p className="text-sm text-muted-foreground">Current Version</p>
              <p className="text-2xl font-semibold">
                {workflow.current_version > 0 ? `v${workflow.current_version}` : "-"}
              </p>
            </div>
            <div className="rounded-lg border border-border bg-card p-4">
              <p className="text-sm text-muted-foreground">Total Versions</p>
              <p className="text-2xl font-semibold">{versions.length}</p>
            </div>
            <div className="rounded-lg border border-border bg-card p-4">
              <p className="text-sm text-muted-foreground">Total Runs</p>
              <p className="text-2xl font-semibold">{runs.length}</p>
            </div>
          </div>
        )}

        <Tabs defaultValue="graph">
          <TabsList>
            <TabsTrigger value="graph">Graph</TabsTrigger>
            <TabsTrigger value="runs">Runs</TabsTrigger>
            <TabsTrigger value="versions">Versions</TabsTrigger>
          </TabsList>

          <TabsContent value="graph" className="mt-4">
            {graphEditing ? (
              /* ---- Edit mode ---- */
              <div className="rounded-lg border border-border bg-card overflow-hidden" style={{ height: 600 }}>
                <DagEditor
                  key={editorKey}
                  functions={functionNames}
                  initialDefinition={editorInitialDef}
                  onSave={handlePublish}
                  onCancel={exitEdit}
                  onFunctionClick={handleFunctionClick}
                  saving={publishing}
                />
              </div>
            ) : (
              /* ---- View mode ---- */
              <div className="space-y-2">
                <div className="flex items-center justify-end gap-2">
                  {currentVersionDetail && (
                    <Button variant="outline" size="sm" onClick={() => enterEdit(currentVersionDetail)}>
                      <Pencil className="mr-1.5 h-3.5 w-3.5" />
                      Edit
                    </Button>
                  )}
                  <Button variant="outline" size="sm" onClick={() => enterEdit()}>
                    <Pencil className="mr-1.5 h-3.5 w-3.5" />
                    New Version
                  </Button>
                </div>
                {currentVersionDetail ? (
                  <DagViewer version={currentVersionDetail} onFunctionClick={handleFunctionClick} />
                ) : (
                  <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-border bg-card text-muted-foreground" style={{ height: 400 }}>
                    <p>No published version yet.</p>
                    <Button variant="outline" size="sm" onClick={() => enterEdit()}>
                      <Pencil className="mr-1.5 h-3.5 w-3.5" />
                      Create First Version
                    </Button>
                  </div>
                )}
              </div>
            )}
          </TabsContent>

          <TabsContent value="runs" className="mt-4">
            <div className="rounded-lg border border-border bg-card">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Run ID</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Version</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Status</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Trigger</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Started</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Finished</th>
                  </tr>
                </thead>
                <tbody>
                  {runs.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="px-4 py-8 text-center text-muted-foreground">
                        No runs yet. Trigger a run to get started.
                      </td>
                    </tr>
                  ) : (
                    runs.map((run) => (
                      <tr key={run.id} className="border-b border-border last:border-0 hover:bg-muted/50">
                        <td className="px-4 py-3">
                          <Link
                            href={`/workflows/${encodeURIComponent(name)}/runs/${run.id}`}
                            className="font-mono text-sm text-foreground hover:text-primary"
                          >
                            {run.id.substring(0, 8)}...
                          </Link>
                        </td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">
                          v{run.version}
                        </td>
                        <td className="px-4 py-3">
                          <Badge variant={statusColor(run.status) as "default" | "secondary" | "destructive" | "outline"}>
                            {run.status}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">
                          {run.trigger_type}
                        </td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">
                          {new Date(run.started_at).toLocaleString()}
                        </td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">
                          {run.finished_at ? new Date(run.finished_at).toLocaleString() : "-"}
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </TabsContent>

          <TabsContent value="versions" className="mt-4">
            <div className="rounded-lg border border-border bg-card">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Version</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Created</th>
                  </tr>
                </thead>
                <tbody>
                  {versions.length === 0 ? (
                    <tr>
                      <td colSpan={2} className="px-4 py-8 text-center text-muted-foreground">
                        No versions published yet.
                      </td>
                    </tr>
                  ) : (
                    versions.map((v) => (
                      <tr key={v.id} className="border-b border-border last:border-0 hover:bg-muted/50">
                        <td className="px-4 py-3 font-medium">v{v.version}</td>
                        <td className="px-4 py-3 text-sm text-muted-foreground">
                          {new Date(v.created_at).toLocaleString()}
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </TabsContent>
        </Tabs>

        {/* Trigger Run Dialog */}
        <Dialog open={isTriggerOpen} onOpenChange={setIsTriggerOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Trigger Run</DialogTitle>
            </DialogHeader>
            <div className="py-4">
              <p className="text-sm text-muted-foreground mb-2">
                Input JSON to pass to root nodes:
              </p>
              <Textarea
                className="font-mono text-sm min-h-[150px]"
                value={triggerInput}
                onChange={(e) => setTriggerInput(e.target.value)}
              />
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setIsTriggerOpen(false)}>Cancel</Button>
              <Button onClick={handleTrigger} disabled={triggering}>
                {triggering ? "Triggering..." : "Trigger"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

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
