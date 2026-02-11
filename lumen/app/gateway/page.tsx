"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useTranslations } from "next-intl"
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
import { Plus, RefreshCw, Trash2, ToggleLeft, ToggleRight, Pencil, Download, Upload } from "lucide-react"

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

type JsonObject = Record<string, unknown>

function isJsonObject(value: unknown): value is JsonObject {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function readStringMap(value: unknown): Record<string, string> | undefined {
  if (!isJsonObject(value)) {
    return undefined
  }
  const entries = Object.entries(value).filter(
    (entry): entry is [string, string] => typeof entry[1] === "string"
  )
  if (entries.length === 0) {
    return undefined
  }
  return Object.fromEntries(entries)
}

function readMethods(value: unknown): string[] | undefined {
  if (typeof value === "string") {
    const parsed = parseMethods(value)
    return parsed.length > 0 ? parsed : undefined
  }
  if (!Array.isArray(value)) {
    return undefined
  }
  const parsed = Array.from(
    new Set(
      value
        .filter((item): item is string => typeof item === "string")
        .map((item) => item.trim().toUpperCase())
        .filter(Boolean)
    )
  )
  return parsed.length > 0 ? parsed : undefined
}

function parseRouteImportPayload(payload: unknown): {
  items: CreateGatewayRouteRequest[]
  invalid: number
} {
  const rows: unknown[] = Array.isArray(payload)
    ? payload
    : isJsonObject(payload) && Array.isArray(payload.routes)
      ? payload.routes
      : []
  const items: CreateGatewayRouteRequest[] = []
  let invalid = 0

  rows.forEach((row) => {
    if (!isJsonObject(row)) {
      invalid += 1
      return
    }

    const path = typeof row.path === "string" ? row.path.trim() : ""
    const functionName =
      typeof row.function_name === "string"
        ? row.function_name.trim()
        : typeof row.function === "string"
          ? row.function.trim()
          : ""
    if (!path || !functionName) {
      invalid += 1
      return
    }

    const req: CreateGatewayRouteRequest = {
      path,
      function_name: functionName,
      methods: readMethods(row.methods),
      auth_strategy:
        typeof row.auth_strategy === "string" && row.auth_strategy.trim()
          ? row.auth_strategy.trim()
          : "none",
      enabled: typeof row.enabled === "boolean" ? row.enabled : true,
    }

    const domain = typeof row.domain === "string" ? row.domain.trim() : ""
    if (domain) {
      req.domain = domain
    }

    const authConfig = readStringMap(row.auth_config ?? row.authConfig)
    if (authConfig) {
      req.auth_config = authConfig
    }

    if (row.request_schema !== undefined) {
      req.request_schema = row.request_schema
    } else if (row.requestSchema !== undefined) {
      req.request_schema = row.requestSchema
    }

    const rateLimitRaw = row.rate_limit ?? row.rateLimit
    if (isJsonObject(rateLimitRaw)) {
      const rps = Number(rateLimitRaw.requests_per_second ?? rateLimitRaw.rps)
      const burst = Number(rateLimitRaw.burst_size ?? rateLimitRaw.burst)
      if (Number.isFinite(rps) && rps > 0 && Number.isFinite(burst) && burst > 0) {
        req.rate_limit = {
          requests_per_second: rps,
          burst_size: Math.floor(burst),
        }
      }
    }

    items.push(req)
  })

  return { items, invalid }
}

function downloadJSON(filename: string, payload: unknown) {
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement("a")
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

export default function GatewayPage() {
  const t = useTranslations("pages")
  const router = useRouter()
  const createOpenedByQueryRef = useRef(false)
  const importInputRef = useRef<HTMLInputElement | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [ioBusy, setIoBusy] = useState(false)
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
  const [selectedRouteIDs, setSelectedRouteIDs] = useState<Set<string>>(new Set())
  const [confirmBulkDelete, setConfirmBulkDelete] = useState(false)
  const [bulkBusy, setBulkBusy] = useState(false)

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

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const shouldOpenCreate = params.get("create") === "1"
    if (!shouldOpenCreate || createOpenedByQueryRef.current) {
      return
    }
    createOpenedByQueryRef.current = true
    setCreateOpen(true)
    params.delete("create")
    const qs = params.toString()
    router.replace(qs ? `/gateway?${qs}` : "/gateway", { scroll: false })
  }, [router])

  useEffect(() => {
    setSelectedRouteIDs((prev) => {
      const valid = new Set(filteredRoutes.map((route) => route.id))
      const next = new Set<string>()
      prev.forEach((id) => {
        if (valid.has(id)) {
          next.add(id)
        }
      })
      return next
    })
    setConfirmBulkDelete(false)
  }, [filteredRoutes])

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

  const toggleSelectRoute = (routeID: string, checked: boolean) => {
    setSelectedRouteIDs((prev) => {
      const next = new Set(prev)
      if (checked) {
        next.add(routeID)
      } else {
        next.delete(routeID)
      }
      return next
    })
    setConfirmBulkDelete(false)
  }

  const toggleSelectAllRoutes = (checked: boolean) => {
    setSelectedRouteIDs((prev) => {
      const next = new Set(prev)
      if (checked) {
        filteredRoutes.forEach((route) => next.add(route.id))
      } else {
        filteredRoutes.forEach((route) => next.delete(route.id))
      }
      return next
    })
    setConfirmBulkDelete(false)
  }

  const selectedRoutes = filteredRoutes.filter((route) => selectedRouteIDs.has(route.id))
  const allFilteredSelected =
    filteredRoutes.length > 0 && filteredRoutes.every((route) => selectedRouteIDs.has(route.id))

  const applyBulkEnableState = async (enabled: boolean) => {
    const targets = Array.from(selectedRouteIDs)
    if (targets.length === 0) return
    try {
      setBulkBusy(true)
      setError(null)
      const results = await Promise.allSettled(
        targets.map((id) => gatewayApi.updateRoute(id, { enabled }))
      )
      const failed = results.filter((result) => result.status === "rejected")
      if (failed.length > 0) {
        setNotice({
          kind: "error",
          text: `Bulk update completed with ${failed.length} failed route(s).`,
        })
      } else {
        setNotice({
          kind: "success",
          text: `Bulk ${enabled ? "enabled" : "disabled"} ${targets.length} route(s).`,
        })
      }
      await loadData()
    } finally {
      setBulkBusy(false)
    }
  }

  const handleBulkDelete = async () => {
    const targets = Array.from(selectedRouteIDs)
    if (targets.length === 0) return
    if (!confirmBulkDelete) {
      setConfirmBulkDelete(true)
      return
    }
    try {
      setBulkBusy(true)
      setError(null)
      const results = await Promise.allSettled(targets.map((id) => gatewayApi.deleteRoute(id)))
      const failed = results.filter((result) => result.status === "rejected")
      if (failed.length > 0) {
        setNotice({
          kind: "error",
          text: `Bulk delete completed with ${failed.length} failed route(s).`,
        })
      } else {
        setNotice({
          kind: "success",
          text: `Deleted ${targets.length} route(s).`,
        })
      }
      setSelectedRouteIDs(new Set())
      setConfirmBulkDelete(false)
      await loadData()
    } finally {
      setBulkBusy(false)
    }
  }

  const handleExportRoutes = () => {
    const selectedTargets = selectedRouteIDs.size > 0
      ? routes.filter((route) => selectedRouteIDs.has(route.id))
      : filteredRoutes
    if (selectedTargets.length === 0) {
      setNotice({ kind: "info", text: "No routes available for export." })
      return
    }

    const rows = selectedTargets.map((route) => ({
      domain: route.domain || undefined,
      path: route.path,
      methods: route.methods,
      function_name: route.function_name,
      auth_strategy: route.auth_strategy,
      auth_config: route.auth_config,
      request_schema: route.request_schema,
      rate_limit: route.rate_limit,
      enabled: route.enabled,
    }))
    const ts = new Date().toISOString().replace(/[:.]/g, "-")
    downloadJSON(`zenith-routes-${ts}.json`, {
      kind: "zenith.gateway.routes.export",
      version: 1,
      exported_at: new Date().toISOString(),
      count: rows.length,
      routes: rows,
    })
    setNotice({ kind: "success", text: `Exported ${rows.length} route(s).` })
  }

  const handleImportRoutes = async (input: HTMLInputElement) => {
    const file = input.files?.[0]
    input.value = ""
    if (!file) {
      return
    }

    try {
      setIoBusy(true)
      setError(null)
      setNotice(null)

      const rawText = await file.text()
      const parsed: unknown = JSON.parse(rawText)
      const { items, invalid } = parseRouteImportPayload(parsed)

      if (items.length === 0) {
        setError("No valid route definitions were found in the import file.")
        setNotice({ kind: "error", text: "Import failed: invalid file format or missing required fields." })
        return
      }

      const results = await Promise.allSettled(items.map((item) => gatewayApi.createRoute(item)))
      const failed = results.filter((result) => result.status === "rejected").length
      const succeeded = results.length - failed
      const invalidSuffix = invalid > 0 ? `, skipped ${invalid} invalid record(s)` : ""

      setSelectedRouteIDs(new Set())
      setConfirmBulkDelete(false)
      await loadData()

      if (succeeded > 0) {
        markOnboardingStep("gateway_route_created", true)
      }

      if (failed > 0) {
        setNotice({
          kind: "error",
          text: `Import finished: ${succeeded} succeeded, ${failed} failed${invalidSuffix}.`,
        })
      } else {
        setNotice({
          kind: "success",
          text: `Import succeeded: ${succeeded} route(s)${invalidSuffix}.`,
        })
      }
    } catch (err) {
      const message = toUserErrorMessage(err)
      setError(message)
      setNotice({ kind: "error", text: `Import failed: ${message}` })
    } finally {
      setIoBusy(false)
    }
  }

  return (
    <DashboardLayout>
      <Header title={t("gateway.title")} description={t("gateway.description")} />

      <div className="space-y-6 p-6">
        <OnboardingFlow
          hasFunctionCreated={functions.length > 0}
          hasFunctionInvoked={hasInvocations}
          hasGatewayRouteCreated={routes.length > 0}
          onCreateFunction={() => router.push("/functions")}
          onCreateGatewayRoute={() => setCreateOpen(true)}
        />

        {error && (
          <ErrorBanner error={error} title="Failed to Load Gateway Configuration" onRetry={loadData} />
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
            <input
              ref={importInputRef}
              type="file"
              accept=".json,application/json"
              className="hidden"
              onChange={(event) => {
                void handleImportRoutes(event.target)
              }}
            />
            <Button
              variant="outline"
              size="sm"
              onClick={handleExportRoutes}
              disabled={loading || busy || bulkBusy || ioBusy || filteredRoutes.length === 0}
            >
              <Download className="mr-2 h-4 w-4" />
              {selectedRouteIDs.size > 0 ? `Export Selected (${selectedRouteIDs.size})` : "Export Filtered"}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => importInputRef.current?.click()}
              disabled={loading || busy || bulkBusy || ioBusy}
            >
              <Upload className="mr-2 h-4 w-4" />
              Import JSON
            </Button>
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
                <Button size="sm" disabled={busy || bulkBusy || ioBusy}>
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

            <Button
              variant="outline"
              size="sm"
              onClick={loadData}
              disabled={loading || busy || bulkBusy || ioBusy}
            >
              <RefreshCw className={cn("mr-2 h-4 w-4", (loading || busy) && "animate-spin")} />
              Refresh
            </Button>
          </div>
        </div>

        {selectedRouteIDs.size > 0 && (
          <div className="rounded-lg border border-border bg-card p-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-sm text-muted-foreground">
                Selected <span className="font-medium text-foreground">{selectedRoutes.length}</span> route(s)
              </p>
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => applyBulkEnableState(true)}
                  disabled={busy || bulkBusy}
                >
                  Bulk Enable
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => applyBulkEnableState(false)}
                  disabled={busy || bulkBusy}
                >
                  Bulk Disable
                </Button>
                {confirmBulkDelete ? (
                  <>
                    <Button
                      size="sm"
                      variant="destructive"
                      onClick={handleBulkDelete}
                      disabled={busy || bulkBusy}
                    >
                      Confirm Bulk Delete
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => setConfirmBulkDelete(false)}
                      disabled={busy || bulkBusy}
                    >
                      Cancel
                    </Button>
                  </>
                ) : (
                  <Button
                    size="sm"
                    variant="destructive"
                    onClick={handleBulkDelete}
                    disabled={busy || bulkBusy}
                  >
                    Bulk Delete
                  </Button>
                )}
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setSelectedRouteIDs(new Set())}
                  disabled={busy || bulkBusy}
                >
                  Clear Selection
                </Button>
              </div>
            </div>
          </div>
        )}

        {!loading && routes.length === 0 ? (
          <EmptyState
            title="No Gateway Routes Yet"
            description="Create a route so UI / CLI / MCP can call through Zenith."
            primaryAction={{
              label: functions.length > 0 ? "Add Route" : "Create Function First",
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
            title="No Matching Routes"
            description="No routes match the current domain filter."
            primaryAction={{ label: "Clear Filter", onClick: () => setDomainFilter("") }}
            compact
          />
        ) : (
          <div className="overflow-x-auto rounded-xl border border-border bg-card">
            <table className="w-full min-w-[980px]">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">
                    <input
                      type="checkbox"
                      checked={allFilteredSelected}
                      onChange={(event) => toggleSelectAllRoutes(event.target.checked)}
                      className="h-4 w-4 rounded border-border"
                    />
                  </th>
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
                      <td className="px-4 py-3" colSpan={11}>
                        <div className="h-4 animate-pulse rounded bg-muted" />
                      </td>
                    </tr>
                  ))
                ) : (
                  filteredRoutes.map((route) => (
                  <tr
                    key={route.id}
                    className={cn(
                      "border-b border-border last:border-0 hover:bg-muted/50",
                      selectedRouteIDs.has(route.id) && "bg-muted/40"
                    )}
                  >
                    <td className="px-4 py-3">
                      <input
                        type="checkbox"
                        checked={selectedRouteIDs.has(route.id)}
                        onChange={(event) => toggleSelectRoute(route.id, event.target.checked)}
                        className="h-4 w-4 rounded border-border"
                      />
                    </td>
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
                          disabled={busy || bulkBusy}
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
                          disabled={busy || bulkBusy}
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
                              disabled={busy || bulkBusy}
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
                              disabled={busy || bulkBusy}
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
                            disabled={busy || bulkBusy}
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
