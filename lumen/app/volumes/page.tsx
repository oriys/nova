"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { volumesApi } from "@/lib/api"
import type { VolumeEntry } from "@/lib/api"
import { HardDrive, Plus, Trash2, RefreshCw } from "lucide-react"
import { cn } from "@/lib/utils"

export default function VolumesPage() {
  const t = useTranslations("pages")
  const tv = useTranslations("volumesPage")
  const tc = useTranslations("common")
  const [volumes, setVolumes] = useState<VolumeEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState("")
  const [newSizeMb, setNewSizeMb] = useState(64)
  const [newShared, setNewShared] = useState(false)
  const [newDescription, setNewDescription] = useState("")

  const fetchVolumes = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await volumesApi.list()
      setVolumes(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : tv("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [tv])

  useEffect(() => {
    fetchVolumes()
  }, [fetchVolumes])

  const handleCreate = async () => {
    if (!newName.trim() || newSizeMb <= 0) return
    try {
      setCreating(true)
      await volumesApi.create({
        name: newName.trim(),
        size_mb: newSizeMb,
        shared: newShared,
        description: newDescription.trim() || undefined,
      })
      setDialogOpen(false); setNewName(""); setNewSizeMb(64)
      setNewShared(false); setNewDescription(""); fetchVolumes()
    } catch (err) {
      setError(err instanceof Error ? err.message : tv("failedToCreate"))
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (name: string) => {
    try { await volumesApi.delete(name); fetchVolumes() }
    catch (err) { setError(err instanceof Error ? err.message : tv("failedToDelete")) }
  }

  return (
    <DashboardLayout>
      <Header title={t("volumes.title")} description={t("volumes.description")} />

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
                {tv("createVolume")}
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>{tv("createVolume")}</DialogTitle>
              </DialogHeader>
              <div className="space-y-4">
                <div className="space-y-2">
                  <label className="text-sm font-medium">{tv("name")}</label>
                  <Input
                    value={newName}
                    onChange={(e) => setNewName(e.target.value)}
                    placeholder={tv("namePlaceholder")}
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">{tv("sizeMb")}</label>
                  <Input
                    type="number"
                    value={newSizeMb}
                    onChange={(e) => setNewSizeMb(Number(e.target.value))}
                    min={1}
                  />
                </div>
                <div className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    id="shared"
                    checked={newShared}
                    onChange={(e) => setNewShared(e.target.checked)}
                    className="h-4 w-4 rounded border-border"
                  />
                  <label htmlFor="shared" className="text-sm font-medium">{tv("shared")}</label>
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">{tv("description")}</label>
                  <Textarea
                    value={newDescription}
                    onChange={(e) => setNewDescription(e.target.value)}
                    placeholder={tv("descriptionPlaceholder")}
                    className="min-h-[80px] text-sm"
                  />
                </div>
                <Button
                  className="w-full"
                  onClick={handleCreate}
                  disabled={creating || !newName.trim() || newSizeMb <= 0}
                >
                  {creating ? tv("creating") : tc("create")}
                </Button>
              </div>
            </DialogContent>
          </Dialog>

          <Button variant="outline" size="sm" onClick={fetchVolumes} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tv("colName")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tv("colSize")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tv("colShared")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tv("colDescription")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tv("colCreated")}</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{tv("colActions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <tr key={i} className="border-b border-border">
                    <td colSpan={6} className="px-4 py-3">
                      <div className="h-4 bg-muted rounded animate-pulse" />
                    </td>
                  </tr>
                ))
              ) : volumes.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-muted-foreground">
                    <HardDrive className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    {tv("noVolumes")}
                  </td>
                </tr>
              ) : (
                volumes.map((vol) => (
                  <tr key={vol.id} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <HardDrive className="h-4 w-4 text-muted-foreground" />
                        <span className="font-medium text-sm font-mono">{vol.name}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{vol.size_mb} MB</td>
                    <td className="px-4 py-3">
                      <span className={cn(
                        "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium",
                        vol.shared ? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400" : "bg-muted text-muted-foreground"
                      )}>
                        {vol.shared ? tv("yes") : tv("no")}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{vol.description || "—"}</td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {new Date(vol.created_at).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDelete(vol.name)}
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
