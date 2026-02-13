"use client"

import { useEffect, useState, useCallback } from "react"
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
import { layersApi } from "@/lib/api"
import type { LayerEntry, CreateLayerRequest } from "@/lib/api"
import { Layers, Plus, Trash2, RefreshCw } from "lucide-react"
import { cn } from "@/lib/utils"

export default function LayersPage() {
  const t = useTranslations("pages")
  const tl = useTranslations("layersPage")
  const tc = useTranslations("common")
  const [layers, setLayers] = useState<LayerEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [newLayerName, setNewLayerName] = useState("")
  const [newLayerRuntime, setNewLayerRuntime] = useState("")
  const [newLayerVersion, setNewLayerVersion] = useState("")
  const [creating, setCreating] = useState(false)

  const fetchLayers = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await layersApi.list()
      setLayers(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : tl("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [tl])

  useEffect(() => {
    fetchLayers()
  }, [fetchLayers])

  const handleCreate = async () => {
    if (!newLayerName.trim() || !newLayerRuntime.trim()) return
    try {
      setCreating(true)
      await layersApi.create({
        name: newLayerName.trim(),
        runtime: newLayerRuntime.trim(),
        files: {},
        ...(newLayerVersion.trim() ? { version: newLayerVersion.trim() } : {}),
      })
      setDialogOpen(false)
      setNewLayerName("")
      setNewLayerRuntime("")
      setNewLayerVersion("")
      fetchLayers()
    } catch (err) {
      setError(err instanceof Error ? err.message : tl("failedToCreate"))
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (name: string) => {
    try {
      await layersApi.delete(name)
      fetchLayers()
    } catch (err) {
      setError(err instanceof Error ? err.message : tl("failedToDelete"))
    }
  }

  return (
    <DashboardLayout>
      <Header title={t("layers.title")} description={t("layers.description")} />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
          </div>
        )}

        <div className="flex items-center justify-between">
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
                  <Input
                    value={newLayerRuntime}
                    onChange={(e) => setNewLayerRuntime(e.target.value)}
                    placeholder={tl("runtimePlaceholder")}
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">{tl("version")}</label>
                  <Input
                    value={newLayerVersion}
                    onChange={(e) => setNewLayerVersion(e.target.value)}
                    placeholder={tl("versionPlaceholder")}
                  />
                </div>
                <Button
                  className="w-full"
                  onClick={handleCreate}
                  disabled={creating || !newLayerName.trim() || !newLayerRuntime.trim()}
                >
                  {creating ? tl("creating") : tc("create")}
                </Button>
              </div>
            </DialogContent>
          </Dialog>

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
              ) : layers.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-muted-foreground">
                    <Layers className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    {tl("noLayers")}
                  </td>
                </tr>
              ) : (
                layers.map((layer) => (
                  <tr key={layer.id} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <Layers className="h-4 w-4 text-muted-foreground" />
                        <span className="font-medium text-sm font-mono">{layer.name}</span>
                      </div>
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
        </div>
      </div>
    </DashboardLayout>
  )
}
