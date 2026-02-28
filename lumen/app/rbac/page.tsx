"use client"

import { useCallback, useEffect, useState } from "react"
import { useTranslations } from "next-intl"
import { ShieldCheck, Plus, Trash2, RefreshCw } from "lucide-react"

import { DashboardLayout } from "@/components/dashboard-layout"
import { Header } from "@/components/header"
import { SubNav } from "@/components/sub-nav"
import { TenantButtonPermissionsPanel } from "@/components/tenant-button-permissions-panel"
import { TenantMenuPermissionsPanel } from "@/components/tenant-menu-permissions-panel"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  rbacApi,
  tenantsApi,
  type PermissionBindingInfo,
  type RBACPermission,
  type RBACRole,
  type RBACRoleAssignment,
  type TenantEntry,
} from "@/lib/api"
import { cn } from "@/lib/utils"

type RBACRecordTab = "roles" | "permissions" | "assignments"
type Tab = RBACRecordTab | "tenants" | "menuPermissions" | "interfacePermissions" | "permissionBindings"

const TAB_WITH_CREATE: Record<Tab, boolean> = {
  roles: true,
  permissions: true,
  assignments: true,
  tenants: true,
  menuPermissions: false,
  interfacePermissions: false,
  permissionBindings: false,
}

export default function RBACPage() {
  const t = useTranslations("pages")
  const tr = useTranslations("rbacPage")
  const tc = useTranslations("common")
  const ta = useTranslations("auditLogsPage")

  const [tab, setTab] = useState<Tab>("roles")
  const [roles, setRoles] = useState<RBACRole[]>([])
  const [permissions, setPermissions] = useState<RBACPermission[]>([])
  const [assignments, setAssignments] = useState<RBACRoleAssignment[]>([])
  const [tenants, setTenants] = useState<TenantEntry[]>([])
  const [bindings, setBindings] = useState<PermissionBindingInfo[]>([])
  const [selectedTenant, setSelectedTenant] = useState("")

  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [creating, setCreating] = useState(false)

  // Role form
  const [roleName, setRoleName] = useState("")
  // Permission form
  const [permCode, setPermCode] = useState("")
  const [permResource, setPermResource] = useState("")
  const [permAction, setPermAction] = useState("")
  const [permDesc, setPermDesc] = useState("")
  // Assignment form
  const [assignPrincipalType, setAssignPrincipalType] = useState("")
  const [assignPrincipalId, setAssignPrincipalId] = useState("")
  const [assignRoleId, setAssignRoleId] = useState("")
  const [assignScopeType, setAssignScopeType] = useState("")
  const [assignScopeId, setAssignScopeId] = useState("")
  // Tenant form
  const [tenantId, setTenantId] = useState("")
  const [tenantName, setTenantName] = useState("")
  const [tenantStatus, setTenantStatus] = useState("active")
  const [tenantTier, setTenantTier] = useState("default")

  const fetchData = useCallback(async () => {
    try {
      setLoading(true)
      setError(null)

      if (tab === "roles") {
        const data = await rbacApi.listRoles()
        setRoles(data || [])
        return
      }
      if (tab === "permissions") {
        const data = await rbacApi.listPermissions()
        setPermissions(data || [])
        return
      }
      if (tab === "assignments") {
        const data = await rbacApi.listRoleAssignments()
        setAssignments(data || [])
        return
      }
      if (tab === "permissionBindings") {
        const data = await rbacApi.listPermissionBindings()
        setBindings(data || [])
        return
      }

      const tenantList = await tenantsApi.list()
      setTenants(tenantList || [])

      if (tenantList.length === 0) {
        setSelectedTenant("")
        return
      }

      const hasSelected = Boolean(selectedTenant) && tenantList.some((tenant) => tenant.id === selectedTenant)
      if (!hasSelected) {
        setSelectedTenant(tenantList[0].id)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : tr("failedToLoad"))
    } finally {
      setLoading(false)
    }
  }, [selectedTenant, tab, tr])

  useEffect(() => {
    void fetchData()
  }, [fetchData])

  const resetForm = () => {
    setRoleName("")
    setPermCode("")
    setPermResource("")
    setPermAction("")
    setPermDesc("")
    setAssignPrincipalType("")
    setAssignPrincipalId("")
    setAssignRoleId("")
    setAssignScopeType("")
    setAssignScopeId("")
    setTenantId("")
    setTenantName("")
    setTenantStatus("active")
    setTenantTier("default")
  }

  const handleCreate = async () => {
    try {
      setCreating(true)

      if (tab === "roles") {
        if (!roleName.trim()) return
        await rbacApi.createRole({ id: crypto.randomUUID(), name: roleName.trim() })
      } else if (tab === "permissions") {
        if (!permCode.trim()) return
        await rbacApi.createPermission({
          id: crypto.randomUUID(),
          code: permCode.trim(),
          resource_type: permResource,
          action: permAction,
          description: permDesc,
        })
      } else if (tab === "assignments") {
        if (
          !assignPrincipalType.trim() ||
          !assignPrincipalId.trim() ||
          !assignRoleId.trim() ||
          !assignScopeType.trim()
        ) {
          return
        }
        await rbacApi.createRoleAssignment({
          id: crypto.randomUUID(),
          principal_type: assignPrincipalType.trim(),
          principal_id: assignPrincipalId.trim(),
          role_id: assignRoleId.trim(),
          scope_type: assignScopeType.trim(),
          scope_id: assignScopeId || undefined,
        })
      } else if (tab === "tenants") {
        if (!tenantId.trim()) return
        await tenantsApi.create({
          id: tenantId.trim(),
          ...(tenantName.trim() ? { name: tenantName.trim() } : {}),
          ...(tenantStatus.trim() ? { status: tenantStatus.trim() } : {}),
          ...(tenantTier.trim() ? { tier: tenantTier.trim() } : {}),
        })
      }

      setDialogOpen(false)
      resetForm()
      await fetchData()
    } catch (err) {
      setError(err instanceof Error ? err.message : tr("failedToCreate"))
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      if (tab === "roles") {
        await rbacApi.deleteRole(id)
      } else if (tab === "permissions") {
        await rbacApi.deletePermission(id)
      } else if (tab === "assignments") {
        await rbacApi.deleteRoleAssignment(id)
      } else if (tab === "tenants") {
        await tenantsApi.delete(id)
      }
      await fetchData()
    } catch (err) {
      setError(err instanceof Error ? err.message : tr("failedToDelete"))
    }
  }

  const tabs: { key: Tab; label: string }[] = [
    { key: "roles", label: tr("roles") },
    { key: "permissions", label: tr("permissions") },
    { key: "assignments", label: tr("assignments") },
    { key: "tenants", label: tr("tenants") },
    { key: "menuPermissions", label: tr("menuPermissions") },
    { key: "interfacePermissions", label: tr("interfacePermissions") },
    { key: "permissionBindings", label: tr("permissionBindings") },
  ]

  const supportsCreate = TAB_WITH_CREATE[tab]
  const createLabel =
    tab === "roles"
      ? tr("createRole")
      : tab === "permissions"
        ? tr("createPermission")
        : tab === "assignments"
          ? tr("createAssignment")
          : tr("createTenant")

  return (
    <DashboardLayout>
      <Header title={t("rbac.title")} description={t("rbac.description")} />
      <div className="px-6 pt-4">
        <SubNav items={[
          { label: t("rbac.title"), href: "/rbac" },
          { label: ta("title"), href: "/audit-logs" },
        ]} />
      </div>

      <div className="space-y-6 p-6">
        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
            {error}
          </div>
        )}

        <div className="flex flex-wrap items-center gap-2">
          {tabs.map((item) => (
            <button
              key={item.key}
              onClick={() => setTab(item.key)}
              className={cn(
                "rounded-lg px-4 py-2 text-sm font-medium transition-colors",
                tab === item.key
                  ? "bg-primary text-primary-foreground"
                  : "bg-muted text-muted-foreground hover:bg-muted/80"
              )}
            >
              {item.label}
            </button>
          ))}
        </div>

        <div className="flex items-center justify-between">
          {supportsCreate ? (
            <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
              <DialogTrigger asChild>
                <Button size="sm">
                  <Plus className="mr-2 h-4 w-4" />
                  {createLabel}
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>{createLabel}</DialogTitle>
                </DialogHeader>
                <div className="space-y-4">
                  {tab === "roles" && (
                    <div className="space-y-2">
                      <label className="text-sm font-medium">{tr("name")}</label>
                      <Input
                        value={roleName}
                        onChange={(event) => setRoleName(event.target.value)}
                        placeholder={tr("name")}
                      />
                    </div>
                  )}

                  {tab === "permissions" && (
                    <>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tr("code")}</label>
                        <Input
                          value={permCode}
                          onChange={(event) => setPermCode(event.target.value)}
                          placeholder={tr("code")}
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tr("resourceType")}</label>
                        <Input
                          value={permResource}
                          onChange={(event) => setPermResource(event.target.value)}
                          placeholder={tr("resourceType")}
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tr("action")}</label>
                        <Input
                          value={permAction}
                          onChange={(event) => setPermAction(event.target.value)}
                          placeholder={tr("action")}
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tr("description")}</label>
                        <Input
                          value={permDesc}
                          onChange={(event) => setPermDesc(event.target.value)}
                          placeholder={tr("description")}
                        />
                      </div>
                    </>
                  )}

                  {tab === "assignments" && (
                    <>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tr("principalType")}</label>
                        <Input
                          value={assignPrincipalType}
                          onChange={(event) => setAssignPrincipalType(event.target.value)}
                          placeholder={tr("principalType")}
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tr("principalId")}</label>
                        <Input
                          value={assignPrincipalId}
                          onChange={(event) => setAssignPrincipalId(event.target.value)}
                          placeholder={tr("principalId")}
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tr("roleId")}</label>
                        <Input
                          value={assignRoleId}
                          onChange={(event) => setAssignRoleId(event.target.value)}
                          placeholder={tr("roleId")}
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tr("scopeType")}</label>
                        <Input
                          value={assignScopeType}
                          onChange={(event) => setAssignScopeType(event.target.value)}
                          placeholder={tr("scopeType")}
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tr("scopeId")}</label>
                        <Input
                          value={assignScopeId}
                          onChange={(event) => setAssignScopeId(event.target.value)}
                          placeholder={tr("scopeId")}
                        />
                      </div>
                    </>
                  )}

                  {tab === "tenants" && (
                    <>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tr("tenantId")}</label>
                        <Input
                          value={tenantId}
                          onChange={(event) => setTenantId(event.target.value)}
                          placeholder={tr("tenantIdPlaceholder")}
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tr("tenantName")}</label>
                        <Input
                          value={tenantName}
                          onChange={(event) => setTenantName(event.target.value)}
                          placeholder={tr("tenantNamePlaceholder")}
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tr("tenantStatus")}</label>
                        <Input
                          value={tenantStatus}
                          onChange={(event) => setTenantStatus(event.target.value)}
                          placeholder="active"
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">{tr("tenantTier")}</label>
                        <Input
                          value={tenantTier}
                          onChange={(event) => setTenantTier(event.target.value)}
                          placeholder="default"
                        />
                      </div>
                    </>
                  )}

                  <Button className="w-full" onClick={handleCreate} disabled={creating}>
                    {creating ? tr("creating") : tc("create")}
                  </Button>
                </div>
              </DialogContent>
            </Dialog>
          ) : (
            <div />
          )}

          <Button variant="outline" size="sm" onClick={() => void fetchData()} disabled={loading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />
            {tc("refresh")}
          </Button>
        </div>

        {(tab === "roles" || tab === "permissions" || tab === "assignments" || tab === "tenants") && (
          <div className="overflow-hidden rounded-xl border border-border bg-card">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  {tab === "roles" && (
                    <>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("name")}</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("system")}</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("created")}</th>
                    </>
                  )}

                  {tab === "permissions" && (
                    <>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("code")}</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("resourceType")}</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("action")}</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("description")}</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("created")}</th>
                    </>
                  )}

                  {tab === "assignments" && (
                    <>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("principal")}</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("roleId")}</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("scope")}</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("created")}</th>
                    </>
                  )}

                  {tab === "tenants" && (
                    <>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("tenantId")}</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("tenantName")}</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("tenantStatus")}</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("tenantTier")}</th>
                      <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("created")}</th>
                    </>
                  )}

                  <th className="px-4 py-3 text-right text-sm font-medium text-muted-foreground">{tc("actions")}</th>
                </tr>
              </thead>

              <tbody>
                {loading ? (
                  Array.from({ length: 3 }).map((_, index) => (
                    <tr key={index} className="border-b border-border">
                      <td colSpan={6} className="px-4 py-3">
                        <div className="h-4 animate-pulse rounded bg-muted" />
                      </td>
                    </tr>
                  ))
                ) : tab === "roles" && roles.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="px-4 py-8 text-center text-muted-foreground">
                      <ShieldCheck className="mx-auto mb-2 h-8 w-8 opacity-50" />
                      {tr("noRoles")}
                    </td>
                  </tr>
                ) : tab === "permissions" && permissions.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="px-4 py-8 text-center text-muted-foreground">
                      <ShieldCheck className="mx-auto mb-2 h-8 w-8 opacity-50" />
                      {tr("noPermissions")}
                    </td>
                  </tr>
                ) : tab === "assignments" && assignments.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="px-4 py-8 text-center text-muted-foreground">
                      <ShieldCheck className="mx-auto mb-2 h-8 w-8 opacity-50" />
                      {tr("noAssignments")}
                    </td>
                  </tr>
                ) : tab === "tenants" && tenants.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="px-4 py-8 text-center text-muted-foreground">
                      <ShieldCheck className="mx-auto mb-2 h-8 w-8 opacity-50" />
                      {tr("noTenants")}
                    </td>
                  </tr>
                ) : tab === "roles" ? (
                  roles.map((role) => (
                    <tr key={role.id} className="border-b border-border hover:bg-muted/50">
                      <td className="px-4 py-3 text-sm font-medium">{role.name}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">{role.is_system ? "✓" : "—"}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {new Date(role.created_at).toLocaleDateString()}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => void handleDelete(role.id)}
                          disabled={role.is_system}
                        >
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </td>
                    </tr>
                  ))
                ) : tab === "permissions" ? (
                  permissions.map((permission) => (
                    <tr key={permission.id} className="border-b border-border hover:bg-muted/50">
                      <td className="px-4 py-3 font-mono text-sm font-medium">{permission.code}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">{permission.resource_type}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">{permission.action}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">{permission.description}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {new Date(permission.created_at).toLocaleDateString()}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <Button variant="ghost" size="sm" onClick={() => void handleDelete(permission.id)}>
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </td>
                    </tr>
                  ))
                ) : tab === "assignments" ? (
                  assignments.map((assignment) => (
                    <tr key={assignment.id} className="border-b border-border hover:bg-muted/50">
                      <td className="px-4 py-3 text-sm font-medium">
                        {assignment.principal_type}:{assignment.principal_id}
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">{assignment.role_id}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {assignment.scope_type}
                        {assignment.scope_id ? `:${assignment.scope_id}` : ""}
                      </td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {new Date(assignment.created_at).toLocaleDateString()}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <Button variant="ghost" size="sm" onClick={() => void handleDelete(assignment.id)}>
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </td>
                    </tr>
                  ))
                ) : (
                  tenants.map((tenant) => (
                    <tr key={tenant.id} className="border-b border-border hover:bg-muted/50">
                      <td className="px-4 py-3 text-sm font-medium">{tenant.id}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">{tenant.name || tenant.id}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">{tenant.status}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">{tenant.tier}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">
                        {new Date(tenant.created_at).toLocaleDateString()}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => void handleDelete(tenant.id)}
                          disabled={tenant.id === "default"}
                        >
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}

        {(tab === "menuPermissions" || tab === "interfacePermissions") && (
          <div className="space-y-4">
            {loading ? (
              <div className="rounded-xl border border-border bg-card p-6 text-sm text-muted-foreground">
                {tc("loading")}
              </div>
            ) : tenants.length === 0 ? (
              <div className="rounded-xl border border-border bg-card p-6 text-sm text-muted-foreground">
                {tr("noTenants")}
              </div>
            ) : (
              <>
                <div className="rounded-xl border border-border bg-card p-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium text-foreground">{tr("tenantId")}</label>
                    <Select value={selectedTenant} onValueChange={setSelectedTenant}>
                      <SelectTrigger className="w-full sm:w-80">
                        <SelectValue placeholder={tr("selectTenant")} />
                      </SelectTrigger>
                      <SelectContent>
                        {tenants.map((tenant) => (
                          <SelectItem key={tenant.id} value={tenant.id}>
                            {tenant.name ? `${tenant.name} (${tenant.id})` : tenant.id}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                {selectedTenant &&
                  (tab === "menuPermissions" ? (
                    <TenantMenuPermissionsPanel key={`menu-${selectedTenant}`} tenantId={selectedTenant} />
                  ) : (
                    <TenantButtonPermissionsPanel
                      key={`interface-${selectedTenant}`}
                      tenantId={selectedTenant}
                    />
                  ))}
              </>
            )}
          </div>
        )}
        {tab === "permissionBindings" && (
          <div className="overflow-hidden rounded-xl border border-border bg-card">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("colPermission")}</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("colDescription")}</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("colApiRoutes")}</th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-muted-foreground">{tr("colUiButtons")}</th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  Array.from({ length: 3 }).map((_, index) => (
                    <tr key={index} className="border-b border-border">
                      <td colSpan={4} className="px-4 py-3">
                        <div className="h-4 animate-pulse rounded bg-muted" />
                      </td>
                    </tr>
                  ))
                ) : bindings.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="px-4 py-8 text-center text-muted-foreground">
                      <ShieldCheck className="mx-auto mb-2 h-8 w-8 opacity-50" />
                      {tr("noBindings")}
                    </td>
                  </tr>
                ) : (
                  bindings.map((b) => (
                    <tr key={b.permission} className="border-b border-border hover:bg-muted/50">
                      <td className="px-4 py-3 font-mono text-sm font-medium">{b.permission}</td>
                      <td className="px-4 py-3 text-sm text-muted-foreground">{b.description}</td>
                      <td className="px-4 py-3 text-sm">
                        {b.api_routes.length > 0 ? (
                          <div className="flex flex-col gap-1">
                            {b.api_routes.map((r, i) => (
                              <code key={i} className="text-xs">
                                <span className="font-semibold text-blue-500">{r.method}</span>{" "}
                                {r.path}
                              </code>
                            ))}
                          </div>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </td>
                      <td className="px-4 py-3 text-sm">
                        {b.ui_buttons.length > 0 ? (
                          <div className="flex flex-wrap gap-1">
                            {b.ui_buttons.map((btn) => (
                              <span
                                key={btn}
                                className="inline-flex items-center rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary"
                              >
                                {btn}
                              </span>
                            ))}
                          </div>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </DashboardLayout>
  )
}
