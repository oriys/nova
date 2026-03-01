"use client"

import { useEffect, useState, useCallback } from "react"
import { useTranslations } from "next-intl"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
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
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { SectionHeader } from "@/components/section-header"
import { SectionEmptyHint } from "@/components/section-empty-hint"
import { SectionTableFrame } from "@/components/section-table-frame"
import { ParamMappingEditor } from "@/components/param-mapping-editor"
import { gatewayApi, functionsApi, type GatewayRoute } from "@/lib/api"
import type { ParamMapping } from "@/lib/types"
import { Plus, Pencil, Trash2, Loader2 } from "lucide-react"

interface FunctionGatewayProps {
  functionName: string
}

interface RouteFormState {
  domain: string
  path: string
  methods: string
  rps: string
  burst: string
  enabled: boolean
}

const emptyForm: RouteFormState = {
  domain: "",
  path: "",
  methods: "",
  rps: "",
  burst: "",
  enabled: false,
}

function formFromRoute(r: GatewayRoute): RouteFormState {
  return {
    domain: r.domain || "",
    path: r.path,
    methods: r.methods?.join(", ") || "",
    rps: r.rate_limit ? String(r.rate_limit.requests_per_second) : "",
    burst: r.rate_limit ? String(r.rate_limit.burst_size) : "",
    enabled: r.enabled,
  }
}

function buildExplicitAPIKeyAuthConfig(route?: GatewayRoute) {
  return {
    ...(route?.auth_config || {}),
    require_explicit_scope: "true",
  }
}

function routeUsesExplicitAPIKey(route: GatewayRoute) {
  return route.auth_strategy === "apikey" && route.auth_config?.require_explicit_scope === "true"
}

function routeAuthLabel(route: GatewayRoute, t: (key: string) => string) {
  if (routeUsesExplicitAPIKey(route)) {
    return t("authScopedApikey")
  }
  switch (route.auth_strategy) {
    case "inherit":
      return t("authInherit")
    case "apikey":
      return t("authApikey")
    case "jwt":
      return t("authJwt")
    default:
      return t("authNone")
  }
}

