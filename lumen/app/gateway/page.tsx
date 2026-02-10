"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { EmptyState } from "@/components/empty-state"
import { OnboardingFlow } from "@/components/onboarding-flow"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Badge } from "@/components/ui/badge"
import { ErrorBanner } from "@/components/ui/error-banner"
import {
  functionsApi,
  gatewayApi,
  metricsApi,
  type CreateGatewayRouteRequest,
  type GatewayRateLimitTemplate,
  type GatewayRoute,
  type NovaFunction,
  type UpdateGatewayRouteRequest,
} from "@/lib/api"
import { toUserErrorMessage } from "@/lib/error-map"
import { markOnboardingStep, syncOnboardingStateFromData } from "@/lib/onboarding-state"
import { cn } from "@/lib/utils"
import { Plus, RefreshCw, Trash2, ToggleLeft, ToggleRight, Pencil } from "lucide-react"

type AuthStrategy = "none" | "inherit" | "apikey" | "jwt"

const DEFAULT_METHODS = "GET"

function parseMethods(raw: string): string[] {
  const methods = raw
    .split(",")
    .map((item) => item.trim().toUpperCase())
    .filter(Boolean)
  return Array.from(new Set(methods))
}

function methodsDisplay(methods?: string[]): string {
  if (!methods || methods.length === 0) {
    return "ALL"
  }
  return methods.join(", ")
}

function formatDate(ts?: string): string {
  if (!ts) return "-"
  const date = new Date(ts)
  if (Number.isNaN(date.getTime())) return ts
  return date.toLocaleString()
}

type Notice = {
  kind: "success" | "error" | "info"
  text: string
}

