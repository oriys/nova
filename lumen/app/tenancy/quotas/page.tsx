"use client"

import Link from "next/link"
import { useCallback, useEffect, useMemo, useState } from "react"
import { useTranslations } from "next-intl"
import {
  ArrowLeft,
  AlertTriangle,
  RefreshCw,
  Save,
  Trash2,
} from "lucide-react"

import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  TenantEntry,
  TenantQuotaEntry,
  TenantUsageEntry,
  tenantsApi,
} from "@/lib/api"
import { getTenantScope } from "@/lib/tenant-scope"
import { cn } from "@/lib/utils"

type DraftQuota = {
  hard_limit: string
  soft_limit: string
  burst: string
  window_s: string
}

type GovernanceDimension = {
  key: string
  labelKey: string
  unit: string
  windowed: boolean
}

const GOVERNANCE_DIMENSIONS: GovernanceDimension[] = [
  { key: "invocations", labelKey: "invocations", unit: "req/window", windowed: true },
  { key: "event_publishes", labelKey: "eventPublishes", unit: "msg/window", windowed: true },
  { key: "async_queue_depth", labelKey: "asyncQueueDepth", unit: "jobs", windowed: false },
  { key: "functions_count", labelKey: "functionsCount", unit: "functions", windowed: false },
  { key: "memory_mb", labelKey: "memoryMb", unit: "MB", windowed: false },
  { key: "vcpu_milli", labelKey: "vcpuMilli", unit: "mCPU", windowed: false },
  { key: "disk_iops", labelKey: "diskIops", unit: "IOPS", windowed: false },
]

function toDraftQuota(quota?: TenantQuotaEntry, windowed = false): DraftQuota {
  if (!quota) {
    return { hard_limit: "0", soft_limit: "0", burst: "0", window_s: "60" }
  }
  return {
    hard_limit: String(quota.hard_limit ?? 0),
    soft_limit: String(quota.soft_limit ?? 0),
    burst: String(quota.burst ?? 0),
    window_s: String(quota.window_s ?? 60),
  }
}

function parseNonNegativeInt(raw: string, fallback: number): number {
  const parsed = Number.parseInt(raw, 10)
  if (Number.isNaN(parsed) || parsed < 0) return fallback
  return parsed
}

