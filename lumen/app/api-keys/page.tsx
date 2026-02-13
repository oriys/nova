"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { apiKeysApi } from "@/lib/api"
import type { APIKeyEntry } from "@/lib/api"
import {
  Plus,
  Trash2,
  RefreshCw,
  Copy,
  Check,
  KeyRound,
  ToggleLeft,
  ToggleRight,
} from "lucide-react"
import { cn } from "@/lib/utils"

export default function APIKeysPage() {
  const t = useTranslations("pages")
  const tk = useTranslations("apiKeysPage")
  const tc = useTranslations("common")
  const [keys, setKeys] = useState<APIKeyEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [newKeyName, setNewKeyName] = useState("")
  const [newKeyTier, setNewKeyTier] = useState("default")
  const [createdKey, setCreatedKey] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [creating, setCreating] = useState(false)

  const fetchKeys = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await apiKeysApi.list()
      setKeys(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : tk("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [tk])

  useEffect(() => {
    fetchKeys()
  }, [fetchKeys])

  const handleCreate = async () => {
    if (!newKeyName.trim()) return
    try {
      setCreating(true)
      const result = await apiKeysApi.create(newKeyName.trim(), newKeyTier)
      setCreatedKey(result.key)
      setNewKeyName("")
      setNewKeyTier("default")
      fetchKeys()
    } catch (err) {
      setError(err instanceof Error ? err.message : tk("failedToCreate"))
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (name: string) => {
    try {
      await apiKeysApi.delete(name)
      fetchKeys()
    } catch (err) {
      setError(err instanceof Error ? err.message : tk("failedToDelete"))
    }
  }

  const handleToggle = async (name: string, currentEnabled: boolean) => {
    try {
      await apiKeysApi.toggle(name, !currentEnabled)
      fetchKeys()
    } catch (err) {
      setError(err instanceof Error ? err.message : tk("failedToToggle"))
    }
  }

  const handleCopy = async (key: string) => {
    await navigator.clipboard.writeText(key)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <DashboardLayout>
      <Header title={t("apiKeys.title")} description={t("apiKeys.description")} />

      <div className="p-6 space-y-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-destructive text-sm">
            {error}
          </div>
        )}

        <div className="flex items-center justify-between">
          <Dialog
            open={dialogOpen}
            onOpenChange={(open) => {
              setDialogOpen(open)
              if (!open) {
                setCreatedKey(null)
                setCopied(false)
              }
            }}
          >
            <DialogTrigger asChild>
              <Button size="sm">
                <Plus className="mr-2 h-4 w-4" />
                {tk("createApiKey")}
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>{tk("createApiKey")}</DialogTitle>
              </DialogHeader>
              {createdKey ? (
                <div className="space-y-4">
                  <p className="text-sm text-muted-foreground">
                    {tk("copyKeyNow")}
                  </p>
                  <div className="flex items-center gap-2">
                    <code className="flex-1 rounded-md border bg-muted p-3 text-sm font-mono break-all">
                      {createdKey}
                    </code>
                    <Button
                      variant="outline"
                      size="icon"
                      onClick={() => handleCopy(createdKey)}
                    >
                      {copied ? (
                        <Check className="h-4 w-4 text-success" />
                      ) : (
                        <Copy className="h-4 w-4" />
                      )}
                    </Button>
                  </div>
                  <Button
                    className="w-full"
                    onClick={() => {
                      setDialogOpen(false)
                      setCreatedKey(null)
                    }}
                  >
                    {tk("done")}
                  </Button>
                </div>
              ) : (
                <div className="space-y-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">{tk("name")}</label>
                    <Input
                      value={newKeyName}
                      onChange={(e) => setNewKeyName(e.target.value)}
                      placeholder={tk("namePlaceholder")}
                    />
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">{tk("tier")}</label>
                    <Select value={newKeyTier} onValueChange={setNewKeyTier}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="default">{tk("tierDefault")}</SelectItem>
                        <SelectItem value="premium">{tk("tierPremium")}</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <Button
                    className="w-full"
                    onClick={handleCreate}
                    disabled={creating || !newKeyName.trim()}
                  >
                    {creating ? tk("creating") : tc("create")}
                  </Button>
                </div>
              )}
            </DialogContent>
          </Dialog>

          <Button variant="outline" size="sm" onClick={fetchKeys} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tk("colName")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tk("colTier")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tk("colStatus")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tk("colCreated")}</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{tk("colActions")}</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <tr key={i} className="border-b border-border">
                    <td colSpan={5} className="px-4 py-3">
                      <div className="h-4 bg-muted rounded animate-pulse" />
                    </td>
                  </tr>
                ))
              ) : keys.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-muted-foreground">
                    <KeyRound className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    {tk("noKeys")}
                  </td>
                </tr>
              ) : (
                keys.map((key) => (
                  <tr key={key.name} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3">
                      <span className="font-medium text-sm">{key.name}</span>
                    </td>
                    <td className="px-4 py-3">
                      <Badge variant="secondary" className="text-xs">
                        {key.tier}
                      </Badge>
                    </td>
                    <td className="px-4 py-3">
                      <Badge
                        variant="secondary"
                        className={cn(
                          "text-xs",
                          key.enabled
                            ? "bg-success/10 text-success border-0"
                            : "bg-destructive/10 text-destructive border-0"
                        )}
                      >
                        {key.enabled ? tk("active") : tk("disabled")}
                      </Badge>
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {new Date(key.created_at).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleToggle(key.name, key.enabled)}
                          title={key.enabled ? tk("disableAction") : tk("enableAction")}
                        >
                          {key.enabled ? (
                            <ToggleRight className="h-4 w-4 text-success" />
                          ) : (
                            <ToggleLeft className="h-4 w-4 text-muted-foreground" />
                          )}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleDelete(key.name)}
                        >
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </div>
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
