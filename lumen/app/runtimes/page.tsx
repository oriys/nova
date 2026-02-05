"use client"

import { useEffect, useState, useCallback, useRef } from "react"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Pagination } from "@/components/pagination"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { runtimesApi, CreateRuntimeRequest, UploadRuntimeRequest } from "@/lib/api"
import { transformRuntime, RuntimeInfo } from "@/lib/types"
import { RefreshCw, CheckCircle, AlertTriangle, Wrench, Plus, Trash2, X, Upload, FileText } from "lucide-react"
import { RuntimeIcon, getRuntimeColor } from "@/components/runtime-logos"
import { cn } from "@/lib/utils"

type CreateMode = "reference" | "upload"

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return bytes + " B"
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB"
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + " MB"
  return (bytes / (1024 * 1024 * 1024)).toFixed(2) + " GB"
}

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
  const [createMode, setCreateMode] = useState<CreateMode>("reference")
  const [uploadFile, setUploadFile] = useState<File | null>(null)
  const [uploadProgress, setUploadProgress] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [formData, setFormData] = useState<CreateRuntimeRequest>({
    id: "",
    name: "",
    version: "",
    status: "available",
    image_name: "",
    entrypoint: [],
    file_extension: "",
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

  const resetForm = () => {
    setFormData({ id: "", name: "", version: "", status: "available", image_name: "", entrypoint: [], file_extension: "" })
    setUploadFile(null)
    setUploadProgress(null)
    setCreateMode("reference")
  }

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return

    if (!file.name.endsWith(".ext4")) {
      setError("File must have .ext4 extension")
      return
    }

    const maxSize = 2 * 1024 * 1024 * 1024 // 2GB
    if (file.size > maxSize) {
      setError("File size must be less than 2GB")
      return
    }

    setUploadFile(file)
    // Auto-fill ID from filename (without extension)
    const baseName = file.name.replace(/\.ext4$/i, "").replace(/[^a-zA-Z0-9_-]/g, "")
    if (!formData.id) {
      setFormData(prev => ({ ...prev, id: baseName }))
    }
  }

  const handleCreate = async () => {
    try {
      setCreating(true)
      setError(null)

      if (createMode === "upload") {
        if (!uploadFile) {
          setError("Please select a file to upload")
          return
        }
        if (!formData.id || !formData.name || !formData.entrypoint.length || !formData.file_extension) {
          setError("Please fill in all required fields")
          return
        }

        setUploadProgress("Uploading...")
        const metadata: UploadRuntimeRequest = {
          id: formData.id,
          name: formData.name,
          version: formData.version || undefined,
          entrypoint: formData.entrypoint,
          file_extension: formData.file_extension,
        }
        await runtimesApi.upload(uploadFile, metadata)
        setUploadProgress(null)
      } else {
        if (!formData.id || !formData.name || !formData.version || !formData.image_name || !formData.entrypoint.length || !formData.file_extension) {
          setError("Please fill in all required fields")
          return
        }
        await runtimesApi.create(formData)
      }

      setShowCreateDialog(false)
      resetForm()
      fetchData()
    } catch (err) {
      console.error("Failed to create runtime:", err)
      setError(err instanceof Error ? err.message : "Failed to create runtime")
      setUploadProgress(null)
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

  const isFormValid = createMode === "upload"
    ? uploadFile && formData.id && formData.name && formData.entrypoint.length > 0 && formData.file_extension
    : formData.id && formData.name && formData.version && formData.image_name && formData.entrypoint.length > 0 && formData.file_extension

  if (error && !showCreateDialog) {
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
              <h3 className="text-sm font-semibold text-card-foreground">Add Custom Runtime</h3>
              <Button variant="ghost" size="sm" onClick={() => { setShowCreateDialog(false); resetForm(); }}>
                <X className="h-4 w-4" />
              </Button>
            </div>

            {/* Mode toggle */}
            <div className="flex gap-2 mb-4">
              <Button
                variant={createMode === "reference" ? "default" : "outline"}
                size="sm"
                onClick={() => setCreateMode("reference")}
              >
                <FileText className="mr-2 h-4 w-4" />
                Reference Existing
              </Button>
              <Button
                variant={createMode === "upload" ? "default" : "outline"}
                size="sm"
                onClick={() => setCreateMode("upload")}
              >
                <Upload className="mr-2 h-4 w-4" />
                Upload New Image
              </Button>
            </div>

            {/* Error display */}
            {error && (
              <div className="mb-4 rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
                {error}
              </div>
            )}

            {/* Upload mode: file picker */}
            {createMode === "upload" && (
              <div className="mb-4">
                <label className="text-xs font-medium text-muted-foreground">Rootfs Image (.ext4) *</label>
                <div
                  className={cn(
                    "mt-1 border-2 border-dashed rounded-lg p-4 text-center cursor-pointer transition-colors",
                    uploadFile ? "border-primary bg-primary/5" : "border-border hover:border-muted-foreground"
                  )}
                  onClick={() => fileInputRef.current?.click()}
                >
                  <input
                    ref={fileInputRef}
                    type="file"
                    accept=".ext4"
                    className="hidden"
                    onChange={handleFileChange}
                  />
                  {uploadFile ? (
                    <div className="flex items-center justify-center gap-2">
                      <FileText className="h-5 w-5 text-primary" />
                      <span className="text-sm font-medium">{uploadFile.name}</span>
                      <span className="text-xs text-muted-foreground">({formatFileSize(uploadFile.size)})</span>
                    </div>
                  ) : (
                    <div>
                      <Upload className="mx-auto h-8 w-8 text-muted-foreground mb-2" />
                      <p className="text-sm text-muted-foreground">Click to select .ext4 file (max 2GB)</p>
                    </div>
                  )}
                </div>
                {uploadProgress && (
                  <p className="mt-2 text-sm text-muted-foreground">{uploadProgress}</p>
                )}
              </div>
            )}

            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              <div>
                <label className="text-xs font-medium text-muted-foreground">ID *</label>
                <input
                  type="text"
                  placeholder="e.g. bash-custom"
                  className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                  value={formData.id}
                  onChange={(e) => setFormData({ ...formData, id: e.target.value })}
                />
              </div>
              <div>
                <label className="text-xs font-medium text-muted-foreground">Name *</label>
                <input
                  type="text"
                  placeholder="e.g. Bash"
                  className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                />
              </div>
              <div>
                <label className="text-xs font-medium text-muted-foreground">
                  Version {createMode === "reference" ? "*" : "(optional)"}
                </label>
                <input
                  type="text"
                  placeholder="e.g. 5.2"
                  className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                  value={formData.version}
                  onChange={(e) => setFormData({ ...formData, version: e.target.value })}
                />
              </div>
              {createMode === "reference" && (
                <div>
                  <label className="text-xs font-medium text-muted-foreground">Image Name *</label>
                  <input
                    type="text"
                    placeholder="e.g. bash.ext4"
                    className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                    value={formData.image_name}
                    onChange={(e) => setFormData({ ...formData, image_name: e.target.value })}
                  />
                </div>
              )}
              <div>
                <label className="text-xs font-medium text-muted-foreground">Entrypoint * (comma-separated)</label>
                <input
                  type="text"
                  placeholder="e.g. /bin/bash"
                  className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                  value={formData.entrypoint.join(", ")}
                  onChange={(e) => setFormData({ ...formData, entrypoint: e.target.value.split(",").map(s => s.trim()).filter(Boolean) })}
                />
              </div>
              <div>
                <label className="text-xs font-medium text-muted-foreground">File Extension *</label>
                <input
                  type="text"
                  placeholder="e.g. .sh"
                  className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                  value={formData.file_extension}
                  onChange={(e) => setFormData({ ...formData, file_extension: e.target.value })}
                />
              </div>
            </div>
            <div className="flex justify-end mt-4">
              <Button
                onClick={handleCreate}
                disabled={creating || !isFormValid}
              >
                {creating ? (uploadProgress || "Creating...") : (createMode === "upload" ? "Upload & Create" : "Create Runtime")}
              </Button>
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
                        {runtime.imageName && (
                          <p className="text-xs text-muted-foreground mt-1">
                            {runtime.imageName}
                          </p>
                        )}
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
