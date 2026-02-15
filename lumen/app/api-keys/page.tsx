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
import { apiKeysApi, functionsApi, workflowsApi } from "@/lib/api"
import type { APIKeyEntry, PolicyBinding } from "@/lib/api"
import {
  Plus,
  Trash2,
  RefreshCw,
  Copy,
  Check,
  KeyRound,
  ToggleLeft,
  ToggleRight,
  X,
  Shield,
} from "lucide-react"
import { cn } from "@/lib/utils"

function PolicyBindingEditor({
  bindings,
  onChange,
  availableFunctions,
  availableWorkflows,
  tk,
  tc,
}: {
  bindings: PolicyBinding[]
  onChange: (bindings: PolicyBinding[]) => void
  availableFunctions: string[]
  availableWorkflows: string[]
  tk: (key: string) => string
  tc: (key: string) => string
}) {
  const [fnInput, setFnInput] = useState("")
  const [wfInput, setWfInput] = useState("")

  const currentBinding = bindings.length > 0 ? bindings[0] : { role: "invoker", functions: [], workflows: [] }
  const boundFunctions = currentBinding.functions || []
  const boundWorkflows = currentBinding.workflows || []

  const updateBinding = (fns: string[], wfs: string[]) => {
    const role = currentBinding.role || "invoker"
    if (fns.length === 0 && wfs.length === 0) {
      onChange([{ role }])
    } else {
      onChange([{
        role,
        ...(fns.length > 0 ? { functions: fns } : {}),
        ...(wfs.length > 0 ? { workflows: wfs } : {}),
      }])
    }
  }

  const addFunction = (name: string) => {
    if (name && !boundFunctions.includes(name)) {
      updateBinding([...boundFunctions, name], boundWorkflows)
    }
    setFnInput("")
  }

  const removeFunction = (name: string) => {
    updateBinding(boundFunctions.filter((f) => f !== name), boundWorkflows)
  }

  const addWorkflow = (name: string) => {
    if (name && !boundWorkflows.includes(name)) {
      updateBinding(boundFunctions, [...boundWorkflows, name])
    }
    setWfInput("")
  }

  const removeWorkflow = (name: string) => {
    updateBinding(boundFunctions, boundWorkflows.filter((w) => w !== name))
  }

  const unusedFunctions = availableFunctions.filter((f) => !boundFunctions.includes(f))
  const unusedWorkflows = availableWorkflows.filter((w) => !boundWorkflows.includes(w))

  return (
    <div className="space-y-4">
      {/* Role */}
      <div className="space-y-2">
        <label className="text-sm font-medium">{tk("role")}</label>
        <Select
          value={currentBinding.role || "invoker"}
          onValueChange={(role) => {
            onChange(bindings.length > 0
              ? bindings.map((b, i) => i === 0 ? { ...b, role } : b)
              : [{ role, functions: [], workflows: [] }]
            )
          }}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="admin">{tk("roleAdmin")}</SelectItem>
            <SelectItem value="operator">{tk("roleOperator")}</SelectItem>
            <SelectItem value="invoker">{tk("roleInvoker")}</SelectItem>
            <SelectItem value="viewer">{tk("roleViewer")}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Bound Functions */}
      <div className="space-y-2">
        <label className="text-sm font-medium">{tk("boundFunctions")}</label>
        <p className="text-xs text-muted-foreground">{tk("boundFunctionsHint")}</p>
        <div className="flex flex-wrap gap-1.5 min-h-[28px]">
          {boundFunctions.map((fn) => (
            <Badge key={fn} variant="secondary" className="text-xs gap-1 pr-1">
              {fn}
              <button onClick={() => removeFunction(fn)} className="ml-0.5 hover:text-destructive">
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>
        <div className="flex gap-2">
          {unusedFunctions.length > 0 ? (
            <Select value="" onValueChange={(v) => addFunction(v)}>
              <SelectTrigger className="flex-1">
                <SelectValue placeholder={tk("selectFunction")} />
              </SelectTrigger>
              <SelectContent>
                {unusedFunctions.map((fn) => (
                  <SelectItem key={fn} value={fn}>{fn}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : null}
          <div className="flex gap-1 flex-1">
            <Input
              value={fnInput}
              onChange={(e) => setFnInput(e.target.value)}
              placeholder={tk("functionPatternPlaceholder")}
              className="flex-1"
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault()
                  addFunction(fnInput.trim())
                }
              }}
            />
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!fnInput.trim()}
              onClick={() => addFunction(fnInput.trim())}
            >
              <Plus className="h-3 w-3" />
            </Button>
          </div>
        </div>
      </div>

      {/* Bound Workflows */}
      <div className="space-y-2">
        <label className="text-sm font-medium">{tk("boundWorkflows")}</label>
        <p className="text-xs text-muted-foreground">{tk("boundWorkflowsHint")}</p>
        <div className="flex flex-wrap gap-1.5 min-h-[28px]">
          {boundWorkflows.map((wf) => (
            <Badge key={wf} variant="secondary" className="text-xs gap-1 pr-1">
              {wf}
              <button onClick={() => removeWorkflow(wf)} className="ml-0.5 hover:text-destructive">
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>
        <div className="flex gap-2">
          {unusedWorkflows.length > 0 ? (
            <Select value="" onValueChange={(v) => addWorkflow(v)}>
              <SelectTrigger className="flex-1">
                <SelectValue placeholder={tk("selectWorkflow")} />
              </SelectTrigger>
              <SelectContent>
                {unusedWorkflows.map((wf) => (
                  <SelectItem key={wf} value={wf}>{wf}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : null}
          <div className="flex gap-1 flex-1">
            <Input
              value={wfInput}
              onChange={(e) => setWfInput(e.target.value)}
              placeholder={tk("workflowPatternPlaceholder")}
              className="flex-1"
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault()
                  addWorkflow(wfInput.trim())
                }
              }}
            />
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!wfInput.trim()}
              onClick={() => addWorkflow(wfInput.trim())}
            >
              <Plus className="h-3 w-3" />
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

function PermissionsSummary({ permissions, tk }: { permissions: PolicyBinding[]; tk: (key: string) => string }) {
  if (!permissions || permissions.length === 0) {
    return <span className="text-muted-foreground text-xs">{tk("allResources")}</span>
  }
  const binding = permissions[0]
  const fns = binding.functions || []
  const wfs = binding.workflows || []
  const hasScope = fns.length > 0 || wfs.length > 0
  return (
    <div className="flex flex-col gap-0.5">
      <Badge variant="outline" className="text-xs w-fit">{binding.role || "invoker"}</Badge>
      {hasScope ? (
        <div className="flex flex-wrap gap-1 mt-0.5">
          {fns.map((f) => (
            <Badge key={"fn:" + f} variant="secondary" className="text-xs">
              ƒ {f}
            </Badge>
          ))}
          {wfs.map((w) => (
            <Badge key={"wf:" + w} variant="secondary" className="text-xs">
              ⚡ {w}
            </Badge>
          ))}
        </div>
      ) : (
        <span className="text-muted-foreground text-xs">{tk("allResources")}</span>
      )}
    </div>
  )
}

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
  const [newKeyPermissions, setNewKeyPermissions] = useState<PolicyBinding[]>([{ role: "invoker" }])
  const [createdKey, setCreatedKey] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [creating, setCreating] = useState(false)
  const [availableFunctions, setAvailableFunctions] = useState<string[]>([])
  const [availableWorkflows, setAvailableWorkflows] = useState<string[]>([])
  const [editingKey, setEditingKey] = useState<string | null>(null)
  const [editPermissions, setEditPermissions] = useState<PolicyBinding[]>([])
  const [editDialogOpen, setEditDialogOpen] = useState(false)

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

  const fetchResources = useCallback(async () => {
    try {
      const [fns, wfs] = await Promise.all([
        functionsApi.list().catch(() => []),
        workflowsApi.list().catch(() => []),
      ])
      setAvailableFunctions(fns.map((f) => f.name))
      setAvailableWorkflows(wfs.map((w) => w.name))
    } catch {
      // ignore – resource lists are optional
    }
  }, [])

  useEffect(() => {
    fetchKeys()
    fetchResources()
  }, [fetchKeys, fetchResources])

  const handleCreate = async () => {
    if (!newKeyName.trim()) return
    try {
      setCreating(true)
      const result = await apiKeysApi.create(newKeyName.trim(), newKeyTier, newKeyPermissions)
      setCreatedKey(result.key)
      setNewKeyName("")
      setNewKeyTier("default")
      setNewKeyPermissions([{ role: "invoker" }])
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

  const openEditPermissions = (key: APIKeyEntry) => {
    setEditingKey(key.name)
    setEditPermissions(key.permissions && key.permissions.length > 0 ? [...key.permissions] : [{ role: "invoker" }])
    setEditDialogOpen(true)
  }

  const handleSavePermissions = async () => {
    if (!editingKey) return
    try {
      await apiKeysApi.updatePermissions(editingKey, editPermissions)
      setEditDialogOpen(false)
      setEditingKey(null)
      fetchKeys()
    } catch (err) {
      setError(err instanceof Error ? err.message : tk("failedToUpdate"))
    }
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
            <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
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
                  <PolicyBindingEditor
                    bindings={newKeyPermissions}
                    onChange={setNewKeyPermissions}
                    availableFunctions={availableFunctions}
                    availableWorkflows={availableWorkflows}
                    tk={tk}
                    tc={tc}
                  />
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

        {/* Edit permissions dialog */}
        <Dialog open={editDialogOpen} onOpenChange={setEditDialogOpen}>
          <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>{tk("editPermissions")} – {editingKey}</DialogTitle>
            </DialogHeader>
            <PolicyBindingEditor
              bindings={editPermissions}
              onChange={setEditPermissions}
              availableFunctions={availableFunctions}
              availableWorkflows={availableWorkflows}
              tk={tk}
              tc={tc}
            />
            <Button className="w-full" onClick={handleSavePermissions}>
              {tc("save")}
            </Button>
          </DialogContent>
        </Dialog>

        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tk("colName")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tk("colTier")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tk("colPermissions")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tk("colStatus")}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tk("colCreated")}</th>
                <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{tk("colActions")}</th>
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
              ) : keys.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-muted-foreground">
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
                      <PermissionsSummary permissions={key.permissions} tk={tk} />
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
                          onClick={() => openEditPermissions(key)}
                          title={tk("editPermissions")}
                        >
                          <Shield className="h-4 w-4" />
                        </Button>
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