export default function TenantQuotasPage() {
  const t = useTranslations("quotaManagement")
  const pt = useTranslations("pages")

  const [tenants, setTenants] = useState<TenantEntry[]>([])
  const [selectedTenant, setSelectedTenant] = useState("")
  const [loading, setLoading] = useState(true)
  const [savingDimension, setSavingDimension] = useState("")
  const [deletingDimension, setDeletingDimension] = useState("")
  const [error, setError] = useState("")

  const [quotas, setQuotas] = useState<TenantQuotaEntry[]>([])
  const [usage, setUsage] = useState<TenantUsageEntry[]>([])
  const [drafts, setDrafts] = useState<Record<string, DraftQuota>>({})

  const usageMap = useMemo(() => {
    const map = new Map<string, TenantUsageEntry>()
    usage.forEach((item) => map.set(item.dimension, item))
    return map
  }, [usage])

  const quotaMap = useMemo(() => {
    const map = new Map<string, TenantQuotaEntry>()
    quotas.forEach((item) => map.set(item.dimension, item))
    return map
  }, [quotas])

  const loadTenants = useCallback(async () => {
    try {
      const list = await tenantsApi.list()
      setTenants(list)
      return list
    } catch {
      setTenants([])
      return []
    }
  }, [])

  const loadQuotas = useCallback(async (tenantID: string) => {
    setLoading(true)
    setError("")
    try {
      const [quotaItems, usageItems] = await Promise.all([
        tenantsApi.listQuotas(tenantID),
        tenantsApi.usage(tenantID, true),
      ])
      setQuotas(quotaItems)
      setUsage(usageItems)

      const nextDrafts: Record<string, DraftQuota> = {}
      GOVERNANCE_DIMENSIONS.forEach((dim) => {
        nextDrafts[dim.key] = toDraftQuota(
          quotaItems.find((q) => q.dimension === dim.key),
          dim.windowed,
        )
      })
      setDrafts(nextDrafts)
    } catch (err) {
      setError(err instanceof Error ? err.message : t("loadError"))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    const init = async () => {
      const list = await loadTenants()
      const scope = getTenantScope()
      const initial = list.find((item) => item.id === scope.tenantId)?.id ?? list[0]?.id ?? ""
      if (initial) {
        setSelectedTenant(initial)
        await loadQuotas(initial)
      } else {
        setLoading(false)
      }
    }
    void init()
  }, [loadTenants, loadQuotas])

  const handleTenantChange = (tenantID: string) => {
    setSelectedTenant(tenantID)
    void loadQuotas(tenantID)
  }

  const updateDraft = (dimension: string, patch: Partial<DraftQuota>) => {
    setDrafts((prev) => ({ ...prev, [dimension]: { ...prev[dimension], ...patch } }))
  }

  const saveQuota = async (dimension: GovernanceDimension) => {
    const draft = drafts[dimension.key] || toDraftQuota(undefined, dimension.windowed)
    const hardLimit = parseNonNegativeInt(draft.hard_limit, 0)
    const softLimit = parseNonNegativeInt(draft.soft_limit, 0)
    const burst = parseNonNegativeInt(draft.burst, 0)
    const windowS = parseNonNegativeInt(draft.window_s, 60)

    setSavingDimension(dimension.key)
    setError("")
    try {
      await tenantsApi.upsertQuota(selectedTenant, dimension.key, {
        hard_limit: hardLimit,
        soft_limit: softLimit,
        burst,
        window_s: dimension.windowed ? windowS : 60,
      })
      await loadQuotas(selectedTenant)
    } catch (err) {
      setError(err instanceof Error ? err.message : t("saveError"))
    } finally {
      setSavingDimension("")
    }
  }

  const deleteQuota = async (dimension: GovernanceDimension) => {
    setDeletingDimension(dimension.key)
    setError("")
    try {
      await tenantsApi.deleteQuota(selectedTenant, dimension.key)
      await loadQuotas(selectedTenant)
    } catch (err) {
      setError(err instanceof Error ? err.message : t("deleteError"))
    } finally {
      setDeletingDimension("")
    }
  }

  const configuredCount = quotas.length
  const totalDimensions = GOVERNANCE_DIMENSIONS.length
  const overLimitCount = GOVERNANCE_DIMENSIONS.filter((dim) => {
    const q = quotaMap.get(dim.key)
    const u = usageMap.get(dim.key)
    if (!q || !u) return false
    const limit = q.hard_limit + Math.max(0, q.burst)
    return limit > 0 && u.used >= limit
  }).length

  return (
    <DashboardLayout>
      <Header
        title={pt("tenancy.quotas.title")}
        description={pt("tenancy.quotas.description")}
      />

      <div className="space-y-6 p-6">
        <div className="flex items-center gap-3">
          <Button asChild variant="outline" size="sm">
            <Link href="/tenancy">
              <ArrowLeft className="mr-1.5 h-4 w-4" />
              {t("backToTenancy")}
            </Link>
          </Button>
        </div>

        {/* Tenant selector */}
        <div className="rounded-xl border border-border bg-card p-4">
          <div className="flex flex-wrap items-center gap-4">
            <div className="space-y-1">
              <label className="text-xs font-medium text-muted-foreground">{t("selectTenant")}</label>
              <Select value={selectedTenant} onValueChange={handleTenantChange} disabled={loading || tenants.length === 0}>
                <SelectTrigger className="w-[220px]">
                  <SelectValue placeholder={t("selectTenantPlaceholder")} />
                </SelectTrigger>
                <SelectContent>
                  {tenants.map((tenant) => (
                    <SelectItem key={tenant.id} value={tenant.id}>
                      {tenant.name || tenant.id}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <Button
              variant="ghost"
              size="icon"
              className="mt-5 h-8 w-8"
              title={t("refreshQuotas")}
              onClick={() => selectedTenant && void loadQuotas(selectedTenant)}
              disabled={loading || !selectedTenant}
            >
              <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
            </Button>

            {selectedTenant && !loading && (
              <div className="mt-5 flex items-center gap-4 text-xs text-muted-foreground">
                <span>
                  {t("configuredCount", { count: configuredCount, total: totalDimensions })}
                </span>
                {overLimitCount > 0 && (
                  <span className="inline-flex items-center gap-1 text-destructive">
                    <AlertTriangle className="h-3.5 w-3.5" />
                    {t("overLimit", { count: overLimitCount })}
                  </span>
                )}
              </div>
            )}
          </div>
        </div>

        {!selectedTenant && !loading && (
          <div className="rounded-xl border border-border bg-card p-6 text-sm text-muted-foreground">
            {t("noTenants")}
          </div>
        )}

        {selectedTenant && (
          <>
            {/* Summary cards */}
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              {GOVERNANCE_DIMENSIONS.map((dim) => {
                const usageItem = usageMap.get(dim.key)
                const quotaItem = quotaMap.get(dim.key)
                const used = usageItem?.used ?? 0
                const limit = quotaItem ? quotaItem.hard_limit + Math.max(0, quotaItem.burst) : 0
                const ratio = limit > 0 ? used / limit : 0

                return (
                  <div key={dim.key} className="rounded-lg border border-border bg-card p-3">
                    <div className="text-xs text-muted-foreground">{t(`dimensions.${dim.labelKey}`)}</div>
                    <div className="mt-1 text-lg font-semibold text-foreground">{used.toLocaleString()}</div>
                    <div className="mt-1 text-[11px] text-muted-foreground">
                      {limit > 0
                        ? `${t("limit")} ${limit.toLocaleString()} ${dim.unit}`
                        : t("unlimited")}
                    </div>
                    {limit > 0 && (
                      <div className="mt-2 h-1.5 rounded-full bg-muted overflow-hidden">
                        <div
                          className={cn(
                            "h-full rounded-full transition-all",
                            ratio >= 0.95 && "bg-destructive",
                            ratio >= 0.8 && ratio < 0.95 && "bg-amber-500",
                            ratio < 0.8 && "bg-primary",
                          )}
                          style={{ width: `${Math.min(ratio * 100, 100)}%` }}
                        />
                      </div>
                    )}
                  </div>
                )
              })}
            </div>

            {/* Editable quota table */}
            <div className="rounded-xl border border-border bg-card p-6">
              <h3 className="text-lg font-semibold text-card-foreground mb-4">{t("quotaRules")}</h3>
              <div className="overflow-x-auto rounded-md border border-border">
                <table className="w-full min-w-[980px]">
                  <thead>
                    <tr className="border-b border-border bg-muted/30">
                      <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                        {t("columnDimension")}
                      </th>
                      <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                        {t("columnUsage")}
                      </th>
                      <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                        {t("columnHardLimit")}
                      </th>
                      <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                        {t("columnSoftLimit")}
                      </th>
                      <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                        {t("columnBurst")}
                      </th>
                      <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                        {t("columnWindow")}
                      </th>
                      <th className="px-3 py-2 text-right text-xs font-medium text-muted-foreground uppercase tracking-wider">
                        {t("columnActions")}
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border">
                    {GOVERNANCE_DIMENSIONS.map((dim) => {
                      const draft = drafts[dim.key] || toDraftQuota(undefined, dim.windowed)
                      const usageItem = usageMap.get(dim.key)
                      const quotaItem = quotaMap.get(dim.key)
                      const used = usageItem?.used ?? 0
                      const limit = quotaItem ? quotaItem.hard_limit + Math.max(0, quotaItem.burst) : 0
                      const ratio = limit > 0 ? used / limit : 0

                      return (
                        <tr key={dim.key}>
                          <td className="px-3 py-2">
                            <div className="text-sm font-medium text-foreground">{t(`dimensions.${dim.labelKey}`)}</div>
                            <div className="text-[11px] text-muted-foreground">{dim.key}</div>
                          </td>
                          <td className="px-3 py-2">
                            <div className="text-sm text-foreground">
                              {used.toLocaleString()} {dim.unit}
                            </div>
                            {limit > 0 && (
                              <div
                                className={cn(
                                  "text-[11px]",
                                  ratio >= 0.95 && "text-destructive",
                                  ratio >= 0.8 && ratio < 0.95 && "text-amber-600",
                                  ratio < 0.8 && "text-muted-foreground",
                                )}
                              >
                                {Math.round(ratio * 100)}% {t("ofLimit")}
                              </div>
                            )}
                          </td>
                          <td className="px-3 py-2">
                            <Input
                              value={draft.hard_limit}
                              onChange={(e) => updateDraft(dim.key, { hard_limit: e.target.value })}
                              className="h-8"
                              inputMode="numeric"
                            />
                          </td>
                          <td className="px-3 py-2">
                            <Input
                              value={draft.soft_limit}
                              onChange={(e) => updateDraft(dim.key, { soft_limit: e.target.value })}
                              className="h-8"
                              inputMode="numeric"
                            />
                          </td>
                          <td className="px-3 py-2">
                            <Input
                              value={draft.burst}
                              onChange={(e) => updateDraft(dim.key, { burst: e.target.value })}
                              className="h-8"
                              inputMode="numeric"
                            />
                          </td>
                          <td className="px-3 py-2">
                            <Input
                              value={draft.window_s}
                              onChange={(e) => updateDraft(dim.key, { window_s: e.target.value })}
                              className="h-8"
                              inputMode="numeric"
                              disabled={!dim.windowed}
                            />
                          </td>
                          <td className="px-3 py-2">
                            <div className="flex items-center justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="icon-sm"
                                title={t("saveQuota")}
                                onClick={() => void saveQuota(dim)}
                                disabled={loading || !!deletingDimension || savingDimension === dim.key}
                              >
                                <Save className="h-4 w-4" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="icon-sm"
                                className="text-destructive hover:text-destructive"
                                title={t("deleteQuota")}
                                onClick={() => void deleteQuota(dim)}
                                disabled={loading || !!savingDimension || deletingDimension === dim.key}
                              >
                                <Trash2 className="h-4 w-4" />
                              </Button>
                            </div>
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          </>
        )}

        {error && (
          <div className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">
            <div className="inline-flex items-center gap-1">
              <AlertTriangle className="h-3.5 w-3.5" />
              <span>{error}</span>
            </div>
          </div>
        )}
      </div>
    </DashboardLayout>
  )
}
