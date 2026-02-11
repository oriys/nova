"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { secretsApi } from "@/lib/api"
import type { SecretEntry } from "@/lib/api"
import { Plus, Trash2, RefreshCw, Lock } from "lucide-react"
import { cn } from "@/lib/utils"

export default function SecretsPage() {
  const t = useTranslations("pages")
  const [secrets, setSecrets] = useState<SecretEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [newSecretName, setNewSecretName] = useState("")
  const [newSecretValue, setNewSecretValue] = useState("")
  const [creating, setCreating] = useState(false)

  const fetchSecrets = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await secretsApi.list()
      setSecrets(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load secrets")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchSecrets()
  }, [fetchSecrets])

  const handleCreate = async () => {
    if (!newSecretName.trim() || !newSecretValue.trim()) return
    try {
      setCreating(true)
      await secretsApi.create(newSecretName.trim(), newSecretValue)
      setDialogOpen(false)
      setNewSecretName("")
      setNewSecretValue("")
      fetchSecrets()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create secret")
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (name: string) => {
    try {
      await secretsApi.delete(name)
      fetchSecrets()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete secret")
    }
  }

  return (
    <DashboardLayout>
      <Header title={t("secrets.title")} description={t("secrets.description")} />

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
                Create Secret
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Create Secret</DialogTitle>
              </DialogHeader>
              <div className="space-y-4">
                <div className="space-y-2">
                  <label className="text-sm font-medium">Name</label>
                  <Input
                    value={newSecretName}
                    onChange={(e) => setNewSecretName(e.target.value)}
                    placeholder="DATABASE_URL"
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">Value</label>
                  <Textarea
                    value={newSecretValue}
                    onChange={(e) => setNewSecretValue(e.target.value)}
                    placeholder="Enter secret value..."
                    className="min-h-[100px] font-mono text-sm"
                  />
                </div>
                <p className="text-xs text-muted-foreground">
                  Values are encrypted at rest. Reference in function env vars
                  as <code className="bg-muted px-1 rounded">$SECRET:name</code>
                </p>
                <Button
                  className="w-full"
                  onClick={handleCreate}
                  disabled={creating || !newSecretName.trim() || !newSecretValue.trim()}
                >
                  {creating ? "Creating..." : "Create"}
                </Button>
              </div>
            </DialogContent>
          </Dialog>

          <Button variant="outline" size="sm" onClick={fetchSecrets} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            Refresh
          </Button>
        </div>

        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Name</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Created</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">Actions</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <tr key={i} className="border-b border-border">
                    <td colSpan={3} className="px-4 py-3">
                      <div className="h-4 bg-muted rounded animate-pulse" />
                    </td>
                  </tr>
                ))
              ) : secrets.length === 0 ? (
                <tr>
                  <td colSpan={3} className="px-4 py-8 text-center text-muted-foreground">
                    <Lock className="mx-auto h-8 w-8 mb-2 opacity-50" />
                    No secrets yet
                  </td>
                </tr>
              ) : (
                secrets.map((secret) => (
                  <tr key={secret.name} className="border-b border-border hover:bg-muted/50">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <Lock className="h-4 w-4 text-muted-foreground" />
                        <span className="font-medium text-sm font-mono">{secret.name}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-sm text-muted-foreground">
                      {new Date(secret.created_at).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDelete(secret.name)}
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