export default function GatewayPage() {
  const router = useRouter()
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<Notice | null>(null)
  const [routes, setRoutes] = useState<GatewayRoute[]>([])
  const [functions, setFunctions] = useState<NovaFunction[]>([])
  const [domainFilter, setDomainFilter] = useState("")
  const [pendingDeleteRouteID, setPendingDeleteRouteID] = useState<string | null>(null)
  const [rateLimitTemplate, setRateLimitTemplate] = useState<GatewayRateLimitTemplate | null>(null)
  const [templateEnabled, setTemplateEnabled] = useState("false")
  const [templateRps, setTemplateRps] = useState("")
  const [templateBurst, setTemplateBurst] = useState("")
  const [templateSaving, setTemplateSaving] = useState(false)
  const [hasInvocations, setHasInvocations] = useState(false)

  const [createOpen, setCreateOpen] = useState(false)
  const [editOpen, setEditOpen] = useState(false)
  const [editingRoute, setEditingRoute] = useState<GatewayRoute | null>(null)

  const [createDomain, setCreateDomain] = useState("")
  const [createPath, setCreatePath] = useState("")
  const [createMethods, setCreateMethods] = useState(DEFAULT_METHODS)
  const [createFunctionName, setCreateFunctionName] = useState("")
  const [createAuth, setCreateAuth] = useState<AuthStrategy>("none")
  const [createEnabled, setCreateEnabled] = useState(true)
  const [createRps, setCreateRps] = useState("")
  const [createBurst, setCreateBurst] = useState("")

  const [editDomain, setEditDomain] = useState("")
  const [editPath, setEditPath] = useState("")
  const [editMethods, setEditMethods] = useState(DEFAULT_METHODS)
  const [editFunctionName, setEditFunctionName] = useState("")
  const [editAuth, setEditAuth] = useState<AuthStrategy>("none")
  const [editEnabled, setEditEnabled] = useState(true)
  const [editRps, setEditRps] = useState("")
  const [editBurst, setEditBurst] = useState("")

  const filteredRoutes = useMemo(() => {
    const next = domainFilter.trim()
    if (!next) return routes
    return routes.filter((route) => (route.domain || "").includes(next))
  }, [domainFilter, routes])

  const loadData = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [routeData, functionData, templateData, metrics] = await Promise.all([
        gatewayApi.listRoutes(),
        functionsApi.list(),
        gatewayApi.getRateLimitTemplate(),
        metricsApi.global().catch(() => null),
      ])
      setRoutes(routeData || [])
      setFunctions(functionData || [])
      setRateLimitTemplate(templateData)
      setTemplateEnabled(templateData.enabled ? "true" : "false")
      setTemplateRps(templateData.requests_per_second > 0 ? String(templateData.requests_per_second) : "")
      setTemplateBurst(templateData.burst_size > 0 ? String(templateData.burst_size) : "")
      const nextHasInvocations = Boolean((metrics?.invocations?.total || 0) > 0)
      setHasInvocations(nextHasInvocations)
      syncOnboardingStateFromData({
        hasFunctionCreated: (functionData?.length || 0) > 0,
        hasFunctionInvoked: nextHasInvocations,
        hasGatewayRouteCreated: (routeData?.length || 0) > 0,
      })
      if (!createFunctionName && functionData?.length) {
        setCreateFunctionName(functionData[0].name)
      }
    } catch (err) {
      setError(toUserErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [createFunctionName])

  useEffect(() => {
    loadData()
  }, [loadData])

  const resetCreateForm = () => {
    setCreateDomain("")
    setCreatePath("")
    setCreateMethods(DEFAULT_METHODS)
    setCreateFunctionName(functions[0]?.name || "")
    setCreateAuth("none")
    setCreateEnabled(true)
    setCreateRps("")
    setCreateBurst("")
  }

  const setEditFromRoute = (route: GatewayRoute) => {
    setEditingRoute(route)
    setEditDomain(route.domain || "")
    setEditPath(route.path || "")
    setEditMethods(route.methods?.length ? route.methods.join(",") : "")
    setEditFunctionName(route.function_name || "")
    setEditAuth((route.auth_strategy as AuthStrategy) || "none")
    setEditEnabled(Boolean(route.enabled))
    setEditRps(route.rate_limit?.requests_per_second ? String(route.rate_limit.requests_per_second) : "")
    setEditBurst(route.rate_limit?.burst_size ? String(route.rate_limit.burst_size) : "")
  }

  const buildRateLimit = (rpsRaw: string, burstRaw: string) => {
    const rps = Number(rpsRaw)
    const burst = Number(burstRaw)
    if (!Number.isFinite(rps) || rps <= 0) return undefined
    if (!Number.isFinite(burst) || burst <= 0) return undefined
    return { requests_per_second: rps, burst_size: Math.floor(burst) }
  }

  const handleSaveTemplate = async () => {
    const enabled = templateEnabled === "true"
    const rps = Number(templateRps)
    const burst = Number(templateBurst)

    if (enabled) {
      if (!Number.isFinite(rps) || rps <= 0) {
        setError("Default template RPS must be > 0 when enabled.")
        return
      }
      if (!Number.isFinite(burst) || burst <= 0) {
        setError("Default template burst must be > 0 when enabled.")
        return
      }
    }

    try {
      setTemplateSaving(true)
      setError(null)
      const updated = await gatewayApi.updateRateLimitTemplate({
        enabled,
        requests_per_second: enabled ? rps : 0,
        burst_size: enabled ? Math.floor(burst) : 0,
      })
      setRateLimitTemplate(updated)
      setTemplateEnabled(updated.enabled ? "true" : "false")
      setTemplateRps(updated.requests_per_second > 0 ? String(updated.requests_per_second) : "")
      setTemplateBurst(updated.burst_size > 0 ? String(updated.burst_size) : "")
      setNotice({ kind: "success", text: "Gateway default rate-limit template saved" })
    } catch (err) {
      setNotice({ kind: "error", text: toUserErrorMessage(err) })
      setError(toUserErrorMessage(err))
    } finally {
      setTemplateSaving(false)
    }
  }

  const handleCreateRoute = async () => {
    if (!createPath.trim()) {
      setError("Path is required.")
      return
    }
    if (!createFunctionName.trim()) {
      setError("Function is required.")
      return
    }

    const payload: CreateGatewayRouteRequest = {
      domain: createDomain.trim() || undefined,
      path: createPath.trim(),
      methods: parseMethods(createMethods),
      function_name: createFunctionName,
      auth_strategy: createAuth,
      enabled: createEnabled,
      rate_limit: buildRateLimit(createRps, createBurst),
    }

    try {
      setBusy(true)
      setError(null)
      await gatewayApi.createRoute(payload)
      markOnboardingStep("gateway_route_created", true)
      setCreateOpen(false)
      resetCreateForm()
      await loadData()
      setNotice({ kind: "success", text: "Gateway route created" })
    } catch (err) {
      setNotice({ kind: "error", text: toUserErrorMessage(err) })
      setError(toUserErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const handleUpdateRoute = async () => {
    if (!editingRoute) return
    if (!editPath.trim()) {
      setError("Path is required.")
      return
    }
    if (!editFunctionName.trim()) {
      setError("Function is required.")
      return
    }

    const payload: UpdateGatewayRouteRequest = {
      domain: editDomain.trim() || "",
      path: editPath.trim(),
      methods: parseMethods(editMethods),
      function_name: editFunctionName,
      auth_strategy: editAuth,
      enabled: editEnabled,
      rate_limit: buildRateLimit(editRps, editBurst),
    }

    try {
      setBusy(true)
      setError(null)
      await gatewayApi.updateRoute(editingRoute.id, payload)
      setEditOpen(false)
      setEditingRoute(null)
      await loadData()
      setNotice({ kind: "success", text: "Gateway route updated" })
    } catch (err) {
      setNotice({ kind: "error", text: toUserErrorMessage(err) })
      setError(toUserErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (pendingDeleteRouteID !== id) {
      setPendingDeleteRouteID(id)
      setNotice({ kind: "info", text: `Click delete again to confirm route "${id}" deletion` })
      return
    }
    try {
      setBusy(true)
      setError(null)
      await gatewayApi.deleteRoute(id)
      await loadData()
      setPendingDeleteRouteID(null)
      setNotice({ kind: "success", text: `Route "${id}" deleted` })
    } catch (err) {
      setNotice({ kind: "error", text: toUserErrorMessage(err) })
      setError(toUserErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const handleToggleEnabled = async (route: GatewayRoute) => {
    try {
      setBusy(true)
      setError(null)
      await gatewayApi.updateRoute(route.id, { enabled: !route.enabled })
      await loadData()
      setNotice({ kind: "success", text: `Route "${route.id}" ${route.enabled ? "disabled" : "enabled"}` })
    } catch (err) {
      setNotice({ kind: "error", text: toUserErrorMessage(err) })
      setError(toUserErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <DashboardLayout>
      <Header title="Gateway" description="Manage HTTP routes mapped to Nova functions" />

      <div className="space-y-6 p-6">
        <OnboardingFlow
          hasFunctionCreated={functions.length > 0}
          hasFunctionInvoked={hasInvocations}
          hasGatewayRouteCreated={routes.length > 0}
          onCreateFunction={() => router.push("/functions")}
          onCreateGatewayRoute={() => setCreateOpen(true)}
        />

        {error && (
          <ErrorBanner error={error} title="加载网关配置失败" onRetry={loadData} />
        )}

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

        <div className="rounded-xl border border-border bg-card p-4">
          <div className="flex flex-wrap items-end gap-3">
            <div className="space-y-1">
              <Label>Default Rate Limit Template</Label>
              <Select value={templateEnabled} onValueChange={setTemplateEnabled}>
                <SelectTrigger className="w-[140px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="true">enabled</SelectItem>
                  <SelectItem value="false">disabled</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-1">
              <Label htmlFor="template-rps">RPS</Label>
              <Input
                id="template-rps"
                type="number"
                min="0"
                value={templateRps}
                onChange={(e) => setTemplateRps(e.target.value)}
                placeholder="20"
                className="w-[140px]"
              />
            </div>

            <div className="space-y-1">
              <Label htmlFor="template-burst">Burst</Label>
              <Input
                id="template-burst"
                type="number"
                min="0"
                value={templateBurst}
                onChange={(e) => setTemplateBurst(e.target.value)}
                placeholder="40"
                className="w-[140px]"
              />
            </div>

            <Button onClick={handleSaveTemplate} disabled={templateSaving || busy}>
              {templateSaving ? "Saving..." : "Save Template"}
            </Button>
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            New routes without explicit rate limits will inherit this template.
          </p>
          {rateLimitTemplate && rateLimitTemplate.enabled && (
            <p className="mt-1 text-xs text-muted-foreground">
              Current default: {rateLimitTemplate.requests_per_second}/s, burst {rateLimitTemplate.burst_size}
            </p>
          )}
        </div>

        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Input
              value={domainFilter}
              onChange={(e) => setDomainFilter(e.target.value)}
              placeholder="Filter by domain..."
              className="w-[240px]"
            />
          </div>
          <div className="flex items-center gap-2">
            <Dialog
              open={createOpen}
              onOpenChange={(open) => {
                setCreateOpen(open)
                if (!open) {
                  resetCreateForm()
                }
              }}
            >
              <DialogTrigger asChild>
                <Button size="sm">
                  <Plus className="mr-2 h-4 w-4" />
                  Add Route
                </Button>
              </DialogTrigger>
              <DialogContent className="sm:max-w-2xl">
                <DialogHeader>
                  <DialogTitle>Create Gateway Route</DialogTitle>
                </DialogHeader>
                <div className="grid gap-4 py-2 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="create-domain">Domain</Label>
                    <Input
                      id="create-domain"
                      value={createDomain}
                      onChange={(e) => setCreateDomain(e.target.value)}
                      placeholder="api.example.com (optional)"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="create-path">Path</Label>
                    <Input
                      id="create-path"
                      value={createPath}
                      onChange={(e) => setCreatePath(e.target.value)}
                      placeholder="/v1/orders"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="create-methods">Methods (comma-separated)</Label>
                    <Input
                      id="create-methods"
                      value={createMethods}
                      onChange={(e) => setCreateMethods(e.target.value)}
                      placeholder="GET,POST"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>Function</Label>
                    <Select value={createFunctionName} onValueChange={setCreateFunctionName}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select function" />
                      </SelectTrigger>
                      <SelectContent>
                        {functions.map((fn) => (
                          <SelectItem key={fn.id} value={fn.name}>
                            {fn.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>Auth Strategy</Label>
                    <Select value={createAuth} onValueChange={(v: AuthStrategy) => setCreateAuth(v)}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="none">none</SelectItem>
                        <SelectItem value="inherit">inherit</SelectItem>
                        <SelectItem value="apikey">apikey</SelectItem>
                        <SelectItem value="jwt">jwt</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>Enabled</Label>
                    <Select
                      value={createEnabled ? "true" : "false"}
                      onValueChange={(v) => setCreateEnabled(v === "true")}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="true">enabled</SelectItem>
                        <SelectItem value="false">disabled</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="create-rps">Rate Limit RPS (optional)</Label>
                    <Input
                      id="create-rps"
                      type="number"
                      min="0"
                      value={createRps}
                      onChange={(e) => setCreateRps(e.target.value)}
                      placeholder="e.g. 20"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="create-burst">Rate Limit Burst (optional)</Label>
                    <Input
                      id="create-burst"
                      type="number"
                      min="0"
                      value={createBurst}
                      onChange={(e) => setCreateBurst(e.target.value)}
                      placeholder="e.g. 40"
                    />
                  </div>
                </div>
                <div className="flex justify-end gap-2">
                  <Button
                    variant="outline"
                    onClick={() => {
                      setCreateOpen(false)
                      resetCreateForm()
                    }}
                    disabled={busy}
                  >
                    Cancel
                  </Button>
                  <Button onClick={handleCreateRoute} disabled={busy || !createPath.trim() || !createFunctionName}>
                    Create
                  </Button>
                </div>
              </DialogContent>
            </Dialog>

            <Button variant="outline" size="sm" onClick={loadData} disabled={loading || busy}>
              <RefreshCw className={cn("mr-2 h-4 w-4", (loading || busy) && "animate-spin")} />
              Refresh
            </Button>
          </div>
        </div>

        {!loading && routes.length === 0 ? (
          <EmptyState
            title="还没有网关路由"
            description="创建路由后，UI / CLI / MCP 都可以通过 Zenith 统一调用。"
            primaryAction={{
              label: functions.length > 0 ? "新增路由" : "先创建函数",
              onClick: () => {
                if (functions.length > 0) {
                  setCreateOpen(true)
                } else {
                  router.push("/functions")
                }
              },
            }}
          />
        ) : !loading && routes.length > 0 && filteredRoutes.length === 0 ? (
          <EmptyState
            title="没有匹配的路由"
            description="当前域名筛选没有命中路由。"
            primaryAction={{ label: "清空筛选", onClick: () => setDomainFilter("") }}
            compact
          />
        ) : (
          <div className="overflow-x-auto rounded-xl border border-border bg-card">
            <table className="w-full min-w-[980px]">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">ID</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Domain</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Path</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Methods</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Function</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Auth</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Rate Limit</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Status</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">Updated</th>
                  <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">Actions</th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  Array.from({ length: 3 }).map((_, i) => (
                    <tr key={i} className="border-b border-border">
                      <td className="px-4 py-3" colSpan={10}>
                        <div className="h-4 animate-pulse rounded bg-muted" />
                      </td>
                    </tr>
                  ))
                ) : (
                  filteredRoutes.map((route) => (
                  <tr key={route.id} className="border-b border-border last:border-0 hover:bg-muted/50">
                    <td className="px-4 py-3 text-sm font-mono">{route.id}</td>
                    <td className="px-4 py-3 text-sm">{route.domain || "-"}</td>
                    <td className="px-4 py-3 text-sm font-mono">{route.path}</td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">{methodsDisplay(route.methods)}</td>
                    <td className="px-4 py-3 text-sm">{route.function_name}</td>
                    <td className="px-4 py-3 text-sm">{route.auth_strategy || "none"}</td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">
                      {route.rate_limit
                        ? `${route.rate_limit.requests_per_second}/s · burst ${route.rate_limit.burst_size}`
                        : "-"}
                    </td>
                    <td className="px-4 py-3">
                      <Badge
                        variant="secondary"
                        className={cn(
                          "text-xs",
                          route.enabled
                            ? "border-0 bg-success/10 text-success"
                            : "border-0 bg-muted text-muted-foreground"
                        )}
                      >
                        {route.enabled ? "enabled" : "disabled"}
                      </Badge>
                    </td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">{formatDate(route.updated_at)}</td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          title={route.enabled ? "Disable route" : "Enable route"}
                          onClick={() => void handleToggleEnabled(route)}
                          disabled={busy}
                        >
                          {route.enabled ? (
                            <ToggleRight className="h-4 w-4 text-success" />
                          ) : (
                            <ToggleLeft className="h-4 w-4 text-muted-foreground" />
                          )}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          title="Edit route"
                          onClick={() => {
                            setEditFromRoute(route)
                            setEditOpen(true)
                          }}
                          disabled={busy}
                        >
                          <Pencil className="h-4 w-4" />
                        </Button>
                        {pendingDeleteRouteID === route.id ? (
                          <div className="flex items-center gap-1">
                            <Button
                              variant="destructive"
                              size="sm"
                              title="Confirm delete route"
                              onClick={() => void handleDelete(route.id)}
                              disabled={busy}
                            >
                              Confirm
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              title="Cancel delete route"
                              onClick={() => {
                                setPendingDeleteRouteID(null)
                                setNotice(null)
                              }}
                              disabled={busy}
                            >
                              Cancel
                            </Button>
                          </div>
                        ) : (
                          <Button
                            variant="ghost"
                            size="sm"
                            title="Delete route"
                            onClick={() => void handleDelete(route.id)}
                            disabled={busy}
                          >
                            <Trash2 className="h-4 w-4 text-destructive" />
                          </Button>
                        )}
                      </div>
                    </td>
                  </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Edit Gateway Route</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 py-2 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="edit-domain">Domain</Label>
              <Input
                id="edit-domain"
                value={editDomain}
                onChange={(e) => setEditDomain(e.target.value)}
                placeholder="api.example.com (optional)"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-path">Path</Label>
              <Input
                id="edit-path"
                value={editPath}
                onChange={(e) => setEditPath(e.target.value)}
                placeholder="/v1/orders"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-methods">Methods (comma-separated)</Label>
              <Input
                id="edit-methods"
                value={editMethods}
                onChange={(e) => setEditMethods(e.target.value)}
                placeholder="GET,POST"
              />
            </div>
            <div className="space-y-2">
              <Label>Function</Label>
              <Select value={editFunctionName} onValueChange={setEditFunctionName}>
                <SelectTrigger>
                  <SelectValue placeholder="Select function" />
                </SelectTrigger>
                <SelectContent>
                  {functions.map((fn) => (
                    <SelectItem key={fn.id} value={fn.name}>
                      {fn.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Auth Strategy</Label>
              <Select value={editAuth} onValueChange={(v: AuthStrategy) => setEditAuth(v)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">none</SelectItem>
                  <SelectItem value="inherit">inherit</SelectItem>
                  <SelectItem value="apikey">apikey</SelectItem>
                  <SelectItem value="jwt">jwt</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Enabled</Label>
              <Select value={editEnabled ? "true" : "false"} onValueChange={(v) => setEditEnabled(v === "true")}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="true">enabled</SelectItem>
                  <SelectItem value="false">disabled</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-rps">Rate Limit RPS (optional)</Label>
              <Input
                id="edit-rps"
                type="number"
                min="0"
                value={editRps}
                onChange={(e) => setEditRps(e.target.value)}
                placeholder="e.g. 20"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-burst">Rate Limit Burst (optional)</Label>
              <Input
                id="edit-burst"
                type="number"
                min="0"
                value={editBurst}
                onChange={(e) => setEditBurst(e.target.value)}
                placeholder="e.g. 40"
              />
            </div>
          </div>
          <div className="flex justify-end gap-2">
            <Button
              variant="outline"
              onClick={() => {
                setEditOpen(false)
                setEditingRoute(null)
              }}
              disabled={busy}
            >
              Cancel
            </Button>
            <Button onClick={handleUpdateRoute} disabled={busy || !editingRoute || !editPath.trim() || !editFunctionName}>
              Save
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  )
}
