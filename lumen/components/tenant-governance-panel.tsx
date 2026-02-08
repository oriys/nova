"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { AlertTriangle, RefreshCw, Save, Trash2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { TenantQuotaEntry, TenantUsageEntry, tenantsApi } from "@/lib/api"
import { getTenantScope } from "@/lib/tenant-scope"
import { cn } from "@/lib/utils"

interface TenantGovernancePanelProps {
  tenantId?: string
}

type DraftQuota = {
  hard_limit: string
  soft_limit: string
  burst: string
  window_s: string
}

type GovernanceDimension = {
  key: string
  label: string
  unit: string
  windowed: boolean
}

const GOVERNANCE_DIMENSIONS: GovernanceDimension[] = [
  { key: "invocations", label: "Invocations", unit: "req/window", windowed: true },
  { key: "event_publishes", label: "Event Publishes", unit: "msg/window", windowed: true },
  { key: "async_queue_depth", label: "Async Queue Depth", unit: "jobs", windowed: false },
  { key: "functions_count", label: "Functions Count", unit: "functions", windowed: false },
  { key: "memory_mb", label: "Memory", unit: "MB", windowed: false },
  { key: "vcpu_milli", label: "vCPU", unit: "mCPU", windowed: false },
  { key: "disk_iops", label: "Disk IO", unit: "IOPS", windowed: false },
]

function toErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message.trim()) {
    return error.message.trim()
  }
  return "Failed to load tenant governance data."
}

