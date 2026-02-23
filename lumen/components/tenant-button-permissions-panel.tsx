"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { AlertTriangle, RefreshCw } from "lucide-react"
import { useTranslations } from "next-intl"

import { Button } from "@/components/ui/button"
import { tenantsApi } from "@/lib/api"
import { getTenantScope } from "@/lib/tenant-scope"
import { cn } from "@/lib/utils"

interface TenantButtonPermissionsPanelProps {
  tenantId?: string
}

const DEFAULT_INTERFACE_PERMISSION_KEYS = [
  "function:create",
  "function:update",
  "function:delete",
  "function:invoke",
  "runtime:write",
  "config:write",
  "secret:manage",
  "apikey:manage",
  "workflow:manage",
  "schedule:manage",
  "gateway:manage",
  "rbac:manage",
]

export function TenantButtonPermissionsPanel({ tenantId }: TenantButtonPermissionsPanelProps) {
  const t = useTranslations("tenantButtonPermissions")

  const [currentTenant, setCurrentTenant] = useState("default")
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState<string | null>(null)
  const [error, setError] = useState("")
  const [permissions, setPermissions] = useState<Record<string, boolean>>({})

  const allKeys = useMemo(() => {
    return Array.from(new Set([...DEFAULT_INTERFACE_PERMISSION_KEYS, ...Object.keys(permissions)])).sort()
  }, [permissions])

  const loadData = useCallback(async (tenantID?: string) => {
    const targetTenant = tenantID || getTenantScope().tenantId
    setLoading(true)
    setError("")
    try {
      const perms = await tenantsApi.listButtonPermissions(targetTenant)
      const permMap: Record<string, boolean> = {}
      for (const p of perms) {
        permMap[p.permission_key] = p.enabled
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

  const togglePermission = async (permissionKey: string) => {
    const newEnabled = !permissions[permissionKey]
    setSaving(permissionKey)
    setError("")
    try {
      await tenantsApi.upsertButtonPermission(currentTenant, permissionKey, newEnabled)
      setPermissions((prev) => ({ ...prev, [permissionKey]: newEnabled }))
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
          <p className="mt-1 text-xs text-muted-foreground">
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
        <div className="grid gap-2 sm:grid-cols-2">
          {allKeys.map((permissionKey) => {
            const enabled = permissions[permissionKey] ?? false
            const isSaving = saving === permissionKey

            return (
              <div
                key={permissionKey}
                className="flex items-center justify-between rounded-lg border border-border bg-muted/20 px-3 py-2.5"
              >
                <span className="font-mono text-xs text-foreground">
                  {permissionKey}
                </span>
                <button
                  type="button"
                  role="switch"
                  aria-checked={enabled}
                  disabled={isSaving}
                  onClick={() => void togglePermission(permissionKey)}
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
