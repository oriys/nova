"use client"

import { useCallback, useEffect, useState } from "react"
import { AlertTriangle, RefreshCw } from "lucide-react"
import { useTranslations } from "next-intl"

import { Button } from "@/components/ui/button"
import { type MenuPermission, tenantsApi } from "@/lib/api"
import { getTenantScope } from "@/lib/tenant-scope"
import { cn } from "@/lib/utils"

interface TenantMenuPermissionsPanelProps {
  tenantId?: string
}

const ALL_MENU_KEYS = [
  "dashboard",
  "functions",
  "gateway",
  "events",
  "workflows",
  "tenancy",
  "asyncJobs",
  "history",
  "runtimes",
  "layers",
  "volumes",
  "snapshots",
  "rbac",
  "notifications",
  "configurations",
  "secrets",
  "apiKeys",
  "apiDocs",
]

export function TenantMenuPermissionsPanel({ tenantId }: TenantMenuPermissionsPanelProps) {
  const t = useTranslations("tenantMenuPermissions")
  const navT = useTranslations("nav")

  const [currentTenant, setCurrentTenant] = useState("default")
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState<string | null>(null)
  const [error, setError] = useState("")
  const [permissions, setPermissions] = useState<Record<string, boolean>>({})

  const loadData = useCallback(async (tenantID?: string) => {
    const targetTenant = tenantID || getTenantScope().tenantId
    setLoading(true)
    setError("")
    try {
      const perms = await tenantsApi.listMenuPermissions(targetTenant)
      const permMap: Record<string, boolean> = {}
      for (const p of perms) {
        permMap[p.menu_key] = p.enabled
      }
      setPermissions(permMap)
    } catch (err) {
      setError(err instanceof Error ? err.message : t("loadFailed"))
    } finally {
      setLoading(false)
    }
  }, [t])

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

  const togglePermission = async (menuKey: string) => {
    const newEnabled = !permissions[menuKey]
    setSaving(menuKey)
    setError("")
    try {
      await tenantsApi.upsertMenuPermission(currentTenant, menuKey, newEnabled)
      setPermissions((prev) => ({ ...prev, [menuKey]: newEnabled }))
    } catch (err) {
      setError(err instanceof Error ? err.message : t("saveFailed"))
    } finally {
      setSaving(null)
    }
  }

  return (
    <div className="rounded-xl border border-border bg-card p-6">
      <div className="mb-4 flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold text-card-foreground">{t("title")}</h3>
          <p className="text-xs text-muted-foreground mt-1">
            {t("description", { tenantId: currentTenant })}
          </p>
        </div>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          title={t("refresh")}
          onClick={() => void loadData(currentTenant)}
          disabled={loading || !!saving}
        >
          <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
        </Button>
      </div>

      {loading ? (
        <div className="text-sm text-muted-foreground">{t("loading")}</div>
      ) : (
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {ALL_MENU_KEYS.map((menuKey) => {
            const enabled = permissions[menuKey] ?? false
            const isSaving = saving === menuKey

            return (
              <div
                key={menuKey}
                className="flex items-center justify-between rounded-lg border border-border bg-muted/20 px-3 py-2.5"
              >
                <span className="text-sm font-medium text-foreground">
                  {navT(menuKey)}
                </span>
                <button
                  type="button"
                  role="switch"
                  aria-checked={enabled}
                  disabled={isSaving}
                  onClick={() => void togglePermission(menuKey)}
                  className={cn(
                    "relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
                    enabled ? "bg-primary" : "bg-input"
                  )}
                >
                  <span
                    className={cn(
                      "pointer-events-none block h-4 w-4 rounded-full bg-background shadow-sm ring-0 transition-transform",
                      enabled ? "translate-x-4" : "translate-x-0.5"
                    )}
                  />
                </button>
              </div>
            )
          })}
        </div>
      )}

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