function toDraftQuota(quota?: TenantQuotaEntry, windowed: boolean = false): DraftQuota {
  if (!quota) {
    return {
      hard_limit: "0",
      soft_limit: "0",
      burst: "0",
      window_s: windowed ? "60" : "60",
    }
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
  if (Number.isNaN(parsed) || parsed < 0) {
    return fallback
  }
  return parsed
}

export function TenantGovernancePanel({ tenantId }: TenantGovernancePanelProps) {
  const [currentTenant, setCurrentTenant] = useState("default")
  const [loading, setLoading] = useState(true)
  const [savingDimension, setSavingDimension] = useState("")
  const [deletingDimension, setDeletingDimension] = useState("")
  const [error, setError] = useState("")

  const [quotas, setQuotas] = useState<TenantQuotaEntry[]>([])
  const [usage, setUsage] = useState<TenantUsageEntry[]>([])
  const [drafts, setDrafts] = useState<Record<string, DraftQuota>>({})

  const usageMap = useMemo(() => {
    const map = new Map<string, TenantUsageEntry>()
    usage.forEach((item) => {
      map.set(item.dimension, item)
    })
    return map
  }, [usage])

  const quotaMap = useMemo(() => {
    const map = new Map<string, TenantQuotaEntry>()
    quotas.forEach((item) => {
      map.set(item.dimension, item)
    })
    return map
  }, [quotas])

  const loadData = useCallback(async (tenantID?: string) => {
    const targetTenant = tenantID || getTenantScope().tenantId
    setLoading(true)
    setError("")
    try {
      const [quotaItems, usageItems] = await Promise.all([
        tenantsApi.listQuotas(targetTenant),
        tenantsApi.usage(targetTenant, true),
      ])
      setQuotas(quotaItems)
      setUsage(usageItems)

      const nextDrafts: Record<string, DraftQuota> = {}
      GOVERNANCE_DIMENSIONS.forEach((dimension) => {
        nextDrafts[dimension.key] = toDraftQuota(
          quotaItems.find((q) => q.dimension === dimension.key),
          dimension.windowed
        )
      })
      setDrafts(nextDrafts)
    } catch (err) {
      setError(toErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (tenantId) {
      setCurrentTenant(tenantId)
      void loadData(tenantId)
      return
    }

    const syncScope = () => {
      const scope = getTenantScope()
      setCurrentTenant(scope.tenantId)
      void loadData(scope.tenantId)
    }
    syncScope()
    window.addEventListener("storage", syncScope)
    window.addEventListener("nova:tenant-scope-changed", syncScope as EventListener)
    return () => {
      window.removeEventListener("storage", syncScope)
      window.removeEventListener("nova:tenant-scope-changed", syncScope as EventListener)
    }
  }, [loadData, tenantId])

  const updateDraft = (dimension: string, patch: Partial<DraftQuota>) => {
    setDrafts((prev) => ({
      ...prev,
      [dimension]: {
        ...prev[dimension],
        ...patch,
      },
    }))
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
      await tenantsApi.upsertQuota(currentTenant, dimension.key, {
        hard_limit: hardLimit,
        soft_limit: softLimit,
        burst,
        window_s: dimension.windowed ? windowS : 60,
      })
      await loadData(currentTenant)
    } catch (err) {
      setError(toErrorMessage(err))
    } finally {
      setSavingDimension("")
    }
  }

  const deleteQuota = async (dimension: GovernanceDimension) => {
    setDeletingDimension(dimension.key)
    setError("")
    try {
      await tenantsApi.deleteQuota(currentTenant, dimension.key)
      await loadData(currentTenant)
    } catch (err) {
      setError(toErrorMessage(err))
    } finally {
      setDeletingDimension("")
    }
  }

  return (
    <div className="rounded-xl border border-border bg-card p-6">
      <div className="mb-4 flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold text-card-foreground">Tenant Governance</h3>
          <p className="text-xs text-muted-foreground mt-1">
            Quotas and live usage for <span className="font-medium text-foreground">{currentTenant}</span>
          </p>
        </div>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          title="Refresh usage and quotas"
          onClick={() => void loadData(currentTenant)}
          disabled={loading || !!savingDimension || !!deletingDimension}
        >
          <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
        </Button>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4 mb-5">
        {GOVERNANCE_DIMENSIONS.map((dimension) => {
          const usageItem = usageMap.get(dimension.key)
          const quotaItem = quotaMap.get(dimension.key)
          const used = usageItem?.used ?? 0
          const limit = quotaItem ? quotaItem.hard_limit + Math.max(0, quotaItem.burst) : 0
          const ratio = limit > 0 ? used / limit : 0

          return (
            <div key={dimension.key} className="rounded-lg border border-border bg-muted/20 p-3">
              <div className="text-xs text-muted-foreground">{dimension.label}</div>
              <div className="mt-1 text-lg font-semibold text-foreground">{used.toLocaleString()}</div>
              <div className="text-[11px] text-muted-foreground mt-1">
                {limit > 0 ? `Limit ${limit.toLocaleString()} ${dimension.unit}` : "Unlimited"}
              </div>
              {limit > 0 && (
                <div className="mt-2 h-1.5 rounded-full bg-muted overflow-hidden">
                  <div
                    className={cn(
                      "h-full rounded-full transition-all",
                      ratio >= 0.95 && "bg-destructive",
                      ratio >= 0.8 && ratio < 0.95 && "bg-amber-500",
                      ratio < 0.8 && "bg-primary"
                    )}
                    style={{ width: `${Math.min(ratio * 100, 100)}%` }}
                  />
                </div>
              )}
            </div>
          )
        })}
      </div>

      <div className="overflow-x-auto rounded-md border border-border">
        <table className="w-full min-w-[980px]">
          <thead>
            <tr className="border-b border-border bg-muted/30">
              <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Dimension
              </th>
              <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Current Usage
              </th>
              <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Hard Limit
              </th>
              <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Soft Limit
              </th>
              <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Burst
              </th>
              <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Window(s)
              </th>
              <th className="px-3 py-2 text-right text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Actions
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {GOVERNANCE_DIMENSIONS.map((dimension) => {
              const draft = drafts[dimension.key] || toDraftQuota(undefined, dimension.windowed)
              const usageItem = usageMap.get(dimension.key)
              const quotaItem = quotaMap.get(dimension.key)
              const used = usageItem?.used ?? 0
              const limit = quotaItem ? quotaItem.hard_limit + Math.max(0, quotaItem.burst) : 0
              const ratio = limit > 0 ? used / limit : 0

              return (
                <tr key={dimension.key}>
                  <td className="px-3 py-2">
                    <div className="text-sm font-medium text-foreground">{dimension.label}</div>
                    <div className="text-[11px] text-muted-foreground">{dimension.key}</div>
                  </td>
                  <td className="px-3 py-2">
                    <div className="text-sm text-foreground">
                      {used.toLocaleString()} {dimension.unit}
                    </div>
                    {limit > 0 && (
                      <div
                        className={cn(
                          "text-[11px]",
                          ratio >= 0.95 && "text-destructive",
                          ratio >= 0.8 && ratio < 0.95 && "text-amber-600",
                          ratio < 0.8 && "text-muted-foreground"
                        )}
                      >
                        {Math.round(ratio * 100)}% of limit
                      </div>
                    )}
                  </td>
                  <td className="px-3 py-2">
                    <Input
                      value={draft.hard_limit}
                      onChange={(e) => updateDraft(dimension.key, { hard_limit: e.target.value })}
                      className="h-8"
                      inputMode="numeric"
                    />
                  </td>
                  <td className="px-3 py-2">
                    <Input
                      value={draft.soft_limit}
                      onChange={(e) => updateDraft(dimension.key, { soft_limit: e.target.value })}
                      className="h-8"
                      inputMode="numeric"
                    />
                  </td>
                  <td className="px-3 py-2">
                    <Input
                      value={draft.burst}
                      onChange={(e) => updateDraft(dimension.key, { burst: e.target.value })}
                      className="h-8"
                      inputMode="numeric"
                    />
                  </td>
                  <td className="px-3 py-2">
                    <Input
                      value={draft.window_s}
                      onChange={(e) => updateDraft(dimension.key, { window_s: e.target.value })}
                      className="h-8"
                      inputMode="numeric"
                      disabled={!dimension.windowed}
                    />
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        title="Save quota"
                        onClick={() => void saveQuota(dimension)}
                        disabled={loading || !!deletingDimension || savingDimension === dimension.key}
                      >
                        <Save className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        className="text-destructive hover:text-destructive"
                        title="Delete quota rule"
                        onClick={() => void deleteQuota(dimension)}
                        disabled={loading || !!savingDimension || deletingDimension === dimension.key}
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

      {error && (
        <div className="mt-4 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">
          <div className="inline-flex items-center gap-1">
            <AlertTriangle className="h-3.5 w-3.5" />
            <span>{error}</span>
          </div>
        </div>
      )}
    </div>
  )
}
