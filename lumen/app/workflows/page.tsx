"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import Link from "next/link"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { EmptyState } from "@/components/empty-state"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { ErrorBanner } from "@/components/ui/error-banner"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { workflowsApi, type Workflow } from "@/lib/api"
import { toUserErrorMessage } from "@/lib/error-map"
import { Plus, RefreshCw, Trash2, GitBranch } from "lucide-react"

type Notice = {
  kind: "success" | "error" | "info"
  text: string
}

export default function WorkflowsPage() {
  const t = useTranslations("pages")
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<Notice | null>(null)
  const [isCreateOpen, setIsCreateOpen] = useState(false)
  const [createName, setCreateName] = useState("")
  const [createDesc, setCreateDesc] = useState("")
  const [creating, setCreating] = useState(false)
  const [pendingDeleteWorkflow, setPendingDeleteWorkflow] = useState<string | null>(null)

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const wfs = await workflowsApi.list()
      setWorkflows(wfs)
    } catch (err) {
      console.error("Failed to fetch workflows:", err)
      setError(toUserErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const handleCreate = async () => {
    if (!createName.trim()) return
    const workflowName = createName.trim()
    try {
      setCreating(true)
      await workflowsApi.create({ name: workflowName, description: createDesc.trim() })
      setIsCreateOpen(false)
      setCreateName("")
      setCreateDesc("")
      fetchData()
      setNotice({ kind: "success", text: `Workflow "${workflowName}" created` })
    } catch (err) {
      console.error("Failed to create workflow:", err)
      setNotice({ kind: "error", text: toUserErrorMessage(err) })
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (name: string) => {
    if (pendingDeleteWorkflow !== name) {
      setPendingDeleteWorkflow(name)
      setNotice({ kind: "info", text: `Click delete again to confirm workflow "${name}" deletion` })
      return
    }
    try {
      await workflowsApi.delete(name)
      fetchData()
      setPendingDeleteWorkflow(null)
      setNotice({ kind: "success", text: `Workflow "${name}" deleted` })
    } catch (err) {
      console.error("Failed to delete workflow:", err)
      setNotice({ kind: "error", text: toUserErrorMessage(err) })
    }
  }

  if (error) {
    return (
      <DashboardLayout>
        <Header title={t("workflows.title")} description={t("workflows.description")} />
        <div className="p-6">
          <ErrorBanner error={error} title="Failed to Load Workflows" onRetry={fetchData} />
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header title={t("workflows.title")} description={t("workflows.description")} />

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
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={fetchData} disabled={loading}>
              <RefreshCw className={`mr-2 h-4 w-4 ${loading ? "animate-spin" : ""}`} />
              Refresh
            </Button>
          </div>
          <Button onClick={() => setIsCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Create Workflow
          </Button>
        </div>

        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">Total Workflows</p>
            <p className="text-2xl font-semibold text-foreground">
              {loading ? "..." : workflows.length}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">Active</p>
            <p className="text-2xl font-semibold text-success">
              {loading ? "..." : workflows.filter((w) => w.status === "active").length}
            </p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">With Versions</p>
            <p className="text-2xl font-semibold text-foreground">
              {loading ? "..." : workflows.filter((w) => w.current_version > 0).length}
            </p>
          </div>
        </div>

        {!loading && workflows.length === 0 ? (
          <EmptyState
            title="No Workflows Yet"
            description="Create a workflow to orchestrate multiple functions as a DAG."
            icon={GitBranch}
            primaryAction={{ label: "Create Workflow", onClick: () => setIsCreateOpen(true) }}
          />
        ) : (
          <div className="rounded-lg border border-border bg-card">
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Name</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Description</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Status</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Version</th>
                    <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Updated</th>
                    <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {loading ? (
                    <tr>
                      <td colSpan={6} className="px-4 py-8 text-center text-muted-foreground">
                        Loading...
                      </td>
                    </tr>
                  ) : (
                    workflows.map((wf) => (
                    <tr key={wf.id} className="border-b border-border last:border-0 hover:bg-muted/50">
                      <td className="px-4 py-3">
                        <Link
                          href={`/workflows/${encodeURIComponent(wf.name)}`}
                          className="font-medium text-foreground hover:text-primary"
                        >
                          {wf.name}
                        </Link>
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground max-w-xs truncate">
                        {wf.description || "-"}
                      </td>
                      <td className="px-4 py-3">
                        <Badge variant={wf.status === "active" ? "default" : "secondary"}>
                          {wf.status}
                        </Badge>
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {wf.current_version > 0 ? `v${wf.current_version}` : "-"}
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {new Date(wf.updated_at).toLocaleDateString()}
                      </td>
                      <td className="px-4 py-3 text-right">
                        {pendingDeleteWorkflow === wf.name ? (
                          <div className="flex items-center justify-end gap-1">
                            <Button variant="destructive" size="sm" onClick={() => handleDelete(wf.name)}>
                              Confirm
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => {
                                setPendingDeleteWorkflow(null)
                                setNotice(null)
                              }}
                            >
                              Cancel
                            </Button>
                          </div>
                        ) : (
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => handleDelete(wf.name)}
                          >
                            <Trash2 className="h-4 w-4 text-destructive" />
                          </Button>
                        )}
                      </td>
                    </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        )}

        <Dialog open={isCreateOpen} onOpenChange={setIsCreateOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Create Workflow</DialogTitle>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="wf-name">Name</Label>
                <Input
                  id="wf-name"
                  placeholder="my-workflow"
                  value={createName}
                  onChange={(e) => setCreateName(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="wf-desc">Description</Label>
                <Textarea
                  id="wf-desc"
                  placeholder="What does this workflow do?"
                  value={createDesc}
                  onChange={(e) => setCreateDesc(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setIsCreateOpen(false)}>Cancel</Button>
              <Button onClick={handleCreate} disabled={creating || !createName.trim()}>
                {creating ? "Creating..." : "Create"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </DashboardLayout>
  )
}
