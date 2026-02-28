"use client"

import { useEffect, useState, useCallback, useRef } from "react"
import Link from "next/link"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Pagination } from "@/components/pagination"
import { layersApi, runtimesApi } from "@/lib/api"
import type { LayerEntry } from "@/lib/api"
import { Layers, Plus, Trash2, RefreshCw, Search, Upload, File } from "lucide-react"
import { cn } from "@/lib/utils"

export default function LayersPage() {
  const t = useTranslations("pages")
  const tl = useTranslations("layersPage")
  const tc = useTranslations("common")
  const [layers, setLayers] = useState<LayerEntry[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [newLayerName, setNewLayerName] = useState("")
  const [newLayerRuntime, setNewLayerRuntime] = useState("")
  const [newLayerVersion, setNewLayerVersion] = useState("")
  const [uploadedFiles, setUploadedFiles] = useState<Map<string, string>>(new Map())
  const [creating, setCreating] = useState(false)
  const [search, setSearch] = useState("")
  const [runtimeFilter, setRuntimeFilter] = useState("all")
  const [runtimes, setRuntimes] = useState<string[]>([])
  const fileInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    runtimesApi.list().then((items) => {
      const names = (items || []).map((r) => r.name).filter(Boolean)
      setRuntimes([...new Set(names)])
    }).catch(() => {})
  }, [])

  const fetchLayers = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const offset = (page - 1) * pageSize
      const result = await layersApi.listPage(pageSize, offset)
      setLayers(result.items || [])
      setTotal(result.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : tl("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [tl, page, pageSize])

  useEffect(() => {
    fetchLayers()
  }, [fetchLayers])

  const handleFileSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files
    if (!files) return
    const newFiles = new Map(uploadedFiles)
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      const buffer = await file.arrayBuffer()
      const base64 = btoa(String.fromCharCode(...new Uint8Array(buffer)))
      newFiles.set(file.name, base64)
    }
    setUploadedFiles(newFiles)
    if (fileInputRef.current) fileInputRef.current.value = ""
  }

  const handleCreate = async () => {
    if (!newLayerName.trim() || !newLayerRuntime.trim()) return
    if (uploadedFiles.size === 0) return
    try {
      setCreating(true)
      const filesObj: Record<string, string> = {}
      uploadedFiles.forEach((v, k) => { filesObj[k] = v })
      await layersApi.create({
        name: newLayerName.trim(),
        runtime: newLayerRuntime.trim(),
        files: filesObj,
        ...(newLayerVersion.trim() ? { version: newLayerVersion.trim() } : {}),
      })
      setDialogOpen(false)
      setNewLayerName("")
      setNewLayerRuntime("")
      setNewLayerVersion("")
      setUploadedFiles(new Map())
      setPage(1)
      fetchLayers()
    } catch (err) {
      setError(err instanceof Error ? err.message : tl("failedToCreate"))
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (name: string) => {
    if (!confirm(tl("deleteConfirmDesc", { name }))) return
    try {
      await layersApi.delete(name)
      fetchLayers()
    } catch (err) {
      setError(err instanceof Error ? err.message : tl("failedToDelete"))
    }
  }

  const filteredLayers = layers.filter((layer) => {
    const matchesSearch = !search || layer.name.toLowerCase().includes(search.toLowerCase())
    const matchesRuntime = runtimeFilter === "all" || layer.runtime === runtimeFilter
    return matchesSearch && matchesRuntime
  })

  return (
    <DashboardLayout>
      <Header title={t("layers.title")} description={t("layers.description")} />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
          </div>
        )}

        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
              <DialogTrigger asChild>
                <Button size="sm">
                  <Plus className="mr-2 h-4 w-4" />
                  {tl("createLayer")}
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>{tl("createLayer")}</DialogTitle>
                </DialogHeader>
                <div className="space-y-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">{tl("name")}</label>
                    <Input
                      value={newLayerName}
                      onChange={(e) => setNewLayerName(e.target.value)}
                      placeholder={tl("namePlaceholder")}
                    />
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">{tl("runtime")}</label>
                    {runtimes.length > 0 ? (
                      <Select value={newLayerRuntime} onValueChange={setNewLayerRuntime}>
                        <SelectTrigger>
                          <SelectValue placeholder={tl("runtimeSelect")} />
                        </SelectTrigger>
                        <SelectContent>
                          {runtimes.map((r) => (
                            <SelectItem key={r} value={r}>{r}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    ) : (
                      <Input
                        value={newLayerRuntime}
                        onChange={(e) => setNewLayerRuntime(e.target.value)}
                        placeholder={tl("runtimePlaceholder")}
                      />
                    )}
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">{tl("version")}</label>
                    <Input
                      value={newLayerVersion}
                      onChange={(e) => setNewLayerVersion(e.target.value)}
                      placeholder={tl("versionPlaceholder")}
                    />
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">{tl("uploadFiles")}</label>
                    <p className="text-xs text-muted-foreground">{tl("uploadFilesDesc")}</p>
                    <input
                      ref={fileInputRef}
                      type="file"
                      multiple
                      onChange={handleFileSelect}
                      className="hidden"
                    />
                    <Button
                      type="button"
                      variant="outline"
                      className="w-full"
                      onClick={() => fileInputRef.current?.click()}
                    >
                      <Upload className="mr-2 h-4 w-4" />
                      {tl("dropzone")}
                    </Button>
                    {uploadedFiles.size > 0 ? (
                      <div className="rounded-lg border border-border p-3 space-y-1 max-h-32 overflow-y-auto">
                        {[...uploadedFiles.keys()].map((name) => (
                          <div key={name} className="flex items-center gap-2 text-xs">
                            <File className="h-3 w-3 text-muted-foreground shrink-0" />
                            <span className="font-mono truncate">{name}</span>
                            <Button
                              type="button"
                              variant="ghost"
                              size="sm"
                              className="h-5 w-5 p-0 ml-auto shrink-0"
                              onClick={() => {
                                const next = new Map(uploadedFiles)
                                next.delete(name)
                                setUploadedFiles(next)
                              }}
                            >
                              <Trash2 className="h-3 w-3 text-destructive" />
                            </Button>
                          </div>
                        ))}
                        <p className="text-xs text-muted-foreground pt-1">
                          {tl("selectedFiles", { count: uploadedFiles.size })}
                        </p>
                      </div>
                    ) : (
                      <p className="text-xs text-muted-foreground">{tl("noFiles")}</p>
                    )}
                  </div>
                  <Button
                    className="w-full"
                    onClick={handleCreate}
                    disabled={creating || !newLayerName.trim() || !newLayerRuntime.trim() || uploadedFiles.size === 0}
                  >
                    {creating ? tl("creating") : tc("create")}
                  </Button>
                </div>
              </DialogContent>
            </Dialog>

            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder={tl("searchPlaceholder")}
                className="pl-9 w-64"
              />
            </div>

            <Select value={runtimeFilter} onValueChange={setRuntimeFilter}>
              <SelectTrigger className="w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{tl("filterRuntime")}</SelectItem>
                {runtimes.map((r) => (
                  <SelectItem key={r} value={r}>{r}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <Button variant="outline" size="sm" onClick={fetchLayers} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tl("colName")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tl("colRuntime")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tl("colVersion")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tl("colSize")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tl("colFiles")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tl("colCreated")}</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{tl("colActions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <tr key={i} className="border-b border-border">
                    <td colSpan={7} className="px-4 py-3">
                      <div className="h-4 bg-muted rounded animate-pulse" />
                    </td>
                  </tr>
                ))
              ) : filteredLayers.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-muted-foreground">
                    <Layers className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    {tl("noLayers")}
                  </td>
                </tr>
              ) : (
                filteredLayers.map((layer) => (
                  <tr key={layer.id} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3">
                      <Link
                        href={`/layers/${encodeURIComponent(layer.name)}`}
                        className="flex items-center gap-2 hover:underline"
                      >
                        <Layers className="h-4 w-4 text-muted-foreground" />
                        <span className="font-medium text-sm font-mono">{layer.name}</span>
                      </Link>
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{layer.runtime}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{layer.version}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{layer.size_mb} MB</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {tl("filesCount", { count: layer.files?.length ?? 0 })}
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {new Date(layer.created_at).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDelete(layer.name)}
                      >
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
          {total > pageSize && (
            <div className="border-t border-border p-4">
              <Pagination
                totalItems={total}
                page={page}
                pageSize={pageSize}
                onPageChange={setPage}
                onPageSizeChange={(size) => { setPageSize(size); setPage(1) }}
                itemLabel="layers"
              />
            </div>
          )}
        </div>
      </div>
    </DashboardLayout>
  )
}