export function FunctionGateway({ functionName }: FunctionGatewayProps) {
  const t = useTranslations("functionDetailPage.gateway")
  const [routes, setRoutes] = useState<GatewayRoute[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  // Dialog state
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingRoute, setEditingRoute] = useState<GatewayRoute | null>(null)
  const [form, setForm] = useState<RouteFormState>(emptyForm)
  const [paramMapping, setParamMapping] = useState<ParamMapping[]>([])
  const [responseMapping, setResponseMapping] = useState<ParamMapping[]>([])
  const [functionCode, setFunctionCode] = useState("")
  const [loadingCode, setLoadingCode] = useState(false)
  const [codeLoadError, setCodeLoadError] = useState<string | null>(null)

  // Delete confirmation
  const [deletingId, setDeletingId] = useState<string | null>(null)

  const fetchRoutes = useCallback(async () => {
    setLoading(true)
    try {
      const allRoutes = await gatewayApi.listRoutes()
      setRoutes(allRoutes.filter((r) => r.function_name === functionName))
    } catch {
      setRoutes([])
    } finally {
      setLoading(false)
    }
  }, [functionName])

  useEffect(() => {
    fetchRoutes()
  }, [fetchRoutes])

  const fetchFunctionCode = useCallback(async () => {
    try {
      setLoadingCode(true)
      setCodeLoadError(null)
      const response = await functionsApi.getCode(functionName)
      setFunctionCode(response.source_code || "")
    } catch {
      setFunctionCode("")
      setCodeLoadError(t("codeUnavailable"))
    } finally {
      setLoadingCode(false)
    }
  }, [functionName, t])

  useEffect(() => {
    if (!dialogOpen) return
    fetchFunctionCode()
  }, [dialogOpen, fetchFunctionCode])

  const openCreate = () => {
    setEditingRoute(null)
    setForm(emptyForm)
    setParamMapping([])
    setResponseMapping([])
    setDialogOpen(true)
  }

  const openEdit = (route: GatewayRoute) => {
    setEditingRoute(route)
    setForm(formFromRoute(route))
    setParamMapping(route.param_mapping || [])
    setResponseMapping(route.response_mapping || [])
    setDialogOpen(true)
  }

  const handleSave = async () => {
    if (!form.path.trim()) return
    setSaving(true)
    try {
      const methods = form.methods
        .split(",")
        .map((m) => m.trim().toUpperCase())
        .filter(Boolean)
      const rateLimit =
        form.rps && parseFloat(form.rps) > 0
          ? {
              requests_per_second: parseFloat(form.rps),
              burst_size: parseInt(form.burst) || 10,
            }
          : undefined

      if (editingRoute) {
        await gatewayApi.updateRoute(editingRoute.id, {
          domain: form.domain || undefined,
          path: form.path,
          methods: methods.length > 0 ? methods : undefined,
          auth_strategy: "apikey",
          auth_config: buildExplicitAPIKeyAuthConfig(editingRoute),
          rate_limit: rateLimit,
          param_mapping: paramMapping.length > 0 ? paramMapping : undefined,
          response_mapping: responseMapping.length > 0 ? responseMapping : undefined,
          enabled: form.enabled,
        })
      } else {
        await gatewayApi.createRoute({
          domain: form.domain || undefined,
          path: form.path,
          methods: methods.length > 0 ? methods : undefined,
          function_name: functionName,
          auth_strategy: "apikey",
          auth_config: buildExplicitAPIKeyAuthConfig(),
          rate_limit: rateLimit,
          param_mapping: paramMapping.length > 0 ? paramMapping : undefined,
          response_mapping: responseMapping.length > 0 ? responseMapping : undefined,
          enabled: form.enabled,
        })
      }
      setDialogOpen(false)
      fetchRoutes()
    } catch {
      // keep dialog open on error
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (deletingId !== id) {
      setDeletingId(id)
      return
    }
    try {
      await gatewayApi.deleteRoute(id)
      setDeletingId(null)
      fetchRoutes()
    } catch {
      // ignore
    }
  }

  const handleToggle = async (route: GatewayRoute) => {
    try {
      await gatewayApi.updateRoute(route.id, {
        enabled: !route.enabled,
        auth_strategy: "apikey",
        auth_config: buildExplicitAPIKeyAuthConfig(route),
      })
      fetchRoutes()
    } catch {
      // ignore
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <SectionHeader
        title={t("title")}
        description={t("description")}
        action={
          <Button size="sm" onClick={openCreate}>
            <Plus className="mr-2 h-3.5 w-3.5" />
            {t("addRoute")}
          </Button>
        }
      />

      <div className="rounded-lg border border-border bg-muted/20 px-4 py-3">
        <p className="text-sm text-muted-foreground">{t("scopeHint")}</p>
      </div>

      {routes.length === 0 ? (
        <SectionEmptyHint className="px-6 py-8 text-center">
          {t("empty")} {t("emptyHint")}
        </SectionEmptyHint>
      ) : (
        <SectionTableFrame>
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/50">
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("domain")}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("path")}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("methods")}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("auth")}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("rateLimit")}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("paramMapping")}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("responseMapping")}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t("status")}</th>
                <th className="px-4 py-2.5 text-right font-medium text-muted-foreground">{t("actions")}</th>
              </tr>
            </thead>
            <tbody>
              {routes.map((route) => (
                <tr key={route.id} className="border-b border-border last:border-0 hover:bg-muted/30">
                  <td className="px-4 py-2.5 font-mono text-xs">{route.domain || "*"}</td>
                  <td className="px-4 py-2.5 font-mono text-xs">{route.path}</td>
                  <td className="px-4 py-2.5">
                    {route.methods && route.methods.length > 0 ? (
                      <div className="flex gap-1 flex-wrap">
                        {route.methods.map((m) => (
                          <Badge key={m} variant="outline" className="text-[10px] px-1.5 py-0">
                            {m}
                          </Badge>
                        ))}
                      </div>
                    ) : (
                      <span className="text-xs text-muted-foreground">{t("allMethods")}</span>
                    )}
                  </td>
                  <td className="px-4 py-2.5">
                    <Badge variant="secondary" className="text-[10px]">
                      {routeAuthLabel(route, t)}
                    </Badge>
                  </td>
                  <td className="px-4 py-2.5 text-xs text-muted-foreground">
                    {route.rate_limit
                      ? t("rps", { rps: route.rate_limit.requests_per_second })
                      : t("noLimit")}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-muted-foreground">
                    {route.param_mapping && route.param_mapping.length > 0 ? (
                      <Badge variant="outline" className="text-[10px]">
                        {route.param_mapping.length}
                      </Badge>
                    ) : (
                      "—"
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-muted-foreground">
                    {route.response_mapping && route.response_mapping.length > 0 ? (
                      <Badge variant="outline" className="text-[10px]">
                        {route.response_mapping.length}
                      </Badge>
                    ) : (
                      "—"
                    )}
                  </td>
                  <td className="px-4 py-2.5">
                    <button onClick={() => handleToggle(route)} className="cursor-pointer">
                      <Badge
                        variant="secondary"
                        className={
                          route.enabled
                            ? "bg-success/10 text-success border-0 text-[10px]"
                            : "bg-muted text-muted-foreground border-0 text-[10px]"
                        }
                      >
                        {route.enabled ? t("enabled") : t("disabled")}
                      </Badge>
                    </button>
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => openEdit(route)}>
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className={`h-7 w-7 ${deletingId === route.id ? "text-destructive" : ""}`}
                        onClick={() => handleDelete(route.id)}
                        onBlur={() => setDeletingId(null)}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </SectionTableFrame>
      )}

      {/* Create / Edit Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-2xl max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{editingRoute ? t("editRoute") : t("createRoute")}</DialogTitle>
          </DialogHeader>
          <div className="rounded-lg border border-border bg-muted/20 px-4 py-3">
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>{t("functionLabel")}</Label>
                <div className="rounded-md border border-border bg-background px-3 py-2 font-mono text-sm text-foreground">
                  {functionName}
                </div>
                <p className="text-xs text-muted-foreground">{t("mappingTargetHint")}</p>
              </div>
              <div className="space-y-2">
                <Label>{t("functionCode")}</Label>
                <div className="rounded-md border border-border bg-background">
                  {loadingCode ? (
                    <div className="flex items-center gap-2 px-3 py-2 text-sm text-muted-foreground">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      <span>{t("loadingCode")}</span>
                    </div>
                  ) : functionCode ? (
                    <pre className="max-h-64 overflow-auto px-3 py-2 font-mono text-xs leading-5 text-foreground whitespace-pre-wrap">
                      {functionCode}
                    </pre>
                  ) : (
                    <div className="px-3 py-2 text-sm text-muted-foreground">
                      {codeLoadError || t("codeUnavailable")}
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4 py-4">
            <div className="space-y-2">
              <Label>{t("domain")}</Label>
              <Input
                value={form.domain}
                onChange={(e) => setForm({ ...form, domain: e.target.value })}
                placeholder={t("domainPlaceholder")}
              />
            </div>
            <div className="space-y-2">
              <Label>{t("path")} *</Label>
              <Input
                value={form.path}
                onChange={(e) => setForm({ ...form, path: e.target.value })}
                placeholder={t("pathPlaceholder")}
              />
            </div>
            <div className="space-y-2">
              <Label>{t("methods")}</Label>
              <Input
                value={form.methods}
                onChange={(e) => setForm({ ...form, methods: e.target.value })}
                placeholder={t("methodsPlaceholder")}
              />
              <p className="text-[10px] text-muted-foreground">{t("methodsHint")}</p>
            </div>
            <div className="space-y-2">
              <Label>{t("authStrategy")}</Label>
              <div className="rounded-md border border-border bg-muted/30 px-3 py-2.5">
                <p className="text-sm font-medium text-foreground">{t("authScopedApikey")}</p>
                <p className="mt-1 text-xs text-muted-foreground">{t("scopeHint")}</p>
              </div>
            </div>
            <div className="space-y-2">
              <Label>{t("requestsPerSecond")}</Label>
              <Input
                type="number"
                value={form.rps}
                onChange={(e) => setForm({ ...form, rps: e.target.value })}
                placeholder="100"
              />
            </div>
            <div className="space-y-2">
              <Label>{t("burstSize")}</Label>
              <Input
                type="number"
                value={form.burst}
                onChange={(e) => setForm({ ...form, burst: e.target.value })}
                placeholder="10"
              />
            </div>
            <div className="space-y-2">
              <Label>{t("status")}</Label>
              <Select value={form.enabled ? "true" : "false"} onValueChange={(v) => setForm({ ...form, enabled: v === "true" })}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="true">{t("enabled")}</SelectItem>
                  <SelectItem value="false">{t("disabled")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="space-y-2 py-2">
            <Label>{t("paramMapping")}</Label>
            <ParamMappingEditor value={paramMapping} onChange={setParamMapping} />
          </div>
          <div className="space-y-2 py-2">
            <Label>{t("responseMapping")}</Label>
            <ParamMappingEditor
              value={responseMapping}
              onChange={setResponseMapping}
              allowedSources={["body"]}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>{t("cancel")}</Button>
            <Button onClick={handleSave} disabled={saving || !form.path.trim()}>
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {editingRoute ? t("saveRoute") : t("createRoute")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
