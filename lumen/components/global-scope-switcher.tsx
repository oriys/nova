"use client"

import Link from "next/link"
import { useCallback, useEffect, useMemo, useState } from "react"
import { Globe2, RefreshCw, Settings2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { NamespaceEntry, TenantEntry, tenantsApi } from "@/lib/api"
import { DEFAULT_NAMESPACE, DEFAULT_TENANT_ID, getTenantScope, normalizeTenantScope, setTenantScope } from "@/lib/tenant-scope"

function toErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message.trim()) {
    return error.message.trim()
  }
  return "Failed to load tenant options."
}

export function GlobalScopeSwitcher() {
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [applying, setApplying] = useState(false)
  const [error, setError] = useState("")

  const [currentTenant, setCurrentTenant] = useState(DEFAULT_TENANT_ID)
  const [currentNamespace, setCurrentNamespace] = useState(DEFAULT_NAMESPACE)

  const [tenantDraft, setTenantDraft] = useState(DEFAULT_TENANT_ID)
  const [namespaceDraft, setNamespaceDraft] = useState(DEFAULT_NAMESPACE)
  const [tenants, setTenants] = useState<TenantEntry[]>([])
  const [namespaces, setNamespaces] = useState<NamespaceEntry[]>([])

  const scopeText = useMemo(() => `${currentTenant}/${currentNamespace}`, [currentNamespace, currentTenant])

  const syncScope = useCallback(() => {
    const scope = getTenantScope()
    setCurrentTenant(scope.tenantId)
    setCurrentNamespace(scope.namespace)
  }, [])

  const loadNamespaces = useCallback(
    async (tenantID: string, preferredNamespace?: string) => {
      const namespaceList = await tenantsApi.listNamespaces(tenantID)
      setNamespaces(namespaceList)
      const nextNamespace =
        namespaceList.find((ns) => ns.name === preferredNamespace)?.name ??
        namespaceList.find((ns) => ns.name === DEFAULT_NAMESPACE)?.name ??
        namespaceList[0]?.name ??
        DEFAULT_NAMESPACE
      setNamespaceDraft(nextNamespace)
    },
    []
  )

  const loadOptions = useCallback(async () => {
    setLoading(true)
    setError("")
    try {
      const scope = getTenantScope()
      const tenantList = await tenantsApi.list()
      setTenants(tenantList)

      const nextTenant =
        tenantList.find((tenant) => tenant.id === scope.tenantId)?.id ??
        tenantList[0]?.id ??
        DEFAULT_TENANT_ID
      setTenantDraft(nextTenant)

      await loadNamespaces(nextTenant, scope.namespace)
    } catch (err) {
      setError(toErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [loadNamespaces])

  useEffect(() => {
    syncScope()
    window.addEventListener("storage", syncScope)
    window.addEventListener("nova:tenant-scope-changed", syncScope as EventListener)
    return () => {
      window.removeEventListener("storage", syncScope)
      window.removeEventListener("nova:tenant-scope-changed", syncScope as EventListener)
    }
  }, [syncScope])

  useEffect(() => {
    if (open) {
      void loadOptions()
    }
  }, [loadOptions, open])

  const handleTenantDraftChange = async (tenantID: string) => {
    setTenantDraft(tenantID)
    setLoading(true)
    setError("")
    try {
      await loadNamespaces(tenantID)
    } catch (err) {
      setError(toErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  const applyScope = () => {
    setApplying(true)
    const normalized = normalizeTenantScope({
      tenantId: tenantDraft,
      namespace: namespaceDraft,
    })
    const changed =
      normalized.tenantId !== currentTenant || normalized.namespace !== currentNamespace

    setTenantScope(normalized)
    setCurrentTenant(normalized.tenantId)
    setCurrentNamespace(normalized.namespace)
    setOpen(false)
    setApplying(false)

    if (changed && typeof window !== "undefined") {
      window.location.reload()
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <div className="group relative">
        <Button
          variant="outline"
          size="icon"
          onClick={() => setOpen(true)}
          title={`Current scope: ${scopeText}`}
          aria-label="Switch tenant scope"
        >
          <Globe2 className="h-4 w-4" />
        </Button>
        <div className="pointer-events-none absolute right-0 top-[calc(100%+0.5rem)] z-50 hidden rounded-md border border-border bg-popover px-2 py-1 text-xs text-popover-foreground shadow-sm group-hover:block">
          {scopeText}
        </div>
      </div>

      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Global Tenant Scope</DialogTitle>
          <DialogDescription>
            Quick switch tenant and namespace for all pages.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <label className="text-sm font-medium">Tenant</label>
              <Button
                variant="ghost"
                size="icon-xs"
                title="Refresh tenant list"
                onClick={() => void loadOptions()}
                disabled={loading}
              >
                <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
              </Button>
            </div>
            <Select
              value={tenantDraft}
              onValueChange={(value) => void handleTenantDraftChange(value)}
              disabled={loading || tenants.length === 0}
            >
              <SelectTrigger className="w-full">
                <SelectValue placeholder="Select tenant" />
              </SelectTrigger>
              <SelectContent align="start">
                {tenants.map((tenant) => (
                  <SelectItem key={tenant.id} value={tenant.id}>
                    {tenant.name === tenant.id ? tenant.id : `${tenant.id} (${tenant.name})`}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <label className="text-sm font-medium">Namespace</label>
            <Select
              value={namespaceDraft}
              onValueChange={setNamespaceDraft}
              disabled={loading || namespaces.length === 0}
            >
              <SelectTrigger className="w-full">
                <SelectValue placeholder="Select namespace" />
              </SelectTrigger>
              <SelectContent align="start">
                {namespaces.map((namespace) => (
                  <SelectItem key={namespace.id} value={namespace.name}>
                    {namespace.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {error && (
            <div className="rounded-md border border-destructive/30 bg-destructive/5 px-2.5 py-2 text-xs text-destructive">
              {error}
            </div>
          )}
        </div>

        <DialogFooter className="sm:justify-between">
          <Button type="button" variant="outline" asChild>
            <Link href="/tenancy" onClick={() => setOpen(false)}>
              <Settings2 className="h-4 w-4 mr-1.5" />
              Manage
            </Link>
          </Button>
          <div className="flex items-center gap-2">
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={applyScope} disabled={loading || applying}>
              Apply
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
