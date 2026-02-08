"use client"

import { useEffect, useMemo, useState } from "react"
import { Building2, RotateCcw } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { DEFAULT_NAMESPACE, DEFAULT_TENANT_ID, getTenantScope, normalizeTenantScope, setTenantScope } from "@/lib/tenant-scope"

export function TenantSwitcher() {
  const [open, setOpen] = useState(false)
  const [currentTenant, setCurrentTenant] = useState(DEFAULT_TENANT_ID)
  const [currentNamespace, setCurrentNamespace] = useState(DEFAULT_NAMESPACE)
  const [tenantDraft, setTenantDraft] = useState(DEFAULT_TENANT_ID)
  const [namespaceDraft, setNamespaceDraft] = useState(DEFAULT_NAMESPACE)

  useEffect(() => {
    const syncScope = () => {
      const scope = getTenantScope()
      setCurrentTenant(scope.tenantId)
      setCurrentNamespace(scope.namespace)
      setTenantDraft(scope.tenantId)
      setNamespaceDraft(scope.namespace)
    }
    syncScope()
    window.addEventListener("storage", syncScope)
    window.addEventListener("nova:tenant-scope-changed", syncScope as EventListener)
    return () => {
      window.removeEventListener("storage", syncScope)
      window.removeEventListener("nova:tenant-scope-changed", syncScope as EventListener)
    }
  }, [])

  const scopeText = useMemo(() => `${currentTenant}/${currentNamespace}`, [currentNamespace, currentTenant])

  const applyScope = () => {
    const normalized = normalizeTenantScope({
      tenantId: tenantDraft,
      namespace: namespaceDraft,
    })
    const changed = normalized.tenantId !== currentTenant || normalized.namespace !== currentNamespace

    setCurrentTenant(normalized.tenantId)
    setCurrentNamespace(normalized.namespace)
    setTenantDraft(normalized.tenantId)
    setNamespaceDraft(normalized.namespace)
    setTenantScope(normalized)
    setOpen(false)

    if (changed && typeof window !== "undefined") {
      window.location.reload()
    }
  }

  const resetScope = () => {
    setTenantDraft(DEFAULT_TENANT_ID)
    setNamespaceDraft(DEFAULT_NAMESPACE)
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="hidden md:flex max-w-[240px]">
          <Building2 className="h-4 w-4 mr-2" />
          <span className="truncate">{scopeText}</span>
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Tenant Scope</DialogTitle>
          <DialogDescription>
            Set the tenant and namespace for all console API requests.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="tenant-id">Tenant</Label>
            <Input
              id="tenant-id"
              value={tenantDraft}
              onChange={(e) => setTenantDraft(e.target.value)}
              placeholder="default"
              autoComplete="off"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="namespace-id">Namespace</Label>
            <Input
              id="namespace-id"
              value={namespaceDraft}
              onChange={(e) => setNamespaceDraft(e.target.value)}
              placeholder="default"
              autoComplete="off"
            />
          </div>
          <p className="text-xs text-muted-foreground">
            Uses request headers: <code>X-Nova-Tenant</code> and <code>X-Nova-Namespace</code>.
          </p>
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={resetScope}>
            <RotateCcw className="h-4 w-4 mr-2" />
            Reset
          </Button>
          <Button type="button" onClick={applyScope}>
            Apply
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
