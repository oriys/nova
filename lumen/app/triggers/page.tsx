"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { triggersApi } from "@/lib/api"
import type { TriggerEntry, TriggerType } from "@/lib/api"
import { Zap, Plus, Trash2, RefreshCw } from "lucide-react"
import { cn } from "@/lib/utils"

const TRIGGER_TYPES: TriggerType[] = ["kafka", "rabbitmq", "redis", "filesystem", "webhook"]

export default function TriggersPage() {
  const t = useTranslations("pages")
  const tt = useTranslations("triggersPage")
  const tc = useTranslations("common")
  const [triggers, setTriggers] = useState<TriggerEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState("")
  const [newType, setNewType] = useState<TriggerType>(TRIGGER_TYPES[0])
  const [newFunctionName, setNewFunctionName] = useState("")
  const [newEnabled, setNewEnabled] = useState(true)

  const fetchTriggers = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await triggersApi.list()
      setTriggers(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [tt])

  useEffect(() => {
    fetchTriggers()
  }, [fetchTriggers])

  const handleCreate = async () => {
    if (!newName.trim() || !newFunctionName.trim()) return
    try {
      setCreating(true)
      await triggersApi.create({
        name: newName.trim(),
        type: newType,
        function_name: newFunctionName.trim(),
        enabled: newEnabled,
      })
      setDialogOpen(false)
      setNewName("")
      setNewType(TRIGGER_TYPES[0])
      setNewFunctionName("")
      setNewEnabled(true)
      fetchTriggers()
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToLoad"))
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm(tt("deleteConfirm"))) return
    try {
      await triggersApi.delete(id)
      fetchTriggers()
    } catch (err) {
      setError(err instanceof Error ? err.message : tt("failedToLoad"))
    }
  }

  return (
    <DashboardLayout>
      <Header title={t("triggers.title")} description={t("triggers.description")} />

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
                {tt("createTrigger")}
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>{tt("createTrigger")}</DialogTitle>
              </DialogHeader>
              <div className="space-y-4">
                <div className="space-y-2">
                  <label className="text-sm font-medium">{tt("colName")}</label>
                  <Input
                    value={newName}
                    onChange={(e) => setNewName(e.target.value)}
                    placeholder="my-trigger"
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">{tt("colType")}</label>
                  <select
                    value={newType}
                    onChange={(e) => setNewType(e.target.value as TriggerType)}
                    className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                  >
                    {TRIGGER_TYPES.map((type) => (
                      <option key={type} value={type}>{type}</option>
                    ))}
                  </select>
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">{tt("colFunction")}</label>
                  <Input
                    value={newFunctionName}
                    onChange={(e) => setNewFunctionName(e.target.value)}
                    placeholder="my-function"
                  />
                </div>
                <div className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    id="enabled"
                    checked={newEnabled}
                    onChange={(e) => setNewEnabled(e.target.checked)}
                    className="h-4 w-4 rounded border-border"
                  />
                  <label htmlFor="enabled" className="text-sm font-medium">{tt("colEnabled")}</label>
                </div>
                <Button
                  className="w-full"
                  onClick={handleCreate}
                  disabled={creating || !newName.trim() || !newFunctionName.trim()}
                >
                  {creating ? tc("loading") : tc("create")}
                </Button>
              </div>
            </DialogContent>
          </Dialog>

          <Button variant="outline" size="sm" onClick={fetchTriggers} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colName")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colType")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colFunction")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colEnabled")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tt("colCreated")}</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{tt("colActions")}</th>
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
              ) : triggers.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-muted-foreground">
                    <Zap className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    {tt("noTriggers")}
                  </td>
                </tr>
              ) : (
                triggers.map((trigger) => (
                  <tr key={trigger.id} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <Zap className="h-4 w-4 text-muted-foreground" />
                        <span className="font-medium text-sm font-mono">{trigger.name}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <span className="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium bg-muted text-muted-foreground">
                        {trigger.type}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">{trigger.function_name}</td>
                    <td className="px-4 py-3">
                      <span className={cn(
                        "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium",
                        trigger.enabled ? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400" : "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400"
                      )}>
                        {trigger.enabled ? "On" : "Off"}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {new Date(trigger.created_at).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDelete(trigger.id)}
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
