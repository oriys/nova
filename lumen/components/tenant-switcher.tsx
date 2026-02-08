"use client"

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react"
import { Building2, FolderTree, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { NamespaceEntry, TenantEntry, tenantsApi } from "@/lib/api"
import { DEFAULT_NAMESPACE, DEFAULT_TENANT_ID, getTenantScope, normalizeTenantScope, setTenantScope } from "@/lib/tenant-scope"
import { cn } from "@/lib/utils"

const SCOPE_PART_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/

function toErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message.trim()) {
    return error.message.trim()
  }
  return "Unexpected error, please try again."
}

function validScopePart(value: string): boolean {
  return SCOPE_PART_PATTERN.test(value)
}

export function TenantSwitcher() {
  const [loading, setLoading] = useState(true)
  const [mutating, setMutating] = useState(false)
  const [error, setError] = useState("")

  const [tenants, setTenants] = useState<TenantEntry[]>([])
  const [namespaces, setNamespaces] = useState<NamespaceEntry[]>([])
  const [currentTenant, setCurrentTenant] = useState(DEFAULT_TENANT_ID)
  const [currentNamespace, setCurrentNamespace] = useState(DEFAULT_NAMESPACE)

  const [createTenantOpen, setCreateTenantOpen] = useState(false)
  const [editTenantOpen, setEditTenantOpen] = useState(false)
  const [deleteTenantOpen, setDeleteTenantOpen] = useState(false)
  const [createNamespaceOpen, setCreateNamespaceOpen] = useState(false)
  const [editNamespaceOpen, setEditNamespaceOpen] = useState(false)
  const [deleteNamespaceOpen, setDeleteNamespaceOpen] = useState(false)

  const [newTenantID, setNewTenantID] = useState("")
  const [newTenantName, setNewTenantName] = useState("")
  const [editTenantName, setEditTenantName] = useState("")
  const [editTenantID, setEditTenantID] = useState(DEFAULT_TENANT_ID)
  const [deleteTenantID, setDeleteTenantID] = useState(DEFAULT_TENANT_ID)
  const [newNamespaceName, setNewNamespaceName] = useState("")
  const [editNamespaceName, setEditNamespaceName] = useState("")

  const selectedTenant = useMemo(
    () => tenants.find((tenant) => tenant.id === currentTenant),
    [currentTenant, tenants]
  )

  const canDeleteCurrentNamespace =
    !(currentTenant === DEFAULT_TENANT_ID && currentNamespace === DEFAULT_NAMESPACE) &&
    namespaces.length > 1

  const loadScopeOptions = useCallback(
    async (preferredTenant?: string, preferredNamespace?: string) => {
      setLoading(true)
      setError("")
      try {
        const storedScope = getTenantScope()
        const normalized = normalizeTenantScope({
          tenantId: preferredTenant ?? storedScope.tenantId,
          namespace: preferredNamespace ?? storedScope.namespace,
        })

        const tenantList = await tenantsApi.list()
        setTenants(tenantList)

        const nextTenant =
          tenantList.find((tenant) => tenant.id === normalized.tenantId)?.id ??
          tenantList[0]?.id ??
          DEFAULT_TENANT_ID

        const namespaceList = await tenantsApi.listNamespaces(nextTenant)
        setNamespaces(namespaceList)

        const nextNamespace =
          namespaceList.find((ns) => ns.name === normalized.namespace)?.name ??
          namespaceList.find((ns) => ns.name === DEFAULT_NAMESPACE)?.name ??
          namespaceList[0]?.name ??
          DEFAULT_NAMESPACE

        setCurrentTenant(nextTenant)
        setCurrentNamespace(nextNamespace)
        setTenantScope({ tenantId: nextTenant, namespace: nextNamespace })
      } catch (err) {
        setError(toErrorMessage(err))
      } finally {
        setLoading(false)
      }
    },
    []
  )

  useEffect(() => {
    void loadScopeOptions()
  }, [loadScopeOptions])

  const switchScope = useCallback(
    (tenantID: string, namespace: string): boolean => {
      const normalized = normalizeTenantScope({
        tenantId: tenantID,
        namespace,
      })
      const changed =
        normalized.tenantId !== currentTenant || normalized.namespace !== currentNamespace

      setCurrentTenant(normalized.tenantId)
      setCurrentNamespace(normalized.namespace)
      setTenantScope(normalized)

      if (changed && typeof window !== "undefined") {
        window.location.reload()
      }
      return changed
    },
    [currentNamespace, currentTenant]
  )

  const handleTenantChange = async (tenantID: string) => {
    setMutating(true)
    setError("")
    try {
      const namespaceList = await tenantsApi.listNamespaces(tenantID)
      setNamespaces(namespaceList)
      const nextNamespace =
        namespaceList.find((ns) => ns.name === DEFAULT_NAMESPACE)?.name ??
        namespaceList[0]?.name ??
        DEFAULT_NAMESPACE
      const changed = switchScope(tenantID, nextNamespace)
      if (!changed) {
        setMutating(false)
      }
    } catch (err) {
      setError(toErrorMessage(err))
      setMutating(false)
    }
  }

  const handleNamespaceChange = (namespace: string) => {
    const changed = switchScope(currentTenant, namespace)
    if (!changed) {
      setMutating(false)
    }
  }

  const submitCreateTenant = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const tenantID = newTenantID.trim()
    const tenantName = newTenantName.trim()
    if (!validScopePart(tenantID)) {
      setError("Tenant ID must match ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$.")
      return
    }

    setMutating(true)
    setError("")
    try {
      await tenantsApi.create({
        id: tenantID,
        ...(tenantName ? { name: tenantName } : {}),
      })
      setCreateTenantOpen(false)
      setNewTenantID("")
      setNewTenantName("")
      const changed = switchScope(tenantID, DEFAULT_NAMESPACE)
      if (!changed) {
        setMutating(false)
        await loadScopeOptions(tenantID, DEFAULT_NAMESPACE)
      }
    } catch (err) {
      setError(toErrorMessage(err))
      setMutating(false)
    }
  }

  const submitEditTenant = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const tenantName = editTenantName.trim()
    const targetTenantID = editTenantID.trim()
    if (!tenantName) {
      setError("Tenant name is required.")
      return
    }
    if (!targetTenantID) {
      setError("Tenant ID is required.")
      return
    }

    setMutating(true)
    setError("")
    try {
      await tenantsApi.update(targetTenantID, { name: tenantName })
      setEditTenantOpen(false)
      setMutating(false)
      await loadScopeOptions(currentTenant, currentNamespace)
    } catch (err) {
      setError(toErrorMessage(err))
      setMutating(false)
    }
  }

  const submitDeleteTenant = async () => {
    const targetTenantID = deleteTenantID.trim()
    if (!targetTenantID) {
      setError("Tenant ID is required.")
      return
    }

    setMutating(true)
    setError("")
    try {
      await tenantsApi.delete(targetTenantID)
      setDeleteTenantOpen(false)
      if (targetTenantID === currentTenant) {
        const changed = switchScope(DEFAULT_TENANT_ID, DEFAULT_NAMESPACE)
        if (!changed) {
          setMutating(false)
          await loadScopeOptions(DEFAULT_TENANT_ID, DEFAULT_NAMESPACE)
        }
      } else {
        setMutating(false)
        await loadScopeOptions(currentTenant, currentNamespace)
      }
    } catch (err) {
      setError(toErrorMessage(err))
      setMutating(false)
    }
  }

  const submitCreateNamespace = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const namespaceName = newNamespaceName.trim()
    if (!validScopePart(namespaceName)) {
      setError("Namespace must match ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$.")
      return
    }

    setMutating(true)
    setError("")
    try {
      await tenantsApi.createNamespace(currentTenant, { name: namespaceName })
      setCreateNamespaceOpen(false)
      setNewNamespaceName("")
      const changed = switchScope(currentTenant, namespaceName)
      if (!changed) {
        setMutating(false)
        await loadScopeOptions(currentTenant, namespaceName)
      }
    } catch (err) {
      setError(toErrorMessage(err))
      setMutating(false)
    }
  }

  const submitEditNamespace = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const nextNamespace = editNamespaceName.trim()
    if (!validScopePart(nextNamespace)) {
      setError("Namespace must match ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$.")
      return
    }

    setMutating(true)
    setError("")
    try {
      await tenantsApi.updateNamespace(currentTenant, currentNamespace, { name: nextNamespace })
      setEditNamespaceOpen(false)
      const changed = switchScope(currentTenant, nextNamespace)
      if (!changed) {
        setMutating(false)
        await loadScopeOptions(currentTenant, nextNamespace)
      }
    } catch (err) {
      setError(toErrorMessage(err))
      setMutating(false)
    }
  }

  const submitDeleteNamespace = async () => {
    setMutating(true)
    setError("")
    try {
      await tenantsApi.deleteNamespace(currentTenant, currentNamespace)
      setDeleteNamespaceOpen(false)

      const namespaceList = await tenantsApi.listNamespaces(currentTenant)
      setNamespaces(namespaceList)
      const fallbackNamespace =
        namespaceList.find((ns) => ns.name === DEFAULT_NAMESPACE)?.name ??
        namespaceList[0]?.name ??
        DEFAULT_NAMESPACE

      const changed = switchScope(currentTenant, fallbackNamespace)
      if (!changed) {
        setMutating(false)
        await loadScopeOptions(currentTenant, fallbackNamespace)
      }
    } catch (err) {
      setError(toErrorMessage(err))
      setMutating(false)
    }
  }

  return (
    <>
      <div className="rounded-xl border border-border bg-card p-6">
        <div className="mb-4 flex items-center justify-between">
          <div>
            <h3 className="text-lg font-semibold text-card-foreground">Tenant Scope</h3>
            <p className="text-xs text-muted-foreground mt-1">
              Switch and manage tenant/namespace for all API requests.
            </p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            title="Refresh tenant and namespace list"
            onClick={() => void loadScopeOptions(currentTenant, currentNamespace)}
            disabled={loading || mutating}
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </Button>
        </div>

        <div className="space-y-4">
          <div className="rounded-lg border border-border bg-muted/20 p-3">
            <div className="text-xs uppercase tracking-wide text-muted-foreground">Current Selected</div>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              <span className="rounded-md bg-primary/10 px-2 py-1 text-xs font-medium text-primary">
                Tenant: {currentTenant}
              </span>
              <span className="rounded-md bg-foreground/5 px-2 py-1 text-xs font-medium text-foreground">
                Namespace: {currentNamespace}
              </span>
            </div>
            {selectedTenant && selectedTenant.name !== selectedTenant.id && (
              <div className="mt-2 text-xs text-muted-foreground">
                Name: <span className="text-foreground">{selectedTenant.name}</span>
              </div>
            )}
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <div className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                <Building2 className="h-3.5 w-3.5" />
                Tenants
              </div>
              <Button
                variant="outline"
                size="sm"
                className="h-7"
                title="Create tenant"
                onClick={() => setCreateTenantOpen(true)}
                disabled={loading || mutating}
              >
                <Plus className="mr-1 h-3.5 w-3.5" />
                New Tenant
              </Button>
            </div>

            <div className="rounded-md border border-border overflow-hidden">
              {tenants.length === 0 ? (
                <div className="px-3 py-4 text-xs text-muted-foreground">No tenants found.</div>
              ) : (
                tenants.map((tenant) => (
                  <div
                    key={tenant.id}
                    className={cn(
                      "flex items-center justify-between gap-3 px-3 py-2.5 border-b border-border last:border-b-0",
                      tenant.id === currentTenant ? "bg-primary/5" : "bg-card"
                    )}
                  >
                    <button
                      type="button"
                      onClick={() => void handleTenantChange(tenant.id)}
                      className="min-w-0 flex-1 text-left"
                      disabled={loading || mutating}
                      title={`Switch to tenant ${tenant.id}`}
                    >
                      <div className="text-sm font-medium text-foreground truncate">{tenant.id}</div>
                      <div className="text-xs text-muted-foreground truncate">
                        {tenant.name || tenant.id}
                      </div>
                    </button>

                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 px-2 text-xs"
                        onClick={() => void handleTenantChange(tenant.id)}
                        disabled={loading || mutating || tenant.id === currentTenant}
                      >
                        Use
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        title="Rename tenant"
                        onClick={() => {
                          setEditTenantID(tenant.id)
                          setEditTenantName(tenant.name || tenant.id)
                          setEditTenantOpen(true)
                        }}
                        disabled={loading || mutating}
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7 text-destructive hover:text-destructive"
                        title="Delete tenant"
                        onClick={() => {
                          setDeleteTenantID(tenant.id)
                          setDeleteTenantOpen(true)
                        }}
                        disabled={loading || mutating || tenant.id === DEFAULT_TENANT_ID}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>

          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <div className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                <FolderTree className="h-3.5 w-3.5" />
                Namespace
              </div>
              <div className="flex items-center gap-0.5">
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6"
                  title="Create namespace"
                  onClick={() => setCreateNamespaceOpen(true)}
                  disabled={loading || mutating || !currentTenant}
                >
                  <Plus className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6"
                  title="Rename namespace"
                  onClick={() => {
                    setEditNamespaceName(currentNamespace)
                    setEditNamespaceOpen(true)
                  }}
                  disabled={loading || mutating || !currentNamespace}
                >
                  <Pencil className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6 text-destructive hover:text-destructive"
                  title="Delete namespace"
                  onClick={() => setDeleteNamespaceOpen(true)}
                  disabled={loading || mutating || !canDeleteCurrentNamespace}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>

            <Select
              value={currentNamespace}
              onValueChange={handleNamespaceChange}
              disabled={loading || mutating || namespaces.length === 0}
            >
              <SelectTrigger className="h-8 w-full text-xs">
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
            <div className="rounded-md border border-destructive/30 bg-destructive/5 px-2 py-1.5 text-[11px] text-destructive">
              {error}
            </div>
          )}
        </div>
      </div>

      <Dialog open={createTenantOpen} onOpenChange={setCreateTenantOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Create Tenant</DialogTitle>
            <DialogDescription>
              Add a new tenant and automatically create its default namespace.
            </DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submitCreateTenant}>
            <div className="space-y-2">
              <Label htmlFor="new-tenant-id">Tenant ID</Label>
              <Input
                id="new-tenant-id"
                value={newTenantID}
                onChange={(e) => setNewTenantID(e.target.value)}
                placeholder="team-a"
                autoComplete="off"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="new-tenant-name">Display Name (optional)</Label>
              <Input
                id="new-tenant-name"
                value={newTenantName}
                onChange={(e) => setNewTenantName(e.target.value)}
                placeholder="Team A"
                autoComplete="off"
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setCreateTenantOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={mutating}>
                Create
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={editTenantOpen} onOpenChange={setEditTenantOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Rename Tenant</DialogTitle>
            <DialogDescription>
              Update the display name of <code>{editTenantID}</code>.
            </DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submitEditTenant}>
            <div className="space-y-2">
              <Label htmlFor="edit-tenant-name">Display Name</Label>
              <Input
                id="edit-tenant-name"
                value={editTenantName}
                onChange={(e) => setEditTenantName(e.target.value)}
                placeholder="Team A"
                autoComplete="off"
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setEditTenantOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={mutating}>
                Save
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={deleteTenantOpen} onOpenChange={setDeleteTenantOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Delete Tenant</DialogTitle>
            <DialogDescription>
              Delete tenant <code>{deleteTenantID}</code>? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteTenantOpen(false)}>
              Cancel
            </Button>
            <Button type="button" variant="destructive" onClick={submitDeleteTenant} disabled={mutating}>
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={createNamespaceOpen} onOpenChange={setCreateNamespaceOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Create Namespace</DialogTitle>
            <DialogDescription>
              Add a namespace under tenant <code>{currentTenant}</code>.
            </DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submitCreateNamespace}>
            <div className="space-y-2">
              <Label htmlFor="new-namespace-name">Namespace</Label>
              <Input
                id="new-namespace-name"
                value={newNamespaceName}
                onChange={(e) => setNewNamespaceName(e.target.value)}
                placeholder="staging"
                autoComplete="off"
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setCreateNamespaceOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={mutating}>
                Create
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={editNamespaceOpen} onOpenChange={setEditNamespaceOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Rename Namespace</DialogTitle>
            <DialogDescription>
              Rename namespace <code>{currentNamespace}</code> in tenant <code>{currentTenant}</code>.
            </DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submitEditNamespace}>
            <div className="space-y-2">
              <Label htmlFor="edit-namespace-name">Namespace</Label>
              <Input
                id="edit-namespace-name"
                value={editNamespaceName}
                onChange={(e) => setEditNamespaceName(e.target.value)}
                placeholder="production"
                autoComplete="off"
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setEditNamespaceOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={mutating}>
                Save
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={deleteNamespaceOpen} onOpenChange={setDeleteNamespaceOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Delete Namespace</DialogTitle>
            <DialogDescription>
              Delete namespace <code>{currentNamespace}</code> from tenant <code>{currentTenant}</code>?
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteNamespaceOpen(false)}>
              Cancel
            </Button>
            <Button type="button" variant="destructive" onClick={submitDeleteNamespace} disabled={mutating}>
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
