"use client"

import { useEffect, useState, useCallback } from "react"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Pagination } from "@/components/pagination"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { runtimesApi, CreateRuntimeRequest } from "@/lib/api"
import { transformRuntime, RuntimeInfo } from "@/lib/types"
import { RefreshCw, CheckCircle, AlertTriangle, Wrench, Plus, Trash2, X } from "lucide-react"
import { RuntimeIcon, getRuntimeColor } from "@/components/runtime-logos"
import { cn } from "@/lib/utils"

export default function RuntimesPage() {
  const [runtimes, setRuntimes] = useState<RuntimeInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(12)
  const [showCreateDialog, setShowCreateDialog] = useState(false)
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [formData, setFormData] = useState<CreateRuntimeRequest>({
    id: "",
    name: "",
    version: "",
    status: "available",
  })

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await runtimesApi.list()
      setRuntimes(data.map(transformRuntime))
    } catch (err) {
      console.error("Failed to fetch runtimes:", err)
      setError(err instanceof Error ? err.message : "Failed to load runtimes")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const totalPages = Math.max(1, Math.ceil(runtimes.length / pageSize))
  useEffect(() => {
    if (page > totalPages) setPage(totalPages)
  }, [page, totalPages])

  const pagedRuntimes = runtimes.slice((page - 1) * pageSize, page * pageSize)

  const handleCreate = async () => {
    if (!formData.id || !formData.name || !formData.version) return
    try {
      setCreating(true)
      await runtimesApi.create(formData)
      setShowCreateDialog(false)
      setFormData({ id: "", name: "", version: "", status: "available" })
      fetchData()
    } catch (err) {
      console.error("Failed to create runtime:", err)
      setError(err instanceof Error ? err.message : "Failed to create runtime")
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      setDeleting(true)
      await runtimesApi.delete(id)
      setDeleteConfirmId(null)
      fetchData()
    } catch (err) {
      console.error("Failed to delete runtime:", err)
      setError(err instanceof Error ? err.message : "Failed to delete runtime")
    } finally {
      setDeleting(false)
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case "available":
        return <CheckCircle className="h-4 w-4 text-success" />
      case "deprecated":
        return <AlertTriangle className="h-4 w-4 text-warning" />
      case "maintenance":
        return <Wrench className="h-4 w-4 text-muted-foreground" />
      default:
        return null
    }
  }

  if (error) {
    return (
      <DashboardLayout>
        <Header title="Runtimes" description="Available execution environments" />
        <div className="p-6">
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive">
            <p className="font-medium">Failed to load runtimes</p>
            <p className="text-sm mt-1">{error}</p>
            <Button variant="outline" size="sm" className="mt-2" onClick={() => { setError(null); fetchData(); }}>
              Retry
            </Button>
          </div>
        </div>
      </DashboardLayout>
    )
  }

  return (
    <DashboardLayout>
      <Header title="Runtimes" description="Available execution environments" />

      <div className="p-6 space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm text-muted-foreground">
              {loading ? "Loading..." : `${runtimes.length} runtimes available`}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={() => setShowCreateDialog(true)}>
              <Plus className="mr-2 h-4 w-4" />
              Add Runtime
            </Button>
            <Button variant="outline" onClick={fetchData} disabled={loading}>
              <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
              Refresh
            </Button>
          </div>
        </div>

        {/* Create Runtime Dialog */}
        {showCreateDialog && (
          <div className="rounded-xl border border-border bg-card p-6">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-card-foreground">Add Runtime</h3>
              <Button variant="ghost" size="sm" onClick={() => setShowCreateDialog(false)}>
                <X className="h-4 w-4" />
              </Button>
            </div>
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <div>
                <label className="text-xs font-medium text-muted-foreground">ID</label>
                <input
                  type="text"
                  placeholder="e.g. python3.13"
                  className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                  value={formData.id}
                  onChange={(e) => setFormData({ ...formData, id: e.target.value })}
                />
              </div>
              <div>
                <label className="text-xs font-medium text-muted-foreground">Name</label>
                <input
                  type="text"
                  placeholder="e.g. Python"
                  className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                />
              </div>
              <div>
                <label className="text-xs font-medium text-muted-foreground">Version</label>
                <input
                  type="text"
                  placeholder="e.g. 3.13"
                  className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                  value={formData.version}
                  onChange={(e) => setFormData({ ...formData, version: e.target.value })}
                />
              </div>
              <div className="flex items-end">
                <Button
                  onClick={handleCreate}
                  disabled={creating || !formData.id || !formData.name || !formData.version}
                  className="w-full"
                >
                  {creating ? "Creating..." : "Create"}
                </Button>
              </div>
            </div>
          </div>
        )}

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {loading
            ? Array.from({ length: 6 }).map((_, i) => (
                <div
                  key={i}
                  className="rounded-xl border border-border bg-card p-6 animate-pulse"
                >
                  <div className="flex items-start gap-4">
                    <div className="h-12 w-12 rounded-lg bg-muted" />
                    <div className="flex-1 space-y-2">
                      <div className="h-5 w-24 bg-muted rounded" />
                      <div className="h-4 w-16 bg-muted rounded" />
                    </div>
                  </div>
                </div>
              ))
            : pagedRuntimes.map((runtime) => {
                const bgColor = getRuntimeColor(runtime.id)

                return (
                  <div
                    key={runtime.id}
                    className="group rounded-xl border border-border bg-card p-6 transition-shadow hover:shadow-md"
                  >
                    <div className="flex items-start gap-4">
                      <div
                        className={cn(
                          "flex h-12 w-12 items-center justify-center rounded-lg",
                          bgColor,
                          runtime.id === "bun" ? "text-black" : "text-white"
                        )}
                      >
                        <RuntimeIcon runtimeId={runtime.id} className="text-2xl" />
                      </div>
                      <div className="flex-1">
                        <div className="flex items-center gap-2">
                          <h3 className="font-semibold text-card-foreground">
                            {runtime.name}
                          </h3>
                          {getStatusIcon(runtime.status)}
                        </div>
                        <p className="text-sm text-muted-foreground">
                          v{runtime.version}
                        </p>
                      </div>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-destructive"
                        onClick={() => setDeleteConfirmId(runtime.id)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>

                    {/* Delete confirmation */}
                    {deleteConfirmId === runtime.id && (
                      <div className="mt-4 flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/10 p-3">
                        <p className="text-xs text-destructive flex-1">Delete this runtime?</p>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setDeleteConfirmId(null)}
                          disabled={deleting}
                        >
                          Cancel
                        </Button>
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={() => handleDelete(runtime.id)}
                          disabled={deleting}
                        >
                          {deleting ? "Deleting..." : "Delete"}
                        </Button>
                      </div>
                    )}

                    {deleteConfirmId !== runtime.id && (
                      <div className="mt-4 flex items-center justify-between">
                        <Badge
                          variant="secondary"
                          className={cn(
                            "text-xs",
                            runtime.status === "available" &&
                              "bg-success/10 text-success border-0",
                            runtime.status === "deprecated" &&
                              "bg-warning/10 text-warning border-0",
                            runtime.status === "maintenance" &&
                              "bg-muted text-muted-foreground border-0"
                          )}
                        >
                          {runtime.status}
                        </Badge>
                        <span className="text-sm text-muted-foreground">
                          {runtime.functionsCount} function
                          {runtime.functionsCount !== 1 ? "s" : ""}
                        </span>
                      </div>
                    )}
                  </div>
                )
              })}
        </div>

        {!loading && runtimes.length > 0 && (
          <div className="rounded-xl border border-border bg-card p-4">
            <Pagination
              totalItems={runtimes.length}
              page={page}
              pageSize={pageSize}
              onPageChange={setPage}
              onPageSizeChange={(size) => {
                setPageSize(size)
                setPage(1)
              }}
              pageSizeOptions={[6, 12, 24, 48]}
              itemLabel="runtimes"
            />
          </div>
        )}
      </div>
    </DashboardLayout>
  )
}
