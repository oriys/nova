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
  type Workflow,
  type WorkflowVersion,
  type WorkflowRun,
} from "@/lib/api"
import { Play, RefreshCw, Plus, ArrowLeft } from "lucide-react"

export default function WorkflowDetailPage() {
  const params = useParams()
  const name = decodeURIComponent(params.name as string)

  const [workflow, setWorkflow] = useState<Workflow | null>(null)
  const [versions, setVersions] = useState<WorkflowVersion[]>([])
  const [runs, setRuns] = useState<WorkflowRun[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [isPublishOpen, setIsPublishOpen] = useState(false)
  const [publishJSON, setPublishJSON] = useState(
    JSON.stringify(
      {
        nodes: [
          { node_key: "step1", function_name: "my-function", timeout_s: 30 },
        ],
        edges: [],
      },
      null,
      2
    )
  )
  const [publishing, setPublishing] = useState(false)

  const [isTriggerOpen, setIsTriggerOpen] = useState(false)
  const [triggerInput, setTriggerInput] = useState("{}")
  const [triggering, setTriggering] = useState(false)

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const [wf, vers, rns] = await Promise.all([
        workflowsApi.get(name),
        workflowsApi.listVersions(name),
        workflowsApi.listRuns(name),
      ])
      setWorkflow(wf)
      setVersions(vers)
      setRuns(rns)
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

  const handlePublish = async () => {
    try {
      setPublishing(true)
      const def = JSON.parse(publishJSON)
      await workflowsApi.publishVersion(name, def)
      setIsPublishOpen(false)
      fetchData()
    } catch (err) {
      console.error("Failed to publish version:", err)
      alert(err instanceof Error ? err.message : "Failed to publish")
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
    } catch (err) {
      console.error("Failed to trigger run:", err)
      alert(err instanceof Error ? err.message : "Failed to trigger")
    } finally {
      setTriggering(false)
    }
  }

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
            <Button variant="outline" onClick={() => setIsPublishOpen(true)}>
              <Plus className="mr-2 h-4 w-4" />
              Publish Version
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

        <Tabs defaultValue="runs">
          <TabsList>
            <TabsTrigger value="runs">Runs</TabsTrigger>
            <TabsTrigger value="versions">Versions</TabsTrigger>
          </TabsList>

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

        {/* Publish Version Dialog */}
        <Dialog open={isPublishOpen} onOpenChange={setIsPublishOpen}>
          <DialogContent className="max-w-2xl">
            <DialogHeader>
              <DialogTitle>Publish New Version</DialogTitle>
            </DialogHeader>
            <div className="py-4">
              <p className="text-sm text-muted-foreground mb-2">
                Define nodes and edges as JSON. Each node must have a node_key and function_name.
              </p>
              <Textarea
                className="font-mono text-sm min-h-[300px]"
                value={publishJSON}
                onChange={(e) => setPublishJSON(e.target.value)}
              />
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setIsPublishOpen(false)}>Cancel</Button>
              <Button onClick={handlePublish} disabled={publishing}>
                {publishing ? "Publishing..." : "Publish"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

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
      </div>
    </DashboardLayout>
  )
}
